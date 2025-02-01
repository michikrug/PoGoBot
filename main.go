package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
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
	ID        int64   `gorm:"primaryKey"`
	Notify    bool    `gorm:"default:true"`
	Language  string  `gorm:"default:'de'"`
	Stickers  bool    `gorm:"default:true"`
	Cleanup   bool    `gorm:"default:true"`
	Latitude  float32 `gorm:"default:0"`
	Longitude float32 `gorm:"default:0"`
	Distance  int     `gorm:"default:0"`
	HundoIV   bool    `gorm:"default:false"`
	ZeroIV    bool    `gorm:"default:false"`
	MinIV     int     `gorm:"default:0"`
	MinLevel  int     `gorm:"default:0"`
}

type FilteredUsers struct {
	AllUsers     map[int64]User
	HundoIVUsers []User
	ZeroIVUsers  []User
}

type Subscription struct {
	ID        int    `gorm:"primaryKey"`
	UserID    int64  `gorm:"index"`
	PokemonID int    `gorm:"index"`
	Filters   string `gorm:"type:json"` // {"min_iv": 0.0, "min_level": 1, "max_distance": 100}
}

type FilteredSubscriptions struct {
	AllSubscriptions    map[int][]Subscription
	ActiveSubscriptions map[int][]Subscription
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
	dbConfig              *gorm.DB // Stores user subscriptions
	dbEncounters          *gorm.DB // Fetches Pokémon encounters
	userStates            map[int64]string
	filteredUsers         FilteredUsers
	filteredSubscriptions FilteredSubscriptions
	notifiedEncounters    map[int64]map[string]struct{}
	pokemonNameToID       map[string]int
	pokemonIDToName       map[string]map[string]string
	moveIDToName          map[string]map[string]string
	timezone              *time.Location // Local timezone
	gender                = map[int]string{
		1: "\u2642", // Male
		2: "\u2640", // Female
		3: "\u26b2", // Genderless
	}
)

func (Pokemon) TableName() string {
	return "pokemon"
}

// Haversine formula to calculate the distance between two points on the Earth
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

// Load Move mappings from JSON file
func loadMoveMappings(lang string, filename string) error {
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
	moveIDToName[lang] = rawMap

	log.Printf("✅ Loaded %d Move mappings for language: %s", len(moveIDToName[lang]), lang)
	return nil
}

