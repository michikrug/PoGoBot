package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/telebot.v3"
)

// NotificationService encapsulates all notification processing with injectable
// dependencies so the logic can be unit-tested without a real DB or Telegram bot.
type NotificationService struct {
	botDB     BotDB
	scannerDB ScannerDB
	sender    BotSender

	gameData     *MasterFile
	translations map[string]map[string]string
	timezone     *time.Location

	// per-user deduplication: encounterID → set of already-notified user IDs
	notificationCache map[string]map[int64]struct{}
	// per-user rate-limit: userID → time until which sends are skipped
	userRateLimitedUntil map[int64]time.Time

	// prometheus metrics (nil-safe: all writes are guarded)
	notificationsCounter prometheus.Counter
	messagesCounter      prometheus.Counter
	cleanupCounter       prometheus.Gauge
	encounterGauge       prometheus.Gauge
}

// newNotificationService creates a ready-to-use NotificationService.
func newNotificationService(
	botDB BotDB,
	scannerDB ScannerDB,
	sender BotSender,
	masterFile *MasterFile,
	translationsMap map[string]map[string]string,
	timezone *time.Location,
	notificationsCounter prometheus.Counter,
	messagesCounter prometheus.Counter,
	cleanupCounter prometheus.Gauge,
	encounterGauge prometheus.Gauge,
) *NotificationService {
	return &NotificationService{
		botDB:                botDB,
		scannerDB:            scannerDB,
		sender:               sender,
		gameData:             masterFile,
		translations:         translationsMap,
		timezone:             timezone,
		notificationCache:    make(map[string]map[int64]struct{}),
		userRateLimitedUntil: make(map[int64]time.Time),
		notificationsCounter: notificationsCounter,
		messagesCounter:      messagesCounter,
		cleanupCounter:       cleanupCounter,
		encounterGauge:       encounterGauge,
	}
}

// incNotifications increments the notifications counter (nil-safe).
func (s *NotificationService) incNotifications() {
	if s.notificationsCounter != nil {
		s.notificationsCounter.Inc()
	}
}

// incMessages increments the messages counter (nil-safe).
func (s *NotificationService) incMessages() {
	if s.messagesCounter != nil {
		s.messagesCounter.Inc()
	}
}

// addCleanup increments the cleanup gauge (nil-safe).
func (s *NotificationService) addCleanup(amount float64) {
	if s.cleanupCounter != nil {
		s.cleanupCounter.Add(amount)
	}
}

// setEncounterGauge sets the encounter gauge (nil-safe).
func (s *NotificationService) setEncounterGauge(count float64) {
	if s.encounterGauge != nil {
		s.encounterGauge.Set(count)
	}
}

// ── Send helpers ──────────────────────────────────────────────────────────────

// retryAfterSeconds extracts the retry-after duration from a Telegram 429 error.
// Returns 0 if the error is not a rate-limit error.
func retryAfterSeconds(err error) int {
	var floodErr telebot.FloodError
	if errors.As(err, &floodErr) {
		return floodErr.RetryAfter
	}
	return 0
}

// isPermanentTelegramError reports whether err is a permanent delivery failure.
func isPermanentTelegramError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "chat not found") ||
		strings.Contains(errMsg, "bot was blocked by the user") ||
		strings.Contains(errMsg, "user is deactivated") ||
		strings.Contains(errMsg, "bot was kicked")
}

// isRateLimited reports whether the user is currently rate-limited.
// Evicts the entry if the window has already expired.
func (s *NotificationService) isRateLimited(userID int64) bool {
	if until, ok := s.userRateLimitedUntil[userID]; ok {
		if time.Now().Before(until) {
			return true
		}
		delete(s.userRateLimitedUntil, userID)
	}
	return false
}

// botSend wraps sender.Send with Telegram rate-limit and permanent-error handling.
func (s *NotificationService) botSend(userID int64, to telebot.Recipient, what interface{}, opts ...interface{}) (*telebot.Message, error) {
	if s.isRateLimited(userID) {
		return nil, fmt.Errorf("rate limited: user %d paused until %s", userID, s.userRateLimitedUntil[userID].Format(time.RFC3339))
	}

	msg, err := s.sender.Send(to, what, opts...)
	if err == nil {
		return msg, nil
	}
	if isPermanentTelegramError(err) {
		log.Printf("🚫 Permanent Telegram error for user %d, disabling notifications: %v", userID, err)
		updateUserPreference(userID, "Notify", false)
		return nil, err
	}
	if secs := retryAfterSeconds(err); secs > 0 {
		until := time.Now().Add(time.Duration(secs) * time.Second)
		s.userRateLimitedUntil[userID] = until
		log.Printf("⏭️ Rate limit (%ds), pausing user %d until %s", secs, userID, until.Format(time.RFC3339))
	}
	return nil, err
}

