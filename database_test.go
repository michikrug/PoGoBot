package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTestBotDB opens an in-memory SQLite DB, auto-migrates the required tables,
// and returns a ready-to-use gormBotDB.
func newTestBotDB(t *testing.T) *gormBotDB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Subscription{}, &User{}, &Encounter{}, &Message{}))
	return &gormBotDB{db: db}
}

// newTestScannerDB opens an in-memory SQLite DB, auto-migrates the scanner
// tables, and returns a ready-to-use gormScannerDB.
func newTestScannerDB(t *testing.T) *gormScannerDB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&EncounterData{}, &GymData{}))
	return &gormScannerDB{db: db}
}

// ptr helpers for nullable fields used in EncounterData / GymData fixtures.
func ptrString(s string) *string    { return &s }
func ptrInt(i int) *int             { return &i }
func ptrFloat32(f float32) *float32 { return &f }

// ── GetUserPreferences ────────────────────────────────────────────────────────

func TestGetUserPreferences_CreatesNewUser(t *testing.T) {
	db := newTestBotDB(t)

	user := db.GetUserPreferences(123)

	assert.Equal(t, int64(123), user.ID)
}

func TestGetUserPreferences_ReturnsSameUserOnSecondCall(t *testing.T) {
	db := newTestBotDB(t)

	first := db.GetUserPreferences(42)
	second := db.GetUserPreferences(42)

	assert.Equal(t, first.ID, second.ID)

	var count int64
	db.db.Model(&User{}).Where("id = ?", 42).Count(&count)
	assert.Equal(t, int64(1), count, "should not create duplicate users")
}

// ── UpdateUserPreference ──────────────────────────────────────────────────────

func TestUpdateUserPreference_UpdatesField(t *testing.T) {
	db := newTestBotDB(t)
	db.GetUserPreferences(1) // ensure row exists

	db.UpdateUserPreference(1, "notify", false)

	user := db.GetUserPreferences(1)
	assert.False(t, user.Notify)
}

func TestUpdateUserPreference_UpdatesLanguage(t *testing.T) {
	db := newTestBotDB(t)
	db.GetUserPreferences(1)

	db.UpdateUserPreference(1, "language", "en")

	user := db.GetUserPreferences(1)
	assert.Equal(t, "en", user.Language)
}

func TestUpdateUserPreference_UpdatesMaxDistance(t *testing.T) {
	db := newTestBotDB(t)
	db.GetUserPreferences(1)

	db.UpdateUserPreference(1, "max_distance", 750)

	user := db.GetUserPreferences(1)
	assert.Equal(t, 750, user.MaxDistance)
}

// ── AddSubscription ───────────────────────────────────────────────────────────

func TestAddSubscription_CreatesRecord(t *testing.T) {
	db := newTestBotDB(t)

	db.AddSubscription(1, 25, 80, 20, 500)

	subs := db.GetSubscriptions()
	require.Len(t, subs, 1)
	assert.Equal(t, int64(1), subs[0].UserID)
	assert.Equal(t, 25, subs[0].PokemonID)
	assert.Equal(t, 80, subs[0].MinIV)
	assert.Equal(t, 20, subs[0].MinLevel)
	assert.Equal(t, 500, subs[0].MaxDistance)
}

func TestAddSubscription_UpsertUpdatesExisting(t *testing.T) {
	db := newTestBotDB(t)
	db.AddSubscription(1, 25, 80, 20, 500)

	// Save with same primary key but different values — should update.
	db.AddSubscription(1, 25, 95, 30, 1000)

	subs := db.GetSubscriptions()
	require.Len(t, subs, 1, "upsert should not create a duplicate row")
	assert.Equal(t, 95, subs[0].MinIV)
	assert.Equal(t, 30, subs[0].MinLevel)
	assert.Equal(t, 1000, subs[0].MaxDistance)
}

// ── GetUsers ──────────────────────────────────────────────────────────────────

func TestGetUsers_Empty(t *testing.T) {
	db := newTestBotDB(t)

	users := db.GetUsers()

	assert.Empty(t, users)
}

