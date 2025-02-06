package migration

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// Legacy JSON Structure
type LegacyUser struct {
	Disabled  bool               `json:"disabled"`
	Location  []float32          `json:"location"`
	Language  string             `json:"language"`
	Stickers  bool               `json:"stickers"`
	Cleanup   bool               `json:"cleanup"`
	MapOnly   bool               `json:"maponly"`
	Perfect   bool               `json:"perfect"`
	IV        int                `json:"iv"`
	Level     int                `json:"level"`
	Pokemon   []int              `json:"pkmids"`
	PkmIV     map[string]float32 `json:"pkmiv"`
	PkmLevel  map[string]float32 `json:"pkmlevel"`
	PkmRadius map[string]float32 `json:"pkmradius"`
}

// Generate SQL for migration
func generateSQL(jsonFile string, userID int64) {
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		log.Fatalf("❌ Failed to read file: %v", err)
	}

	var legacyUser LegacyUser
	err = json.Unmarshal(data, &legacyUser)
	if err != nil {
		log.Fatalf("❌ Failed to parse JSON: %v", err)
	}
	if legacyUser.Disabled {
		log.Printf("❌ Skipping disabled user %d from %s", userID, jsonFile)
		return
	}
	if len(legacyUser.Pokemon) == 0 && !legacyUser.Perfect {
		log.Printf("❌ Skipping user %d with no subscriptions from %s", userID, jsonFile)
		return
	}

	// Open file for writing SQL
	sqlFile, err := os.OpenFile("migration.sql", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("❌ Failed to open SQL file: %v", err)
	}
	defer sqlFile.Close()

	// Generate SQL for inserting user
	userSQL := fmt.Sprintf(
		"INSERT INTO users (id, notify, cleanup, language, min_iv, min_level, latitude, longitude, max_distance, hundo_iv, stickers, only_map) "+
			"VALUES (%d, %t, %t, '%s', %d, %d, %.10f, %.6f, %d, %t, %t, %t) "+
			"ON DUPLICATE KEY UPDATE notify=VALUES(notify), cleanup=VALUES(cleanup), language=VALUES(language), "+
			"min_iv=VALUES(min_iv), min_level=VALUES(min_level), latitude=VALUES(latitude), longitude=VALUES(longitude), "+
			"max_distance=VALUES(max_distance), hundo_iv=VALUES(hundo_iv), stickers=VALUES(stickers), only_map=VALUES(only_map);\n",
		userID, !legacyUser.Disabled, legacyUser.Cleanup, legacyUser.Language, legacyUser.IV, legacyUser.Level,
		legacyUser.Location[0], legacyUser.Location[1], int(legacyUser.Location[2]*1000),
		legacyUser.Perfect, legacyUser.Stickers, legacyUser.MapOnly,
	)

	if _, err := sqlFile.WriteString(userSQL); err != nil {
		log.Fatalf("❌ Failed to write user SQL: %v", err)
	}

	// Generate SQL for inserting subscriptions
	for _, pokemonID := range legacyUser.Pokemon {
		subSQL := fmt.Sprintf(
			"INSERT INTO subscriptions (user_id, pokemon_id, min_iv, min_level, max_distance) VALUES (%d, %d, %d, %d, %d) ON DUPLICATE KEY UPDATE min_iv=VALUES(min_iv), min_level=VALUES(min_level), max_distance=VALUES(max_distance);\n",
			userID, pokemonID, int(getOrDefault(legacyUser.PkmIV, fmt.Sprint(pokemonID), 0)), int(getOrDefault(legacyUser.PkmLevel, fmt.Sprint(pokemonID), 0)), int(getOrDefault(legacyUser.PkmRadius, fmt.Sprint(pokemonID), 0)*1000),
		)

		if _, err := sqlFile.WriteString(subSQL); err != nil {
			log.Fatalf("❌ Failed to write subscription SQL: %v", err)
		}
	}

	log.Printf("✅ Generated SQL for user %d from %s", userID, jsonFile)
}

func getOrDefault(m map[string]float32, key string, defaultValue float32) float32 {
	if val, ok := m[key]; ok {
		return val
	}
	return defaultValue
}

func main() {
	if len(os.Args) < 1 {
		log.Fatalf("❌ Usage: %s <json_file>", os.Args[0])
	}

	if len(os.Args) > 1 {
		jsonFile := os.Args[1]
		userID, err := strconv.ParseInt(strings.Split(strings.Split(jsonFile, "/")[1], ".")[0], 10, 64)
		if err != nil {
			log.Fatalf("❌ Invalid user ID: %v", err)
		}
		generateSQL(jsonFile, userID)
	} else {
		entries, err := os.ReadDir("userdata")
		if err != nil {
			log.Fatalf("❌ Failed to read directory: %v", err)
		}

		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				jsonFile := "userdata/" + entry.Name()
				userID, err := strconv.ParseInt(strings.Split(strings.Split(jsonFile, "/")[1], ".")[0], 10, 64)
				if err != nil {
					log.Fatalf("❌ Invalid user ID: %v", err)
				}
				generateSQL(jsonFile, userID)
			}
		}
	}
}
