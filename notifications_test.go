package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/telebot.v3"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newTestService builds a NotificationService with mocked deps and no
// Prometheus counters (all nil-safe guards in the service handle this).
func newTestService(db BotDB, sender *mockBotSender) *NotificationService {
	return &NotificationService{
		botDB:                db,
		scannerDB:            &mockScannerDB{},
		sender:               sender,
		gameData:             &testGameDataPtr,
		translations:         testTranslations(),
		timezone:             time.UTC,
		notificationCache:    make(map[string]map[int64]struct{}),
		userRateLimitedUntil: make(map[int64]time.Time),
	}
}

// testGameDataPtr is a package-level value so &testGameDataPtr is valid.
var testGameDataPtr = testGameData()

// pointerTo returns a pointer to a value — avoids verbose local-var dance in tests.
func pointerToInt(v int) *int             { return &v }
func pointerToFloat32(v float32) *float32 { return &v }
func pointerToStr(v string) *string       { return &v }

// minimalEncounter builds an EncounterData with all mandatory pointer fields set.
func minimalEncounter(id string, pokemonID int) EncounterData {
	iv := float32(80.0)
	atkIV, defIV, staIV := 12, 13, 14
	cp := 1200
	level := 25
	expire := int(time.Now().Add(30 * time.Minute).Unix())
	return EncounterData{
		ID:              id,
		PokemonID:       pokemonID,
		Lat:             48.137,
		Lon:             11.575,
		IV:              &iv,
		AtkIV:           &atkIV,
		DefIV:           &defIV,
		StaIV:           &staIV,
		CP:              &cp,
		Level:           &level,
		ExpireTimestamp: &expire,
	}
}

// notifyUser returns a User with notifications enabled.
func notifyUser(id int64) User {
	return User{ID: id, Notify: true, Language: "en"}
}

// ── retryAfterSeconds ─────────────────────────────────────────────────────────

func TestRetryAfterSeconds_NilError(t *testing.T) {
	assert.Equal(t, 0, retryAfterSeconds(nil))
}

func TestRetryAfterSeconds_NonRateLimitError(t *testing.T) {
	assert.Equal(t, 0, retryAfterSeconds(errors.New("some other error")))
}

func TestRetryAfterSeconds_FloodError(t *testing.T) {
	err := telebot.FloodError{RetryAfter: 15}
	assert.Equal(t, 15, retryAfterSeconds(err))
}

func TestRetryAfterSeconds_ZeroRetryAfter(t *testing.T) {
	err := telebot.FloodError{RetryAfter: 0}
	assert.Equal(t, 0, retryAfterSeconds(err))
}

// ── isPermanentTelegramError ──────────────────────────────────────────────────

func TestIsPermanentTelegramError_Nil(t *testing.T) {
	assert.False(t, isPermanentTelegramError(nil))
}

func TestIsPermanentTelegramError_ChatNotFound(t *testing.T) {
	assert.True(t, isPermanentTelegramError(errors.New("chat not found")))
}

func TestIsPermanentTelegramError_Blocked(t *testing.T) {
	assert.True(t, isPermanentTelegramError(errors.New("bot was blocked by the user")))
}

func TestIsPermanentTelegramError_Deactivated(t *testing.T) {
	assert.True(t, isPermanentTelegramError(errors.New("user is deactivated")))
}

func TestIsPermanentTelegramError_Kicked(t *testing.T) {
	assert.True(t, isPermanentTelegramError(errors.New("bot was kicked")))
}

func TestIsPermanentTelegramError_RateLimit(t *testing.T) {
	assert.False(t, isPermanentTelegramError(errors.New("retry after 10")))
}

func TestIsPermanentTelegramError_GenericError(t *testing.T) {
	assert.False(t, isPermanentTelegramError(errors.New("some transient network error")))
}

// ── buildFormSuffix ───────────────────────────────────────────────────────────

func TestBuildFormSuffix_NilForm(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	enc := minimalEncounter("e1", 25) // Form is nil
	assert.Equal(t, "", svc.buildFormSuffix(enc, "en"))
}

func TestBuildFormSuffix_FormZero(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	enc := minimalEncounter("e1", 25)
	enc.Form = pointerToInt(0)
	assert.Equal(t, "", svc.buildFormSuffix(enc, "en"))
}

func TestBuildFormSuffix_NormalForm(t *testing.T) {
	// Form 1 on Pikachu is "Normal" → should return "".
	svc := newTestService(nil, &mockBotSender{})
	enc := minimalEncounter("e1", 25)
	enc.Form = pointerToInt(1)
	assert.Equal(t, "", svc.buildFormSuffix(enc, "en"))
}

func TestBuildFormSuffix_NamedForm(t *testing.T) {
	// Form 2 on Pikachu is "Halloween" with IsCostume=true.
	svc := newTestService(nil, &mockBotSender{})
	enc := minimalEncounter("e1", 25)
	enc.Form = pointerToInt(2)
	suffix := svc.buildFormSuffix(enc, "en")
	assert.Contains(t, suffix, "Halloween")
	assert.Contains(t, suffix, "👕")
}

func TestBuildFormSuffix_UnknownPokemon(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	enc := minimalEncounter("e1", 9999)
	enc.Form = pointerToInt(1)
	assert.Equal(t, "", svc.buildFormSuffix(enc, "en"))
}

// ── generateNotificationTitle ─────────────────────────────────────────────────

func TestGenerateNotificationTitle_ContainsPokemonName(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("e1", 25)
	title := svc.generateNotificationTitle(user, enc)
	assert.Contains(t, title, "Pikachu")
}

