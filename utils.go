package main

import (
	"log"
	"math"
	"os"
)

// boolToEmoji converts a boolean value to an emoji representation
func boolToEmoji(value bool) string {
	if value {
		return "✅"
	}
	return "❌"
}

// haversine calculates the distance between two points on Earth using the Haversine formula
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371e3 // Earth radius in meters
	phi1 := lat1 * (math.Pi / 180)
	phi2 := lat2 * (math.Pi / 180)
	deltaPhi := (lat2 - lat1) * (math.Pi / 180)
	deltaLambda := (lon2 - lon1) * (math.Pi / 180)

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// checkEnvVars ensures all required environment variables are set
func checkEnvVars(vars []string) {
	for _, v := range vars {
		if os.Getenv(v) == "" {
			log.Fatalf("❌ Missing required environment variable: %s", v)
		}
	}
}

// withinDistance checks if the encounter is within the user's allowed distance
// Returns true if the check passes or if no distance filtering is set
func withinDistance(user User, encounter EncounterData, maxDistance int) bool {
	if user.Latitude == 0 || user.Longitude == 0 || maxDistance == 0 {
		return true
	}
	distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
	return distance <= float64(maxDistance)
}

// Gender and weather emoji mappings
var (
	genderMap = map[int]string{
		1: "\u2642", // Male
		2: "\u2640", // Female
		3: "\u26b2", // Genderless
	}
	weatherMap = map[int]string{
		0: "",
		1: "☀️",
		2: "☔️",
		3: "⛅",
		4: "☁️",
		5: "💨",
		6: "⛄️",
		7: "🌁",
	}
)
