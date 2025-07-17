package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Database connections and data lookup caches
var (
	botDB            *gorm.DB // Stores user subscriptions
	scannerDB        *gorm.DB // Fetches Pokémon encounters
	pokemonNameCache map[string]int
)

// Get Pokemon ID by name
func getPokemonID(name string) (int, error) {
	pokemonID, exists := pokemonNameCache[strings.ToLower(name)]
	if !exists {
		return 0, fmt.Errorf("pokémon not found: %s", name)
	}
	return pokemonID, nil
}

// Get Pokémon name by ID and language
func getPokemonName(pokemonID int, language string) string {
	if pokemon, exists := gameData.Pokemon[strconv.Itoa(pokemonID)]; exists {
		return getTranslation(pokemon.Name, language)
	}
	return getTranslation("Unknown", language)
}

// Get move name by ID and language
func getMoveName(moveID int, language string) string {
	if move, exists := gameData.Moves[strconv.Itoa(moveID)]; exists {
		return getTranslation(move.Name, language)
	}
	return getTranslation("Unknown", language)
}

// Get translation for a key in the specified language
func getTranslation(key string, language string) string {
	if language == "en" {
		return key
	}
	if translations, exists := translations[language]; exists {
		if translation, exists := translations[key]; exists {
			return translation
		}
		log.Printf("❌ Translation key not found: %s", key)
	} else {
		log.Printf("❌ Translation language not found: %s", language)
	}
	return key
}

// Ensure consistency in user preferences - get existing user or create new one
func getUserPreferences(userID int64) User {
	var user User
	botDB.FirstOrCreate(&user, User{ID: userID})
	return user
}

// Update a specific user preference field in the database
func updateUserPreference(userID int64, field string, value interface{}) {
	botDB.Model(&User{}).Where("id = ?", userID).Update(field, value)
	getUsersByFilters()
}

// Add a new subscription for a user
func addSubscription(userID int64, pokemonID int, minIV int, minLevel int, maxDistance int) {
	subscription := Subscription{UserID: userID, PokemonID: pokemonID, MinIV: minIV, MinLevel: minLevel, MaxDistance: maxDistance}
	botDB.Save(&subscription)
	getActiveSubscriptions()
}

// Load all users from database and organize them by notification preferences
func getUsersByFilters() {
	userCache = FilteredUsers{
		All:      make(map[int64]User),
		HundoIV:  []User{},
		ZeroIV:   []User{},
		TopPVP:   []User{},
		Channels: []User{},
	}

	var allUsers []User
	botDB.Find(&allUsers)
	for _, user := range allUsers {
		userCache.All[user.ID] = user
	}
	usersGauge.Set(float64(len(userCache.All)))
	log.Printf("📋 Loaded %d users", len(userCache.All))

	for _, user := range userCache.All {
		if user.Notify {
			if user.HundoIV {
				userCache.HundoIV = append(userCache.HundoIV, user)
			}
			if user.ZeroIV {
				userCache.ZeroIV = append(userCache.ZeroIV, user)
			}
			if user.TopPVP {
				userCache.TopPVP = append(userCache.TopPVP, user)
			}
			if strings.HasPrefix(strconv.FormatInt(user.ID, 10), "-100") {
				userCache.Channels = append(userCache.Channels, user)
			}
		}
	}
}

// Load all subscriptions from database and organize them by Pokémon ID for active users
func getActiveSubscriptions() {
	activeSubscriptions = make(map[int][]Subscription)
	activeSubscriptionCount := 0
	var subscriptions []Subscription
	botDB.Find(&subscriptions)
	for _, subscription := range subscriptions {
		if userCache.All[subscription.UserID].Notify {
			activeSubscriptionCount++
			activeSubscriptions[subscription.PokemonID] = append(activeSubscriptions[subscription.PokemonID], subscription)
		}
	}
	log.Printf("📋 Loaded %d active of %d subscriptions", activeSubscriptionCount, len(subscriptions))
	subscriptionGauge.Set(float64(len(subscriptions)))
	activeSubscriptionGauge.Set(float64(activeSubscriptionCount))
}

// Save a message for cleanup tracking
func saveMessageForCleanup(chatID int64, messageID int, encounterID string) {
	botDB.Create(&Message{ChatID: chatID, MessageID: messageID, EncounterID: encounterID})
}

// Save encounter for expiration tracking
func saveEncounterForCleanup(encounterID string, expiration int64) {
	botDB.Save(&Encounter{ID: encounterID, Expiration: int(expiration)})
}

// Get recent encounters from scanner database
func getRecentEncounters() ([]EncounterData, error) {
	var lastCheck = time.Now().Unix() - 30
	var encounters []EncounterData
	err := scannerDB.Where("iv IS NOT NULL AND updated > ? AND expire_timestamp > ?", lastCheck, lastCheck).Find(&encounters).Error
	return encounters, err
}

// Get user subscriptions by user ID
func getUserSubscriptions(userID int64) []Subscription {
	var subs []Subscription
	botDB.Where("user_id = ?", userID).Order("pokemon_id").Find(&subs)
	return subs
}

// Delete specific subscription by user ID and Pokemon ID
func deleteSubscription(userID int64, pokemonID int) {
	botDB.Where("user_id = ? AND pokemon_id = ?", userID, pokemonID).Delete(&Subscription{})
}

// Delete all subscriptions for a user
func deleteAllUserSubscriptions(userID int64) {
	botDB.Where("user_id = ?", userID).Delete(&Subscription{})
}

// Get expired encounters and their messages for cleanup
func getExpiredEncountersWithMessages() ([]Encounter, map[string][]Message) {
	var encounters []Encounter
	botDB.Where("expiration < ?", time.Now().Unix()).Find(&encounters)

	messagesMap := make(map[string][]Message)
	for _, encounter := range encounters {
		var messages []Message
		botDB.Where("encounter_id = ?", encounter.ID).Find(&messages)
		messagesMap[encounter.ID] = messages
	}

	return encounters, messagesMap
}

// Delete message from database
func deleteMessage(message Message) {
	botDB.Delete(&message)
}

// Delete encounter from database
func deleteEncounter(encounter Encounter) {
	botDB.Delete(&encounter)
}

// Search for gyms by name
func searchGymsByName(gymName string) []GymData {
	var gyms []GymData
	scannerDB.Where("lower(name) LIKE ?", "%"+strings.ToLower(gymName)+"%").Find(&gyms)
	return gyms
}

// Get gym by ID
func getGymByID(gymID string) GymData {
	var gym GymData
	scannerDB.First(&gym, GymData{ID: gymID})
	return gym
}