func TestGenerateNotificationTitle_ContainsIVAndLevel(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("e1", 25)
	title := svc.generateNotificationTitle(user, enc)
	assert.Contains(t, title, "80.0%")
	assert.Contains(t, title, "L25")
}

func TestGenerateNotificationTitle_EnglishUsesCPLabel(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	user.Language = "en"
	enc := minimalEncounter("e1", 25)
	title := svc.generateNotificationTitle(user, enc)
	assert.Contains(t, title, "CP")
	assert.NotContains(t, title, "WP")
}

func TestGenerateNotificationTitle_GermanUsesWPLabel(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	user.Language = "de"
	enc := minimalEncounter("e1", 25)
	title := svc.generateNotificationTitle(user, enc)
	assert.Contains(t, title, "WP")
}

func TestGenerateNotificationTitle_IncludesSizeEmoji(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("e1", 25)
	enc.Size = pointerToInt(1) // small
	title := svc.generateNotificationTitle(user, enc)
	assert.Contains(t, title, "🔹")
}

// ── generateNotificationText ──────────────────────────────────────────────────

func TestGenerateNotificationText_ContainsExpireTime(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("e1", 25)
	text := svc.generateNotificationText(user, enc)
	// The body always contains the expire time and a time-left string.
	assert.Contains(t, text, "💨")
	assert.Contains(t, text, "⏳")
}

func TestGenerateNotificationText_ContainsMoves(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("e1", 25)
	enc.Move1 = pointerToInt(200) // Thunderbolt
	enc.Move2 = pointerToInt(13)  // Wrap
	text := svc.generateNotificationText(user, enc)
	assert.Contains(t, text, "Thunderbolt")
	assert.Contains(t, text, "Wrap")
}

func TestGenerateNotificationText_ContainsDistance(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	// User ~111 m from encounter.
	user := notifyUser(1)
	user.Latitude = 48.0
	user.Longitude = 11.0
	enc := minimalEncounter("e1", 25)
	enc.Lat = 48.001
	enc.Lon = 11.0
	text := svc.generateNotificationText(user, enc)
	assert.Contains(t, text, "📍")
	assert.Contains(t, text, "m") // distance in metres
}

func TestGenerateNotificationText_NoDistanceWhenNoUserLocation(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1) // Latitude/Longitude both 0
	enc := minimalEncounter("e1", 25)
	text := svc.generateNotificationText(user, enc)
	assert.NotContains(t, text, "📍")
}

// ── filterAndSendEncounters ───────────────────────────────────────────────────

// sendCallCount counts how many Send calls a mockBotSender received.
func sendCallCount(sender *mockBotSender) int {
	count := 0
	for _, call := range sender.Calls {
		if call.Method == "Send" {
			count++
		}
	}
	return count
}

// setupSenderForAnyEncounter configures sender to return a fake message for any Send call.
func setupSenderForAnyEncounter(sender *mockBotSender) {
	fakeMsg := &telebot.Message{ID: 42}
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(fakeMsg, nil)
}

// setupBotDbForEncounter configures the bot db mocks needed when a
// notification is delivered (SaveEncounter + SaveMessage).
func setupBotDbForEncounter(botDB *mockBotDB) {
	botDB.On("SaveEncounter", mock.Anything, mock.Anything).Return()
	botDB.On("SaveMessage", mock.Anything, mock.Anything, mock.Anything).Return()
}

func TestFilterAndSendEncounters_HundoIV_NotifiesHundoUsers(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(db, sender)

	iv := float32(100.0)
	enc := minimalEncounter("hundo1", 25)
	enc.IV = &iv

	user := notifyUser(1)
	users := FilteredUsers{
		All:     map[int64]User{1: user},
		HundoIV: []User{user},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})

	// At minimum a location + message are sent (no stickers — user.Stickers=false by default).
	assert.GreaterOrEqual(t, sendCallCount(sender), 2)
}

func TestFilterAndSendEncounters_ZeroIV_NotifiesZeroUsers(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(db, sender)

	iv := float32(0.0)
	enc := minimalEncounter("zero1", 25)
	enc.IV = &iv

	user := notifyUser(2)
	users := FilteredUsers{
		All:    map[int64]User{2: user},
		ZeroIV: []User{user},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})

	assert.GreaterOrEqual(t, sendCallCount(sender), 2)
}

func TestFilterAndSendEncounters_Subscription_AboveMinIV_Notifies(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(db, sender)

	enc := minimalEncounter("sub1", 25) // IV = 80%

	user := notifyUser(3)
	users := FilteredUsers{
		All: map[int64]User{3: user},
	}
	subs := map[int][]Subscription{
		25: {{UserID: 3, PokemonID: 25, MinIV: 70}}, // threshold 70 < 80 → notify
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, subs)

	assert.GreaterOrEqual(t, sendCallCount(sender), 2)
}

func TestFilterAndSendEncounters_Subscription_BelowMinIV_NoNotification(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	sender := &mockBotSender{}

	svc := newTestService(nil, sender)

	enc := minimalEncounter("sub2", 25) // IV = 80%

	user := notifyUser(4)
	users := FilteredUsers{
		All: map[int64]User{4: user},
	}
	subs := map[int][]Subscription{
		25: {{UserID: 4, PokemonID: 25, MinIV: 90}}, // threshold 90 > 80 → skip
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, subs)

	sender.AssertNotCalled(t, "Send")
}

