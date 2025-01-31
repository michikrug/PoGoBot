package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/telebot.v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Models
type User struct {
	ID         int64  `gorm:"primaryKey"`
	Notify     bool   `gorm:"default:true"`
	Language   string `gorm:"default:'de'"`
	Stickers   bool   `gorm:"default:true"`
	Cleanup    bool   `gorm:"default:true"`
	Latitude   float32
	Longtitude float32
	Distance   float32
	HundoIV    bool    `gorm:"default:false"`
	ZeroIV     bool    `gorm:"default:false"`
	MinIV      float32 `gorm:"default:0"`
	MinLevel   int     `gorm:"default:0"`
}

type Subscription struct {
	ID        int    `gorm:"primaryKey"`
	UserID    int64  `gorm:"index"`
	PokemonID int    `gorm:"index"`
	Filters   string `gorm:"type:json"` // {"min_iv": 0.0, "min_level": 1, "distance": 100}
}

type Message struct {
	ID         int `gorm:"primaryKey"`
	MessageID  string
	ChatID     int64
	Expiration int `gorm:"index"`
}

type Pokemon struct {
	Id                      string `gorm:"primaryKey"`
	PokestopID              *string
	SpawnID                 *int64
	Lat                     float32
	Lon                     float32
	Weight                  *float32
	Size                    *int
	Height                  *float32
	ExpireTimestamp         *int
	Updated                 *int
	PokemonId               int
	Move1                   *int `gorm:"column:move_1"`
	Move2                   *int `gorm:"column:move_2"`
	Gender                  *int
	Cp                      *int
	AtkIV                   *int
	DefIV                   *int
	StaIV                   *int
	GolbatInternal          []byte
	Form                    *int
	Level                   *int
	IsStrong                *bool
	Weather                 *int
	Costume                 *int
	FirstSeenTimestamp      int
	Changed                 int
	CellId                  *int64
	ExpireTimestampVerified bool
	DisplayPokemonId        *int
	IsDitto                 bool
	SeenType                *string
	Shiny                   *bool
	Username                *string
	Capture1                *float32 `gorm:"column:capture_1"`
	Capture2                *float32 `gorm:"column:capture_2"`
	Capture3                *float32 `gorm:"column:capture_2"`
	Pvp                     *string
	IsEvent                 int
	IV                      *float32
}

var (
	dbConfig         *gorm.DB // Stores user subscriptions
	dbEncounters     *gorm.DB // Fetches Pokémon encounters
	allUsers         map[int64]User
	allSubscriptions map[int][]Subscription
	pokemonNameToID  map[string]int
	pokemonIDToName  map[string]map[string]string // Language → (Name → ID)
)

func (Pokemon) TableName() string {
	return "pokemon"
}

// Check if all required environment variables are set
func checkEnvVars(vars []string) {
	for _, v := range vars {
		if os.Getenv(v) == "" {
			log.Fatalf("❌ Missing required environment variable: %s", v)
		}
	}
}