func TestGetUsers_ReturnsAllUsers(t *testing.T) {
	db := newTestBotDB(t)
	db.GetUserPreferences(1)
	db.GetUserPreferences(2)
	db.GetUserPreferences(3)

	users := db.GetUsers()

	assert.Len(t, users, 3)
}

// ── GetSubscriptions ──────────────────────────────────────────────────────────

func TestGetSubscriptions_Empty(t *testing.T) {
	db := newTestBotDB(t)
	result := db.GetSubscriptions()
	assert.Empty(t, result, "expected empty slice when no subscriptions exist")
}

func TestGetSubscriptions_SingleEntry(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Subscription{UserID: 1, PokemonID: 25, MinIV: 80})

	result := db.GetSubscriptions()

	require.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].UserID)
	assert.Equal(t, 25, result[0].PokemonID)
	assert.Equal(t, 80, result[0].MinIV)
}

func TestGetSubscriptions_MultipleEntries(t *testing.T) {
	db := newTestBotDB(t)
	// Two users subscribed to Pikachu (25), one to Eevee (133).
	db.db.Create(&Subscription{UserID: 1, PokemonID: 25, MinIV: 90})
	db.db.Create(&Subscription{UserID: 2, PokemonID: 25, MinIV: 80})
	db.db.Create(&Subscription{UserID: 1, PokemonID: 133, MinIV: 0})

	result := db.GetSubscriptions()

	assert.Len(t, result, 3, "expected all three subscriptions returned")
}

func TestGetSubscriptions_PreservesFields(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Subscription{
		UserID:      42,
		PokemonID:   150,
		MinIV:       95,
		MinLevel:    30,
		MaxDistance: 500,
	})

	result := db.GetSubscriptions()

	require.Len(t, result, 1)
	subscription := result[0]
	assert.Equal(t, int64(42), subscription.UserID)
	assert.Equal(t, 150, subscription.PokemonID)
	assert.Equal(t, 95, subscription.MinIV)
	assert.Equal(t, 30, subscription.MinLevel)
	assert.Equal(t, 500, subscription.MaxDistance)
}

// ── SaveMessage / SaveEncounter ───────────────────────────────────────────────

func TestSaveMessage_CreatesRecord(t *testing.T) {
	db := newTestBotDB(t)

	db.SaveMessage(100, 42, "enc-001")

	var msg Message
	require.NoError(t, db.db.First(&msg, "chat_id = ? AND message_id = ?", 100, 42).Error)
	assert.Equal(t, "enc-001", msg.EncounterID)
}

func TestSaveEncounter_CreatesRecord(t *testing.T) {
	db := newTestBotDB(t)
	expiration := time.Now().Add(10 * time.Minute).Unix()

	db.SaveEncounter("enc-001", expiration)

	var enc Encounter
	require.NoError(t, db.db.First(&enc, "id = ?", "enc-001").Error)
	assert.Equal(t, int(expiration), enc.Expiration)
}

func TestSaveEncounter_UpsertUpdatesExpiration(t *testing.T) {
	db := newTestBotDB(t)
	first := time.Now().Add(5 * time.Minute).Unix()
	second := time.Now().Add(15 * time.Minute).Unix()

	db.SaveEncounter("enc-001", first)
	db.SaveEncounter("enc-001", second)

	var enc Encounter
	require.NoError(t, db.db.First(&enc, "id = ?", "enc-001").Error)
	assert.Equal(t, int(second), enc.Expiration)
}

// ── GetUserSubscriptions ──────────────────────────────────────────────────────

func TestGetUserSubscriptions_ReturnsOnlyForUser(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Subscription{UserID: 1, PokemonID: 25})
	db.db.Create(&Subscription{UserID: 1, PokemonID: 133})
	db.db.Create(&Subscription{UserID: 2, PokemonID: 25})

	subs := db.GetUserSubscriptions(1)

	assert.Len(t, subs, 2)
	for _, s := range subs {
		assert.Equal(t, int64(1), s.UserID)
	}
}

