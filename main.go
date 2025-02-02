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
	ID          int64   `gorm:"primaryKey;autoIncrement:false"`
	Notify      bool    `gorm:"not null;default:true"`
	Language    string  `gorm:"not null;default:'de';type:varchar(5)"`
	Stickers    bool    `gorm:"not null;default:true"`
	OnlyMap     bool    `gorm:"not null;default:false"`
	Cleanup     bool    `gorm:"not null;default:true"`
	Latitude    float32 `gorm:"not null;default:0;type:double(14,10)"`
	Longitude   float32 `gorm:"not null;default:0;type:double(14,10)"`
	MaxDistance int     `gorm:"not null;default:0;type:mediumint(6)"`
	HundoIV     bool    `gorm:"not null;default:false"`
	ZeroIV      bool    `gorm:"not null;default:false"`
	MinIV       int     `gorm:"not null;default:0;type:tinyint(3)"`
	MinLevel    int     `gorm:"not null;default:0;type:tinyint(2)"`
}

type FilteredUsers struct {
	AllUsers     map[int64]User
	HundoIVUsers []User
	ZeroIVUsers  []User
	ChannelUser  []User
}

type Subscription struct {
	UserID      int64 `gorm:"primaryKey;autoIncrement:false"`
	PokemonID   int   `gorm:"primaryKey;autoIncrement:false;type=smallint(5)"`
	MinIV       int   `gorm:"not null;default:0;type:tinyint(3)"`
	MinLevel    int   `gorm:"not null;default:0;type:tinyint(2)"`
	MaxDistance int   `gorm:"not null;default:0;type:mediumint(6)"`
}

type Encounter struct {
	ID         string `gorm:"primaryKey;autoIncrement:false;type:varchar(25)"`
	Expiration int    `gorm:"index;not null;type:int(10)"`
}

type Message struct {
	ChatID      int64  `gorm:"primaryKey;autoIncrement:false"`
	MessageID   int    `gorm:"primaryKey;autoIncrement:false"`
	EncounterID string `gorm:"index;not null;type:varchar(25)"`
}

type Pokemon struct {
	ID                      string `gorm:"primaryKey"`
	PokestopID              *string
	SpawnID                 *int64
	Lat                     float32
	Lon                     float32
	Weight                  *float32
	Size                    *int
	Height                  *float32
	ExpireTimestamp         *int
	Updated                 *int
	PokemonID               int
	Move1                   *int `gorm:"column:move_1"`
	Move2                   *int `gorm:"column:move_2"`
	Gender                  *int
	CP                      *int
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
	CellID                  *int64
	ExpireTimestampVerified bool
	DisplayPokemonID        *int
	IsDitto                 bool
	SeenType                *string
	Shiny                   *bool
	Username                *string
	Capture1                *float32 `gorm:"column:capture_1"`
	Capture2                *float32 `gorm:"column:capture_2"`
	Capture3                *float32 `gorm:"column:capture_2"`
	PVP                     *string
	IsEvent                 int
	IV                      *float32
}

