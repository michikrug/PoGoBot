package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"gopkg.in/telebot.v3"
)

// retryAfterSeconds extracts the retry-after duration from a Telegram 429 error.
// Returns 0 if the error is not a rate-limit error.
func retryAfterSeconds(err error) int {
	if err == nil {
		return 0
	}
	s := err.Error()
	// Telegram errors look like: "telegram: retry after 39270 (429)"
	const prefix = "retry after "
	idx := strings.Index(s, prefix)
	if idx == -1 {
		return 0
	}
	rest := s[idx+len(prefix):]
	// rest may be "39270 (429)" – grab the first token
	end := strings.IndexAny(rest, " (")
	if end != -1 {
		rest = rest[:end]
	}
	secs, parseErr := strconv.Atoi(rest)
	if parseErr != nil {
		return 0
	}
	return secs
}

// isPermanentTelegramError reports whether err is a permanent delivery failure
// (user blocked the bot, chat deleted/not found, etc.) that will never succeed on retry.
func isPermanentTelegramError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "chat not found") ||
		strings.Contains(s, "bot was blocked by the user") ||
		strings.Contains(s, "user is deactivated") ||
		strings.Contains(s, "bot was kicked")
}

// maxRateLimitWaitSeconds is the longest we are willing to wait for a Telegram
// rate-limit retry. Encounters expire quickly, so waiting longer than this
// would make the notification irrelevant.
const maxRateLimitWaitSeconds = 30

// userRateLimitedUntil tracks per-user rate-limit expiry times.
// If the current time is before the stored value, all sends for that user are skipped.
var userRateLimitedUntil = make(map[int64]time.Time)

// botSend wraps bot.Send with automatic retry on Telegram 429 rate-limit errors.
// If the required wait exceeds maxRateLimitWaitSeconds the send is abandoned immediately.
// Permanent errors (chat not found, bot blocked, etc.) disable the user's notifications.
func botSend(userID int64, to telebot.Recipient, what interface{}, opts ...interface{}) (*telebot.Message, error) {
	const maxRetries = 3

	// Skip immediately if the user is still within a rate-limit window.
	if until, ok := userRateLimitedUntil[userID]; ok && time.Now().Before(until) {
		return nil, fmt.Errorf("rate limited: user %d paused until %s", userID, until.Format(time.RFC3339))
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		msg, err := bot.Send(to, what, opts...)
		if err == nil {
			return msg, nil
		}
		if isPermanentTelegramError(err) {
			log.Printf("🚫 Permanent Telegram error for user %d, disabling notifications: %v", userID, err)
			updateUserPreference(userID, "notify", false)
			return nil, err
		}
		secs := retryAfterSeconds(err)
		if secs <= 0 || attempt == maxRetries {
			return nil, err
		}
		if secs > maxRateLimitWaitSeconds {
			until := time.Now().Add(time.Duration(secs) * time.Second)
			userRateLimitedUntil[userID] = until
			log.Printf("⏭️ Telegram rate limit too long (%ds > %ds), skipping and pausing user %d until %s", secs, maxRateLimitWaitSeconds, userID, until.Format(time.RFC3339))
			return nil, err
		}
		log.Printf("⏳ Telegram rate limit hit, retrying in %d seconds (attempt %d/%d)…", secs, attempt, maxRetries)
		time.Sleep(time.Duration(secs) * time.Second)
	}
	return nil, fmt.Errorf("unreachable")
}

// sendSticker sends a Pokémon sticker to a user and stores the message for cleanup
func sendSticker(UserID int64, URL string, EncounterID string) error {
	message, err := botSend(UserID, &telebot.User{ID: UserID}, &telebot.Sticker{File: telebot.FromURL(URL)}, &telebot.SendOptions{DisableNotification: true})
	if err != nil {
		log.Printf("❌ Failed to send sticker: %v", err)
	} else {
		messagesCounter.Inc()
		// Store message ID for cleanup
		saveMessageForCleanup(UserID, message.ID, EncounterID)
	}
	return err
}