func TestFilterAndSendEncounters_DeduplicatesNotifications(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(db, sender)

	iv := float32(100.0)
	enc := minimalEncounter("hundo_dup", 25)
	enc.IV = &iv

	user := notifyUser(5)
	users := FilteredUsers{
		All:     map[int64]User{5: user},
		HundoIV: []User{user},
	}

	// Send the same encounter twice.
	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})
	firstCallCount := sendCallCount(sender)

	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})
	secondCallCount := sendCallCount(sender)

	// Second pass must not add any new Send calls.
	assert.Equal(t, firstCallCount, secondCallCount, "duplicate encounter should not send again")
}

func TestFilterAndSendEncounters_OutOfDistance_NoNotification(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	sender := &mockBotSender{}

	svc := newTestService(nil, sender)

	enc := minimalEncounter("dist1", 25) // IV = 80%

	// User is at Berlin, encounter at Munich (~504 km).
	user := notifyUser(6)
	user.Latitude = 52.52
	user.Longitude = 13.405
	user.MaxDistance = 1000 // 1 km max
	users := FilteredUsers{
		All: map[int64]User{6: user},
	}
	subs := map[int][]Subscription{
		25: {{UserID: 6, PokemonID: 25, MinIV: 0}},
	}

	enc.Lat = 48.14 // Munich
	enc.Lon = 11.58

	svc.filterAndSendEncounters(users, []EncounterData{enc}, subs)

	sender.AssertNotCalled(t, "Send")
}

func TestFilterAndSendEncounters_Channel_AboveThreshold_Notifies(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(db, sender)

	enc := minimalEncounter("ch1", 1) // IV=80, Level=25

	user := notifyUser(7)
	user.MinIV = 75
	user.MinLevel = 20
	users := FilteredUsers{
		All:      map[int64]User{7: user},
		Channels: []User{user},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})

	assert.GreaterOrEqual(t, sendCallCount(sender), 2)
}

// ── cleanupMessages ───────────────────────────────────────────────────────────

func TestCleanupMessages_DeletesExpiredMessages(t *testing.T) {
	db := &mockBotDB{}
	sender := &mockBotSender{}

	enc := Encounter{ID: "expired1", Expiration: int(time.Now().Add(-1 * time.Minute).Unix())}
	msg := Message{ChatID: 10, MessageID: 99, EncounterID: enc.ID}

	db.On("GetExpiredEncountersWithMessages").Return(
		[]Encounter{enc},
		map[string][]Message{enc.ID: {msg}},
	)
	db.On("DeleteMessage", msg).Return()
	db.On("DeleteEncounter", enc).Return()

	// User has Cleanup=true → sender.Delete should be called.
	sender.On("Delete", mock.Anything).Return(nil)

	svc := newTestService(db, sender)

	user := User{ID: 10, Cleanup: true}
	users := FilteredUsers{
		All: map[int64]User{10: user},
	}

	svc.cleanupMessages(users)

	db.AssertCalled(t, "DeleteMessage", msg)
	db.AssertCalled(t, "DeleteEncounter", enc)
	sender.AssertCalled(t, "Delete", mock.Anything)
}

func TestCleanupMessages_SkipsDeleteWhenCleanupDisabled(t *testing.T) {
	db := &mockBotDB{}
	sender := &mockBotSender{}

	enc := Encounter{ID: "expired2"}
	msg := Message{ChatID: 20, MessageID: 88, EncounterID: enc.ID}

	db.On("GetExpiredEncountersWithMessages").Return(
		[]Encounter{enc},
		map[string][]Message{enc.ID: {msg}},
	)
	db.On("DeleteMessage", msg).Return()
	db.On("DeleteEncounter", enc).Return()

	svc := newTestService(db, sender)

	// Cleanup=false → sender.Delete must NOT be called.
	user := User{ID: 20, Cleanup: false}
	users := FilteredUsers{
		All: map[int64]User{20: user},
	}

	svc.cleanupMessages(users)

	sender.AssertNotCalled(t, "Delete")
	db.AssertCalled(t, "DeleteMessage", msg)
	db.AssertCalled(t, "DeleteEncounter", enc)
}

func TestCleanupMessages_ClearsNotificationCache(t *testing.T) {
	db := &mockBotDB{}
	sender := &mockBotSender{}

	enc := Encounter{ID: "cached1"}
	db.On("GetExpiredEncountersWithMessages").Return(
		[]Encounter{enc},
		map[string][]Message{},
	)
	db.On("DeleteEncounter", enc).Return()

	svc := newTestService(db, sender)
	// Pre-populate cache.
	svc.notificationCache["cached1"] = map[int64]struct{}{99: {}}

	users := FilteredUsers{All: map[int64]User{}}
	svc.cleanupMessages(users)

	require.Contains(t, svc.notificationCache, "cached1")
	assert.Nil(t, svc.notificationCache["cached1"])
}

func TestCleanupMessages_NoEncounters_NoOp(t *testing.T) {
	db := &mockBotDB{}
	sender := &mockBotSender{}

	db.On("GetExpiredEncountersWithMessages").Return(
		[]Encounter{},
		map[string][]Message{},
	)

	svc := newTestService(db, sender)
	users := FilteredUsers{All: map[int64]User{}}

	// Should not panic or call any other method.
	assert.NotPanics(t, func() {
		svc.cleanupMessages(users)
	})
	sender.AssertNotCalled(t, "Delete")
}

// ── permanent error → disables notifications ──────────────────────────────────