var (
	dbConfig            *gorm.DB // Stores user subscriptions
	dbScanner           *gorm.DB // Fetches Pokémon encounters
	botAdmins           map[int64]int64
	userStates          map[int64]string
	filteredUsers       FilteredUsers
	activeSubscriptions map[int][]Subscription
	sentNotifications   map[string]map[int64]struct{}
	pokemonNameToID     map[string]int
	pokemonIDToName     map[string]map[string]string
	moveIDToName        map[string]map[string]string
	timezone            *time.Location // Local timezone
	genderMap           = map[int]string{
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
	customRegistry       = prometheus.NewRegistry()
	notificationsCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bot_notifications_total",
			Help: "Total number of notifications sent",
		},
	)
	messagesCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bot_messages_total",
			Help: "Total number of messages sent",
		},
	)
	encounterGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_encounters_count",
			Help: "Total number of Pokémon encounters retrieved",
		},
	)
	cleanupGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_cleanup_count",
			Help: "Total number of expired notifications cleaned up",
		},
	)
	usersGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_users_count",
			Help: "Total number of users subscribed to notifications",
		},
	)
	subscriptionGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_subscription_count",
			Help: "Total number of Pokémon subscriptions",
		},
	)
	activeSubscriptionGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_subscription_active_count",
			Help: "Total number of active Pokémon subscriptions",
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

	scannerDBUser := os.Getenv("SCANNER_DB_USER")
	scannerDBPass := os.Getenv("SCANNER_DB_PASS")
	scannerDBName := os.Getenv("SCANNER_DB_NAME")
	scannerDBHost := os.Getenv("SCANNER_DB_HOST")

	// Bot-specific database (for user subscriptions)
	configDSN := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", configDBUser, configDBPass, configDBHost, configDBName)
	var err error
	dbConfig, err = gorm.Open(mysql.Open(configDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to bot database: %v", err)
	}
	log.Println("✅ Connected to bot database")

	dbConfig.AutoMigrate(&User{}, &Subscription{}, &Message{}, &Encounter{})

	// Existing Pokémon encounter database
	scannerDSN := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", scannerDBUser, scannerDBPass, scannerDBHost, scannerDBName)
	dbScanner, err = gorm.Open(mysql.Open(scannerDSN), &gorm.Config{})
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
	subscription := Subscription{UserID: userID, PokemonID: pokemonID, MinIV: minIV, MinLevel: minLevel, MaxDistance: maxDistance}
	dbConfig.Save(&subscription)
	getActiveSubscriptions()
}

func getUsersByFilters() {
	filteredUsers = FilteredUsers{
		AllUsers:     make(map[int64]User),
		HundoIVUsers: []User{},
		ZeroIVUsers:  []User{},
		ChannelUser:  []User{},
	}

	var users []User
	dbConfig.Find(&users)
	for _, user := range users {
		filteredUsers.AllUsers[user.ID] = user
	}
	usersGauge.Set(float64(len(filteredUsers.AllUsers)))
	log.Printf("📋 Loaded %d users", len(filteredUsers.AllUsers))

	for _, user := range filteredUsers.AllUsers {
		if user.Notify {
			if user.HundoIV {
				filteredUsers.HundoIVUsers = append(filteredUsers.HundoIVUsers, user)
			}
			if user.ZeroIV {
				filteredUsers.ZeroIVUsers = append(filteredUsers.ZeroIVUsers, user)
			}
			if strings.HasPrefix(strconv.FormatInt(user.ID, 10), "-100") {
				filteredUsers.ChannelUser = append(filteredUsers.ChannelUser, user)
			}
		}
	}
}

func getActiveSubscriptions() {
	activeSubscriptions := make(map[int][]Subscription)
	activeSubscriptionCount := 0
	var subscriptions []Subscription
	dbConfig.Find(&subscriptions)
	for _, subscription := range subscriptions {
		if filteredUsers.AllUsers[subscription.UserID].Notify {
			activeSubscriptionCount++
			activeSubscriptions[subscription.PokemonID] = append(activeSubscriptions[subscription.PokemonID], subscription)
		}
	}
	log.Printf("📋 Loaded %d active of %d subscriptions", activeSubscriptionCount, len(subscriptions))
	subscriptionGauge.Set(float64(len(subscriptions)))
	activeSubscriptionGauge.Set(float64(activeSubscriptionCount))
}

func sendSticker(bot *telebot.Bot, UserID int64, URL string, EncounterID string) {
	message, err := bot.Send(&telebot.User{ID: UserID}, &telebot.Sticker{File: telebot.FromURL(URL)}, &telebot.SendOptions{DisableNotification: true})
	if err != nil {
		log.Printf("❌ Failed to send sticker: %v", err)
	} else {
		messagesCounter.Inc()
		// Store message ID for cleanup
		dbConfig.Create(&Message{ChatID: UserID, MessageID: message.ID, EncounterID: EncounterID})
	}
}