// Initialize Database
func initDB() {
	// Load environment variables
	configDBUser := os.Getenv("BOT_DB_USER")
	configDBPass := os.Getenv("BOT_DB_PASS")
	configDBName := os.Getenv("BOT_DB_NAME")
	configDBHost := os.Getenv("BOT_DB_HOST")

	encounterDBUser := os.Getenv("ENCOUNTER_DB_USER")
	encounterDBPass := os.Getenv("ENCOUNTER_DB_PASS")
	encounterDBName := os.Getenv("ENCOUNTER_DB_NAME")
	encounterDBHost := os.Getenv("ENCOUNTER_DB_HOST")

	// Bot-specific database (for user subscriptions)
	configDSN := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", configDBUser, configDBPass, configDBHost, configDBName)
	var err error
	dbConfig, err = gorm.Open(mysql.Open(configDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to bot database: %v", err)
	}
	log.Println("✅ Connected to bot database")

	dbConfig.AutoMigrate(&User{}, &Subscription{}, &Message{})

	// Existing Pokémon encounter database
	encounterDSN := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", encounterDBUser, encounterDBPass, encounterDBHost, encounterDBName)
	dbEncounters, err = gorm.Open(mysql.Open(encounterDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to encounter database: %v", err)
	}
	log.Println("✅ Connected to encounter database")
}

// Load Pokémon mappings from JSON file
func loadPokemonMappings(lang string, filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	// Temporary map for original JSON structure
	var rawMap map[string]string
	err = json.Unmarshal(data, &rawMap)
	if err != nil {
		return err
	}
	pokemonIDToName[lang] = rawMap

	// Convert from ID → Name to Name → ID
	for idStr, name := range rawMap {
		var id int
		fmt.Sscanf(idStr, "%d", &id) // Convert string key to integer
		pokemonNameToID[strings.ToLower(name)] = id
	}

	log.Printf("✅ Loaded %d Pokémon mappings for language: %s", len(pokemonIDToName[lang]), lang)
	return nil
}

func loadAllLanguages() {
	pokemonNameToID = make(map[string]int)
	pokemonIDToName = make(map[string]map[string]string)

	languages := map[string]string{
		"en": "pokemon_en.json",
		"de": "pokemon_de.json",
	}

	for lang, file := range languages {
		err := loadPokemonMappings(lang, file)
		if err != nil {
			log.Fatalf("❌ Failed to load %s: %v", file, err)
		}
	}
}

// Convert Pokémon name to ID
func getPokemonID(name string) (int, error) {
	pokemonID, exists := pokemonNameToID[strings.ToLower(name)]
	if !exists {
		return 0, fmt.Errorf("pokémon not found: %s", name)
	}
	return pokemonID, nil
}

// Subscribe User
func addSubscription(userID int64, pokemonID int, minIV float32) {
	var user User
	dbConfig.FirstOrCreate(&user, User{ID: userID})

	// Encode filters as JSON
	filters := fmt.Sprintf(`{"min_iv": %.2f}`, minIV)

	newSub := Subscription{UserID: user.ID, PokemonID: pokemonID, Filters: filters}
	var existingSub Subscription
	if err := dbConfig.Where("user_id = ? AND pokemon_id = ?", userID, pokemonID).First(&existingSub).Error; err != nil {
		// If subscription does not exist, create a new one
		dbConfig.Create(&newSub)
	} else {
		// If subscription exists, update the filters
		existingSub.Filters = filters
		dbConfig.Save(&existingSub)
	}
}

// Get Subscriptions
func getSubscriptions() {
	var subs []Subscription
	dbConfig.Find(&subs)

	allSubscriptions = make(map[int][]Subscription)
	for _, sub := range subs {
		allSubscriptions[sub.PokemonID] = append(allSubscriptions[sub.PokemonID], sub)
	}
}

// Get Users
func getUsers() {
	var users []User
	dbConfig.Find(&users)

	allUsers = make(map[int64]User)
	for _, user := range users {
		allUsers[user.ID] = user
	}
}

func sendSticker(bot *telebot.Bot, UserID int64, URL string, Expiration int) {
	message, err := bot.Send(&telebot.User{ID: UserID}, &telebot.Sticker{File: telebot.FromURL(URL)})
	if err != nil {
		log.Printf("❌ Failed to send sticker: %v", err)
	} else {
		// Store message ID for cleanup
		dbConfig.Create(&Message{MessageID: strconv.Itoa(message.ID), ChatID: UserID, Expiration: Expiration})
	}
}

func sendLocation(bot *telebot.Bot, UserID int64, Lat float32, Lon float32, Expiration int) {
	message, err := bot.Send(&telebot.User{ID: UserID}, &telebot.Location{Lat: Lat, Lng: Lon})
	if err != nil {
		log.Printf("❌ Failed to send location: %v", err)
	} else {
		// Store message ID for cleanup
		dbConfig.Create(&Message{MessageID: strconv.Itoa(message.ID), ChatID: UserID, Expiration: Expiration})
	}
}

func sendNotification(bot *telebot.Bot, UserID int64, Text string, Expiration int) {
	message, err := bot.Send(&telebot.User{ID: UserID}, Text, telebot.ModeMarkdown)
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
	} else {
		// Store message ID for cleanup
		dbConfig.Create(&Message{MessageID: strconv.Itoa(message.ID), ChatID: UserID, Expiration: Expiration})
	}
}

func sendEncounterNotification(bot *telebot.Bot, user User, encounter Pokemon) {
	pokemonName := pokemonIDToName[user.Language][strconv.Itoa(encounter.PokemonId)]
	gender := "\u2642"
	if *encounter.Gender == 2 {
		gender = "\u2640"
	} else if *encounter.Gender == 3 {
		gender = "\u26b2"
	}

	var url = fmt.Sprintf("https://raw.githubusercontent.com/WatWowMap/wwm-uicons-webp/main/pokemon/%d.webp", encounter.PokemonId)

	sendSticker(bot, user.ID, url, *encounter.ExpireTimestamp)
	sendLocation(bot, user.ID, encounter.Lat, encounter.Lon, *encounter.ExpireTimestamp)
	sendNotification(bot, user.ID, fmt.Sprintf("*🔔 %s %s %.1f%% (%d | %d | %d) 📍 %f, %fm*\n💨 %s ⏳ %s\n⚔ %d / %d",
		pokemonName,
		gender,
		*encounter.IV,
		*encounter.AtkIV,
		*encounter.DefIV,
		*encounter.StaIV,
		encounter.Lat,
		encounter.Lon,
		time.Unix(int64(*encounter.ExpireTimestamp), 0).Format(time.RFC822),
		time.Unix(int64(*encounter.Updated), 0).Format(time.RFC822),
		*encounter.Move1,
		*encounter.Move2,
	), *encounter.ExpireTimestamp)
}

func FilterUsersWithHundoIV(users map[int64]User) []User {
	var filteredUsers []User
	for _, user := range users {
		if user.HundoIV {
			filteredUsers = append(filteredUsers, user)
		}
	}
	return filteredUsers
}

func FilterUsersWithZeroIV(users map[int64]User) []User {
	var filteredUsers []User
	for _, user := range users {
		if user.ZeroIV {
			filteredUsers = append(filteredUsers, user)
		}
	}
	return filteredUsers
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found, using system environment variables")
	}
	// Required environment variables
	requiredVars := []string{
		"BOT_TOKEN", "BOT_DB_USER", "BOT_DB_PASS", "BOT_DB_NAME", "BOT_DB_HOST",
		"ENCOUNTER_DB_USER", "ENCOUNTER_DB_PASS", "ENCOUNTER_DB_NAME", "ENCOUNTER_DB_HOST",
	}
	checkEnvVars(requiredVars)

	// Load Pokémon mappings
	loadAllLanguages()

	// Initialize databases
	initDB()

	// Load users into a map
	getUsers()

	// Load subscriptions into a map
	getSubscriptions()

	telegramBotToken := os.Getenv("BOT_TOKEN")
	pref := telebot.Settings{
		Token:  telegramBotToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		log.Fatal(err)
	}

	// /subscribe <pokemon_name> [min_iv]
	bot.Handle("/subscribe", func(c telebot.Context) error {
		args := c.Args()
		if len(args) < 1 {
			return c.Reply("Usage: /subscribe <pokemon_name> [min_iv]")
		}

		pokemonName := args[0]
		pokemonID, err := getPokemonID(pokemonName)
		if err != nil {
			return c.Reply(fmt.Sprintf("Can't find Pokedex # for Pokémon: %s", pokemonName))
		}

		minIV := float32(0)
		if len(args) > 1 {
			fmt.Sscanf(args[1], "%f", &minIV)
		}

		addSubscription(c.Sender().ID, pokemonID, minIV)

		getUsers()
		getSubscriptions()

		var user User
		dbConfig.First(&user, c.Sender().ID)
		return c.Reply(fmt.Sprintf("Subscribed to %s (Min IV: %.2f)", pokemonIDToName[user.Language][strconv.Itoa(pokemonID)], minIV))
	})

	// /list
	bot.Handle("/list", func(c telebot.Context) error {
		userID := c.Sender().ID
		var subs []Subscription
		dbConfig.Where("user_id = ?", userID).Find(&subs)

		if len(subs) == 0 {
			return c.Reply("You have no subscriptions.")
		}

		var text strings.Builder
		for _, sub := range subs {
			var filters map[string]float32
			json.Unmarshal([]byte(sub.Filters), &filters)
			text.WriteString(fmt.Sprintf("Subscribed to %s (Min IV: %.2f)\n", pokemonIDToName[allUsers[userID].Language][strconv.Itoa(sub.PokemonID)], filters["min_iv"]))
		}
		return c.Reply(text.String())
	})

	// /unsubscribe <pokemon_name>
	bot.Handle("/unsubscribe", func(c telebot.Context) error {
		args := c.Args()
		if len(args) < 1 {
			return c.Reply("Usage: /unsubscribe <pokemon_name>")
		}

		pokemonName := args[0]
		pokemonID, err := getPokemonID(pokemonName)
		if err != nil {
			return c.Reply(fmt.Sprintf("Can't find Pokedex # for Pokémon: %s", pokemonName))
		}

		userID := c.Sender().ID
		dbConfig.Where("user_id = ? AND pokemon_id = ?", userID, pokemonID).Delete(&Subscription{})

		var user User
		dbConfig.FirstOrCreate(&user, User{ID: userID})

		getUsers()
		getSubscriptions()

		return c.Reply(fmt.Sprintf("Unsubscribed from %s alerts", pokemonIDToName[user.Language][strconv.Itoa(pokemonID)]))
	})

	bot.Handle("/language", func(c telebot.Context) error {
		// userLang := c.Sender().LanguageCode
		args := c.Args()
		if len(args) < 1 {
			return c.Reply("Usage: /language <en|de>")
		}

		lang := args[0]
		if lang != "en" && lang != "de" {
			return c.Reply("Supported languages: en, de")
		}

		userID := c.Sender().ID
		dbConfig.Save(&User{ID: userID, Language: lang})

		getUsers()

		return c.Reply(fmt.Sprintf("✅ Language set to %s", lang))
	})

	// Background process to match encounters with subscriptions
	go func() {
		for {
			var now = time.Now().Unix()
			time.Sleep(30 * time.Second)

			// Cleanup expired messages
			var messages []Message
			if err := dbConfig.Where("expiration < ?", time.Now().Unix()).Find(&messages).Error; err != nil {
				log.Printf("❌ Failed to fetch expired messages: %v", err)
			} else {
				log.Printf("🗑️ Found %d expired messages", len(messages))
				for _, message := range messages {
					user := allUsers[message.ChatID]
					if user.Cleanup {
						bot.Delete(&telebot.StoredMessage{MessageID: message.MessageID, ChatID: message.ChatID})
					}
					dbConfig.Delete(&message)
				}
			}

			// Fetch current Pokémon encounters
			var encounters []Pokemon
			if err := dbEncounters.Where("iv IS NOT NULL").Where("updated > ?", now).Where("expire_timestamp > ?", now).Find(&encounters).Error; err != nil {
				log.Printf("❌ Failed to fetch Pokémon encounters: %v", err)
			} else {
				log.Printf("✅ Found %d Pokémon", len(encounters))

				// Match encounters with subscriptions
				for _, encounter := range encounters {
					// Check for 100% IV Pokémon
					if encounter.IV != nil && *encounter.IV == 100 {
						filteredUsers := FilterUsersWithHundoIV(allUsers)
						for _, user := range filteredUsers {
							sendEncounterNotification(bot, user, encounter)
						}
					}
					// Check for 0% IV Pokémon
					if encounter.IV != nil && *encounter.IV == 0 {
						filteredUsers := FilterUsersWithZeroIV(allUsers)
						for _, user := range filteredUsers {
							sendEncounterNotification(bot, user, encounter)
						}
					}
					// Check for subscribed Pokémon
					if subs, exists := allSubscriptions[encounter.PokemonId]; exists {
						for _, sub := range subs {
							var filters map[string]float32
							json.Unmarshal([]byte(sub.Filters), &filters)

							if encounter.IV != nil && *encounter.IV >= filters["min_iv"] {
								sendEncounterNotification(bot, allUsers[sub.UserID], encounter)
							}
						}
					}
				}
			}
		}
	}()

	bot.Start()
}