func loadAllLanguages() {
	pokemonNameToID = make(map[string]int)
	pokemonIDToName = make(map[string]map[string]string)
	moveIDToName = make(map[string]map[string]string)

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

	languages = map[string]string{
		"en": "moves_en.json",
		"de": "moves_de.json",
	}

	for lang, file := range languages {
		err := loadMoveMappings(lang, file)
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

// Ensure consistency in user preferences
func getUserPreferences(userID int64) User {
	var user User
	dbConfig.FirstOrCreate(&user, User{ID: userID})
	return user
}

func updateUserPreference(userID int64, field string, value interface{}) {
	dbConfig.Model(&User{}).Where("id = ?", userID).Update(field, value)
	getUsersByFilters()
}

// Subscribe User
func addSubscription(userID int64, pokemonID int, minIV int, minLevel int, maxDistance int) {
	// Encode filters as JSON
	filters := fmt.Sprintf(`{"min_iv": %d, "min_level": %d, "max_distance": %d}`, minIV, minLevel, maxDistance)

	newSub := Subscription{UserID: userID, PokemonID: pokemonID, Filters: filters}
	var existingSub Subscription
	if err := dbConfig.Where("user_id = ? AND pokemon_id = ?", userID, pokemonID).First(&existingSub).Error; err != nil {
		// If subscription does not exist, create a new one
		dbConfig.Create(&newSub)
	} else {
		// If subscription exists, update the filters
		existingSub.Filters = filters
		dbConfig.Save(&existingSub)
	}
	getSubscriptionsByFilters()
}

func getUsersByFilters() {
	filteredUsers = FilteredUsers{
		AllUsers:     make(map[int64]User),
		HundoIVUsers: []User{},
		ZeroIVUsers:  []User{},
	}

	var users []User
	dbConfig.Find(&users)
	for _, user := range users {
		filteredUsers.AllUsers[user.ID] = user
	}

	for _, user := range filteredUsers.AllUsers {
		if user.Notify {
			if user.HundoIV {
				filteredUsers.HundoIVUsers = append(filteredUsers.HundoIVUsers, user)
			}
			if user.ZeroIV {
				filteredUsers.ZeroIVUsers = append(filteredUsers.ZeroIVUsers, user)
			}

		}
	}
}

func getSubscriptionsByFilters() {
	filteredSubscriptions = FilteredSubscriptions{
		AllSubscriptions:    make(map[int][]Subscription),
		ActiveSubscriptions: make(map[int][]Subscription),
	}

	var subscriptions []Subscription
	dbConfig.Find(&subscriptions)
	for _, subscription := range subscriptions {
		filteredSubscriptions.AllSubscriptions[subscription.PokemonID] = append(filteredSubscriptions.AllSubscriptions[subscription.PokemonID], subscription)
		if filteredUsers.AllUsers[subscription.UserID].Notify {
			filteredSubscriptions.ActiveSubscriptions[subscription.PokemonID] = append(filteredSubscriptions.ActiveSubscriptions[subscription.PokemonID], subscription)
		}
	}
}

func sendSticker(bot *telebot.Bot, UserID int64, URL string, Expiration int) {
	message, err := bot.Send(&telebot.User{ID: UserID}, &telebot.Sticker{File: telebot.FromURL(URL)}, &telebot.SendOptions{DisableNotification: true})
	if err != nil {
		log.Printf("❌ Failed to send sticker: %v", err)
	} else {
		// Store message ID for cleanup
		dbConfig.Create(&Message{MessageID: strconv.Itoa(message.ID), ChatID: UserID, Expiration: Expiration})
	}
}

func sendLocation(bot *telebot.Bot, UserID int64, Lat float32, Lon float32, Expiration int) {
	message, err := bot.Send(&telebot.User{ID: UserID}, &telebot.Location{Lat: Lat, Lng: Lon}, &telebot.SendOptions{DisableNotification: true})
	if err != nil {
		log.Printf("❌ Failed to send location: %v", err)
	} else {
		// Store message ID for cleanup
		dbConfig.Create(&Message{MessageID: strconv.Itoa(message.ID), ChatID: UserID, Expiration: Expiration})
	}
}

func sendMessage(bot *telebot.Bot, UserID int64, Text string, Expiration int) {
	message, err := bot.Send(&telebot.User{ID: UserID}, Text, telebot.ModeMarkdown)
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
	} else {
		// Store message ID for cleanup
		dbConfig.Create(&Message{MessageID: strconv.Itoa(message.ID), ChatID: UserID, Expiration: Expiration})
	}
}

func sendEncounterNotification(bot *telebot.Bot, user User, encounter Pokemon) {
	if _, alreadySent := notifiedEncounters[user.ID][encounter.Id]; alreadySent {
		return
	}

	genderSymbol := gender[*encounter.Gender]

	var url = fmt.Sprintf("https://raw.githubusercontent.com/WatWowMap/wwm-uicons-webp/main/pokemon/%d.webp", encounter.PokemonId)

	if user.Stickers {
		sendSticker(bot, user.ID, url, *encounter.ExpireTimestamp)
	}
	sendLocation(bot, user.ID, encounter.Lat, encounter.Lon, *encounter.ExpireTimestamp)

	expireTime := time.Unix(int64(*encounter.ExpireTimestamp), 0).In(timezone)
	timeLeft := time.Until(expireTime)

	var notificationText strings.Builder
	notificationText.WriteString(fmt.Sprintf("*🔔 %s %s %.1f%% %d|%d|%d %dCP L%d*\n",
		pokemonIDToName[user.Language][strconv.Itoa(encounter.PokemonId)],
		genderSymbol,
		*encounter.IV,
		*encounter.AtkIV,
		*encounter.DefIV,
		*encounter.StaIV,
		*encounter.Cp,
		*encounter.Level,
	))

	if user.Latitude != 0 && user.Longitude != 0 {
		distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
		if distance < 1000 {
			notificationText.WriteString(fmt.Sprintf("📍 %.0fm\n", distance))
		} else {
			notificationText.WriteString(fmt.Sprintf("📍 %.2fkm\n", distance/1000))
		}
	}

	notificationText.WriteString(fmt.Sprintf("💨 %s ⏳ %s\n",
		expireTime.Format(time.TimeOnly),
		timeLeft.Truncate(time.Second).String()))

	notificationText.WriteString(fmt.Sprintf("💥 %s / %s",
		moveIDToName[user.Language][strconv.Itoa(*encounter.Move1)],
		moveIDToName[user.Language][strconv.Itoa(*encounter.Move2)]))

	sendMessage(bot, user.ID, notificationText.String(), *encounter.ExpireTimestamp)
	notifiedEncounters[user.ID][encounter.Id] = struct{}{}
}

func setupBotHandlers(bot *telebot.Bot) {

	// /subscribe <pokemon_name> [min_iv]
	bot.Handle("/subscribe", func(c telebot.Context) error {
		args := c.Args()
		if len(args) < 1 {
			return c.Reply("Usage: /subscribe <pokemon_name> [min-iv] [min-level] [max-distance]")
		}

		pokemonName := args[0]
		pokemonID, err := getPokemonID(pokemonName)
		if err != nil {
			return c.Reply(fmt.Sprintf("Can't find Pokedex # for Pokémon: %s", pokemonName))
		}

		minIV := int(0)
		minLevel := int(0)
		maxDistance := int(0)
		if len(args) > 1 {
			minIV, err = strconv.Atoi(args[1])
			if err != nil {
				return c.Reply("❌ Invalid IV input! Please enter a valid IV percentage (0-100).")
			}
		}
		if len(args) > 2 {
			minLevel, err = strconv.Atoi(args[2])
			if err != nil {
				return c.Reply("❌ Invalid level input! Please enter a valid level (0-40).")
			}
		}
		if len(args) > 3 {
			maxDistance, err = strconv.Atoi(args[3])
			if err != nil {
				return c.Reply("❌ Invalid distance input! Please enter a valid distance in m.")
			}
		}

		userID := c.Sender().ID
		addSubscription(userID, pokemonID, minIV, minLevel, maxDistance)

		user := getUserPreferences(userID)
		return c.Reply(fmt.Sprintf("Subscribed to %s alerts (Min IV: %d%%, Min Level: %d, Max Distance: %dm)",
			pokemonIDToName[user.Language][strconv.Itoa(pokemonID)],
			minIV, minLevel, maxDistance,
		))
	})

	// /list
	bot.Handle("/list", func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)

		var subs []Subscription
		dbConfig.Where("user_id = ?", user.ID).Find(&subs)

		if len(subs) == 0 {
			return c.Reply("You have no subscriptions.")
		}

		var text strings.Builder
		text.WriteString("📋 *Your Subscriptions:*\n\n")
		for _, sub := range subs {
			var filters map[string]int
			json.Unmarshal([]byte(sub.Filters), &filters)
			text.WriteString(fmt.Sprintf("🔹 %s (Min IV: %d%%, Min Level: %d, Max Distance: %dm)\n",
				pokemonIDToName[user.Language][strconv.Itoa(sub.PokemonID)],
				filters["min_iv"], filters["min_level"], filters["max_distance"],
			))
		}
		return c.Reply(text.String(), telebot.ModeMarkdown)
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

		getSubscriptionsByFilters()

		user := getUserPreferences(userID)

		return c.Reply(fmt.Sprintf("Unsubscribed from %s alerts", pokemonIDToName[user.Language][strconv.Itoa(pokemonID)]))
	})

	bot.Handle(telebot.OnLocation, func(c telebot.Context) error {
		userID := c.Sender().ID
		location := c.Message().Location

		updateUserPreference(userID, "Latitude", location.Lat)
		updateUserPreference(userID, "Longitude", location.Lng)

		return c.Reply("📍 Location updated! Your preferences will now consider this.")
	})

	bot.Handle("/start", func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)

		lang := c.Sender().LanguageCode // Auto-detect Telegram locale
		if lang != "en" && lang != "de" {
			lang = "en"
		}
		updateUserPreference(user.ID, "Language", lang)

		// Create a location request button
		btnShareLocation := telebot.ReplyButton{
			Text:     "📍 Send Location",
			Location: true, // This makes Telegram prompt the user to share their location
		}

		// Welcome message
		startMessage := fmt.Sprintf(
			"👋 Welcome to the Pokémon Notification Bot!\n\n"+
				"🔹 Language (for Pokémon and Moves) detected: *%s*\n"+
				"🔹 Send me your 📍 *location* to enable area-based notifications.\n"+
				"✅ Use /settings to update your preferences.\n"+
				"✅ Use /subscribe <pokemon_name> [min-iv] [min-level] [max-distance] to get notified about specific Pokémon!",
			lang,
		)

		return c.Reply(startMessage, &telebot.ReplyMarkup{
			ReplyKeyboard:  [][]telebot.ReplyButton{{btnShareLocation}},
			ResizeKeyboard: true, // Makes the keyboard smaller
		})
	})

	bot.Handle("/settings", func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)

		// Create interactive buttons
		btnToggleNotifications := telebot.InlineButton{Text: "🔔 Toggle Notifications", Unique: "toggle_notifications"}
		btnChangeLanguage := telebot.InlineButton{Text: "🌍 Change Language (for Pokémon and Moves)", Unique: "change_lang"}
		btnUpdateLocation := telebot.InlineButton{Text: "📍 Update Location", Unique: "update_location"}
		btnSetDistance := telebot.InlineButton{Text: "📏 Set Max Distance", Unique: "set_distance"}
		btnSetMinIV := telebot.InlineButton{Text: "✨ Set Min IV", Unique: "set_min_iv"}
		btnSetMinLevel := telebot.InlineButton{Text: "🔢 Set Min Level", Unique: "set_min_level"}
		btnToggleStickers := telebot.InlineButton{Text: "🎭 Toggle Pokémon Stickers", Unique: "toggle_stickers"}
		btnToogleHundoIV := telebot.InlineButton{Text: "💯 Toggle 100% IV Notifications", Unique: "toggle_hundo_iv"}
		btnToogleZeroIV := telebot.InlineButton{Text: "🚫 Toggle 0% IV Notifications", Unique: "toggle_zero_iv"}
		btnToggleCleanup := telebot.InlineButton{Text: "🗑️ Toggle Cleanup Expired Notifications", Unique: "toggle_cleanup"}

		// Settings message
		settingsMessage := fmt.Sprintf(
			"⚙️ *Your Settings:*\n"+
				"----------------------------------------------\n"+
				"🔔 *Notifications:* %t\n"+
				"🌍 *Language (for Pokémon and Moves):* %s\n"+
				"📍 *Location:* %.5f, %.5f\n"+
				"📏 *Max Distance:* %dm\n"+
				"✨ *Min IV:* %d%%\n"+
				"🔢 *Min Level:* %d\n"+
				"🎭 *Pokémon Stickers:* %t\n"+
				"💯 *100%% IV Notifications:* %t\n"+
				"🚫 *0%% IV Notifications:* %t\n"+
				"🗑️ *Cleanup Expired Notifications:* %t\n\n"+
				"Use the buttons below to update your settings.",
			user.Notify, user.Language, user.Latitude, user.Longitude, user.Distance,
			user.MinIV, user.MinLevel, user.Stickers, user.HundoIV, user.ZeroIV, user.Cleanup,
		)

		return c.Reply(settingsMessage, &telebot.ReplyMarkup{
			InlineKeyboard: [][]telebot.InlineButton{
				{btnToggleNotifications},
				{btnChangeLanguage},
				{btnUpdateLocation},
				{btnSetDistance},
				{btnSetMinIV},
				{btnSetMinLevel},
				{btnToggleStickers},
				{btnToogleHundoIV},
				{btnToogleZeroIV},
				{btnToggleCleanup},
			},
		}, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_notifications"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		updateUserPreference(user.ID, "Notify", !user.Notify)
		if !user.Notify {
			return c.Reply("🔕 Notifications disabled! Use /settings to re-enable.")
		}
		return c.Reply("🔔 Notifications enabled!")
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_stickers"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		updateUserPreference(user.ID, "Stickers", !user.Stickers)
		if !user.Stickers {
			return c.Reply("🎭 Pokémon Sstickers disabled! Use /settings to re-enable.")
		}
		return c.Reply("🎭 Pokémon Stickers enabled!")
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_hundo_iv"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		updateUserPreference(user.ID, "HundoIV", !user.HundoIV)
		if !user.HundoIV {
			return c.Reply("💯 100% IV Notifications disabled! Use /settings to re-enable.")
		}
		return c.Reply("💯 100% IV Notifications enabled!")
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_zero_iv"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		updateUserPreference(user.ID, "ZeroIV", !user.ZeroIV)
		if !user.ZeroIV {
			return c.Reply("🚫 0% IV Notifications disabled! Use /settings to re-enable.")
		}
		return c.Reply("🚫 0% IV Notifications enabled!")
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_cleanup"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		updateUserPreference(user.ID, "Cleanup", !user.Cleanup)
		if !user.Cleanup {
			return c.Reply("🗑️ Cleanup Expired Notifications disabled! Use /settings to re-enable.")
		}
		return c.Reply("🗑️ Cleanup Expired Notifications enabled!")
	})

	bot.Handle(&telebot.InlineButton{Unique: "change_lang"}, func(c telebot.Context) error {
		// Create language selection buttons
		btnEn := telebot.InlineButton{Text: "🇬🇧 English", Unique: "set_lang_en"}
		btnDe := telebot.InlineButton{Text: "🇩🇪 Deutsch", Unique: "set_lang_de"}
		return c.Edit("🌍 *Select a language:*", &telebot.ReplyMarkup{
			InlineKeyboard: [][]telebot.InlineButton{{btnEn, btnDe}},
		}, telebot.ModeMarkdown)
	})

	// Handle setting language
	bot.Handle(&telebot.InlineButton{Unique: "set_lang_en"}, func(c telebot.Context) error {
		updateUserPreference(c.Sender().ID, "Language", "en")
		return c.Edit("✅ Language (for Pokémon and Moves) set to *English*", telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_lang_de"}, func(c telebot.Context) error {
		updateUserPreference(c.Sender().ID, "Language", "de")
		return c.Edit("✅ Language (for Pokémon and Moves) set to *Deutsch*", telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "update_location"}, func(c telebot.Context) error {
		// Prompt user to send location
		btnShareLocation := telebot.ReplyButton{
			Text:     "📍 Send Location",
			Location: true,
		}
		return c.Reply("📍 Please send your current location:", &telebot.ReplyMarkup{
			ReplyKeyboard:  [][]telebot.ReplyButton{{btnShareLocation}},
			ResizeKeyboard: true,
		})
	})

	// Handle receiving location
	bot.Handle(telebot.OnLocation, func(c telebot.Context) error {
		location := c.Message().Location
		// Update user location in the database
		updateUserPreference(c.Sender().ID, "Latitude", location.Lat)
		updateUserPreference(c.Sender().ID, "Longitude", location.Lng)
		return c.Reply("✅ Location updated!")
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_distance"}, func(c telebot.Context) error {
		userStates[c.Sender().ID] = "set_distance"
		return c.Reply("📏 Enter your preferred max distance (in m):")
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_min_iv"}, func(c telebot.Context) error {
		userStates[c.Sender().ID] = "set_min_iv"
		return c.Reply("✨ Enter the minimum IV percentage (0-100):")
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_min_level"}, func(c telebot.Context) error {
		userStates[c.Sender().ID] = "set_min_level"
		return c.Reply("🔢 Enter the minimum Pokémon level (1-40):")
	})

	// Handle text input for max distance
	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		userID := c.Sender().ID
		if userStates[userID] == "set_distance" {
			var maxDistance int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &maxDistance)
			if err != nil || maxDistance <= 0 {
				return c.Reply("❌ Invalid input! Please enter a valid distance in m.")
			}

			// Update max distance in the database
			updateUserPreference(userID, "Distance", maxDistance)

			userStates[userID] = ""

			return c.Reply(fmt.Sprintf("✅ Max distance updated to %dm!", maxDistance))
		}
		if userStates[userID] == "set_min_iv" {
			var minIV int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &minIV)
			if err != nil || minIV < 0 || minIV > 100 {
				return c.Reply("❌ Invalid input! Please enter a valid IV percentage (0-100).")
			}

			// Update min IV in the database
			updateUserPreference(userID, "MinIV", minIV)

			userStates[userID] = ""

			return c.Reply(fmt.Sprintf("✅ Minimum IV updated to %d%%!", minIV))
		}
		if userStates[userID] == "set_min_level" {
			var minLevel int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &minLevel)
			if err != nil || minLevel < 0 || minLevel > 40 {
				return c.Reply("❌ Invalid input! Please enter a valid level (0-40).")
			}

			// Update min IV in the database
			updateUserPreference(userID, "MinLevel", minLevel)

			userStates[userID] = ""

			return c.Reply(fmt.Sprintf("✅ Minimum Level updated to %d!", minLevel))
		}
		return nil
	})
}

