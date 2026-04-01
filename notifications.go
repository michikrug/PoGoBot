package main

import (
	"time"

	"gopkg.in/telebot.v3"
)

// notificationService is the singleton used by the production application.
// Tests create their own NotificationService with mocked dependencies instead.
var notificationService *NotificationService

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

// ── Package-level wrappers kept for backward-compat with handlers.go ─────────

func botSend(userID int64, to telebot.Recipient, what interface{}, opts ...interface{}) (*telebot.Message, error) {
	return notificationService.botSend(userID, to, what, opts...)
}

func sendSticker(userID int64, url string, encounterID string) error {
	return notificationService.sendSticker(userID, url, encounterID)
}

func sendLocation(userID int64, lat float32, lon float32, encounterID string) error {
	return notificationService.sendLocation(userID, lat, lon, encounterID)
}

func sendVenue(userID int64, lat float32, lon float32, title string, address string, encounterID string) error {
	return notificationService.sendVenue(userID, lat, lon, title, address, encounterID)
}

func sendMessage(userID int64, text string, encounterID string) error {
	return notificationService.sendMessage(userID, text, encounterID)
}

func sendEncounterNotification(user User, encounter EncounterData) {
	notificationService.sendEncounterNotification(user, encounter)
}

func filterAndSendEncounters(users FilteredUsers, encounters []EncounterData) {
	notificationService.filterAndSendEncounters(users, encounters, activeSubscriptions)
}

func cleanupMessages() {
	notificationService.cleanupMessages(userCache)
}

func processEncounters() {
	notificationService.processEncounters(userCache, activeSubscriptions)
}

func generateNotificationTitle(user User, encounter EncounterData) string {
	return notificationService.generateNotificationTitle(user, encounter)
}

func generateNotificationText(user User, encounter EncounterData) string {
	return notificationService.generateNotificationText(user, encounter)
}

func buildFormSuffix(encounter EncounterData, language string) string {
	return notificationService.buildFormSuffix(encounter, language)
}

// startNotificationProcessing starts the background goroutine.
func startNotificationProcessing() {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			cleanupMessages()
			processEncounters()
		}
	}()
}
