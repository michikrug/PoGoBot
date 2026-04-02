package main

import "gopkg.in/telebot.v3"

// BotDB defines all operations against the bot database.
type BotDB interface {
	GetUserPreferences(userID int64) User
	UpdateUserPreference(userID int64, field string, value interface{})
	AddSubscription(userID int64, pokemonID, minIV, minLevel, maxDistance int)
	GetUsers() []User
	GetSubscriptions() []Subscription
	SaveMessage(chatID int64, messageID int, encounterID string)
	SaveEncounter(encounterID string, expiration int64)
	GetUserSubscriptions(userID int64) []Subscription
	DeleteSubscription(userID int64, pokemonID int)
	DeleteAllUserSubscriptions(userID int64)
	GetExpiredEncountersWithMessages() ([]Encounter, map[string][]Message)
	DeleteMessage(message Message)
	DeleteEncounter(encounter Encounter)
}

// ScannerDB defines all read operations against the scanner database.
type ScannerDB interface {
	GetRecentEncounters() ([]EncounterData, error)
	SearchGymsByName(name string) []GymData
	GetGymByID(id string) GymData
}

// BotSender wraps the Telegram bot send/delete surface so it can be mocked in tests.
type BotSender interface {
	Send(to telebot.Recipient, what interface{}, opts ...interface{}) (*telebot.Message, error)
	Delete(msg telebot.Editable) error
}

// telegramBotSender adapts *telebot.Bot to the BotSender interface.
type telegramBotSender struct {
	bot *telebot.Bot
}

func (t *telegramBotSender) Send(to telebot.Recipient, what interface{}, opts ...interface{}) (*telebot.Message, error) {
	return t.bot.Send(to, what, opts...)
}

func (t *telegramBotSender) Delete(msg telebot.Editable) error {
	return t.bot.Delete(msg)
}
