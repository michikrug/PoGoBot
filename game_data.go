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

// Translator provides localised string helpers bound to a specific language.
// Create one per handler via newTranslator(language) and use T / Tf instead of
// calling getTranslation(key, language) everywhere.
type Translator struct{ lang string }

// newTranslator returns a Translator for the given language code.
func newTranslator(language string) Translator {
	return Translator{lang: language}
}

// T returns the translation for key.
func (tr Translator) T(key string) string {
	return getTranslation(key, tr.lang)
}

// Tf translates key and formats it with the supplied arguments, combining the
// common fmt.Sprintf(getTranslation(key, lang), args...) pattern.
func (tr Translator) Tf(key string, args ...any) string {
	return fmt.Sprintf(tr.T(key), args...)
}

// PokemonName returns the localised Pokémon name for the given Pokédex ID.
func (tr Translator) PokemonName(pokemonID int) string {
	if pokemonData, exists := gameData.Pokemon[strconv.Itoa(pokemonID)]; exists {
		return tr.T(pokemonData.Name)
	}
	return tr.T("Unknown")
}

// MoveName returns the localised move name for the given move ID.
func (tr Translator) MoveName(moveID int) string {
	if moveData, exists := gameData.Moves[strconv.Itoa(moveID)]; exists {
		return tr.T(moveData.Name)
	}
	return tr.T("Unknown")
}

// RaidLevelName returns the localised raid-level label for the given level
// (e.g. "Legendärer Raid" for level 5 in German, "Legendary Raid" in English).
// Falls back to "L<n>" for any level without a translation.
func (tr Translator) RaidLevelName(level int) string {
	key := fmt.Sprintf("raid_%d", level)
	translated := tr.T(key)
	if translated == key {
		// No translation found — produce a generic numeric label.
		return fmt.Sprintf("L%d", level)
	}
	return translated
}

// TeamName returns the localised team name for the given team ID (0=Unset,
// 1=Mystic, 2=Valor, 3=Instinct). Falls back to "Team <n>" for unknown IDs.
func (tr Translator) TeamName(teamID int) string {
	key := fmt.Sprintf("team_%d", teamID)
	translated := tr.T(key)
	if translated == key {
		// No translation found — produce a generic numeric label.
		return fmt.Sprintf("Team %d", teamID)
	}
	return translated
}

// AlignmentName returns the localised alignment name for the given alignment ID
// (0=Unset, 1=Shadow, 2=Purified). Falls back to "Alignment <n>" for unknown IDs.
func (tr Translator) AlignmentName(alignmentID int) string {
	key := fmt.Sprintf("alignment_%d", alignmentID)
	translated := tr.T(key)
	if translated == key {
		// No translation found — produce a generic numeric label.
		return fmt.Sprintf("Alignment %d", alignmentID)
	}
	return translated
}

// getTranslation returns the translation for key in the requested language.
// For English it first checks the "en" section of translations (which holds
// canonical brand names for team_<id> and raid_<id> keys), then falls back to
// returning the key itself so that regular bot-string keys work without needing
// an explicit English entry.  For other languages it falls back to the key if
// the translation is missing.
func getTranslation(key string, language string) string {
	if languageMap, exists := translations[language]; exists {
		if translatedText, exists := languageMap[key]; exists {
			return translatedText
		}
	}
	if language == "en" {
		// For English, the key is the display string for all regular bot strings.
		return key
	}
	log.Printf("❌ Translation key not found: %s (lang=%s)", key, language)
	return key
}