func TestBotSend_PermanentError_DisablesNotify(t *testing.T) {
	db := &mockBotDB{}
	sender := &mockBotSender{}

	permErr := errors.New("bot was blocked by the user")
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(nil, permErr)
	db.On("UpdateUserPreference", int64(1), "Notify", false).Return()
	db.On("GetUsers").Return([]User{})
	db.On("GetSubscriptions").Return([]Subscription{})

	svc := newTestService(db, sender)

	_, err := svc.botSend(1, &telebot.User{ID: 1}, "hello")
	assert.Error(t, err)
	db.AssertCalled(t, "UpdateUserPreference", int64(1), "Notify", false)
}

// ── rate-limit gate ───────────────────────────────────────────────────────────

func TestBotSend_UserRateLimited_SkipsSend(t *testing.T) {
	sender := &mockBotSender{}

	svc := newTestService(nil, sender)
	// Mark user as rate-limited until far in the future.
	svc.userRateLimitedUntil[1] = time.Now().Add(1 * time.Hour)

	_, err := svc.botSend(1, &telebot.User{ID: 1}, "hello")
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "rate limited"))
	sender.AssertNotCalled(t, "Send")
}

// ── nil-pointer safety ────────────────────────────────────────────────────────

// encounterWithNilIVLevel returns an encounter where IV and Level are nil.
func encounterWithNilIVLevel(id string, pokemonID int) EncounterData {
	atkIV, defIV, staIV := 12, 13, 14
	cp := 1200
	expire := int(time.Now().Add(30 * time.Minute).Unix())
	return EncounterData{
		ID:              id,
		PokemonID:       pokemonID,
		Lat:             48.137,
		Lon:             11.575,
		IV:              nil, // explicitly nil
		AtkIV:           &atkIV,
		DefIV:           &defIV,
		StaIV:           &staIV,
		CP:              &cp,
		Level:           nil, // explicitly nil
		ExpireTimestamp: &expire,
	}
}

func TestFilterAndSendEncounters_Channel_NilIV_NoPanic(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	sender := &mockBotSender{}
	svc := newTestService(nil, sender)

	enc := encounterWithNilIVLevel("nil_iv_ch", 25)

	user := notifyUser(10)
	user.MinIV = 75
	user.MinLevel = 20
	users := FilteredUsers{
		All:      map[int64]User{10: user},
		Channels: []User{user},
	}

	// Must not panic — nil IV/Level should be skipped for the channel path.
	assert.NotPanics(t, func() {
		svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})
	})
	sender.AssertNotCalled(t, "Send")
}

func TestFilterAndSendEncounters_Subscription_NilIV_SkipsThresholdCheck(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)
	svc := newTestService(db, sender)

	enc := encounterWithNilIVLevel("nil_iv_sub", 25)

	user := notifyUser(11)
	users := FilteredUsers{
		All: map[int64]User{11: user},
	}
	// MinIV=0 and MinLevel=0 — no threshold checks, so nil IV/Level is fine.
	subs := map[int][]Subscription{
		25: {{UserID: 11, PokemonID: 25, MinIV: 0, MinLevel: 0}},
	}

	assert.NotPanics(t, func() {
		svc.filterAndSendEncounters(users, []EncounterData{enc}, subs)
	})
	assert.GreaterOrEqual(t, sendCallCount(sender), 1)
}

func TestFilterAndSendEncounters_Subscription_NilIV_WithMinIV_Skips(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	sender := &mockBotSender{}
	svc := newTestService(nil, sender)

	enc := encounterWithNilIVLevel("nil_iv_sub2", 25)

	user := notifyUser(12)
	users := FilteredUsers{
		All: map[int64]User{12: user},
	}
	// effectiveMinIV=80 but encounter.IV is nil → should be skipped, not panic.
	subs := map[int][]Subscription{
		25: {{UserID: 12, PokemonID: 25, MinIV: 80}},
	}

	assert.NotPanics(t, func() {
		svc.filterAndSendEncounters(users, []EncounterData{enc}, subs)
	})
	sender.AssertNotCalled(t, "Send")
}

func TestGenerateNotificationTitle_NilFields_ReturnsSafeFallback(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := encounterWithNilIVLevel("nil_title", 25)

	assert.NotPanics(t, func() {
		title := svc.generateNotificationTitle(user, enc)
		// Should still contain the Pokémon name even with nil IV/Level.
		assert.Contains(t, title, "Pikachu")
		// Should NOT contain a % sign (no IV percentage in fallback).
		assert.NotContains(t, title, "%")
	})
}

func TestGenerateNotificationTitle_NilCP_ReturnsSafeFallback(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("nil_cp", 25)
	enc.CP = nil // only CP is nil

	assert.NotPanics(t, func() {
		title := svc.generateNotificationTitle(user, enc)
		assert.Contains(t, title, "Pikachu")
		assert.NotContains(t, title, "CP")
	})
}

func TestGenerateNotificationText_NilExpireTimestamp_NoPanic(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("nil_expire", 25)
	enc.ExpireTimestamp = nil

	assert.NotPanics(t, func() {
		text := svc.generateNotificationText(user, enc)
		// Without expire timestamp, timing lines are absent.
		assert.NotContains(t, text, "💨")
		assert.NotContains(t, text, "⏳")
	})
}