func TestGetUserSubscriptions_OrderedByPokemonID(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Subscription{UserID: 1, PokemonID: 133})
	db.db.Create(&Subscription{UserID: 1, PokemonID: 25})
	db.db.Create(&Subscription{UserID: 1, PokemonID: 60})

	subs := db.GetUserSubscriptions(1)

	require.Len(t, subs, 3)
	assert.Equal(t, 25, subs[0].PokemonID)
	assert.Equal(t, 60, subs[1].PokemonID)
	assert.Equal(t, 133, subs[2].PokemonID)
}

func TestGetUserSubscriptions_EmptyForUnknownUser(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Subscription{UserID: 1, PokemonID: 25})

	subs := db.GetUserSubscriptions(99)

	assert.Empty(t, subs)
}

// ── DeleteSubscription ────────────────────────────────────────────────────────

func TestDeleteSubscription_RemovesTargetRow(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Subscription{UserID: 1, PokemonID: 25})
	db.db.Create(&Subscription{UserID: 1, PokemonID: 133})

	db.DeleteSubscription(1, 25)

	subs := db.GetUserSubscriptions(1)
	require.Len(t, subs, 1)
	assert.Equal(t, 133, subs[0].PokemonID)
}

func TestDeleteSubscription_DoesNotAffectOtherUsers(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Subscription{UserID: 1, PokemonID: 25})
	db.db.Create(&Subscription{UserID: 2, PokemonID: 25})

	db.DeleteSubscription(1, 25)

	subs := db.GetUserSubscriptions(2)
	assert.Len(t, subs, 1)
}

// ── DeleteAllUserSubscriptions ────────────────────────────────────────────────

func TestDeleteAllUserSubscriptions_RemovesAllForUser(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Subscription{UserID: 1, PokemonID: 25})
	db.db.Create(&Subscription{UserID: 1, PokemonID: 133})
	db.db.Create(&Subscription{UserID: 2, PokemonID: 25})

	db.DeleteAllUserSubscriptions(1)

	assert.Empty(t, db.GetUserSubscriptions(1))
	assert.Len(t, db.GetUserSubscriptions(2), 1)
}

func TestDeleteAllUserSubscriptions_NoOpForUnknownUser(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Subscription{UserID: 1, PokemonID: 25})

	db.DeleteAllUserSubscriptions(99)

	assert.Len(t, db.GetSubscriptions(), 1)
}

// ── GetExpiredEncountersWithMessages ─────────────────────────────────────────

func TestGetExpiredEncountersWithMessages_ReturnsOnlyExpired(t *testing.T) {
	db := newTestBotDB(t)
	past := int(time.Now().Add(-1 * time.Minute).Unix())
	future := int(time.Now().Add(10 * time.Minute).Unix())

	db.db.Create(&Encounter{ID: "expired-1", Expiration: past})
	db.db.Create(&Encounter{ID: "active-1", Expiration: future})

	encounters, _ := db.GetExpiredEncountersWithMessages()

	require.Len(t, encounters, 1)
	assert.Equal(t, "expired-1", encounters[0].ID)
}

func TestGetExpiredEncountersWithMessages_ReturnsAssociatedMessages(t *testing.T) {
	db := newTestBotDB(t)
	past := int(time.Now().Add(-1 * time.Minute).Unix())

	db.db.Create(&Encounter{ID: "enc-1", Expiration: past})
	db.db.Create(&Message{ChatID: 10, MessageID: 1, EncounterID: "enc-1"})
	db.db.Create(&Message{ChatID: 10, MessageID: 2, EncounterID: "enc-1"})

	encounters, messagesMap := db.GetExpiredEncountersWithMessages()

	require.Len(t, encounters, 1)
	msgs := messagesMap["enc-1"]
	assert.Len(t, msgs, 2)
}

