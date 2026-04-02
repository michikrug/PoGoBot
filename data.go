package main

import (
	"log"
	"strings"
)

// Global database instances — wired up in initializeApplication.
var (
	botDB     BotDB
	scannerDB ScannerDB
)

// ── Thin wrappers around botDB ───────────────────────────────────────
// These keep handler and notification files unchanged while still delegating
// to the injectable db interface.

func getUserPreferences(userID int64) User {
	return botDB.GetUserPreferences(userID)
}

func updateUserPreference(userID int64, field string, value interface{}) {
	botDB.UpdateUserPreference(userID, field, value)
	getUsersByFilters()
	if strings.EqualFold(field, "notify") {
		getActiveSubscriptions(botDB)
	}
}

func addSubscription(userID int64, pokemonID, minIV, minLevel, maxDistance int) {
	botDB.AddSubscription(userID, pokemonID, minIV, minLevel, maxDistance)
	getActiveSubscriptions(botDB)
}

func getUsersByFilters() {
	allUsers := botDB.GetUsers()
	userCache = FilteredUsers{
		All:      make(map[int64]User),
		HundoIV:  []User{},
		ZeroIV:   []User{},
		TopPVP:   []User{},
		Channels: []User{},
	}
	for _, user := range allUsers {
		userCache.All[user.ID] = user
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
			if isChannelID(user.ID) {
				userCache.Channels = append(userCache.Channels, user)
			}
		}
	}
	log.Printf("📋 Loaded %d users", len(userCache.All))
	usersGauge.Set(float64(len(userCache.All)))
}

func getActiveSubscriptions(db BotDB) {
	activeSubscriptions = make(map[int][]Subscription)
	activeSubscriptionCount := 0
	subscriptions := db.GetSubscriptions()
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

func getUserSubscriptions(userID int64) []Subscription {
	return botDB.GetUserSubscriptions(userID)
}

// userFromCache returns the cached User for the given ID.
// Falls back to a zero-value User with "en" language if not found.
// Use this for read-only display when no preference write has just occurred.
// Use getUserPreferences when you need authoritative data after a write,
// or when accessing a user that may not be in the cache (e.g. impersonation).
func userFromCache(userID int64) User {
	if user, ok := userCache.All[userID]; ok {
		return user
	}
	return User{ID: userID, Language: "en"}
}

func deleteSubscription(userID int64, pokemonID int) {
	botDB.DeleteSubscription(userID, pokemonID)
}

func deleteAllUserSubscriptions(userID int64) {
	botDB.DeleteAllUserSubscriptions(userID)
}

func searchGymsByName(gymName string) []GymData {
	return scannerDB.SearchGymsByName(gymName)
}

func getGymByID(gymID string) GymData {
	return scannerDB.GetGymByID(gymID)
}