func TestSendEncounterNotification_NilExpireTimestamp_NoPanic(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	// No SaveEncounter expectation — it should not be called when ExpireTimestamp is nil.
	db.On("SaveMessage", mock.Anything, mock.Anything, mock.Anything).Return()
	setupSenderForAnyEncounter(sender)
	svc := newTestService(db, sender)

	enc := minimalEncounter("nil_exp_send", 25)
	enc.ExpireTimestamp = nil
	user := notifyUser(99)

	assert.NotPanics(t, func() {
		svc.sendEncounterNotification(user, enc)
	})
	db.AssertNotCalled(t, "SaveEncounter")
}

// ── newNotificationService ────────────────────────────────────────────────────

func TestNewNotificationService_FieldsInitialised(t *testing.T) {
	db := &mockBotDB{}
	scanDB := &mockScannerDB{}
	sender := &mockBotSender{}
	mf := testGameData()
	tr := testTranslations()
	tz := time.UTC

	svc := newNotificationService(db, scanDB, sender, &mf, tr, tz, nil, nil, nil, nil)

	require.NotNil(t, svc)
	assert.NotNil(t, svc.notificationCache)
	assert.NotNil(t, svc.userRateLimitedUntil)
	assert.Equal(t, tz, svc.timezone)
}

func TestNewNotificationService_WithPrometheusCounters(t *testing.T) {
	db := &mockBotDB{}
	scanDB := &mockScannerDB{}
	sender := &mockBotSender{}
	mf := testGameData()
	tr := testTranslations()

	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_counter"})
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_gauge"})

	svc := newNotificationService(db, scanDB, sender, &mf, tr, time.UTC, counter, counter, gauge, gauge)

	require.NotNil(t, svc)
	assert.NotNil(t, svc.notificationsCounter)
	assert.NotNil(t, svc.messagesCounter)
	assert.NotNil(t, svc.cleanupCounter)
	assert.NotNil(t, svc.encounterGauge)
}

// ── prometheus counter nil-safety ────────────────────────────────────────────

func TestIncNotifications_NilCounter_NoPanic(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	assert.NotPanics(t, func() { svc.incNotifications() })
}

func TestIncNotifications_WithCounter(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "notif_counter_test"})
	svc.notificationsCounter = counter
	assert.NotPanics(t, func() { svc.incNotifications() })
}

func TestIncMessages_NilCounter_NoPanic(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	assert.NotPanics(t, func() { svc.incMessages() })
}

func TestIncMessages_WithCounter(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "messages_counter_test"})
	svc.messagesCounter = counter
	assert.NotPanics(t, func() { svc.incMessages() })
}

func TestAddCleanup_NilGauge_NoPanic(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	assert.NotPanics(t, func() { svc.addCleanup(5.0) })
}

func TestAddCleanup_WithGauge(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "cleanup_gauge_test"})
	svc.cleanupCounter = gauge
	assert.NotPanics(t, func() { svc.addCleanup(3.0) })
}

func TestSetEncounterGauge_NilGauge_NoPanic(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	assert.NotPanics(t, func() { svc.setEncounterGauge(10.0) })
}

func TestSetEncounterGauge_WithGauge(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "encounter_gauge_test"})
	svc.encounterGauge = gauge
	assert.NotPanics(t, func() { svc.setEncounterGauge(7.0) })
}

// ── isRateLimited ─────────────────────────────────────────────────────────────

func TestIsRateLimited_NotPresent_ReturnsFalse(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	assert.False(t, svc.isRateLimited(1))
}

func TestIsRateLimited_ActiveLimit_ReturnsTrue(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	svc.userRateLimitedUntil[1] = time.Now().Add(1 * time.Hour)
	assert.True(t, svc.isRateLimited(1))
}

func TestIsRateLimited_ExpiredEntry_EvictsAndReturnsFalse(t *testing.T) {
	svc := newTestService(nil, &mockBotSender{})
	// Set expiry in the past so it's already expired.
	svc.userRateLimitedUntil[1] = time.Now().Add(-1 * time.Second)

	result := svc.isRateLimited(1)

	assert.False(t, result)
	_, stillPresent := svc.userRateLimitedUntil[1]
	assert.False(t, stillPresent, "expired entry should be evicted from the map")
}

// ── botSend flood error → rate-limit stored ───────────────────────────────────

// floodError wraps a telebot.FloodError so errors.As can unwrap it while also
// satisfying the error interface without dereferencing a nil *telebot.Error.
type floodError struct {
	retryAfter int
}

func (e floodError) Error() string { return "Too Many Requests: retry after" }
func (e floodError) As(target interface{}) bool {
	if fe, ok := target.(*telebot.FloodError); ok {
		fe.RetryAfter = e.retryAfter
		return true
	}
	return false
}

func TestBotSend_FloodError_StoresRateLimit(t *testing.T) {
	sender := &mockBotSender{}
	// Use our wrapper so FloodError.Error() doesn't panic on nil *Error.
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(nil, floodError{retryAfter: 30})

	svc := newTestService(nil, sender)

	_, err := svc.botSend(1, &telebot.User{ID: 1}, "hello")

	assert.Error(t, err)
	until, ok := svc.userRateLimitedUntil[1]
	assert.True(t, ok, "rate limit entry should be stored after flood error")
	assert.True(t, until.After(time.Now()), "rate limit deadline should be in the future")
}

func TestBotSend_TransientError_DoesNotStoreRateLimit(t *testing.T) {
	sender := &mockBotSender{}
	transientErr := errors.New("network timeout")
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(nil, transientErr)

	svc := newTestService(nil, sender)

	_, err := svc.botSend(1, &telebot.User{ID: 1}, "hello")

	assert.Error(t, err)
	_, ok := svc.userRateLimitedUntil[1]
	assert.False(t, ok, "transient error should not store a rate limit")
}

