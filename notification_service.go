package main

import (
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
	notificationsCounter     prometheus.Counter
	messagesCounter          prometheus.Counter
	cleanupCounter           prometheus.Gauge
	encounterGauge           prometheus.Gauge
	raidNotificationsCounter prometheus.Counter
	raidEncounterGauge       prometheus.Gauge
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
	raidNotificationsCounter prometheus.Counter,
	raidEncounterGauge prometheus.Gauge,
) *NotificationService {
	return &NotificationService{
		botDB:                    botDB,
		scannerDB:                scannerDB,
		sender:                   sender,
		gameData:                 masterFile,
		translations:             translationsMap,
		timezone:                 timezone,
		notificationCache:        make(map[string]map[int64]struct{}),
		userRateLimitedUntil:     make(map[int64]time.Time),
		notificationsCounter:     notificationsCounter,
		messagesCounter:          messagesCounter,
		cleanupCounter:           cleanupCounter,
		encounterGauge:           encounterGauge,
		raidNotificationsCounter: raidNotificationsCounter,
		raidEncounterGauge:       raidEncounterGauge,
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

// incRaidNotifications increments the raid notifications counter (nil-safe).
func (s *NotificationService) incRaidNotifications() {
	if s.raidNotificationsCounter != nil {
		s.raidNotificationsCounter.Inc()
	}
}

// setRaidEncounterGauge sets the raid encounter gauge (nil-safe).
func (s *NotificationService) setRaidEncounterGauge(count float64) {
	if s.raidEncounterGauge != nil {
		s.raidEncounterGauge.Set(count)
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
		s.botDB.UpdateUserPreference(userID, "Notify", false)
		getActiveSubscriptions(s.botDB)
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
		return
	}
	if _, exists := s.notificationCache[encounter.ID][user.ID]; exists {
		log.Printf("🔕 Skipping notification for Pokémon #%d to %d (already sent)", encounter.PokemonID, user.ID)
		return
	}
	log.Printf("🔔 Sending notification for Pokémon #%d to %d", encounter.PokemonID, user.ID)

	if s.notificationCache[encounter.ID] == nil {
		s.notificationCache[encounter.ID] = make(map[int64]struct{})
		if encounter.ExpireTimestamp != nil {
			s.botDB.SaveEncounter(encounter.ID, int64(*encounter.ExpireTimestamp))
		}
	}
	s.notificationCache[encounter.ID][user.ID] = struct{}{}

	if !user.OnlyMap && user.Stickers {
		formSuffix := ""
		if key := s.resolveFormKey(encounter.PokemonID, encounter.Form); key != "" {
			formSuffix = "_f" + key
		}
		stickerURL := fmt.Sprintf("https://raw.githubusercontent.com/WatWowMap/wwm-uicons-webp/main/pokemon/%d%s.webp", encounter.PokemonID, formSuffix)
		if err := s.sendSticker(user.ID, stickerURL, encounter.ID); err != nil {
			return
		}
	}

	title := s.generateNotificationTitle(user, encounter)
	body := s.generateNotificationText(user, encounter)

	if !user.OnlyMap {
		if err := s.sendLocation(user.ID, encounter.Lat, encounter.Lon, encounter.ID); err != nil {
			return
		}
		if err := s.sendMessage(user.ID, title+"\n"+body, encounter.ID); err != nil {
			return
		}
	} else {
		if err := s.sendVenue(user.ID, encounter.Lat, encounter.Lon, title, body, encounter.ID); err != nil {
			return
		}
	}
	s.incNotifications()
}

// filterAndSendEncounters matches each encounter against every user category
// and subscription, then fires notifications.
func (s *NotificationService) filterAndSendEncounters(users FilteredUsers, encounters []EncounterData, activeSubs map[int][]Subscription) {
	for _, encounter := range encounters {
		type distKey struct {
			userID  int64
			maxDist int
		}
		distCache := make(map[distKey]bool)
		inRange := func(user User, maxDist int) bool {
			key := distKey{user.ID, maxDist}
			if v, ok := distCache[key]; ok {
				return v
			}
			v := withinDistance(user, encounter.Lat, encounter.Lon, maxDist)
			distCache[key] = v
			return v
		}

		// PVP top-3 notifications.
		for league, entries := range encounter.PVPData {
			for _, entry := range entries {
				if entry.Rank >= 4 {
					continue
				}
				log.Printf("🎉 Top 3 %s league - %s CP:%d Rank:%d", league, newTranslator("en").PokemonName(entry.Pokemon), entry.CP, entry.Rank)
				for _, user := range users.TopPVP {
					if inRange(user, user.MaxDistance) {
						s.sendEncounterNotification(user, encounter)
					}
				}
			}
		}

		// 100% IV notifications.
		if encounter.IV != nil && *encounter.IV == 100 {
			for _, user := range users.HundoIV {
				if inRange(user, user.MaxDistance) {
					s.sendEncounterNotification(user, encounter)
				}
			}
		}

		// 0% IV notifications.
		if encounter.IV != nil && *encounter.IV == 0 {
			for _, user := range users.ZeroIV {
				if inRange(user, user.MaxDistance) {
					s.sendEncounterNotification(user, encounter)
				}
			}
		}

		// Channel threshold notifications.
		// Channels serve a group of people so distance is intentionally not checked here.
		if encounter.IV != nil && encounter.Level != nil {
			for _, user := range users.Channels {
				if user.MinIV == 0 && user.MinLevel == 0 {
					continue
				}
				if *encounter.IV >= float32(user.MinIV) && *encounter.Level >= user.MinLevel {
					s.sendEncounterNotification(user, encounter)
				}
			}
		}

		// Per-Pokémon subscription notifications.
		if subscriptions, exists := activeSubs[encounter.PokemonID]; exists {
			for _, subscription := range subscriptions {
				user, ok := users.All[subscription.UserID]
				if !ok {
					continue
				}

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
				if !inRange(user, effectiveMaxDistance) {
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
	tr := newTranslator(user.Language)
	name := tr.PokemonName(encounter.PokemonID)
	formSuffix := s.buildFormSuffix(encounter.PokemonID, encounter.Form, user.Language)
	genderEmoji := getGenderEmoji(encounter.Gender)
	cpLabel := tr.T("CP")
	sizeEmoji := getSizeEmoji(encounter.Size)
	weatherEmoji := getWeatherEmoji(encounter.Weather)

	if encounter.IV == nil || encounter.AtkIV == nil || encounter.DefIV == nil ||
		encounter.StaIV == nil || encounter.CP == nil || encounter.Level == nil {
		return fmt.Sprintf("*🔔 %s%s%s*%s%s",
			name, formSuffix, genderEmoji, sizeEmoji, weatherEmoji)
	}

	return fmt.Sprintf("*🔔 %s%s%s %.1f%% %d|%d|%d %d%s L%d*%s%s",
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

func (s *NotificationService) buildFormSuffix(pokemonID int, formID *int, language string) string {
	formKey := s.resolveFormKey(pokemonID, formID)
	if formKey == "" {
		return ""
	}
	// resolveFormKey already validated both the pokemon and form entries exist.
	form := s.gameData.Pokemon[strconv.Itoa(pokemonID)].Forms[formKey]
	costumeEmoji := ""
	if form.IsCostume {
		costumeEmoji = "👕 "
	}
	return fmt.Sprintf(" (%s%s)", costumeEmoji, getTranslation(form.Name, language))
}

// resolveFormKey returns the form-key string for a non-normal form, or "" if
// the form is nil, zero, unknown, or Normal.
func (s *NotificationService) resolveFormKey(pokemonID int, formID *int) string {
	if formID == nil || *formID <= 0 {
		return ""
	}
	pokemon, exists := s.gameData.Pokemon[strconv.Itoa(pokemonID)]
	if !exists {
		return ""
	}
	formKey := strconv.Itoa(*formID)
	form, exists := pokemon.Forms[formKey]
	if !exists || form.Name == "Normal" {
		return ""
	}
	return formKey
}

// formatDistance formats a haversine distance (in metres) as a human-readable
// distance line with a trailing newline, using metres below 1 km and km above.
func formatDistance(distance float64) string {
	if distance < 1000 {
		return fmt.Sprintf("📍 %.0fm\n", distance)
	}
	return fmt.Sprintf("📍 %.2fkm\n", distance/1000)
}

func (s *NotificationService) generateNotificationText(user User, encounter EncounterData) string {
	var sb strings.Builder
	tr := newTranslator(user.Language)

	if user.Latitude != 0 && user.Longitude != 0 {
		distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
		sb.WriteString(formatDistance(distance))
	}
	if encounter.ExpireTimestamp != nil {
		expireTime := time.Unix(int64(*encounter.ExpireTimestamp), 0).In(s.timezone)
		timeLeft := time.Until(expireTime)
		sb.WriteString(fmt.Sprintf("💨 %s ⏳ %s\n",
			expireTime.Format(time.TimeOnly),
			timeLeft.Truncate(time.Second).String()))
	}

	if encounter.Move1 != nil && encounter.Move2 != nil {
		sb.WriteString(fmt.Sprintf("💥 %s / %s",
			tr.MoveName(*encounter.Move1),
			tr.MoveName(*encounter.Move2)))
	}

	if encounter.PVPData != nil {
		for league, entries := range encounter.PVPData {
			leagueName := strings.ToUpper(string(league[0])) + league[1:]
			for _, entry := range entries {
				if entry.Rank < 4 {
					sb.WriteString("\n" +
						tr.Tf("🏅 *%s League Rank", leagueName) +
						tr.Tf(" %d*: %s %dCP L%.1f",
							entry.Rank,
							tr.PokemonName(entry.Pokemon),
							entry.CP,
							entry.Level,
						))
				}
			}
		}
	}

	return sb.String()
}

// ── Application-level singleton ───────────────────────────────────────────────

// notificationService is the singleton used by the production application.
// Tests create their own NotificationService with mocked dependencies instead.
var notificationService *NotificationService

// startNotificationProcessing starts the background notification goroutine.
// It stops cleanly when stop is closed.
func startNotificationProcessing(stop <-chan struct{}) {
	go func() {
		for {
			select {
			case <-stop:
				log.Println("⏹️ Notification processing stopped")
				return
			case <-time.After(30 * time.Second):
				notificationService.cleanupMessages(userCache)
				notificationService.processEncounters(userCache, activeSubscriptions)
				notificationService.processRaids(userCache, activeRaidSubscriptions)
			}
		}
	}()
}

// ── Raid notification logic ───────────────────────────────────────────────────

// raidEncounterID returns a stable dedup key for a raid: gymID_endTimestamp.
func raidEncounterID(gym GymData) string {
	if gym.RaidEndTimestamp == nil {
		return gym.ID + "_0"
	}
	return fmt.Sprintf("%s_%d", gym.ID, *gym.RaidEndTimestamp)
}

// sendRaidNotification delivers a full raid notification to one user.
func (s *NotificationService) sendRaidNotification(user User, gym GymData) {
	encounterID := raidEncounterID(gym)

	if s.isRateLimited(user.ID) {
		return
	}
	if _, exists := s.notificationCache[encounterID][user.ID]; exists {
		return
	}

	raidLevel := 0
	if gym.RaidLevel != nil {
		raidLevel = *gym.RaidLevel
	}
	pokemonID := 0
	if gym.RaidPokemonID != nil {
		pokemonID = *gym.RaidPokemonID
	}
	log.Printf("⚔️ Sending raid notification for Pokémon #%d L%d to %d", pokemonID, raidLevel, user.ID)

	if s.notificationCache[encounterID] == nil {
		s.notificationCache[encounterID] = make(map[int64]struct{})
		if gym.RaidEndTimestamp != nil {
			s.botDB.SaveEncounter(encounterID, int64(*gym.RaidEndTimestamp))
		}
	}
	s.notificationCache[encounterID][user.ID] = struct{}{}

	if !user.OnlyMap && user.Stickers {
		formSuffix := ""
		if key := s.resolveFormKey(pokemonID, gym.RaidPokemonForm); key != "" {
			formSuffix = "_f" + key
		}
		stickerURL := fmt.Sprintf("https://raw.githubusercontent.com/WatWowMap/wwm-uicons-webp/main/pokemon/%d%s.webp", pokemonID, formSuffix)
		if err := s.sendSticker(user.ID, stickerURL, encounterID); err != nil {
			return
		}
	}

	title := s.generateRaidNotificationTitle(user, gym)
	body := s.generateRaidNotificationText(user, gym)

	if !user.OnlyMap {
		if err := s.sendLocation(user.ID, gym.Lat, gym.Lon, encounterID); err != nil {
			return
		}
		if err := s.sendMessage(user.ID, title+"\n"+body, encounterID); err != nil {
			return
		}
	} else {
		if err := s.sendVenue(user.ID, gym.Lat, gym.Lon, title, body, encounterID); err != nil {
			return
		}
	}
	s.incRaidNotifications()
}

// filterAndSendRaids matches each raid against every applicable user category
// and fires notifications.
func (s *NotificationService) filterAndSendRaids(
	users FilteredUsers,
	raids []GymData,
	activeSubs map[int][]RaidSubscription,
) {
	for _, gym := range raids {
		raidLevel := 0
		if gym.RaidLevel != nil {
			raidLevel = *gym.RaidLevel
		}
		pokemonID := 0
		if gym.RaidPokemonID != nil {
			pokemonID = *gym.RaidPokemonID
		}

		distCache := make(map[int64]bool)
		inRange := func(user User) bool {
			if v, ok := distCache[user.ID]; ok {
				return v
			}
			v := withinDistance(user, gym.Lat, gym.Lon, user.MaxDistance)
			distCache[user.ID] = v
			return v
		}

		// Tier 1: AllRaids users — level gate + distance check.
		for _, user := range users.AllRaids {
			if user.RaidMinLevel != 0 && user.RaidMinLevel > raidLevel {
				continue
			}
			if !inRange(user) {
				continue
			}
			s.sendRaidNotification(user, gym)
		}

		// Tier 2: Level-only subscriptions — (userID, 0, raidLevel).
		if levelSubs, ok := activeSubs[0]; ok {
			for _, sub := range levelSubs {
				if sub.RaidLevel != raidLevel {
					continue
				}
				user, ok := users.All[sub.UserID]
				if !ok {
					continue
				}
				if !inRange(user) {
					continue
				}
				s.sendRaidNotification(user, gym)
			}
		}

		// Tier 3: Per-Pokémon subscriptions — (userID, pokemonID, 0|raidLevel).
		if pokemonID > 0 {
			if pokemonSubs, ok := activeSubs[pokemonID]; ok {
				for _, sub := range pokemonSubs {
					if sub.RaidLevel != 0 && sub.RaidLevel != raidLevel {
						continue
					}
					user, ok := users.All[sub.UserID]
					if !ok {
						continue
					}
					if !inRange(user) {
						continue
					}
					s.sendRaidNotification(user, gym)
				}
			}
		}

	}
}

// processRaids fetches active raids from the scanner and dispatches notifications.
func (s *NotificationService) processRaids(users FilteredUsers, activeSubs map[int][]RaidSubscription) {
	raids, err := s.scannerDB.GetActiveRaids()
	if err != nil {
		log.Printf("❌ Failed to fetch active raids: %v", err)
		return
	}
	s.setRaidEncounterGauge(float64(len(raids)))
	log.Printf("✅ Found %d active raids", len(raids))
	s.filterAndSendRaids(users, raids, activeSubs)
}

// ── Raid notification text generation ────────────────────────────────────────

func (s *NotificationService) generateRaidNotificationTitle(user User, gym GymData) string {
	tr := newTranslator(user.Language)
	raidLevel := 0
	if gym.RaidLevel != nil {
		raidLevel = *gym.RaidLevel
	}
	pokemonID := 0
	if gym.RaidPokemonID != nil {
		pokemonID = *gym.RaidPokemonID
	}
	name := tr.PokemonName(pokemonID)
	formSuffix := s.buildFormSuffix(pokemonID, gym.RaidPokemonForm, user.Language)
	genderEmoji := getGenderEmoji(gym.RaidPokemonGender)
	alignmentSuffix := ""
	if gym.RaidPokemonAlignment != nil && *gym.RaidPokemonAlignment != 0 {
		alignmentSuffix = " " + tr.AlignmentName(*gym.RaidPokemonAlignment)
	}
	cpSuffix := ""
	if gym.RaidPokemonCp != nil {
		cpSuffix = fmt.Sprintf(" %d%s", *gym.RaidPokemonCp, tr.T("CP"))
	}
	gymName := gym.ID
	if gym.Name != nil {
		gymName = *gym.Name
	}
	return fmt.Sprintf("*⚔️ %s%s%s%s%s (%s) @ %s*",
		name, formSuffix, genderEmoji, alignmentSuffix, cpSuffix,
		tr.RaidLevelName(raidLevel), gymName)
}

func (s *NotificationService) generateRaidNotificationText(user User, gym GymData) string {
	var sb strings.Builder
	tr := newTranslator(user.Language)

	if user.Latitude != 0 && user.Longitude != 0 {
		distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(gym.Lat), float64(gym.Lon))
		sb.WriteString(formatDistance(distance))
	}

	if gym.RaidEndTimestamp != nil {
		endTime := time.Unix(int64(*gym.RaidEndTimestamp), 0).In(s.timezone)
		timeLeft := time.Until(endTime)
		sb.WriteString(fmt.Sprintf("💨 %s ⏳ %s\n",
			endTime.Format(time.TimeOnly),
			timeLeft.Truncate(time.Second).String()))
	}

	if gym.RaidPokemonMove1 != nil && gym.RaidPokemonMove2 != nil {
		sb.WriteString(fmt.Sprintf("💥 %s / %s\n",
			tr.MoveName(*gym.RaidPokemonMove1),
			tr.MoveName(*gym.RaidPokemonMove2)))
	}

	// Team line: team + EX flag (gym name is already in the title).
	teamName := tr.TeamName(0)
	if gym.TeamID != nil {
		teamName = tr.TeamName(*gym.TeamID)
	}
	exFlag := ""
	if gym.RaidIsExclusive != nil && *gym.RaidIsExclusive != 0 {
		exFlag = " ✨EX"
	}
	sb.WriteString(fmt.Sprintf("🏟️ %s%s", teamName, exFlag))

	return sb.String()
}