func (s *NotificationService) sendSticker(userID int64, url, encounterID string) error {
	msg, err := s.botSend(userID, &telebot.User{ID: userID}, &telebot.Sticker{File: telebot.FromURL(url)}, &telebot.SendOptions{DisableNotification: true})
	if err != nil {
		log.Printf("❌ Failed to send sticker: %v", err)
		return err
	}
	s.incMessages()
	s.botDB.SaveMessage(userID, msg.ID, encounterID)
	return nil
}

func (s *NotificationService) sendLocation(userID int64, lat, lon float32, encounterID string) error {
	msg, err := s.botSend(userID, &telebot.User{ID: userID}, &telebot.Location{Lat: lat, Lng: lon}, &telebot.SendOptions{DisableNotification: true})
	if err != nil {
		log.Printf("❌ Failed to send location: %v", err)
		return err
	}
	s.incMessages()
	s.botDB.SaveMessage(userID, msg.ID, encounterID)
	return nil
}

func (s *NotificationService) sendVenue(userID int64, lat, lon float32, title, address, encounterID string) error {
	msg, err := s.botSend(userID, &telebot.User{ID: userID}, &telebot.Venue{
		Location: telebot.Location{Lat: lat, Lng: lon},
		Title:    title,
		Address:  address,
	})
	if err != nil {
		log.Printf("❌ Failed to send venue: %v", err)
		return err
	}
	s.incMessages()
	s.botDB.SaveMessage(userID, msg.ID, encounterID)
	return nil
}

func (s *NotificationService) sendMessage(userID int64, text, encounterID string) error {
	msg, err := s.botSend(userID, &telebot.User{ID: userID}, text, telebot.ModeMarkdown)
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
		return err
	}
	s.incMessages()
	s.botDB.SaveMessage(userID, msg.ID, encounterID)
	return nil
}

// ── Core notification logic ───────────────────────────────────────────────────

// sendEncounterNotification delivers a full encounter notification to one user.
func (s *NotificationService) sendEncounterNotification(user User, encounter EncounterData) {
	if s.isRateLimited(user.ID) {
		log.Printf("⏭️ Skipping notification for Pokémon #%d to %d (rate limited)", encounter.PokemonID, user.ID)
		return
	}
	if _, exists := s.notificationCache[encounter.ID][user.ID]; exists {
		log.Printf("🔕 Skipping notification for Pokémon #%d to %d (already sent)", encounter.PokemonID, user.ID)
		return
	}
	log.Printf("🔔 Sending notification for Pokémon #%d to %d", encounter.PokemonID, user.ID)

	if encounter.ExpireTimestamp != nil {
		s.botDB.SaveEncounter(encounter.ID, int64(*encounter.ExpireTimestamp))
	}
	if s.notificationCache[encounter.ID] == nil {
		s.notificationCache[encounter.ID] = make(map[int64]struct{})
	}
	s.notificationCache[encounter.ID][user.ID] = struct{}{}
	s.incNotifications()

	if !user.OnlyMap && user.Stickers {
		formSuffix := ""
		if encounter.Form != nil && *encounter.Form > 0 {
			pokemonKey := strconv.Itoa(encounter.PokemonID)
			formKey := strconv.Itoa(*encounter.Form)
			if pokemon, exists := s.gameData.Pokemon[pokemonKey]; exists {
				if form, exists := pokemon.Forms[formKey]; exists && form.Name != "Normal" {
					formSuffix = fmt.Sprintf("_f%s", formKey)
				}
			}
		}
		stickerURL := fmt.Sprintf("https://raw.githubusercontent.com/WatWowMap/wwm-uicons-webp/main/pokemon/%d%s.webp", encounter.PokemonID, formSuffix)
		s.sendSticker(user.ID, stickerURL, encounter.ID)
	}
	if !user.OnlyMap {
		s.sendLocation(user.ID, encounter.Lat, encounter.Lon, encounter.ID)
	}

	title := s.generateNotificationTitle(user, encounter)
	body := s.generateNotificationText(user, encounter)

	if !user.OnlyMap {
		s.sendMessage(user.ID, title+"\n"+body, encounter.ID)
	} else {
		s.sendVenue(user.ID, encounter.Lat, encounter.Lon, title, body, encounter.ID)
	}
}