func TestBotSend_Success_ReturnsMessage(t *testing.T) {
	sender := &mockBotSender{}
	fakeMsg := &telebot.Message{ID: 7}
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(fakeMsg, nil)

	svc := newTestService(nil, sender)

	msg, err := svc.botSend(1, &telebot.User{ID: 1}, "hello")

	require.NoError(t, err)
	assert.Equal(t, 7, msg.ID)
}

// ── sendSticker ───────────────────────────────────────────────────────────────

func TestSendSticker_Success_SavesMessage(t *testing.T) {
	db := &mockBotDB{}
	sender := &mockBotSender{}
	fakeMsg := &telebot.Message{ID: 55}
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(fakeMsg, nil)
	db.On("SaveMessage", int64(1), 55, "enc-sticker").Return()

	svc := newTestService(db, sender)

	err := svc.sendSticker(1, "https://example.com/sticker.webp", "enc-sticker")

	require.NoError(t, err)
	db.AssertCalled(t, "SaveMessage", int64(1), 55, "enc-sticker")
}

func TestSendSticker_Failure_ReturnsError(t *testing.T) {
	sender := &mockBotSender{}
	sendErr := errors.New("sticker send failed")
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(nil, sendErr)

	svc := newTestService(nil, sender)

	err := svc.sendSticker(1, "https://example.com/sticker.webp", "enc-sticker-fail")

	assert.Error(t, err)
}

// ── sendLocation failure ──────────────────────────────────────────────────────

func TestSendLocation_Failure_ReturnsError(t *testing.T) {
	sender := &mockBotSender{}
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("location error"))

	svc := newTestService(nil, sender)

	err := svc.sendLocation(1, 48.0, 11.0, "enc-loc-fail")

	assert.Error(t, err)
}

// ── sendVenue ─────────────────────────────────────────────────────────────────

func TestSendVenue_Success_SavesMessage(t *testing.T) {
	db := &mockBotDB{}
	sender := &mockBotSender{}
	fakeMsg := &telebot.Message{ID: 77}
	sender.On("Send", mock.Anything, mock.Anything).Return(fakeMsg, nil)
	db.On("SaveMessage", int64(1), 77, "enc-venue").Return()

	svc := newTestService(db, sender)

	err := svc.sendVenue(1, 48.0, 11.0, "Title", "Address", "enc-venue")

	require.NoError(t, err)
	db.AssertCalled(t, "SaveMessage", int64(1), 77, "enc-venue")
}

func TestSendVenue_Failure_ReturnsError(t *testing.T) {
	sender := &mockBotSender{}
	sender.On("Send", mock.Anything, mock.Anything).Return(nil, errors.New("venue error"))

	svc := newTestService(nil, sender)

	err := svc.sendVenue(1, 48.0, 11.0, "Title", "Address", "enc-venue-fail")

	assert.Error(t, err)
}

// ── sendMessage failure ───────────────────────────────────────────────────────

func TestSendMessage_Failure_ReturnsError(t *testing.T) {
	sender := &mockBotSender{}
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("message error"))

	svc := newTestService(nil, sender)

	err := svc.sendMessage(1, "hello", "enc-msg-fail")

	assert.Error(t, err)
}

// ── sendEncounterNotification with stickers ───────────────────────────────────

func TestSendEncounterNotification_Stickers_SendsStickerFirst(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(db, sender)

	enc := minimalEncounter("sticker-enc", 25)
	user := notifyUser(1)
	user.Stickers = true // enable stickers

	svc.sendEncounterNotification(user, enc)

	// With stickers enabled: sticker + location + message = 3 sends minimum.
	assert.GreaterOrEqual(t, sendCallCount(sender), 3)
}

func TestSendEncounterNotification_StickerFails_AbortsEarly(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	// SaveEncounter is called before any sends.
	db.On("SaveEncounter", mock.Anything, mock.Anything).Return()
	// First Send (sticker) fails; no further sends should happen.
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("sticker fail")).Once()

	svc := newTestService(db, sender)

	enc := minimalEncounter("sticker-fail", 25)
	user := notifyUser(1)
	user.Stickers = true

	svc.sendEncounterNotification(user, enc)

	assert.Equal(t, 1, sendCallCount(sender), "should stop after sticker failure")
}

func TestSendEncounterNotification_LocationFails_AbortsEarly(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	db.On("SaveEncounter", mock.Anything, mock.Anything).Return()
	// Stickers disabled; first send is location and it fails.
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("location fail")).Once()

	svc := newTestService(db, sender)

	enc := minimalEncounter("loc-fail", 25)
	user := notifyUser(1)
	user.Stickers = false

	svc.sendEncounterNotification(user, enc)

	assert.Equal(t, 1, sendCallCount(sender), "should stop after location failure")
}

func TestSendEncounterNotification_MessageFails_AbortsEarly(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	fakeMsg := &telebot.Message{ID: 1}
	db.On("SaveMessage", mock.Anything, mock.Anything, mock.Anything).Return()
	db.On("SaveEncounter", mock.Anything, mock.Anything).Return()
	// Location succeeds, message fails.
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).
		Return(fakeMsg, nil).Once()
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("message fail")).Once()

	svc := newTestService(db, sender)

	enc := minimalEncounter("msg-fail", 25)
	user := notifyUser(1)
	user.Stickers = false

	svc.sendEncounterNotification(user, enc)

	assert.Equal(t, 2, sendCallCount(sender), "should stop after message failure")
}

