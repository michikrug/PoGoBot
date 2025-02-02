package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	OnlyMap   bool    `gorm:"default:false"`
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
	notificationsCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bot_notifications_total",
			Help: "Total number of notifications sent.",
		},
	)
	encounterGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_encounters_total",
			Help: "Total number of Pokémon encounters retrieved.",
		},
	)
	cleanupGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_cleanup_total",
			Help: "Total number of expired notifications cleaned up.",
		},
	)
)

func (Pokemon) TableName() string {
	return "pokemon"
}

func boolToEmoji(value bool) string {
	if value {
		return "✅"
	}
	return "❌"
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
		notifiedEncounters[user.ID] = make(map[string]struct{})
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
	notificationsCounter.Inc()
}

func buildSettings(user User) (string, *telebot.ReplyMarkup) {
	// Create interactive buttons
	btnChangeLanguage := telebot.InlineButton{Text: "🌍 Change Language (Pokémon & Moves)", Unique: "change_lang"}
	btnUpdateLocation := telebot.InlineButton{Text: "📍 Update Location", Unique: "update_location"}
	btnSetDistance := telebot.InlineButton{Text: "📏 Set Max Distance", Unique: "set_distance"}
	btnSetMinIV := telebot.InlineButton{Text: "✨ Set Min IV", Unique: "set_min_iv"}
	btnSetMinLevel := telebot.InlineButton{Text: "🔢 Set Min Level", Unique: "set_min_level"}
	btnAddSubscription := telebot.InlineButton{Text: "📣 Add Pokémon Alert", Unique: "add_subscription"}
	btnListSubscriptions := telebot.InlineButton{Text: "📋 List Pokémon Alerts", Unique: "list_subscriptions"}
	btnClearSubscriptions := telebot.InlineButton{Text: "🗑️ Clear Pokémon Alerts", Unique: "clear_subscriptions"}
	notificationsText := "🔔 Disable all Notifications"
	if !user.Notify {
		notificationsText = "🔕 Enable all Notifications"
	}
	btnToggleNotifications := telebot.InlineButton{Text: notificationsText, Unique: "toggle_notifications"}
	stickersText := "🎭 Do not show Pokémon Stickers"
	if !user.Stickers {
		stickersText = "🎭 Show Pokémon Stickers"
	}
	btnToggleStickers := telebot.InlineButton{Text: stickersText, Unique: "toggle_stickers"}
	hundoText := "💯 Disable 100% IV Notifications"
	if !user.HundoIV {
		hundoText = "💯 Enable 100% IV Notifications"
	}
	btnToogleHundoIV := telebot.InlineButton{Text: hundoText, Unique: "toggle_hundo_iv"}
	zeroText := "🚫 Disable 0% IV Notifications"
	if !user.ZeroIV {
		zeroText = "🚫 Enable 0% IV Notifications"
	}
	btnToogleZeroIV := telebot.InlineButton{Text: zeroText, Unique: "toggle_zero_iv"}
	cleanupText := "🗑️ Keep Expired Notifications"
	if !user.Cleanup {
		cleanupText = "🗑️ Remove Expired Notifications"
	}
	btnToggleCleanup := telebot.InlineButton{Text: cleanupText, Unique: "toggle_cleanup"}
	btnClose := telebot.InlineButton{Text: "Close", Unique: "close"}

	// Settings message
	settingsMessage := fmt.Sprintf(
		"⚙️ *Your Settings:*\n"+
			"----------------------------------------------\n"+
			"🌍 *Language (Pokémon & Moves):* %s\n"+
			"📍 *Location:* %.5f, %.5f\n"+
			"📏 *Max Distance:* %dm\n"+
			"✨ *Min IV:* %d%%\n"+
			"🔢 *Min Level:* %d\n"+
			"🔔 *Notifications:* %s\n"+
			"🎭 *Pokémon Stickers:* %s\n"+
			"💯 *100%% IV Notifications:* %s\n"+
			"🚫 *0%% IV Notifications:* %s\n"+
			"🗑️ *Cleanup Expired Notifications:* %s\n\n"+
			"Use the buttons below to update your settings.",
		user.Language, user.Latitude, user.Longitude, user.Distance, user.MinIV, user.MinLevel,
		boolToEmoji(user.Notify), boolToEmoji(user.Stickers), boolToEmoji(user.HundoIV), boolToEmoji(user.ZeroIV), boolToEmoji(user.Cleanup),
	)

	return settingsMessage, &telebot.ReplyMarkup{
		InlineKeyboard: [][]telebot.InlineButton{
			{btnChangeLanguage},
			{btnUpdateLocation},
			{btnSetDistance},
			{btnSetMinIV},
			{btnSetMinLevel},
			{btnAddSubscription},
			{btnListSubscriptions},
			{btnClearSubscriptions},
			{btnToggleNotifications},
			{btnToggleStickers},
			{btnToogleHundoIV},
			{btnToogleZeroIV},
			{btnToggleCleanup},
			{btnClose},
		},
	}
}

