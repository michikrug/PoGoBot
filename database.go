package main

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// gormBotDB is the production implementation of BotDB backed by GORM.
type gormBotDB struct {
	db *gorm.DB
}

// gormScannerDB is the production implementation of ScannerDB backed by GORM.
type gormScannerDB struct {
	db *gorm.DB
}

// ── BotDB implementation ──────────────────────────────────────────────────────

func (r *gormBotDB) GetUserPreferences(userID int64) User {
	var user User
	r.db.FirstOrCreate(&user, User{ID: userID})
	return user
}

func (r *gormBotDB) UpdateUserPreference(userID int64, field string, value interface{}) {
	r.db.Model(&User{}).Where("id = ?", userID).Update(field, value)
}

func (r *gormBotDB) AddSubscription(userID int64, pokemonID, minIV, minLevel, maxDistance int) {
	subscription := Subscription{
		UserID:      userID,
		PokemonID:   pokemonID,
		MinIV:       minIV,
		MinLevel:    minLevel,
		MaxDistance: maxDistance,
	}
	r.db.Save(&subscription)
}

func (r *gormBotDB) GetUsers() []User {
	var users []User
	r.db.Find(&users)
	return users
}

func (r *gormBotDB) GetSubscriptions() []Subscription {
	var subscriptions []Subscription
	r.db.Find(&subscriptions)
	return subscriptions
}

func (r *gormBotDB) SaveMessage(chatID int64, messageID int, encounterID string) {
	r.db.Create(&Message{ChatID: chatID, MessageID: messageID, EncounterID: encounterID})
}

func (r *gormBotDB) SaveEncounter(encounterID string, expiration int64) {
	r.db.Save(&Encounter{ID: encounterID, Expiration: int(expiration)})
}

func (r *gormBotDB) GetUserSubscriptions(userID int64) []Subscription {
	var subscriptions []Subscription
	r.db.Where("user_id = ?", userID).Order("pokemon_id").Find(&subscriptions)
	return subscriptions
}

func (r *gormBotDB) DeleteSubscription(userID int64, pokemonID int) {
	r.db.Where("user_id = ? AND pokemon_id = ?", userID, pokemonID).Delete(&Subscription{})
}

func (r *gormBotDB) DeleteAllUserSubscriptions(userID int64) {
	r.db.Where("user_id = ?", userID).Delete(&Subscription{})
}

func (r *gormBotDB) GetExpiredEncountersWithMessages() ([]Encounter, map[string][]Message) {
	var encounters []Encounter
	r.db.Where("expiration < ?", time.Now().Unix()).Find(&encounters)

	messagesMap := make(map[string][]Message)
	for _, encounter := range encounters {
		var messages []Message
		r.db.Where("encounter_id = ?", encounter.ID).Find(&messages)
		messagesMap[encounter.ID] = messages
	}
	return encounters, messagesMap
}

func (r *gormBotDB) DeleteMessage(message Message) {
	r.db.Delete(&message)
}

func (r *gormBotDB) DeleteEncounter(encounter Encounter) {
	r.db.Delete(&encounter)
}

func (r *gormBotDB) AddRaidSubscription(userID int64, pokemonID, raidLevel int) {
	r.db.Save(&RaidSubscription{UserID: userID, PokemonID: pokemonID, RaidLevel: raidLevel})
}

func (r *gormBotDB) GetRaidSubscriptions() []RaidSubscription {
	var subscriptions []RaidSubscription
	r.db.Find(&subscriptions)
	return subscriptions
}

func (r *gormBotDB) GetUserRaidSubscriptions(userID int64) []RaidSubscription {
	var subscriptions []RaidSubscription
	r.db.Where("user_id = ?", userID).Order("pokemon_id, raid_level").Find(&subscriptions)
	return subscriptions
}

func (r *gormBotDB) DeleteRaidSubscription(userID int64, pokemonID, raidLevel int) {
	r.db.Where("user_id = ? AND pokemon_id = ? AND raid_level = ?", userID, pokemonID, raidLevel).Delete(&RaidSubscription{})
}

func (r *gormBotDB) DeleteAllUserRaidSubscriptions(userID int64) {
	r.db.Where("user_id = ?", userID).Delete(&RaidSubscription{})
}

// ── ScannerDB implementation ──────────────────────────────────────────────────

func (r *gormScannerDB) GetRecentEncounters() ([]EncounterData, error) {
	lastCheck := time.Now().Unix() - 30
	var encounters []EncounterData
	err := r.db.Where("iv IS NOT NULL AND updated > ? AND expire_timestamp > ?", lastCheck, lastCheck).Find(&encounters).Error
	if err != nil {
		return encounters, err
	}
	// Parse PVP JSON once at fetch time so the notification hot-path never
	// needs to unmarshal the same string repeatedly.
	for i := range encounters {
		if encounters[i].PVP == nil || *encounters[i].PVP == "" {
			continue
		}
		var pvpData PVP
		if err := json.Unmarshal([]byte(*encounters[i].PVP), &pvpData); err != nil {
			log.Printf("❌ Failed to decode PVP data for encounter %s: %v", encounters[i].ID, err)
			continue
		}
		encounters[i].PVPData = pvpData
	}
	return encounters, nil
}

func (r *gormScannerDB) SearchGymsByName(name string) []GymData {
	var gyms []GymData
	r.db.Where("lower(name) LIKE ?", "%"+strings.ToLower(name)+"%").Find(&gyms)
	return gyms
}

func (r *gormScannerDB) GetGymByID(id string) GymData {
	var gym GymData
	r.db.First(&gym, GymData{ID: id})
	return gym
}

// GetActiveRaids returns all gyms that currently have an active raid boss visible
// (battle_timestamp <= now < end_timestamp, raid_pokemon_id IS NOT NULL and != 0)
// and whose row was updated within the last 30 seconds.
func (r *gormScannerDB) GetActiveRaids() ([]GymData, error) {
	lastCheck := time.Now().Unix() - 30
	var gyms []GymData
	err := r.db.Where(
		"raid_pokemon_id IS NOT NULL AND raid_pokemon_id != 0 AND raid_battle_timestamp <= ? AND raid_end_timestamp > ? AND updated > ?",
		lastCheck+30, lastCheck, lastCheck,
	).Find(&gyms).Error
	return gyms, err
}