func sendLocation(bot *telebot.Bot, UserID int64, Lat float32, Lon float32, EncounterID string) {
	message, err := bot.Send(&telebot.User{ID: UserID}, &telebot.Location{Lat: Lat, Lng: Lon}, &telebot.SendOptions{DisableNotification: true})
	if err != nil {
		log.Printf("❌ Failed to send location: %v", err)
	} else {
		messagesCounter.Inc()
		// Store message ID for cleanup
		dbConfig.Create(&Message{ChatID: UserID, MessageID: message.ID, EncounterID: EncounterID})
	}
}

func sendVenue(bot *telebot.Bot, UserID int64, Lat float32, Lon float32, Title string, Address string, EncounterID string) {
	message, err := bot.Send(&telebot.User{ID: UserID}, &telebot.Venue{Location: telebot.Location{Lat: Lat, Lng: Lon}, Title: Title, Address: Address})
	if err != nil {
		log.Printf("❌ Failed to send venue: %v", err)
	} else {
		messagesCounter.Inc()
		// Store message ID for cleanup
		dbConfig.Create(&Message{ChatID: UserID, MessageID: message.ID, EncounterID: EncounterID})
	}
}

func sendMessage(bot *telebot.Bot, UserID int64, Text string, EncounterID string) {
	message, err := bot.Send(&telebot.User{ID: UserID}, Text, telebot.ModeMarkdown)
	if err != nil {
		log.Printf("❌ Failed to send message: %v", err)
	} else {
		messagesCounter.Inc()
		// Store message ID for cleanup
		dbConfig.Create(&Message{ChatID: UserID, MessageID: message.ID, EncounterID: EncounterID})
	}
}

func sendEncounterNotification(bot *telebot.Bot, user User, encounter Pokemon) {
	// Check if encounter has already been notified
	if _, exists := sentNotifications[encounter.ID][user.ID]; exists {
		log.Printf("🔕 Skipping notification for Pokémon #%d to %d (already sent)", encounter.PokemonID, user.ID)
		return
	}
	log.Printf("🔔 Sending notification for Pokémon #%d to %d", encounter.PokemonID, user.ID)
	dbConfig.Save(&Encounter{ID: encounter.ID, Expiration: *encounter.ExpireTimestamp})
	if sentNotifications[encounter.ID] == nil {
		sentNotifications[encounter.ID] = make(map[int64]struct{})
	}
	sentNotifications[encounter.ID][user.ID] = struct{}{}
	notificationsCounter.Inc()

	if !user.OnlyMap && user.Stickers {
		url := fmt.Sprintf("https://raw.githubusercontent.com/WatWowMap/wwm-uicons-webp/main/pokemon/%d.webp", encounter.PokemonID)
		sendSticker(bot, user.ID, url, encounter.ID)
	}
	if !user.OnlyMap {
		sendLocation(bot, user.ID, encounter.Lat, encounter.Lon, encounter.ID)
	}

	expireTime := time.Unix(int64(*encounter.ExpireTimestamp), 0).In(timezone)
	timeLeft := time.Until(expireTime)

	notificationTitle := fmt.Sprintf("*🔔 %s %s %.1f%% %d|%d|%d %d%s L%d* %s",
		pokemonIDToName[user.Language][strconv.Itoa(encounter.PokemonID)],
		genderMap[*encounter.Gender],
		*encounter.IV,
		*encounter.AtkIV,
		*encounter.DefIV,
		*encounter.StaIV,
		*encounter.CP,
		func() string {
			if user.Language == "en" {
				return "CP"
			}
			return "WP"
		}(),
		*encounter.Level,
		weatherMap[*encounter.Weather],
	)

	var notificationText strings.Builder
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

	if !user.OnlyMap {
		sendMessage(bot, user.ID, notificationTitle+"\n"+notificationText.String(), encounter.ID)
	} else {
		sendVenue(bot, user.ID, encounter.Lat, encounter.Lon, notificationTitle, notificationText.String(), encounter.ID)
	}
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
			"Use the buttons below to update your settings",
		user.Language, user.Latitude, user.Longitude, user.MaxDistance, user.MinIV, user.MinLevel,
		boolToEmoji(user.Notify), boolToEmoji(user.Stickers), boolToEmoji(user.HundoIV), boolToEmoji(user.ZeroIV), boolToEmoji(user.Cleanup),
	)

	inlineKeyboard := [][]telebot.InlineButton{
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
	}

	if _, ok := botAdmins[user.ID]; ok {
		// Admin-only buttons
		btnBroadcast := telebot.InlineButton{Text: "📢 Broadcast Message", Unique: "broadcast"}
		btnListUsers := telebot.InlineButton{Text: "📋 List Users", Unique: "list_users"}
		btnImpersonateUser := telebot.InlineButton{Text: "👤 Impersonate User", Unique: "impersonate_user"}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnBroadcast, btnListUsers, btnImpersonateUser})
	}

	return settingsMessage, &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}
}

