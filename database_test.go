package main

import (
	"testing"

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

// ── GetSubscriptions ──────────────────────────────────────────────────────────

func TestGetSubscriptions_Empty(t *testing.T) {
	repo := newTestBotDB(t)
	result := repo.GetSubscriptions()
	assert.Empty(t, result, "expected empty slice when no subscriptions exist")
}

func TestGetSubscriptions_SingleEntry(t *testing.T) {
	repo := newTestBotDB(t)
	repo.db.Create(&Subscription{UserID: 1, PokemonID: 25, MinIV: 80})

	result := repo.GetSubscriptions()

	require.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].UserID)
	assert.Equal(t, 25, result[0].PokemonID)
	assert.Equal(t, 80, result[0].MinIV)
}

func TestGetSubscriptions_MultipleEntries(t *testing.T) {
	repo := newTestBotDB(t)
	// Two users subscribed to Pikachu (25), one to Eevee (133).
	repo.db.Create(&Subscription{UserID: 1, PokemonID: 25, MinIV: 90})
	repo.db.Create(&Subscription{UserID: 2, PokemonID: 25, MinIV: 80})
	repo.db.Create(&Subscription{UserID: 1, PokemonID: 133, MinIV: 0})

	result := repo.GetSubscriptions()

	assert.Len(t, result, 3, "expected all three subscriptions returned")
}

func TestGetSubscriptions_PreservesFields(t *testing.T) {
	repo := newTestBotDB(t)
	repo.db.Create(&Subscription{
		UserID:      42,
		PokemonID:   150,
		MinIV:       95,
		MinLevel:    30,
		MaxDistance: 500,
	})

	result := repo.GetSubscriptions()

	require.Len(t, result, 1)
	subscription := result[0]
	assert.Equal(t, int64(42), subscription.UserID)
	assert.Equal(t, 150, subscription.PokemonID)
	assert.Equal(t, 95, subscription.MinIV)
	assert.Equal(t, 30, subscription.MinLevel)
	assert.Equal(t, 500, subscription.MaxDistance)
}
