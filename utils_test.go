package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── haversine ─────────────────────────────────────────────────────────────────

func TestHaversine_SamePoint(t *testing.T) {
	assert.Equal(t, 0.0, haversine(52.0, 13.0, 52.0, 13.0))
}

func TestHaversine_KnownDistance(t *testing.T) {
	// Berlin (52.5200, 13.4050) to Munich (48.1351, 11.5820).
	// Approximate great-circle distance is ~504 km.
	dist := haversine(52.5200, 13.4050, 48.1351, 11.5820)
	assert.InDelta(t, 504_000, dist, 5_000, "expected ~504 km between Berlin and Munich")
}

func TestHaversine_ShortDistance(t *testing.T) {
	// Two points ~111 m apart (1/1000 of a degree of latitude).
	dist := haversine(48.0, 11.0, 48.001, 11.0)
	assert.InDelta(t, 111.0, dist, 5.0, "1/1000 degree of latitude ≈ 111 m")
}

func TestHaversine_AcrossAntimeridian(t *testing.T) {
	// Points on either side of the 180th meridian — result must be positive.
	dist := haversine(0.0, 179.9, 0.0, -179.9)
	assert.Greater(t, dist, 0.0)
}

// ── withinDistance ────────────────────────────────────────────────────────────

func userAt(lat, lon float32, maxDist int) User {
	return User{Latitude: lat, Longitude: lon, MaxDistance: maxDist}
}

func TestWithinDistance_NoLocation_ReturnsTrue(t *testing.T) {
	u := User{Latitude: 0, Longitude: 0, MaxDistance: 500}
	assert.True(t, withinDistance(u, 48.0, 11.0, u.MaxDistance))
}

func TestWithinDistance_NoMaxDistance_ReturnsTrue(t *testing.T) {
	u := userAt(48.0, 11.0, 0)
	assert.True(t, withinDistance(u, 48.5, 11.5, 0))
}

func TestWithinDistance_CloseEnough(t *testing.T) {
	// User and target ~111 m apart; limit 200 m.
	u := userAt(48.0, 11.0, 200)
	assert.True(t, withinDistance(u, 48.001, 11.0, u.MaxDistance))
}

func TestWithinDistance_TooFar(t *testing.T) {
	// User and target ~111 m apart; limit 50 m.
	u := userAt(48.0, 11.0, 50)
	assert.False(t, withinDistance(u, 48.001, 11.0, u.MaxDistance))
}

func TestWithinDistance_ExactlyOnBoundary(t *testing.T) {
	// Just assert the function doesn't panic and returns a bool.
	u := userAt(0, 0, 1000)
	_ = withinDistance(u, 0.009, 0, u.MaxDistance)
}

// ── boolToEmoji ───────────────────────────────────────────────────────────────

func TestBoolToEmoji_True(t *testing.T) {
	assert.Equal(t, "✅", boolToEmoji(true))
}

func TestBoolToEmoji_False(t *testing.T) {
	assert.Equal(t, "❌", boolToEmoji(false))
}

// ── getGenderEmoji ────────────────────────────────────────────────────────────

func TestGetGenderEmoji_Nil(t *testing.T) {
	assert.Equal(t, "", getGenderEmoji(nil))
}

func TestGetGenderEmoji_Male(t *testing.T) {
	g := 1
	assert.Equal(t, " ♂", getGenderEmoji(&g))
}

func TestGetGenderEmoji_Female(t *testing.T) {
	g := 2
	assert.Equal(t, " ♀", getGenderEmoji(&g))
}

func TestGetGenderEmoji_Genderless(t *testing.T) {
	g := 3
	assert.Equal(t, " ⚲", getGenderEmoji(&g))
}

func TestGetGenderEmoji_Unknown(t *testing.T) {
	g := 99
	assert.Equal(t, "", getGenderEmoji(&g)) // not in map → zero value ""
}

// ── getSizeEmoji ──────────────────────────────────────────────────────────────

func TestGetSizeEmoji_Nil(t *testing.T) {
	assert.Equal(t, "", getSizeEmoji(nil))
}

func TestGetSizeEmoji_Small(t *testing.T) {
	s := 1
	assert.Equal(t, " 🔹", getSizeEmoji(&s))
}

func TestGetSizeEmoji_Large(t *testing.T) {
	s := 5
	assert.Equal(t, " 🔶", getSizeEmoji(&s))
}

func TestGetSizeEmoji_Normal(t *testing.T) {
	s := 3
	assert.Equal(t, "", getSizeEmoji(&s))
}

// ── getWeatherEmoji ───────────────────────────────────────────────────────────

func TestGetWeatherEmoji_Nil(t *testing.T) {
	assert.Equal(t, "", getWeatherEmoji(nil))
}

func TestGetWeatherEmoji_Clear(t *testing.T) {
	w := 1
	assert.Equal(t, " ☀️", getWeatherEmoji(&w))
}

func TestGetWeatherEmoji_Rain(t *testing.T) {
	w := 2
	assert.Equal(t, " ☔️", getWeatherEmoji(&w))
}

func TestGetWeatherEmoji_Snow(t *testing.T) {
	w := 6
	assert.Equal(t, " ⛄️", getWeatherEmoji(&w))
}

func TestGetWeatherEmoji_Unknown(t *testing.T) {
	w := 99
	assert.Equal(t, "", getWeatherEmoji(&w))
}