func TestGetExpiredEncountersWithMessages_EmptyWhenNoneExpired(t *testing.T) {
	db := newTestBotDB(t)
	future := int(time.Now().Add(10 * time.Minute).Unix())
	db.db.Create(&Encounter{ID: "active-1", Expiration: future})

	encounters, messagesMap := db.GetExpiredEncountersWithMessages()

	assert.Empty(t, encounters)
	assert.Empty(t, messagesMap)
}

func TestGetExpiredEncountersWithMessages_NoMessagesForExpiredEncounter(t *testing.T) {
	db := newTestBotDB(t)
	past := int(time.Now().Add(-1 * time.Minute).Unix())
	db.db.Create(&Encounter{ID: "enc-lonely", Expiration: past})

	encounters, messagesMap := db.GetExpiredEncountersWithMessages()

	require.Len(t, encounters, 1)
	assert.Empty(t, messagesMap["enc-lonely"])
}

// ── DeleteMessage / DeleteEncounter ──────────────────────────────────────────

func TestDeleteMessage_RemovesRecord(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Message{ChatID: 10, MessageID: 1, EncounterID: "enc-1"})

	db.DeleteMessage(Message{ChatID: 10, MessageID: 1, EncounterID: "enc-1"})

	var count int64
	db.db.Model(&Message{}).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestDeleteEncounter_RemovesRecord(t *testing.T) {
	db := newTestBotDB(t)
	db.db.Create(&Encounter{ID: "enc-1", Expiration: 999})

	db.DeleteEncounter(Encounter{ID: "enc-1", Expiration: 999})

	var count int64
	db.db.Model(&Encounter{}).Count(&count)
	assert.Equal(t, int64(0), count)
}

// ── gormScannerDB: GetRecentEncounters ───────────────────────────────────────

func TestGetRecentEncounters_ReturnsRecentRows(t *testing.T) {
	db := newTestScannerDB(t)
	now := int(time.Now().Unix())
	iv := float32(90.0)

	db.db.Create(&EncounterData{
		ID:              "enc-recent",
		PokemonID:       25,
		IV:              &iv,
		Updated:         &now,
		ExpireTimestamp: &now,
	})

	encounters, err := db.GetRecentEncounters()

	require.NoError(t, err)
	require.Len(t, encounters, 1)
	assert.Equal(t, "enc-recent", encounters[0].ID)
}

func TestGetRecentEncounters_ExcludesRowsWithNullIV(t *testing.T) {
	db := newTestScannerDB(t)
	now := int(time.Now().Unix())

	// IV is nil — should be excluded.
	db.db.Create(&EncounterData{
		ID:              "enc-no-iv",
		PokemonID:       25,
		IV:              nil,
		Updated:         &now,
		ExpireTimestamp: &now,
	})

	encounters, err := db.GetRecentEncounters()

	require.NoError(t, err)
	assert.Empty(t, encounters)
}

func TestGetRecentEncounters_ExcludesStaleRows(t *testing.T) {
	db := newTestScannerDB(t)
	old := int(time.Now().Unix() - 120) // 2 minutes ago — outside 30-second window
	iv := float32(80.0)

	db.db.Create(&EncounterData{
		ID:              "enc-old",
		PokemonID:       25,
		IV:              &iv,
		Updated:         &old,
		ExpireTimestamp: &old,
	})

	encounters, err := db.GetRecentEncounters()

	require.NoError(t, err)
	assert.Empty(t, encounters)
}

func TestGetRecentEncounters_ParsesPVPJSON(t *testing.T) {
	db := newTestScannerDB(t)
	now := int(time.Now().Unix())
	iv := float32(95.0)

	pvpData := PVP{
		"great": {{Pokemon: 25, Rank: 1, Percentage: 98.5}},
	}
	pvpJSON, err := json.Marshal(pvpData)
	require.NoError(t, err)
	pvpStr := string(pvpJSON)

	db.db.Create(&EncounterData{
		ID:              "enc-pvp",
		PokemonID:       25,
		IV:              &iv,
		Updated:         &now,
		ExpireTimestamp: &now,
		PVP:             &pvpStr,
	})

	encounters, err := db.GetRecentEncounters()

	require.NoError(t, err)
	require.Len(t, encounters, 1)
	great := encounters[0].PVPData["great"]
	require.Len(t, great, 1)
	assert.Equal(t, int16(1), great[0].Rank)
	assert.Equal(t, 98.5, great[0].Percentage)
}