func processEncounters(bot *telebot.Bot) {
	var lastCheck = time.Now().Unix() - 30
	// Fetch current Pokémon encounters
	var encounters []Pokemon
	if err := dbEncounters.Where("iv IS NOT NULL").Where("updated > ?", lastCheck).Where("expire_timestamp > ?", lastCheck).Find(&encounters).Error; err != nil {
		log.Printf("❌ Failed to fetch Pokémon encounters: %v", err)
	} else {
		log.Printf("✅ Found %d Pokémon", len(encounters))

		// Match encounters with subscriptions
		for _, encounter := range encounters {
			// Check for 100% IV Pokémon
			if encounter.IV != nil && *encounter.IV == 100 {
				for _, user := range filteredUsers.HundoIVUsers {
					if user.Latitude != 0 && user.Longitude != 0 && user.Distance > 0 {
						distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
						if distance > float64(user.Distance) {
							continue
						}
					}
					sendEncounterNotification(bot, user, encounter)
				}
			}
			// Check for 0% IV Pokémon
			if encounter.IV != nil && *encounter.IV == 0 {
				for _, user := range filteredUsers.ZeroIVUsers {
					if user.Latitude != 0 && user.Longitude != 0 && user.Distance > 0 {
						distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
						if distance > float64(user.Distance) {
							continue
						}
					}
					sendEncounterNotification(bot, user, encounter)
				}
			}
			// Check for subscribed Pokémon
			if subs, exists := filteredSubscriptions.ActiveSubscriptions[encounter.PokemonId]; exists {
				for _, sub := range subs {
					var filters map[string]int
					json.Unmarshal([]byte(sub.Filters), &filters)
					user := filteredUsers.AllUsers[sub.UserID]

					if filters["min_iv"] > 0 && *encounter.IV < float32(filters["min_iv"]) {
						continue
					}
					if user.MinIV > 0 && *encounter.IV < float32(user.MinIV) {
						continue
					}
					if filters["min_level"] > 0 && *encounter.Level < filters["min_level"] {
						continue
					}
					if user.MinLevel > 0 && *encounter.Level < user.MinLevel {
						continue
					}
					if user.Latitude != 0 && user.Longitude != 0 && (user.Distance > 0 || filters["max_distance"] > 0) {
						distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
						if distance > float64(filters["max_distance"]) && distance > float64(user.Distance) {
							continue
						}
					}
					sendEncounterNotification(bot, user, encounter)
				}
			}
		}
	}
}