func getUserID(c telebot.Context) int64 {
	userID := c.Sender().ID
	if adminID, ok := botAdmins[userID]; ok && adminID != userID {
		c.Send("🔒 You are impersonating another user")
		return adminID
	}
	return userID
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
			if err != nil || minIV < 0 || minIV > 100 {
				return c.Send("❌ Invalid IV input! Please enter a valid IV percentage (0-100)")
			}
		}
		if len(args) > 2 {
			minLevel, err = strconv.Atoi(args[2])
			if err != nil || minLevel < 0 || minLevel > 40 {
				return c.Send("❌ Invalid level input! Please enter a valid level (0-40)")
			}
		}
		if len(args) > 3 {
			maxDistance, err = strconv.Atoi(args[3])
			if err != nil || maxDistance < 0 {
				return c.Send("❌ Invalid distance input! Please enter a valid distance in m")
			}
		}

		userID := getUserID(c)
		addSubscription(userID, pokemonID, minIV, minLevel, maxDistance)

		user := getUserPreferences(userID)
		return c.Send(fmt.Sprintf("Subscribed to %s alerts (Min IV: %d%%, Min Level: %d, Max Distance: %dm)",
			pokemonIDToName[user.Language][strconv.Itoa(pokemonID)],
			minIV, minLevel, maxDistance,
		))
	})

	// /list
	bot.Handle("/list", func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))

		var text strings.Builder
		text.WriteString("📋 *Your Pokémon Alerts:*\n\n")
		if user.HundoIV {
			text.WriteString(fmt.Sprintf("🔹 *All* (Min IV: 100%%, Min Level: 0, Max Distance: %dm)\n", user.MaxDistance))
		}
		if user.ZeroIV {
			text.WriteString(fmt.Sprintf("🔹 *All* (Max IV: 0%%, Min Level: 0, Max Distance: %dm)\n", user.MaxDistance))
		}
		c.Send(text.String(, telebot.ModeMarkdown)
		text.Reset()

		var subs []Subscription
		dbConfig.Where("user_id = ?", user.ID).Order("pokemon_id").Find(&subs)

		if len(subs) == 0 {
			return c.Send("🔹 You have no specific Pokémon alerts")
		}

		for _, sub := range subs {
			entry :=
				fmt.Sprintf("🔹 %s (Min IV: %d%%, Min Level: %d, Max Distance: %dm)\n",
					pokemonIDToName[user.Language][strconv.Itoa(sub.PokemonID)],
					sub.MinIV, sub.MinLevel, sub.MaxDistance,
				)
			if text.Len()+len(entry) > 4000 { // Telegram message limit is 4096 bytes
				c.Send(text.String())
				text.Reset()
			}
			text.WriteString(entry)
		}
		return c.Send(text.String())
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

		userID := getUserID(c)
		dbConfig.Where("user_id = ? AND pokemon_id = ?", userID, pokemonID).Delete(&Subscription{})

		getActiveSubscriptions()

		user := getUserPreferences(userID)

		return c.Send(fmt.Sprintf("Unsubscribed from %s alerts", pokemonIDToName[user.Language][strconv.Itoa(pokemonID)]))
	})

	bot.Handle(telebot.OnLocation, func(c telebot.Context) error {
		userID := getUserID(c)
		location := c.Message().Location

		updateUserPreference(userID, "Latitude", location.Lat)
		updateUserPreference(userID, "Longitude", location.Lng)

		return c.Send("📍 Location updated! Your preferences will now consider this")
	})

	bot.Handle("/start", func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))

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
				"✅ Use /settings to update your preferences\n"+
				"✅ Use /subscribe <pokemon_name> [min-iv] [min-level] [max-distance] to get notified about specific Pokémon",
			lang,
		)

		return c.Send(startMessage, telebot.ModeMarkdown)
	})

	bot.Handle("/settings", func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Send(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle("/help", func(c telebot.Context) error {
		helpMessage := "🤖 *PoGo Notification Bot Commands:*\n\n" +
			"🔔 /settings - Update your preferences\n" +
			"📋 /list - List your Pokémon alerts\n" +
			"📣 /subscribe <pokemon_name> [min-iv] [min-level] [max-distance] - Subscribe to Pokémon alerts\n" +
			"🚫 /unsubscribe <pokemon_name> - Unsubscribe from Pokémon alerts"
		return c.Send(helpMessage, telebot.ModeMarkdown)
	})

	bot.Handle("/reset", func(c telebot.Context) error {
		userID := c.Sender().ID
		if _, ok := botAdmins[userID]; !ok {
			return c.Edit("❌ You are not authorized to use this command")
		}
		if botAdmins[userID] == userID {
			return c.Send("🔒 You are not impersonating another user")
		}
		botAdmins[userID] = userID
		return c.Send("🔒 You are now back as yourself")
	})

	bot.Handle(&telebot.InlineButton{Unique: "close"}, func(c telebot.Context) error {
		return c.Edit("✅ Settings closed")
	})

	bot.Handle(&telebot.InlineButton{Unique: "add_subscription"}, func(c telebot.Context) error {
		userStates[c.Sender().ID] = "add_subscription"
		return c.Edit("📣 Enter the Pokémon name you want to subscribe to:")
	})

	bot.Handle(&telebot.InlineButton{Unique: "list_subscriptions"}, func(c telebot.Context) error {
		c.Delete()
		return bot.Trigger("/list", c)
	})

	bot.Handle(&telebot.InlineButton{Unique: "clear_subscriptions"}, func(c telebot.Context) error {
		userID := getUserID(c)
		dbConfig.Where("user_id = ?", userID).Delete(&Subscription{})
		getActiveSubscriptions()
		return c.Edit("🗑️ All Pokémon alerts cleared")
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_notifications"}, func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))
		user.Notify = !user.Notify
		updateUserPreference(user.ID, "Notify", user.Notify)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_stickers"}, func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))
		user.Stickers = !user.Stickers
		updateUserPreference(user.ID, "Stickers", user.Stickers)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_hundo_iv"}, func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))
		user.HundoIV = !user.HundoIV
		updateUserPreference(user.ID, "HundoIV", user.HundoIV)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_zero_iv"}, func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))
		user.ZeroIV = !user.ZeroIV
		updateUserPreference(user.ID, "ZeroIV", user.ZeroIV)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_cleanup"}, func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))
		user.Cleanup = !user.Cleanup
		updateUserPreference(user.ID, "Cleanup", user.Cleanup)
		settingsMessage, replyMarkup := buildSettings(user)
		return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "change_lang"}, func(c telebot.Context) error {
		btnEn := telebot.InlineButton{Text: "🇬🇧 English", Unique: "set_lang_en"}
		btnDe := telebot.InlineButton{Text: "🇩🇪 Deutsch", Unique: "set_lang_de"}
		return c.Edit("🌍 *Select a language:*", &telebot.ReplyMarkup{
			InlineKeyboard: [][]telebot.InlineButton{{btnEn, btnDe}},
		}, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_lang_en"}, func(c telebot.Context) error {
		updateUserPreference(getUserID(c), "Language", "en")
		return c.Edit("✅ Language (Pokémon & Moves) set to *English*", telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_lang_de"}, func(c telebot.Context) error {
		updateUserPreference(getUserID(c), "Language", "de")
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

	bot.Handle(&telebot.InlineButton{Unique: "broadcast"}, func(c telebot.Context) error {
		if _, ok := botAdmins[c.Sender().ID]; !ok {
			return c.Edit("❌ You are not authorized to use this command")
		}
		userStates[c.Sender().ID] = "broadcast"
		return c.Edit("📢 Enter the message you want to broadcast:")
	})

	bot.Handle(&telebot.InlineButton{Unique: "list_users"}, func(c telebot.Context) error {
		if _, ok := botAdmins[c.Sender().ID]; !ok {
			return c.Edit("❌ You are not authorized to use this command")
		}

		var text strings.Builder
		c.Send(fmt.Sprintf("📋 *All Users:* %d\n\n", len(filteredUsers.AllUsers)), telebot.ModeMarkdown)

		for _, user := range filteredUsers.AllUsers {
			chat, _ := bot.ChatByID(user.ID)
			entry := fmt.Sprintf("🔹 %d: %s (Notify: %s)\n", user.ID, chat.Username, boolToEmoji(user.Notify))
			if text.Len()+len(entry) > 4000 { // Telegram message limit is 4096 bytes
				c.Send(text.String())
				text.Reset()
			}
			text.WriteString(entry)
		}

		return c.Send(text.String())
	})

	bot.Handle(&telebot.InlineButton{Unique: "impersonate_user"}, func(c telebot.Context) error {
		if _, ok := botAdmins[c.Sender().ID]; !ok {
			return c.Edit("❌ You are not authorized to use this command")
		}
		userStates[c.Sender().ID] = "impersonate_user"
		return c.Edit("👤 Enter the user ID you want to impersonate:")
	})

	// Handle location input
	bot.Handle(telebot.OnLocation, func(c telebot.Context) error {
		location := c.Message().Location
		// Update user location in the database
		userID := getUserID(c)
		updateUserPreference(userID, "Latitude", location.Lat)
		updateUserPreference(userID, "Longitude", location.Lng)
		return c.Edit("✅ Location updated")
	})

	// Handle text input
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
				return c.Send("❌ Invalid input! Please enter a valid IV percentage (0-100)")
			}
			userStates[userID] = fmt.Sprintf("add_subscription_level_%d_%d", pokemonID, minIV)
			return c.Send(fmt.Sprintf("✨ Minimum IV set to %d%%. Please enter the minimum Pokémon level (0-40):", minIV))
		}
		if strings.HasPrefix(userStates[userID], "add_subscription_level") {
			pokemonID, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[3])
			minIV, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[4])
			var minLevel int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &minLevel)
			if err != nil || minLevel < 0 || minLevel > 40 {
				return c.Send("❌ Invalid input! Please enter a valid level (0-40)")
			}
			userStates[userID] = fmt.Sprintf("add_subscription_distance_%d_%d_%d", pokemonID, minIV, minLevel)
			return c.Send(fmt.Sprintf("🔢 Minimum level set to %d. Please enter the maximum distance (in m):", minLevel))
		}
		if strings.HasPrefix(userStates[userID], "add_subscription_distance") {
			pokemonID, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[3])
			minIV, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[4])
			minLevel, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[5])
			var maxDistance int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &maxDistance)
			if err != nil || maxDistance < 0 {
				return c.Send("❌ Invalid input! Please enter a valid distance in m")
			}

			// Subscribe user to Pokémon
			addSubscription(getUserID(c), pokemonID, minIV, minLevel, maxDistance)

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
			if err != nil || maxDistance < 0 {
				return c.Send("❌ Invalid input! Please enter a valid distance in m")
			}

			// Update max distance in the database
			updateUserPreference(getUserID(c), "MaxDistance", maxDistance)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf("✅ Max distance updated to %dm", maxDistance))
		}
		if userStates[userID] == "set_min_iv" {
			var minIV int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &minIV)
			if err != nil || minIV < 0 || minIV > 100 {
				return c.Send("❌ Invalid input! Please enter a valid IV percentage (0-100)")
			}

			// Update min IV in the database
			updateUserPreference(getUserID(c), "MinIV", minIV)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf("✅ Minimum IV updated to %d%%", minIV))
		}
		if userStates[userID] == "set_min_level" {
			var minLevel int

			// Parse user input
			_, err := fmt.Sscanf(c.Text(), "%d", &minLevel)
			if err != nil || minLevel < 0 || minLevel > 40 {
				return c.Send("❌ Invalid input! Please enter a valid level (0-40)")
			}

			// Update min IV in the database
			updateUserPreference(getUserID(c), "MinLevel", minLevel)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf("✅ Minimum Level updated to %d", minLevel))
		}
		if userStates[userID] == "broadcast" {
			if _, ok := botAdmins[userID]; !ok {
				return c.Send("❌ You are not authorized to use this command")
			}

			message := c.Text()
			for _, user := range filteredUsers.AllUsers {
				if user.Notify {
					bot.Send(&telebot.User{ID: user.ID}, message, telebot.ModeMarkdown)
				}
			}

			userStates[userID] = ""

			return c.Send("📢 Broadcast sent to all users")
		}
		if userStates[userID] == "impersonate_user" {
			if _, ok := botAdmins[userID]; !ok {
				return c.Send("❌ You are not authorized to use this command")
			}
			impersonatedUserID, err := strconv.Atoi(c.Text())
			if err != nil {
				return c.Send("❌ Invalid user ID")
			}

			userStates[userID] = ""

			botAdmins[userID] = int64(impersonatedUserID)
			user := getUserPreferences(int64(impersonatedUserID))
			settingsMessage, replyMarkup := buildSettings(user)

			return c.Send(settingsMessage, replyMarkup, telebot.ModeMarkdown)
		}
		return nil
	})
}