// filterAndSendEncounters matches each encounter against every user category
// and subscription, then fires notifications.
func (s *NotificationService) filterAndSendEncounters(users FilteredUsers, encounters []EncounterData, activeSubs map[int][]Subscription) {
	for _, encounter := range encounters {

		// PVP top-3 notifications.
		if encounter.PVP != nil && *encounter.PVP != "" {
			var pvpData PVP
			if err := json.Unmarshal([]byte(*encounter.PVP), &pvpData); err != nil {
				log.Printf("❌ Failed to decode PVP data for encounter %s: %v", encounter.ID, err)
			} else {
				for league, entries := range pvpData {
					for _, entry := range entries {
						if entry.Rank >= 4 {
							continue
						}
						encounter.PVPData = pvpData
						log.Printf("🎉 Top 3 %s league – %s CP:%d Rank:%d", league, getPokemonName(entry.Pokemon, "en"), entry.CP, entry.Rank)
						for _, user := range users.TopPVP {
							if withinDistance(user, encounter, user.MaxDistance) {
								s.sendEncounterNotification(user, encounter)
							}
						}
					}
				}
			}
		}

		// 100% IV notifications.
		if encounter.IV != nil && *encounter.IV == 100 {
			for _, user := range users.HundoIV {
				if withinDistance(user, encounter, user.MaxDistance) {
					s.sendEncounterNotification(user, encounter)
				}
			}
		}

		// 0% IV notifications.
		if encounter.IV != nil && *encounter.IV == 0 {
			for _, user := range users.ZeroIV {
				if withinDistance(user, encounter, user.MaxDistance) {
					s.sendEncounterNotification(user, encounter)
				}
			}
		}

		// Channel threshold notifications.
		for _, user := range users.Channels {
			if user.MinIV == 0 && user.MinLevel == 0 {
				continue
			}
			if encounter.IV == nil || encounter.Level == nil {
				continue
			}
			ivOk := (user.MinLevel == 0 && *encounter.IV >= float32(user.MinIV)) ||
				(user.MinIV == 0 && *encounter.Level >= user.MinLevel) ||
				(*encounter.IV >= float32(user.MinIV) && *encounter.Level >= user.MinLevel)
			if ivOk {
				s.sendEncounterNotification(user, encounter)
			}
		}

		// Per-Pokémon subscription notifications.
		if subscriptions, exists := activeSubs[encounter.PokemonID]; exists {
			for _, subscription := range subscriptions {
				user := users.All[subscription.UserID]

				effectiveMinIV := subscription.MinIV
				if effectiveMinIV == 0 {
					effectiveMinIV = user.MinIV
				}
				effectiveMinLevel := subscription.MinLevel
				if effectiveMinLevel == 0 {
					effectiveMinLevel = user.MinLevel
				}
				effectiveMaxDistance := subscription.MaxDistance
				if effectiveMaxDistance == 0 {
					effectiveMaxDistance = user.MaxDistance
				}

				if effectiveMinIV > 0 && (encounter.IV == nil || *encounter.IV < float32(effectiveMinIV)) {
					continue
				}
				if effectiveMinLevel > 0 && (encounter.Level == nil || *encounter.Level < effectiveMinLevel) {
					continue
				}
				if !withinDistance(user, encounter, effectiveMaxDistance) {
					continue
				}
				s.sendEncounterNotification(user, encounter)
			}
		}
	}
}

// cleanupMessages deletes Telegram messages for expired encounters.
func (s *NotificationService) cleanupMessages(users FilteredUsers) {
	deletedCount := 0
	encounters, messagesMap := s.botDB.GetExpiredEncountersWithMessages()
	log.Printf("🗑️ Found %d expired encounters", len(encounters))

	for _, encounter := range encounters {
		messages := messagesMap[encounter.ID]
		if len(messages) > 0 {
			log.Printf("🗑️ Found %d expired messages for encounter %s", len(messages), encounter.ID)
		}
		for _, message := range messages {
			user := users.All[message.ChatID]
			if user.Cleanup {
				deletedCount++
				if err := s.sender.Delete(&telebot.StoredMessage{
					MessageID: strconv.Itoa(message.MessageID),
					ChatID:    message.ChatID,
				}); err != nil {
					log.Printf("❌ Failed to delete message %d for user %d: %v", message.MessageID, message.ChatID, err)
				}
			}
			s.botDB.DeleteMessage(message)
		}
		s.botDB.DeleteEncounter(encounter)
		s.notificationCache[encounter.ID] = nil
	}

	s.addCleanup(float64(deletedCount))
}

