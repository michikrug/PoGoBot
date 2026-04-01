package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/telebot.v3"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newTestService builds a NotificationService with mocked deps and no
// Prometheus counters (all nil-safe guards in the service handle this).
func newTestService(botDB *mockBotDB, sender *mockBotSender) *NotificationService {
	return &NotificationService{
		botDB:                botDB,
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
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
	enc := minimalEncounter("e1", 25) // Form is nil
	assert.Equal(t, "", svc.buildFormSuffix(enc, "en"))
}

func TestBuildFormSuffix_FormZero(t *testing.T) {
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
	enc := minimalEncounter("e1", 25)
	enc.Form = pointerToInt(0)
	assert.Equal(t, "", svc.buildFormSuffix(enc, "en"))
}

func TestBuildFormSuffix_NormalForm(t *testing.T) {
	// Form 1 on Pikachu is "Normal" → should return "".
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
	enc := minimalEncounter("e1", 25)
	enc.Form = pointerToInt(1)
	assert.Equal(t, "", svc.buildFormSuffix(enc, "en"))
}

func TestBuildFormSuffix_NamedForm(t *testing.T) {
	// Form 2 on Pikachu is "Halloween" with IsCostume=true.
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
	enc := minimalEncounter("e1", 25)
	enc.Form = pointerToInt(2)
	suffix := svc.buildFormSuffix(enc, "en")
	assert.Contains(t, suffix, "Halloween")
	assert.Contains(t, suffix, "👕")
}

func TestBuildFormSuffix_UnknownPokemon(t *testing.T) {
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
	enc := minimalEncounter("e1", 9999)
	enc.Form = pointerToInt(1)
	assert.Equal(t, "", svc.buildFormSuffix(enc, "en"))
}

// ── generateNotificationTitle ─────────────────────────────────────────────────

func TestGenerateNotificationTitle_ContainsPokemonName(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("e1", 25)
	title := svc.generateNotificationTitle(user, enc)
	assert.Contains(t, title, "Pikachu")
}

func TestGenerateNotificationTitle_ContainsIVAndLevel(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
	user := notifyUser(1)
	enc := minimalEncounter("e1", 25)
	title := svc.generateNotificationTitle(user, enc)
	assert.Contains(t, title, "80.0%")
	assert.Contains(t, title, "L25")
}

func TestGenerateNotificationTitle_EnglishUsesCPLabel(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
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
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
	user := notifyUser(1)
	user.Language = "de"
	enc := minimalEncounter("e1", 25)
	title := svc.generateNotificationTitle(user, enc)
	assert.Contains(t, title, "WP")
}

func TestGenerateNotificationTitle_IncludesSizeEmoji(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
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
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
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
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
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
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
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
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
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

// setupBotRepoForEncounter configures the bot repo mocks needed when a
// notification is delivered (SaveEncounter + SaveMessage).
func setupBotRepoForEncounter(botDB *mockBotDB) {
	botDB.On("SaveEncounter", mock.Anything, mock.Anything).Return()
	botDB.On("SaveMessage", mock.Anything, mock.Anything, mock.Anything).Return()
}

func TestFilterAndSendEncounters_HundoIV_NotifiesHundoUsers(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()

	repo := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotRepoForEncounter(repo)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(repo, sender)

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

	repo := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotRepoForEncounter(repo)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(repo, sender)

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

	repo := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotRepoForEncounter(repo)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(repo, sender)

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

	repo := &mockBotDB{}
	sender := &mockBotSender{}

	svc := newTestService(repo, sender)

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

	repo := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotRepoForEncounter(repo)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(repo, sender)

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

	repo := &mockBotDB{}
	sender := &mockBotSender{}

	svc := newTestService(repo, sender)

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

	repo := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotRepoForEncounter(repo)
	setupSenderForAnyEncounter(sender)

	svc := newTestService(repo, sender)

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
	repo := &mockBotDB{}
	sender := &mockBotSender{}

	enc := Encounter{ID: "expired1", Expiration: int(time.Now().Add(-1 * time.Minute).Unix())}
	msg := Message{ChatID: 10, MessageID: 99, EncounterID: enc.ID}

	repo.On("GetExpiredEncountersWithMessages").Return(
		[]Encounter{enc},
		map[string][]Message{enc.ID: {msg}},
	)
	repo.On("DeleteMessage", msg).Return()
	repo.On("DeleteEncounter", enc).Return()

	// User has Cleanup=true → sender.Delete should be called.
	sender.On("Delete", mock.Anything).Return(nil)

	svc := newTestService(repo, sender)

	user := User{ID: 10, Cleanup: true}
	users := FilteredUsers{
		All: map[int64]User{10: user},
	}

	svc.cleanupMessages(users)

	repo.AssertCalled(t, "DeleteMessage", msg)
	repo.AssertCalled(t, "DeleteEncounter", enc)
	sender.AssertCalled(t, "Delete", mock.Anything)
}

func TestCleanupMessages_SkipsDeleteWhenCleanupDisabled(t *testing.T) {
	repo := &mockBotDB{}
	sender := &mockBotSender{}

	enc := Encounter{ID: "expired2"}
	msg := Message{ChatID: 20, MessageID: 88, EncounterID: enc.ID}

	repo.On("GetExpiredEncountersWithMessages").Return(
		[]Encounter{enc},
		map[string][]Message{enc.ID: {msg}},
	)
	repo.On("DeleteMessage", msg).Return()
	repo.On("DeleteEncounter", enc).Return()

	svc := newTestService(repo, sender)

	// Cleanup=false → sender.Delete must NOT be called.
	user := User{ID: 20, Cleanup: false}
	users := FilteredUsers{
		All: map[int64]User{20: user},
	}

	svc.cleanupMessages(users)

	sender.AssertNotCalled(t, "Delete")
	repo.AssertCalled(t, "DeleteMessage", msg)
	repo.AssertCalled(t, "DeleteEncounter", enc)
}

func TestCleanupMessages_ClearsNotificationCache(t *testing.T) {
	repo := &mockBotDB{}
	sender := &mockBotSender{}

	enc := Encounter{ID: "cached1"}
	repo.On("GetExpiredEncountersWithMessages").Return(
		[]Encounter{enc},
		map[string][]Message{},
	)
	repo.On("DeleteEncounter", enc).Return()

	svc := newTestService(repo, sender)
	// Pre-populate cache.
	svc.notificationCache["cached1"] = map[int64]struct{}{99: {}}

	users := FilteredUsers{All: map[int64]User{}}
	svc.cleanupMessages(users)

	require.Contains(t, svc.notificationCache, "cached1")
	assert.Nil(t, svc.notificationCache["cached1"])
}

func TestCleanupMessages_NoEncounters_NoOp(t *testing.T) {
	repo := &mockBotDB{}
	sender := &mockBotSender{}

	repo.On("GetExpiredEncountersWithMessages").Return(
		[]Encounter{},
		map[string][]Message{},
	)

	svc := newTestService(repo, sender)
	users := FilteredUsers{All: map[int64]User{}}

	// Should not panic or call any other method.
	assert.NotPanics(t, func() {
		svc.cleanupMessages(users)
	})
	sender.AssertNotCalled(t, "Delete")
}

// ── permanent error → disables notifications ──────────────────────────────────

func TestBotSend_PermanentError_DisablesNotify(t *testing.T) {
	repo := &mockBotDB{}
	sender := &mockBotSender{}

	permErr := errors.New("bot was blocked by the user")
	sender.On("Send", mock.Anything, mock.Anything, mock.Anything).Return(nil, permErr)
	repo.On("UpdateUserPreference", int64(1), "Notify", false).Return()
	repo.On("GetUsers").Return([]User{})
	repo.On("GetSubscriptions").Return([]Subscription{})

	// updateUserPreference uses botDB, so wire it up for this test.
	botDB = repo
	t.Cleanup(func() { botDB = nil })

	svc := newTestService(repo, sender)

	_, err := svc.botSend(1, &telebot.User{ID: 1}, "hello")
	assert.Error(t, err)
	repo.AssertCalled(t, "UpdateUserPreference", int64(1), "Notify", false)
}

// ── rate-limit gate ───────────────────────────────────────────────────────────

func TestBotSend_UserRateLimited_SkipsSend(t *testing.T) {
	repo := &mockBotDB{}
	sender := &mockBotSender{}

	svc := newTestService(repo, sender)
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

	repo := &mockBotDB{}
	sender := &mockBotSender{}
	svc := newTestService(repo, sender)

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

	repo := &mockBotDB{}
	sender := &mockBotSender{}
	setupBotRepoForEncounter(repo)
	setupSenderForAnyEncounter(sender)
	svc := newTestService(repo, sender)

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

	repo := &mockBotDB{}
	sender := &mockBotSender{}
	svc := newTestService(repo, sender)

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
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
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
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
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
	svc := newTestService(&mockBotDB{}, &mockBotSender{})
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

	repo := &mockBotDB{}
	sender := &mockBotSender{}
	// No SaveEncounter expectation — it should not be called when ExpireTimestamp is nil.
	repo.On("SaveMessage", mock.Anything, mock.Anything, mock.Anything).Return()
	setupSenderForAnyEncounter(sender)
	svc := newTestService(repo, sender)

	enc := minimalEncounter("nil_exp_send", 25)
	enc.ExpireTimestamp = nil
	user := notifyUser(99)

	assert.NotPanics(t, func() {
		svc.sendEncounterNotification(user, enc)
	})
	repo.AssertNotCalled(t, "SaveEncounter")
}