func TestGetRecentEncounters_InvalidPVPJSONIsSkipped(t *testing.T) {
	db := newTestScannerDB(t)
	now := int(time.Now().Unix())
	iv := float32(80.0)
	badJSON := "{not valid json}"

	db.db.Create(&EncounterData{
		ID:              "enc-badjson",
		PokemonID:       25,
		IV:              &iv,
		Updated:         &now,
		ExpireTimestamp: &now,
		PVP:             &badJSON,
	})

	encounters, err := db.GetRecentEncounters()

	// Should not return an error — bad PVP is logged and skipped.
	require.NoError(t, err)
	require.Len(t, encounters, 1)
	assert.Nil(t, encounters[0].PVPData)
}

func TestGetRecentEncounters_EmptyPVPNotParsed(t *testing.T) {
	db := newTestScannerDB(t)
	now := int(time.Now().Unix())
	iv := float32(80.0)
	emptyPVP := ""

	db.db.Create(&EncounterData{
		ID:              "enc-nopvp",
		PokemonID:       25,
		IV:              &iv,
		Updated:         &now,
		ExpireTimestamp: &now,
		PVP:             &emptyPVP,
	})

	encounters, err := db.GetRecentEncounters()

	require.NoError(t, err)
	require.Len(t, encounters, 1)
	assert.Nil(t, encounters[0].PVPData)
}

// ── gormScannerDB: SearchGymsByName ──────────────────────────────────────────

func TestSearchGymsByName_MatchesSubstring(t *testing.T) {
	db := newTestScannerDB(t)
	name := "Central Park"
	db.db.Create(&GymData{ID: "gym-1", Name: &name})

	results := db.SearchGymsByName("central")

	require.Len(t, results, 1)
	assert.Equal(t, "gym-1", results[0].ID)
}

func TestSearchGymsByName_CaseInsensitive(t *testing.T) {
	db := newTestScannerDB(t)
	name := "Central Park"
	db.db.Create(&GymData{ID: "gym-1", Name: &name})

	results := db.SearchGymsByName("CENTRAL")

	assert.Len(t, results, 1)
}

func TestSearchGymsByName_NoMatch(t *testing.T) {
	db := newTestScannerDB(t)
	name := "Central Park"
	db.db.Create(&GymData{ID: "gym-1", Name: &name})

	results := db.SearchGymsByName("downtown")

	assert.Empty(t, results)
}

func TestSearchGymsByName_MultipleMatches(t *testing.T) {
	db := newTestScannerDB(t)
	name1, name2, name3 := "Park North", "Park South", "Downtown"
	db.db.Create(&GymData{ID: "gym-1", Name: &name1})
	db.db.Create(&GymData{ID: "gym-2", Name: &name2})
	db.db.Create(&GymData{ID: "gym-3", Name: &name3})

	results := db.SearchGymsByName("park")

	assert.Len(t, results, 2)
}

// ── gormScannerDB: GetGymByID ─────────────────────────────────────────────────

func TestGetGymByID_ReturnsMatchingGym(t *testing.T) {
	db := newTestScannerDB(t)
	name := "Test Gym"
	db.db.Create(&GymData{ID: "gym-abc", Name: &name})

	gym := db.GetGymByID("gym-abc")

	assert.Equal(t, "gym-abc", gym.ID)
	require.NotNil(t, gym.Name)
	assert.Equal(t, "Test Gym", *gym.Name)
}

func TestGetGymByID_ReturnsEmptyForUnknownID(t *testing.T) {
	db := newTestScannerDB(t)

	gym := db.GetGymByID("nonexistent")

	assert.Empty(t, gym.ID)
}