func setupBotHandlers(bot *telebot.Bot) {

	// /subscribe <pokemon_name> [min_iv]
	bot.Handle("/subscribe", func(c telebot.Context) error {
		args := c.Args()
		if len(args) < 1 {
			return c.Send("Usage: /subscribe <pokemon_name> [min-iv] [min-level] [max-distance]")
		}

		pokemonName := args[0]
		pokemonID, err := getPokemonID(pokemonName)
		if err != nil {
			return c.Send(fmt.Sprintf("Can't find Pokedex # for Pokémon: %s", pokemonName))
		}

		minIV := int(0)
		minLevel := int(0)
		maxDistance := int(0)
		if len(args) > 1 {
			minIV, err = strconv.Atoi(args[1])
			if err != nil {
				return c.Send("❌ Invalid IV input! Please enter a valid IV percentage (0-100).")
			}
		}
		if len(args) > 2 {
			minLevel, err = strconv.Atoi(args[2])
			if err != nil {
				return c.Send("❌ Invalid level input! Please enter a valid level (0-40).")
			}
		}
		if len(args) > 3 {
			maxDistance, err = strconv.Atoi(args[3])
			if err != nil {
				return c.Send("❌ Invalid distance input! Please enter a valid distance in m.")
			}
		}

		userID := c.Sender().ID
		addSubscription(userID, pokemonID, minIV, minLevel, maxDistance)

		user := getUserPreferences(userID)
		return c.Send(fmt.Sprintf("Subscribed to %s alerts (Min IV: %d%%, Min Level: %d, Max Distance: %dm)",
			pokemonIDToName[user.Language][strconv.Itoa(pokemonID)],
			minIV, minLevel, maxDistance,
		))
	})

	// /list
	bot.Handle("/list", func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)

		var subs []Subscription
		dbConfig.Where("user_id = ?", user.ID).Order("pokemon_id").Find(&subs)

		var text strings.Builder
		text.WriteString("📋 *Your Pokémon Alerts:*\n\n")
		if user.HundoIV {
			text.WriteString(fmt.Sprintf("🔹 *All* (Min IV: 100%, Min Level: 0, Max Distance: %dm)\n", user.Distance))
		}
		if user.ZeroIV {
			text.WriteString(fmt.Sprintf("🔹 *All* (Max IV: 0%, Min Level: 0, Max Distance: %dm)\n", user.Distance))
		}

		if len(subs) == 0 {
			text.WriteString("🔹 You have no specific Pokémon alerts")
		}

		for _, sub := range subs {
			var filters map[string]int
			json.Unmarshal([]byte(sub.Filters), &filters)
			text.WriteString(fmt.Sprintf("🔹 %s (Min IV: %d%%, Min Level: %d, Max Distance: %dm)\n",
				pokemonIDToName[user.Language][strconv.Itoa(sub.PokemonID)],
				filters["min_iv"], filters["min_level"], filters["max_distance"],
			))
		}
		return c.Send(text.String(), telebot.ModeMarkdown)
	})

	// /unsubscribe <pokemon_name>
	bot.Handle("/unsubscribe", func(c telebot.Context) error {
		args := c.Args()
		if len(args) < 1 {
			return c.Send("Usage: /unsubscribe <pokemon_name>")
		}

		pokemonName := args[0]
		pokemonID, err := getPokemonID(pokemonName)
		if err != nil {
			return c.Send(fmt.Sprintf("Can't find Pokedex # for Pokémon: %s", pokemonName))
		}

		userID := c.Sender().ID
		dbConfig.Where("user_id = ? AND pokemon_id = ?", userID, pokemonID).Delete(&Subscription{})

		getSubscriptionsByFilters()

		user := getUserPreferences(userID)

		return c.Send(fmt.Sprintf("Unsubscribed from %s alerts", pokemonIDToName[user.Language][strconv.Itoa(pokemonID)]))
	})

	bot.Handle(telebot.OnLocation, func(c telebot.Context) error {
		userID := c.Sender().ID
		location := c.Message().Location

		updateUserPreference(userID, "Latitude", location.Lat)
		updateUserPreference(userID, "Longitude", location.Lng)

		return c.Send("📍 Location updated! Your preferences will now consider this.")
	})

	bot.Handle("/start", func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)

		lang := c.Sender().LanguageCode // Auto-detect Telegram locale
		if lang != "en" && lang != "de" {
			lang = "en"
		}
		updateUserPreference(user.ID, "Language", lang)

		// Welcome message
		startMessage := fmt.Sprintf(
			"👋 Welcome to the PoGo Notification Bot!\n\n"+
				"🔹 Language (Pokémon & Moves) detected: *%s*\n"+
				"🔹 Send me your 📍 *location* to enable distance-based notifications.\n"+
				"✅ Use /settings to update your preferences.\n"+
				"✅ Use /subscribe <pokemon_name> [min-iv] [min-level] [max-distance] to get notified about specific Pokémon!",
			lang,
		)

		return c.Send(startMessage, telebot.ModeMarkdown)
	})

	bot.Handle("/settings", func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Send(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "close"}, func(c telebot.Context) error {
		return c.Edit("✅ Settings closed.")
	})

	bot.Handle(&telebot.InlineButton{Unique: "add_subscription"}, func(c telebot.Context) error {
		userStates[c.Sender().ID] = "add_subscription"
		return c.Edit("📣 Enter the Pokémon name you want to subscribe to:")
	})

	bot.Handle(&telebot.InlineButton{Unique: "list_subscriptions"}, func(c telebot.Context) error {
		return c.Edit("/list")
	})

	bot.Handle(&telebot.InlineButton{Unique: "clear_subscriptions"}, func(c telebot.Context) error {
		userID := c.Sender().ID
		dbConfig.Where("user_id = ?", userID).Delete(&Subscription{})
		getSubscriptionsByFilters()
		return c.Edit("🗑️ All Pokémon alerts cleared!")
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_notifications"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		user.Notify = !user.Notify
		updateUserPreference(user.ID, "Notify", user.Notify)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_stickers"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		user.Stickers = !user.Stickers
		updateUserPreference(user.ID, "Stickers", user.Stickers)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_hundo_iv"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		user.HundoIV = !user.HundoIV
		updateUserPreference(user.ID, "HundoIV", user.HundoIV)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_zero_iv"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		user.ZeroIV = !user.ZeroIV
		updateUserPreference(user.ID, "ZeroIV", user.ZeroIV)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_cleanup"}, func(c telebot.Context) error {
		user := getUserPreferences(c.Sender().ID)
		user.Cleanup = !user.Cleanup
		updateUserPreference(user.ID, "Cleanup", user.Cleanup)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
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
		return c.Edit("✅ Language (Pokémon & Moves) set to *English*", telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_lang_de"}, func(c telebot.Context) error {
		updateUserPreference(c.Sender().ID, "Language", "de")
		return c.Edit("✅ Language (Pokémon & Moves) set to *Deutsch*", telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "update_location"}, func(c telebot.Context) error {
		// Prompt user to send location
		btnShareLocation := telebot.ReplyButton{
			Text:     "📍 Send Location",
			Location: true,
		}
		return c.Edit("📍 Please send your current location:", &telebot.ReplyMarkup{
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
		return c.Edit("✅ Location updated!")
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_distance"}, func(c telebot.Context) error {
		userStates[c.Sender().ID] = "set_distance"
		return c.Edit("📏 Enter your preferred max distance (in m):")
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_min_iv"}, func(c telebot.Context) error {
		userStates[c.Sender().ID] = "set_min_iv"
		return c.Edit("✨ Enter the minimum IV percentage (0-100):")
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_min_level"}, func(c telebot.Context) error {
		userStates[c.Sender().ID] = "set_min_level"
		return c.Edit("🔢 Enter the minimum Pokémon level (1-40):")
	})

	// Handle text input for max distance
	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		userID := c.Sender().ID
		if userStates[userID] == "add_subscription" {
			pokemonName := c.Text()
			pokemonID, err := getPokemonID(pokemonName)
			if err != nil {
				return c.Send(fmt.Sprintf("❌ Can't find Pokedex # for Pokémon: %s", pokemonName))
			}
			userStates[userID] = fmt.Sprintf("add_subscription_iv_%d", pokemonID)
			return c.Send(fmt.Sprintf("📣 Subscribing to %s alerts. Please enter the minimum IV percentage (0-100):", pokemonIDToName[filteredUsers.AllUsers[userID].Language][strconv.Itoa(pokemonID)]))
		}
		if strings.HasPrefix(userStates[userID], "add_subscription_iv") {
			pokemonID, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[3])
			var minIV int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &minIV)
			if err != nil || minIV < 0 || minIV > 100 {
				return c.Send("❌ Invalid input! Please enter a valid IV percentage (0-100).")
			}
			userStates[userID] = fmt.Sprintf("add_subscription_level_%d_%d", pokemonID, minIV)
			return c.Send(fmt.Sprintf("✨ Minimum IV set to %d%%. Please enter the minimum Pokémon level (0-40):", minIV))
		}
		if userStates[userID] == "add_subscription_level" {
			pokemonID, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[3])
			minIV, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[4])
			var minLevel int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &minLevel)
			if err != nil || minLevel < 0 || minLevel > 40 {
				return c.Send("❌ Invalid input! Please enter a valid level (0-40).")
			}
			userStates[userID] = fmt.Sprintf("add_subscription_distance_%d_%d_%d", pokemonID, minIV, minLevel)
			return c.Send(fmt.Sprintf("🔢 Minimum level set to %d. Please enter the maximum distance (in m):", minLevel))
		}
		if userStates[userID] == "add_subscription_distance" {
			pokemonID, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[3])
			minIV, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[4])
			minLevel, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[5])
			var maxDistance int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &maxDistance)
			if err != nil || maxDistance <= 0 {
				return c.Send("❌ Invalid input! Please enter a valid distance in m.")
			}

			// Subscribe user to Pokémon
			addSubscription(userID, pokemonID, minIV, minLevel, maxDistance)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf("✅ Subscribed to %s alerts (Min IV: %d%%, Min Level: %d, Max Distance: %dm)",
				pokemonIDToName[filteredUsers.AllUsers[userID].Language][strconv.Itoa(pokemonID)],
				minIV, minLevel, maxDistance,
			))
		}
		if userStates[userID] == "set_distance" {
			var maxDistance int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &maxDistance)
			if err != nil || maxDistance <= 0 {
				return c.Send("❌ Invalid input! Please enter a valid distance in m.")
			}

			// Update max distance in the database
			updateUserPreference(userID, "Distance", maxDistance)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf("✅ Max distance updated to %dm!", maxDistance))
		}
		if userStates[userID] == "set_min_iv" {
			var minIV int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &minIV)
			if err != nil || minIV < 0 || minIV > 100 {
				return c.Send("❌ Invalid input! Please enter a valid IV percentage (0-100).")
			}

			// Update min IV in the database
			updateUserPreference(userID, "MinIV", minIV)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf("✅ Minimum IV updated to %d%%!", minIV))
		}
		if userStates[userID] == "set_min_level" {
			var minLevel int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &minLevel)
			if err != nil || minLevel < 0 || minLevel > 40 {
				return c.Send("❌ Invalid input! Please enter a valid level (0-40).")
			}

			// Update min IV in the database
			updateUserPreference(userID, "MinLevel", minLevel)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf("✅ Minimum Level updated to %d!", minLevel))
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
		encounterGauge.Set(float64(len(encounters)))
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
		cleanupGauge.Set(float64(len(messages)))
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

	// Start Prometheus metrics server
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Println("🚀 Prometheus metrics available at /metrics")
		log.Fatal(http.ListenAndServe(":9001", nil))
	}()

	userStates = make(map[int64]string)
	notifiedEncounters = make(map[int64]map[string]struct{})

	// Load Pokémon mappings
	loadAllLanguages()

	// Initialize databases
	initDB()

	// Load users into a map
	getUsersByFilters()

	// Load subscriptions into a map
	getSubscriptionsByFilters()

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