// ── sendEncounterNotification OnlyMap path ────────────────────────────────────

func TestSendEncounterNotification_OnlyMap_SendsVenue(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	fakeMsg := &telebot.Message{ID: 88}
	// OnlyMap skips sticker and location; only sendVenue is called (no variadic opts).
	sender.On("Send", mock.Anything, mock.Anything).Return(fakeMsg, nil)
	db.On("SaveMessage", mock.Anything, mock.Anything, mock.Anything).Return()
	db.On("SaveEncounter", mock.Anything, mock.Anything).Return()

	svc := newTestService(db, sender)

	enc := minimalEncounter("onlymap-enc", 25)
	user := notifyUser(1)
	user.OnlyMap = true

	svc.sendEncounterNotification(user, enc)

	assert.GreaterOrEqual(t, sendCallCount(sender), 1)
}

func TestSendEncounterNotification_OnlyMap_VenueFails_AbortsEarly(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	db.On("SaveEncounter", mock.Anything, mock.Anything).Return()
	sender.On("Send", mock.Anything, mock.Anything).Return(nil, errors.New("venue fail"))

	svc := newTestService(db, sender)

	enc := minimalEncounter("onlymap-venuefail", 25)
	user := notifyUser(1)
	user.OnlyMap = true

	svc.sendEncounterNotification(user, enc)

	assert.Equal(t, 1, sendCallCount(sender))
}

// ── filterAndSendEncounters: TopPVP ──────────────────────────────────────────

func TestFilterAndSendEncounters_TopPVP_Top3_Notifies(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(db, sender)

	enc := minimalEncounter("pvp-top3", 25)
	enc.PVPData = PVP{
		"great": {{Pokemon: 25, Rank: 1, CP: 1499, Level: 50, Percentage: 99.0}},
	}

	user := notifyUser(10)
	users := FilteredUsers{
		All:    map[int64]User{10: user},
		TopPVP: []User{user},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})

	assert.GreaterOrEqual(t, sendCallCount(sender), 2)
}

func TestFilterAndSendEncounters_TopPVP_Rank4_NoNotification(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	sender := &mockBotSender{}
	svc := newTestService(nil, sender)

	enc := minimalEncounter("pvp-rank4", 25)
	enc.PVPData = PVP{
		"great": {{Pokemon: 25, Rank: 4, CP: 1499, Level: 50, Percentage: 97.0}},
	}

	user := notifyUser(10)
	users := FilteredUsers{
		All:    map[int64]User{10: user},
		TopPVP: []User{user},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})

	sender.AssertNotCalled(t, "Send")
}

func TestFilterAndSendEncounters_TopPVP_OutOfRange_NoNotification(t *testing.T) {
	sender := &mockBotSender{}
	svc := newTestService(nil, sender)

	enc := minimalEncounter("pvp-dist", 25)
	enc.Lat = 48.14 // Munich
	enc.Lon = 11.58
	enc.PVPData = PVP{
		"great": {{Pokemon: 25, Rank: 1, CP: 1499, Level: 50, Percentage: 99.0}},
	}

	user := notifyUser(10)
	user.Latitude = 52.52  // Berlin
	user.Longitude = 13.40 // Berlin
	user.MaxDistance = 500 // 500 m — Munich is ~504 km away
	users := FilteredUsers{
		All:    map[int64]User{10: user},
		TopPVP: []User{user},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})

	sender.AssertNotCalled(t, "Send")
}

// ── filterAndSendEncounters: channel zero-threshold skip ─────────────────────

func TestFilterAndSendEncounters_Channel_ZeroThresholds_Skipped(t *testing.T) {
	sender := &mockBotSender{}
	svc := newTestService(nil, sender)

	enc := minimalEncounter("ch-zero", 1)

	// Channel user with both MinIV and MinLevel at zero → should be skipped.
	user := notifyUser(20)
	user.MinIV = 0
	user.MinLevel = 0
	users := FilteredUsers{
		All:      map[int64]User{20: user},
		Channels: []User{user},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})

	sender.AssertNotCalled(t, "Send")
}

func TestFilterAndSendEncounters_Channel_BelowThreshold_NoNotification(t *testing.T) {
	sender := &mockBotSender{}
	svc := newTestService(nil, sender)

	enc := minimalEncounter("ch-below", 1) // IV=80, Level=25

	user := notifyUser(21)
	user.MinIV = 90 // 90 > 80 → no notification
	user.MinLevel = 20
	users := FilteredUsers{
		All:      map[int64]User{21: user},
		Channels: []User{user},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, map[int][]Subscription{})

	sender.AssertNotCalled(t, "Send")
}

// ── filterAndSendEncounters: subscription falls back to user-level thresholds ──

func TestFilterAndSendEncounters_Subscription_FallsBackToUserMinIV(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)
	svc := newTestService(db, sender)

	enc := minimalEncounter("fb-iv", 25) // IV = 80

	user := notifyUser(30)
	user.MinIV = 70 // user-level fallback: 70 < 80 → should notify
	users := FilteredUsers{All: map[int64]User{30: user}}
	// Subscription MinIV=0 → falls back to user.MinIV=70.
	subs := map[int][]Subscription{
		25: {{UserID: 30, PokemonID: 25, MinIV: 0}},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, subs)

	assert.GreaterOrEqual(t, sendCallCount(sender), 2)
}

