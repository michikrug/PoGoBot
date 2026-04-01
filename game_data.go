package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

// pokemonNameCache maps lowercased Pokémon names to their Pokédex IDs.
// Populated at startup from gameData.
var pokemonNameCache map[string]int

// getPokemonID returns the Pokédex ID for a given name (case-insensitive).
func getPokemonID(name string) (int, error) {
	pokemonID, exists := pokemonNameCache[strings.ToLower(name)]
	if !exists {
		return 0, fmt.Errorf("pokémon not found: %s", name)
	}
	return pokemonID, nil
}

// getPokemonName returns the localised name for the given Pokédex ID.
func getPokemonName(pokemonID int, language string) string {
	if pokemonData, exists := gameData.Pokemon[strconv.Itoa(pokemonID)]; exists {
		return getTranslation(pokemonData.Name, language)
	}
	return getTranslation("Unknown", language)
}

// getMoveName returns the localised name for the given move ID.
func getMoveName(moveID int, language string) string {
	if moveData, exists := gameData.Moves[strconv.Itoa(moveID)]; exists {
		return getTranslation(moveData.Name, language)
	}
	return getTranslation("Unknown", language)
}

// getTranslation returns the translation for key in the requested language.
// For English it is a no-op; for other languages it falls back to the key if
// the translation is missing.
func getTranslation(key string, language string) string {
	if language == "en" {
		return key
	}
	if languageMap, exists := translations[language]; exists {
		if translatedText, exists := languageMap[key]; exists {
			return translatedText
		}
		log.Printf("❌ Translation key not found: %s", key)
	} else {
		log.Printf("❌ Translation language not found: %s", language)
	}
	return key
}