func processEncounters(bot *telebot.Bot) {
	var lastCheck = time.Now().Unix() - 30
	// Fetch current Pokémon encounters
	var encounters []Pokemon
	if err := dbScanner.Where("iv IS NOT NULL AND updated > ? AND expire_timestamp > ?", lastCheck, lastCheck).Find(&encounters).Error; err != nil {
		log.Printf("❌ Failed to fetch Pokémon encounters: %v", err)
	} else {
		encounterGauge.Set(float64(len(encounters)))
		log.Printf("✅ Found %d Pokémon", len(encounters))

		// Match encounters with subscriptions
		for _, encounter := range encounters {
			// Check for 100% IV Pokémon
			if *encounter.IV == 100 {
				for _, user := range filteredUsers.HundoIVUsers {
					if user.Latitude != 0 && user.Longitude != 0 && user.MaxDistance > 0 {
						distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
						if distance > float64(user.MaxDistance) {
							continue
						}
					}
					sendEncounterNotification(bot, user, encounter)
				}
			}
			// Check for 0% IV Pokémon
			if *encounter.IV == 0 {
				for _, user := range filteredUsers.ZeroIVUsers {
					if user.Latitude != 0 && user.Longitude != 0 && user.MaxDistance > 0 {
						distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
						if distance > float64(user.MaxDistance) {
							continue
						}
					}
					sendEncounterNotification(bot, user, encounter)
				}
			}
			// Check for Channel Users
			for _, user := range filteredUsers.ChannelUser {
				if user.MinIV == 0 && user.MinLevel == 0 {
					continue
				}
				if user.MinLevel == 0 && *encounter.IV >= float32(user.MinIV) ||
					user.MinIV == 0 && *encounter.Level >= user.MinLevel ||
					*encounter.IV >= float32(user.MinIV) && *encounter.Level >= user.MinLevel {
					sendEncounterNotification(bot, user, encounter)
				}
			}
			// Check for subscribed Pokémon
			if subs, exists := activeSubscriptions[encounter.PokemonID]; exists {
				for _, sub := range subs {
					user := filteredUsers.AllUsers[sub.UserID]

					if sub.MinIV > 0 && *encounter.IV < float32(sub.MinIV) {
						continue
					}
					if user.MinIV > 0 && *encounter.IV < float32(user.MinIV) {
						continue
					}
					if sub.MinLevel > 0 && *encounter.Level < sub.MinLevel {
						continue
					}
					if user.MinLevel > 0 && *encounter.Level < user.MinLevel {
						continue
					}
					if user.Latitude != 0 && user.Longitude != 0 && (user.MaxDistance > 0 || sub.MaxDistance > 0) {
						distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
						if distance > float64(sub.MaxDistance) && distance > float64(user.MaxDistance) {
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
	deletedMessagesCount := 0
	var encounters []Encounter
	dbConfig.Where("expiration < ?", time.Now().Unix()).Find(&encounters)
	log.Printf("🗑️ Found %d expired encounters", len(encounters))

	for _, encounter := range encounters {
		var messages []Message
		dbConfig.Where("encounter_id = ?", encounter.ID).Find(&messages)
		log.Printf("🗑️ Found %d expired messages for encounter %s", len(messages), encounter.ID)

		for _, message := range messages {
			user := filteredUsers.AllUsers[message.ChatID]
			if user.Cleanup {
				deletedMessagesCount++
				if err := bot.Delete(&telebot.StoredMessage{MessageID: strconv.Itoa(message.MessageID), ChatID: message.ChatID}); err != nil {
					log.Printf("❌ Failed to delete message %d for user %d: %v", message.MessageID, message.ChatID, err)
				}
			}
			dbConfig.Delete(&message)
		}
		dbConfig.Delete(&encounter)
		sentNotifications[encounter.ID] = nil
	}

	cleanupGauge.Set(float64(deletedMessagesCount))
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

func init() {
	customRegistry.MustRegister(notificationsCounter)
	customRegistry.MustRegister(messagesCounter)
	customRegistry.MustRegister(encounterGauge)
	customRegistry.MustRegister(cleanupGauge)
	customRegistry.MustRegister(usersGauge)
	customRegistry.MustRegister(subscriptionGauge)
	customRegistry.MustRegister(activeSubscriptionGauge)
}

func startMetricsServer() {
	http.Handle("/metrics", promhttp.HandlerFor(customRegistry, promhttp.HandlerOpts{}))
	go func() {
		log.Println("🚀 Prometheus metrics available at /metrics")
		log.Fatal(http.ListenAndServe(":9001", nil))
	}()
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found, using system environment variables")
	}
	// Required environment variables
	requiredVars := []string{
		"BOT_TOKEN", "BOT_ADMINS", "BOT_DB_USER", "BOT_DB_PASS", "BOT_DB_NAME", "BOT_DB_HOST",
		"SCANNER_DB_USER", "SCANNER_DB_PASS", "SCANNER_DB_NAME", "SCANNER_DB_HOST",
	}
	checkEnvVars(requiredVars)

	admins := strings.Split(os.Getenv("BOT_ADMINS"), ",")
	botAdmins = make(map[int64]int64)
	for _, admin := range admins {
		id, err := strconv.ParseInt(admin, 10, 64)
		if err != nil {
			log.Fatalf("❌ Invalid admin ID: %v", err)
		}
		botAdmins[id] = id
	}

	// Start Prometheus metrics server
	startMetricsServer()

	userStates = make(map[int64]string)
	sentNotifications = make(map[string]map[int64]struct{})

	// Load Pokémon mappings
	loadAllLanguages()

	// Initialize databases
	initDB()

	// Load users into a map
	getUsersByFilters()

	// Load subscriptions into a map
	getActiveSubscriptions()

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