func TestFilterAndSendEncounters_Subscription_FallsBackToUserMinLevel(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	sender := &mockBotSender{}
	svc := newTestService(nil, sender)

	enc := minimalEncounter("fb-lvl", 25) // Level = 25

	user := notifyUser(31)
	user.MinLevel = 30 // user-level fallback: 30 > 25 → no notification
	users := FilteredUsers{All: map[int64]User{31: user}}
	// Subscription MinLevel=0 → falls back to user.MinLevel=30.
	subs := map[int][]Subscription{
		25: {{UserID: 31, PokemonID: 25, MinIV: 0, MinLevel: 0}},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, subs)

	sender.AssertNotCalled(t, "Send")
}

func TestFilterAndSendEncounters_Subscription_FallsBackToUserMaxDistance(t *testing.T) {
	sender := &mockBotSender{}
	svc := newTestService(nil, sender)

	enc := minimalEncounter("fb-dist", 25)
	enc.Lat = 48.14 // Munich
	enc.Lon = 11.58

	user := notifyUser(32)
	user.Latitude = 52.52   // Berlin
	user.Longitude = 13.40  // Berlin
	user.MaxDistance = 1000 // 1 km → Munich is far away
	users := FilteredUsers{All: map[int64]User{32: user}}
	// Subscription MaxDistance=0 → falls back to user.MaxDistance=1000.
	subs := map[int][]Subscription{
		25: {{UserID: 32, PokemonID: 25, MinIV: 0, MaxDistance: 0}},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, subs)

	sender.AssertNotCalled(t, "Send")
}

func TestFilterAndSendEncounters_Subscription_UnknownUserSkipped(t *testing.T) {
	sender := &mockBotSender{}
	svc := newTestService(nil, sender)

	enc := minimalEncounter("unknown-user", 25)

	// Subscription references user 99, but users.All has no entry for 99.
	users := FilteredUsers{All: map[int64]User{}}
	subs := map[int][]Subscription{
		25: {{UserID: 99, PokemonID: 25, MinIV: 0}},
	}

	svc.filterAndSendEncounters(users, []EncounterData{enc}, subs)

	sender.AssertNotCalled(t, "Send")
}

// ── processEncounters ─────────────────────────────────────────────────────────

func TestProcessEncounters_ScannerError_NoNotifications(t *testing.T) {
	db := &mockBotDB{}
	scanDB := &mockScannerDB{}
	sender := &mockBotSender{}

	scanDB.On("GetRecentEncounters").Return([]EncounterData{}, errors.New("db error"))

	svc := newTestService(db, sender)
	svc.scannerDB = scanDB

	users := FilteredUsers{All: map[int64]User{}}
	assert.NotPanics(t, func() {
		svc.processEncounters(users, map[int][]Subscription{})
	})
	sender.AssertNotCalled(t, "Send")
}

func TestProcessEncounters_Success_DispatchesNotifications(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	db := &mockBotDB{}
	scanDB := &mockScannerDB{}
	sender := &mockBotSender{}

	enc := minimalEncounter("proc-enc", 25)
	scanDB.On("GetRecentEncounters").Return([]EncounterData{enc}, nil)
	setupBotDbForEncounter(db)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(db, sender)
	svc.scannerDB = scanDB

	user := notifyUser(1)
	users := FilteredUsers{All: map[int64]User{1: user}}
	subs := map[int][]Subscription{
		25: {{UserID: 1, PokemonID: 25, MinIV: 0}},
	}

	svc.processEncounters(users, subs)

	assert.GreaterOrEqual(t, sendCallCount(sender), 2)
}

// ── formatDistance ────────────────────────────────────────────────────────────

func TestFormatDistance_BelowOneKm_UsesMetre(t *testing.T) {
	result := formatDistance(350.0)
	assert.Contains(t, result, "350m")
	assert.NotContains(t, result, "km")
}

func TestFormatDistance_AboveOneKm_UsesKilometres(t *testing.T) {
	result := formatDistance(1500.0)
	assert.Contains(t, result, "km")
	assert.Contains(t, result, "1.50km")
}

func TestFormatDistance_ExactlyOneKm_UsesKilometres(t *testing.T) {
	result := formatDistance(1000.0)
	assert.Contains(t, result, "km")
}

// ── generateNotificationText: PVP data ───────────────────────────────────────

func TestGenerateNotificationText_PVPData_IncludesLeagueRank(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("pvp-text", 25)
	enc.PVPData = PVP{
		"great": {{Pokemon: 25, Rank: 2, CP: 1499, Level: 50, Percentage: 98.5}},
	}

	text := svc.generateNotificationText(user, enc)

	assert.Contains(t, text, "Great")
	assert.Contains(t, text, "Rank")
	assert.Contains(t, text, "2")
}

func TestGenerateNotificationText_PVPData_Rank4_NotIncluded(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("pvp-rank4-text", 25)
	enc.PVPData = PVP{
		"great": {{Pokemon: 25, Rank: 4, CP: 1499, Level: 50, Percentage: 95.0}},
	}

	text := svc.generateNotificationText(user, enc)

	assert.NotContains(t, text, "Great")
}

// ── generateNotificationText: km distance branch ─────────────────────────────

func TestGenerateNotificationText_LargeDistance_ShowsKilometres(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(nil, &mockBotSender{})
	// User at Berlin, encounter at Munich (~504 km).
	user := notifyUser(1)
	user.Latitude = 52.52
	user.Longitude = 13.40
	enc := minimalEncounter("km-dist", 25)
	enc.Lat = 48.14
	enc.Lon = 11.58

	text := svc.generateNotificationText(user, enc)

	assert.Contains(t, text, "📍")
	assert.Contains(t, text, "km")
}