// sendLocation sends a location to a user and stores the message for cleanup
func sendLocation(UserID int64, Lat float32, Lon float32, EncounterID string) error {
	message, err := botSend(UserID, &telebot.User{ID: UserID}, &telebot.Location{Lat: Lat, Lng: Lon}, &telebot.SendOptions{DisableNotification: true})
	if err != nil {
		log.Printf("❌ Failed to send location: %v", err)
	} else {
		messagesCounter.Inc()
		// Store message ID for cleanup
		saveMessageForCleanup(UserID, message.ID, EncounterID)
	}
	return err
}

// sendVenue sends a venue (location with title and address) to a user and stores the message for cleanup
func sendVenue(UserID int64, Lat float32, Lon float32, Title string, Address string, EncounterID string) error {
	message, err := botSend(UserID, &telebot.User{ID: UserID}, &telebot.Venue{Location: telebot.Location{Lat: Lat, Lng: Lon}, Title: Title, Address: Address})
	if err != nil {
		log.Printf("❌ Failed to send venue: %v", err)
	} else {
		messagesCounter.Inc()
		// Store message ID for cleanup
		saveMessageForCleanup(UserID, message.ID, EncounterID)
	}
	return err
}

// sendMessage sends a text message to a user and stores the message for cleanup
func sendMessage(UserID int64, Text string, EncounterID string) error {
	message, err := botSend(UserID, &telebot.User{ID: UserID}, Text, telebot.ModeMarkdown)
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
	} else {
		messagesCounter.Inc()
		// Store message ID for cleanup
		saveMessageForCleanup(UserID, message.ID, EncounterID)
	}
	return err
}

// sendEncounterNotification sends a complete notification for a Pokémon encounter to a user
func sendEncounterNotification(user User, encounter EncounterData) {
	// Check if encounter has already been notified
	if _, exists := notificationCache[encounter.ID][user.ID]; exists {
		log.Printf("🔕 Skipping notification for Pokémon #%d to %d (already sent)", encounter.PokemonID, user.ID)
		return
	}
	log.Printf("🔔 Sending notification for Pokémon #%d to %d", encounter.PokemonID, user.ID)
	saveEncounterForCleanup(encounter.ID, int64(*encounter.ExpireTimestamp))
	if notificationCache[encounter.ID] == nil {
		notificationCache[encounter.ID] = make(map[int64]struct{})
	}
	notificationCache[encounter.ID][user.ID] = struct{}{}
	notificationsCounter.Inc()

	if !user.OnlyMap && user.Stickers {
		var formSuffix string
		// Determine if a non-default form sticker should be used.
		if encounter.Form != nil && *encounter.Form > 0 {
			pokemonKey := strconv.Itoa(encounter.PokemonID)
			formKey := strconv.Itoa(*encounter.Form)
			if pkm, exists := gameData.Pokemon[pokemonKey]; exists {
				if form, exists := pkm.Forms[formKey]; exists && form.Name != "Normal" {
					formSuffix = fmt.Sprintf("_f%s", formKey)
				}
			}
		}
		// Build and send the sticker URL.
		stickerURL := fmt.Sprintf("https://raw.githubusercontent.com/WatWowMap/wwm-uicons-webp/main/pokemon/%d%s.webp", encounter.PokemonID, formSuffix)
		sendSticker(user.ID, stickerURL, encounter.ID)
	}
	if !user.OnlyMap {
		sendLocation(user.ID, encounter.Lat, encounter.Lon, encounter.ID)
	}

	// Generate notification title and content
	notificationTitle := generateNotificationTitle(user, encounter)
	notificationText := generateNotificationText(user, encounter)

	if !user.OnlyMap {
		sendMessage(user.ID, notificationTitle+"\n"+notificationText, encounter.ID)
	} else {
		sendVenue(user.ID, encounter.Lat, encounter.Lon, notificationTitle, notificationText, encounter.ID)
	}
}