// processEncounters fetches scanner encounters and dispatches notifications.
func (s *NotificationService) processEncounters(users FilteredUsers, activeSubs map[int][]Subscription) {
	encounters, err := s.scannerDB.GetRecentEncounters()
	if err != nil {
		log.Printf("❌ Failed to fetch Pokémon encounters: %v", err)
		return
	}
	s.setEncounterGauge(float64(len(encounters)))
	log.Printf("✅ Found %d Pokémon", len(encounters))
	s.filterAndSendEncounters(users, encounters, activeSubs)
}

// ── Notification text generation ─────────────────────────────────────────────
// These are methods so they can read gameData/translations from the service,
// but they have no side effects and are therefore straightforward to unit-test.

func (s *NotificationService) generateNotificationTitle(user User, encounter EncounterData) string {
	name := getPokemonName(encounter.PokemonID, user.Language)
	formSuffix := s.buildFormSuffix(encounter, user.Language)
	genderEmoji := getGenderEmoji(encounter.Gender)
	cpLabel := getTranslation("CP", user.Language)
	sizeEmoji := getSizeEmoji(encounter.Size)
	weatherEmoji := getWeatherEmoji(encounter.Weather)

	if encounter.IV == nil || encounter.AtkIV == nil || encounter.DefIV == nil ||
		encounter.StaIV == nil || encounter.CP == nil || encounter.Level == nil {
		return fmt.Sprintf("*🔔 %s%s %s*%s%s",
			name, formSuffix, genderEmoji, sizeEmoji, weatherEmoji)
	}

	return fmt.Sprintf("*🔔 %s%s %s %.1f%% %d|%d|%d %d%s L%d*%s%s",
		name,
		formSuffix,
		genderEmoji,
		*encounter.IV,
		*encounter.AtkIV,
		*encounter.DefIV,
		*encounter.StaIV,
		*encounter.CP,
		cpLabel,
		*encounter.Level,
		sizeEmoji,
		weatherEmoji,
	)
}

func (s *NotificationService) buildFormSuffix(encounter EncounterData, language string) string {
	if encounter.Form == nil || *encounter.Form <= 0 {
		return ""
	}
	pokemon, exists := s.gameData.Pokemon[strconv.Itoa(encounter.PokemonID)]
	if !exists {
		return ""
	}
	form, exists := pokemon.Forms[strconv.Itoa(*encounter.Form)]
	if !exists || form.Name == "Normal" {
		return ""
	}
	costumeEmoji := ""
	if form.IsCostume {
		costumeEmoji = "👕 "
	}
	return fmt.Sprintf(" (%s%s)", costumeEmoji, getTranslation(form.Name, language))
}

func (s *NotificationService) generateNotificationText(user User, encounter EncounterData) string {
	var sb strings.Builder

	if encounter.ExpireTimestamp != nil {
		expireTime := time.Unix(int64(*encounter.ExpireTimestamp), 0).In(s.timezone)
		timeLeft := time.Until(expireTime)
		if user.Latitude != 0 && user.Longitude != 0 {
			distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
			if distance < 1000 {
				sb.WriteString(fmt.Sprintf("📍 %.0fm\n", distance))
			} else {
				sb.WriteString(fmt.Sprintf("📍 %.2fkm\n", distance/1000))
			}
		}
		sb.WriteString(fmt.Sprintf("💨 %s ⏳ %s\n",
			expireTime.Format(time.TimeOnly),
			timeLeft.Truncate(time.Second).String()))
	} else if user.Latitude != 0 && user.Longitude != 0 {
		distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
		if distance < 1000 {
			sb.WriteString(fmt.Sprintf("📍 %.0fm\n", distance))
		} else {
			sb.WriteString(fmt.Sprintf("📍 %.2fkm\n", distance/1000))
		}
	}

	if encounter.Move1 != nil && encounter.Move2 != nil {
		sb.WriteString(fmt.Sprintf("💥 %s / %s",
			getMoveName(*encounter.Move1, user.Language),
			getMoveName(*encounter.Move2, user.Language)))
	}

	if encounter.PVPData != nil {
		for league, entries := range encounter.PVPData {
			leagueName := strings.ToUpper(string(league[0])) + league[1:]
			for _, entry := range entries {
				if entry.Rank < 4 {
					sb.WriteString("\n" +
						getTranslation(fmt.Sprintf("🏅 *%s League Rank", leagueName), user.Language) +
						fmt.Sprintf(getTranslation(" %d*: %s %dCP L%.1f", user.Language),
							entry.Rank,
							getPokemonName(entry.Pokemon, user.Language),
							entry.CP,
							entry.Level,
						))
				}
			}
		}
	}

	return sb.String()
}
