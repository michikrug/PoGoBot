package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGameData returns a minimal MasterFile suitable for unit tests.
func testGameData() MasterFile {
	return MasterFile{
		Pokemon: map[string]Pokemon{
			"25": {
				Name:      "Pikachu",
				PokedexID: 25,
				Forms: map[string]Form{
					"1": {Name: "Normal"},
					"2": {Name: "Halloween", IsCostume: true},
				},
			},
			"1": {
				Name:      "Bulbasaur",
				PokedexID: 1,
				Forms:     map[string]Form{},
			},
		},
		Moves: map[string]Move{
			"200": {Name: "Thunderbolt"},
			"13":  {Name: "Wrap"},
		},
	}
}

// testTranslations returns a small translation table for German.
func testTranslations() map[string]map[string]string {
	return map[string]map[string]string{
		"de": {
			"Pikachu":     "Pikachu",
			"Bulbasaur":   "Bisasam",
			"Thunderbolt": "Donnerblitz",
			"Unknown":     "Unbekannt",
			"CP":          "WP",
		},
	}
}

// ── getTranslation ────────────────────────────────────────────────────────────

func TestGetTranslation_English_IsNoOp(t *testing.T) {
	translations = testTranslations()
	assert.Equal(t, "Pikachu", getTranslation("Pikachu", "en"))
}

func TestGetTranslation_German_KnownKey(t *testing.T) {
	translations = testTranslations()
	assert.Equal(t, "Bisasam", getTranslation("Bulbasaur", "de"))
}

func TestGetTranslation_German_MissingKey_FallsBackToKey(t *testing.T) {
	translations = testTranslations()
	// "Squirtle" is not in the German table → falls back to the key itself.
	assert.Equal(t, "Squirtle", getTranslation("Squirtle", "de"))
}

func TestGetTranslation_UnknownLanguage_FallsBackToKey(t *testing.T) {
	translations = testTranslations()
	assert.Equal(t, "Pikachu", getTranslation("Pikachu", "zz"))
}

// ── getPokemonName ────────────────────────────────────────────────────────────

func TestGetPokemonName_English_KnownID(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	assert.Equal(t, "Pikachu", getPokemonName(25, "en"))
}

func TestGetPokemonName_German_KnownID(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	assert.Equal(t, "Bisasam", getPokemonName(1, "de"))
}

func TestGetPokemonName_English_UnknownID(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	assert.Equal(t, "Unknown", getPokemonName(9999, "en"))
}

func TestGetPokemonName_German_UnknownID(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	assert.Equal(t, "Unbekannt", getPokemonName(9999, "de"))
}

// ── getMoveName ───────────────────────────────────────────────────────────────

func TestGetMoveName_English_KnownID(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	assert.Equal(t, "Thunderbolt", getMoveName(200, "en"))
}

func TestGetMoveName_German_KnownID(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	assert.Equal(t, "Donnerblitz", getMoveName(200, "de"))
}

func TestGetMoveName_English_UnknownID(t *testing.T) {
	gameData = testGameData()
	translations = testTranslations()
	assert.Equal(t, "Unknown", getMoveName(9999, "en"))
}

// ── getPokemonID ──────────────────────────────────────────────────────────────

func TestGetPokemonID_KnownName(t *testing.T) {
	pokemonNameCache = map[string]int{
		"pikachu":   25,
		"bulbasaur": 1,
	}
	id, err := getPokemonID("Pikachu") // input is title-case, cache is lower
	require.NoError(t, err)
	assert.Equal(t, 25, id)
}

func TestGetPokemonID_CaseInsensitive(t *testing.T) {
	pokemonNameCache = map[string]int{"pikachu": 25}
	id, err := getPokemonID("PIKACHU")
	require.NoError(t, err)
	assert.Equal(t, 25, id)
}

func TestGetPokemonID_UnknownName(t *testing.T) {
	pokemonNameCache = map[string]int{"pikachu": 25}
	_, err := getPokemonID("Squirtle")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Squirtle")
}

func TestGetPokemonID_EmptyCache(t *testing.T) {
	pokemonNameCache = map[string]int{}
	_, err := getPokemonID("Pikachu")
	require.Error(t, err)
}
