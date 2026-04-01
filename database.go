package main

import (
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

// ── ScannerDB implementation ──────────────────────────────────────────────────

func (r *gormScannerDB) GetRecentEncounters() ([]EncounterData, error) {
	lastCheck := time.Now().Unix() - 30
	var encounters []EncounterData
	err := r.db.Where("iv IS NOT NULL AND updated > ? AND expire_timestamp > ?", lastCheck, lastCheck).Find(&encounters).Error
	return encounters, err
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