// generateNotificationTitle creates the title text for a Pokémon encounter notification
func generateNotificationTitle(user User, encounter EncounterData) string {
	// Retrieve Pokémon name
	name := getPokemonName(encounter.PokemonID, user.Language)

	// Build form suffix if applicable
	formSuffix := buildFormSuffix(encounter, user.Language)

	// Get display components
	genderEmoji := getGenderEmoji(encounter.Gender)
	cpLabel := getCPLabel(user.Language)
	sizeEmoji := getSizeEmoji(encounter.Size)
	weatherEmoji := getWeatherEmoji(encounter.Weather)

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

// buildFormSuffix constructs the form suffix string for non-normal forms
func buildFormSuffix(encounter EncounterData, language string) string {
	if encounter.Form == nil || *encounter.Form <= 0 {
		return ""
	}

	pkm, exists := gameData.Pokemon[strconv.Itoa(encounter.PokemonID)]
	if !exists {
		return ""
	}

	form, exists := pkm.Forms[strconv.Itoa(*encounter.Form)]
	if !exists || form.Name == "Normal" {
		return ""
	}

	costumeEmoji := ""
	if form.IsCostume {
		costumeEmoji = "👕 "
	}

	return fmt.Sprintf(" (%s%s)", costumeEmoji, getTranslation(form.Name, language))
}

// getGenderEmoji returns the appropriate gender emoji
func getGenderEmoji(gender *int) string {
	if gender == nil {
		return ""
	}
	return genderMap[*gender]
}

// getCPLabel returns the appropriate CP label based on language
func getCPLabel(language string) string {
	if language == "en" {
		return "CP"
	}
	return "WP"
}

// getSizeEmoji returns the appropriate size emoji
func getSizeEmoji(size *int) string {
	if size == nil {
		return ""
	}

	switch *size {
	case 1:
		return " 🔹"
	case 5:
		return " 🔶"
	default:
		return ""
	}
}

// getWeatherEmoji returns the appropriate weather emoji
func getWeatherEmoji(weather *int) string {
	if weather == nil {
		return ""
	}
	return " " + weatherMap[*weather]
}

// generateNotificationText creates the body text for a Pokémon encounter notification
func generateNotificationText(user User, encounter EncounterData) string {
	expireTime := time.Unix(int64(*encounter.ExpireTimestamp), 0).In(timezone)
	timeLeft := time.Until(expireTime)

	var notificationText strings.Builder
	if user.Latitude != 0 && user.Longitude != 0 {
		distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
		if distance < 1000 {
			notificationText.WriteString(fmt.Sprintf("📍 %.0fm\n", distance))
		} else {
			notificationText.WriteString(fmt.Sprintf("📍 %.2fkm\n", distance/1000))
		}
	}

	notificationText.WriteString(fmt.Sprintf("💨 %s ⏳ %s\n",
		expireTime.Format(time.TimeOnly),
		timeLeft.Truncate(time.Second).String()))

	if encounter.Move1 != nil && encounter.Move2 != nil {
		notificationText.WriteString(fmt.Sprintf("💥 %s / %s",
			getMoveName(*encounter.Move1, user.Language),
			getMoveName(*encounter.Move2, user.Language)))
	}

	if encounter.PVPData != nil {
		for league, entries := range encounter.PVPData {
			// Capitalize the league name
			leagueName := strings.ToUpper(string(league[0])) + league[1:]
			for _, entry := range entries {
				if entry.Rank < 4 {
					notificationText.WriteString("\n" +
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

	return notificationText.String()
}

// processEncounters fetches and processes Pokémon encounters for notifications
func processEncounters() {
	// Fetch current Pokémon encounters
	encounters, err := getRecentEncounters()
	if err != nil {
		log.Printf("❌ Failed to fetch Pokémon encounters: %v", err)
	} else {
		encounterGauge.Set(float64(len(encounters)))
		log.Printf("✅ Found %d Pokémon", len(encounters))
		filterAndSendEncounters(userCache, encounters)
	}
}

// filterAndSendEncounters matches encounters with user subscriptions and sends notifications
func filterAndSendEncounters(users FilteredUsers, encounters []EncounterData) {
	// Match encounters with subscriptions
	for _, encounter := range encounters {

		// Process PVP data if available.
		if encounter.PVP != nil && *encounter.PVP != "" {
			var pvpData PVP
			if err := json.Unmarshal([]byte(*encounter.PVP), &pvpData); err != nil {
				log.Printf("❌ Failed to decode PVP data for encounter %s: %v", encounter.ID, err)
			} else {
				for league, entries := range pvpData {
					for _, entry := range entries {
						// Only consider top 3 rankings.
						if entry.Rank >= 4 {
							continue
						}
						encounter.PVPData = pvpData
						log.Printf("🎉 Top 3 %s league encounter - Pokemon: %s, CP: %d, Rank: %d, Percentage: %f, Level: %f",
							league, getPokemonName(entry.Pokemon, "en"), entry.CP, entry.Rank, entry.Percentage, entry.Level)
						for _, user := range users.TopPVP {
							if withinDistance(user, encounter, user.MaxDistance) {
								sendEncounterNotification(user, encounter)
							}
						}
					}
				}
			}
		}

		// Process 100% IV Pokémon notifications.
		if encounter.IV != nil && *encounter.IV == 100 {
			for _, user := range users.HundoIV {
				if withinDistance(user, encounter, user.MaxDistance) {
					sendEncounterNotification(user, encounter)
				}
			}
		}

		// Process 0% IV Pokémon notifications.
		if encounter.IV != nil && *encounter.IV == 0 {
			for _, user := range users.ZeroIV {
				if withinDistance(user, encounter, user.MaxDistance) {
					sendEncounterNotification(user, encounter)
				}
			}
		}

		// Process channel user notifications.
		for _, user := range users.Channels {
			// If both thresholds are zero, skip.
			if user.MinIV == 0 && user.MinLevel == 0 {
				continue
			}
			ivOk := (user.MinLevel == 0 && *encounter.IV >= float32(user.MinIV)) ||
				(user.MinIV == 0 && *encounter.Level >= user.MinLevel) ||
				(*encounter.IV >= float32(user.MinIV) && *encounter.Level >= user.MinLevel)
			if ivOk {
				sendEncounterNotification(user, encounter)
			}
		}

		// Process subscribed Pokémon notifications.
		if subs, exists := activeSubscriptions[encounter.PokemonID]; exists {
			for _, sub := range subs {
				user := users.All[sub.UserID]

				// Determine effective subscription limits (fallback to user defaults).
				effectiveMinIV := sub.MinIV
				if effectiveMinIV == 0 {
					effectiveMinIV = user.MinIV
				}
				effectiveMinLevel := sub.MinLevel
				if effectiveMinLevel == 0 {
					effectiveMinLevel = user.MinLevel
				}
				effectiveMaxDistance := sub.MaxDistance
				if effectiveMaxDistance == 0 {
					effectiveMaxDistance = user.MaxDistance
				}

				// Validate encounter IV and level.
				if effectiveMinIV > 0 && *encounter.IV < float32(effectiveMinIV) {
					continue
				}
				if effectiveMinLevel > 0 && *encounter.Level < effectiveMinLevel {
					continue
				}
				if !withinDistance(user, encounter, effectiveMaxDistance) {
					continue
				}
				sendEncounterNotification(user, encounter)
			}
		}
	}
}

// cleanupMessages removes expired messages and encounters from the database
func cleanupMessages() {
	deletedMessagesCount := 0
	encounters, messagesMap := getExpiredEncountersWithMessages()
	log.Printf("🗑️ Found %d expired encounters", len(encounters))

	for _, encounter := range encounters {
		messages := messagesMap[encounter.ID]
		if len(messages) > 0 {
			log.Printf("🗑️ Found %d expired messages for encounter %s", len(messages), encounter.ID)
		}

		for _, message := range messages {
			user := userCache.All[message.ChatID]
			if user.Cleanup {
				deletedMessagesCount++
				if err := bot.Delete(&telebot.StoredMessage{MessageID: strconv.Itoa(message.MessageID), ChatID: message.ChatID}); err != nil {
					log.Printf("❌ Failed to delete message %d for user %d: %v", message.MessageID, message.ChatID, err)
				}
			}
			deleteMessage(message)
		}
		deleteEncounter(encounter)
		notificationCache[encounter.ID] = nil
	}

	cleanupCounter.Add(float64(deletedMessagesCount))
}

// startNotificationProcessing starts the background processes for handling notifications
func startNotificationProcessing() {
	// Background process to match encounters with subscriptions
	go func() {
		for {
			time.Sleep(30 * time.Second)
			cleanupMessages()
			processEncounters()
		}
	}()
}