func cleanupMessages(bot *telebot.Bot) {
	// Cleanup expired messages
	var messages []Message
	if err := dbConfig.Where("expiration < ?", time.Now().Unix()).Find(&messages).Error; err != nil {
		log.Printf("❌ Failed to fetch expired messages: %v", err)
	} else {
		log.Printf("🗑️ Found %d expired messages", len(messages))
		for _, message := range messages {
			user := filteredUsers.AllUsers[message.ChatID]
			if user.Cleanup {
				bot.Delete(&telebot.StoredMessage{MessageID: message.MessageID, ChatID: message.ChatID})
			}
			dbConfig.Delete(&message)
		}
	}
}

func startBackgroundProcessing(bot *telebot.Bot) {
	// Background process to match encounters with subscriptions
	go func() {
		for {
			time.Sleep(30 * time.Second)
			cleanupMessages(bot)
			processEncounters(bot)
		}
	}()
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
	getUsersByFilters()

	// Load subscriptions into a map
	getSubscriptionsByFilters()

	userStates = make(map[int64]string)
	notifiedEncounters = make(map[int64]map[string]struct{})

	var err error
	if timezone, err = time.LoadLocation("Local"); err != nil {
		log.Printf("❌ Failed to load local timezone: %v", err)
		timezone = time.UTC
	}

	telegramBotToken := os.Getenv("BOT_TOKEN")
	pref := telebot.Settings{
		Token:  telegramBotToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		log.Fatal(err)
	}

	setupBotHandlers(bot)

	startBackgroundProcessing(bot)

	bot.Start()
}
