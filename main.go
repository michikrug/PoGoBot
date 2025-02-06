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
	All      map[int64]User
	HundoIV  []User
	ZeroIV   []User
	Channels []User
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

type EncounterData struct {
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

type MasterFile struct {
	Pokemon          map[string]Pokemon `json:"pokemon"`
	Types            map[string]string  `json:"types"`
	Items            map[string]string  `json:"items"`
	Moves            map[string]Move    `json:"moves"`
	QuestRewardTypes map[string]string  `json:"questRewardTypes"`
	Weather          map[string]Weather `json:"weather"`
	Raids            map[string]string  `json:"raids"`
	Teams            map[string]string  `json:"teams"`
}

type Pokemon struct {
	Name          string          `json:"name"`
	PokedexId     int             `json:"pokedexId"`
	DefaultFormId int             `json:"defaultFormId"`
	Types         []int           `json:"types"`
	QuickMoves    []int           `json:"quickMoves"`
	ChargedMoves  []int           `json:"chargedMoves"`
	GenId         int             `json:"genId"`
	Generation    string          `json:"generation"`
	Forms         map[string]Form `json:"forms"`
	Height        float64         `json:"height"`
	Weight        float64         `json:"weight"`
	Family        int             `json:"family"`
	Legendary     bool            `json:"legendary"`
	Mythical      bool            `json:"mythical"`
	UltraBeast    bool            `json:"ultraBeast"`
}

type Form struct {
	Name      string `json:"name"`
	IsCostume bool   `json:"isCostume,omitempty"`
}

type Move struct {
	Name string `json:"name"`
}

type Weather struct {
	Name  string `json:"name"`
	Types []int  `json:"types"`
}

var (
	dbConfig            *gorm.DB // Stores user subscriptions
	dbScanner           *gorm.DB // Fetches Pokémon encounters
	botAdmins           map[int64]int64
	userStates          map[int64]string
	users               FilteredUsers
	activeSubscriptions map[int][]Subscription
	sentNotifications   map[string]map[int64]struct{}
	pokemonNameToID     map[string]int
	MasterFileData      MasterFile
	TranslationData     map[string]map[string]string
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
			Help: "Total number of notifications triggered",
		},
	)
	messagesCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bot_messages_total",
			Help: "Total number of messages sent",
		},
	)
	cleanupCounter = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_cleanup_total",
			Help: "Total number of expired messages cleaned up",
		},
	)
	encounterGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_encounters_count",
			Help: "Total number of Pokémon encounters retrieved",
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

func (EncounterData) TableName() string {
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

func loadMasterFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("❌ Failed to load Master File: %v", err)
		return nil
	}

	err = json.Unmarshal(data, &MasterFileData)
	if err != nil {
		log.Printf("❌ Failed to parse Master File: %v", err)
		return nil
	}

	log.Printf("✅ Loaded Master File with %d Pokémon & %d Moves", len(MasterFileData.Pokemon), len(MasterFileData.Moves))
	return nil
}

func loadTranslationFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("❌ Failed to load Translation File: %v", err)
		return nil
	}

	err = json.Unmarshal(data, &TranslationData)
	if err != nil {
		log.Printf("❌ Failed to parse Translation File: %v", err)
		return nil
	}

	for lang, translations := range TranslationData {
		log.Printf("✅ Loaded Translation File with %d translations for language: %s", len(translations), lang)
	}

	return nil
}

func loadPokemonNameMappings() {
	pokemonNameToID = make(map[string]int)

	for _, pokemon := range MasterFileData.Pokemon {
		pokemonNameToID[strings.ToLower(pokemon.Name)] = pokemon.PokedexId
		for _, translations := range TranslationData {
			translation := translations[pokemon.Name]
			pokemonNameToID[strings.ToLower(translation)] = pokemon.PokedexId
		}
	}

	log.Printf("✅ Loaded %d Pokémon Name to ID mappings", len(pokemonNameToID))
}

// Convert Pokémon name to ID
func getPokemonID(name string) (int, error) {
	pokemonID, exists := pokemonNameToID[strings.ToLower(name)]
	if !exists {
		return 0, fmt.Errorf("pokémon not found: %s", name)
	}
	return pokemonID, nil
}

func getPokemonName(pokemonID int, language string) string {
	if pokemon, exists := MasterFileData.Pokemon[strconv.Itoa(pokemonID)]; exists {
		return getTranslation(pokemon.Name, language)
	}
	return getTranslation("Unknown", language)
}

func getMoveName(moveID int, language string) string {
	if move, exists := MasterFileData.Moves[strconv.Itoa(moveID)]; exists {
		return getTranslation(move.Name, language)
	}
	return getTranslation("Unknown", language)
}

func getTranslation(key string, language string) string {
	if language == "en" {
		return key
	}
	if translations, exists := TranslationData[language]; exists {
		if translation, exists := translations[key]; exists {
			return translation
		}
		log.Printf("❌ Translation key not found: %s", key)
	} else {
		log.Printf("❌ Translation language not found: %s", language)
	}
	return key
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
	users = FilteredUsers{
		All:      make(map[int64]User),
		HundoIV:  []User{},
		ZeroIV:   []User{},
		Channels: []User{},
	}

	var allUsers []User
	dbConfig.Find(&allUsers)
	for _, user := range allUsers {
		users.All[user.ID] = user
	}
	usersGauge.Set(float64(len(users.All)))
	log.Printf("📋 Loaded %d users", len(users.All))

	for _, user := range users.All {
		if user.Notify {
			if user.HundoIV {
				users.HundoIV = append(users.HundoIV, user)
			}
			if user.ZeroIV {
				users.ZeroIV = append(users.ZeroIV, user)
			}
			if strings.HasPrefix(strconv.FormatInt(user.ID, 10), "-100") {
				users.Channels = append(users.Channels, user)
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
		if users.All[subscription.UserID].Notify {
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

func sendEncounterNotification(bot *telebot.Bot, user User, encounter EncounterData) {
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
		getPokemonName(encounter.PokemonID, user.Language),
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
		getMoveName(*encounter.Move1, user.Language),
		getMoveName(*encounter.Move2, user.Language)))

	if !user.OnlyMap {
		sendMessage(bot, user.ID, notificationTitle+"\n"+notificationText.String(), encounter.ID)
	} else {
		sendVenue(bot, user.ID, encounter.Lat, encounter.Lon, notificationTitle, notificationText.String(), encounter.ID)
	}
}

func buildSettings(user User) (string, *telebot.ReplyMarkup) {
	// Create interactive buttons
	btnChangeLanguage := telebot.InlineButton{Text: getTranslation("🌍 Change Language", user.Language), Unique: "change_lang"}
	btnUpdateLocation := telebot.InlineButton{Text: getTranslation("📍 Update Location", user.Language), Unique: "update_location"}
	btnSetDistance := telebot.InlineButton{Text: getTranslation("📏 Set Maximal Distance", user.Language), Unique: "set_distance"}
	btnSetMinIV := telebot.InlineButton{Text: getTranslation("✨ Set Minimal IV", user.Language), Unique: "set_min_iv"}
	btnSetMinLevel := telebot.InlineButton{Text: getTranslation("🔢 Set Minimal Level", user.Language), Unique: "set_min_level"}
	btnAddSubscription := telebot.InlineButton{Text: getTranslation("📣 Add Pokémon Subscription", user.Language), Unique: "add_subscription"}
	btnListSubscriptions := telebot.InlineButton{Text: getTranslation("📋 List all Pokémon Subscriptions", user.Language), Unique: "list_subscriptions"}
	btnClearSubscriptions := telebot.InlineButton{Text: getTranslation("🗑️ Clear all Pokémon Subscriptions", user.Language), Unique: "clear_subscriptions"}
	notificationsText := getTranslation("🔔 Disable all Notifications", user.Language)
	if !user.Notify {
		notificationsText = getTranslation("🔕 Enable all Notifications", user.Language)
	}
	btnToggleNotifications := telebot.InlineButton{Text: notificationsText, Unique: "toggle_notifications"}
	stickersText := getTranslation("🎭 Do not show Pokémon Stickers", user.Language)
	if !user.Stickers {
		stickersText = getTranslation("🎭 Show Pokémon Stickers", user.Language)
	}
	btnToggleStickers := telebot.InlineButton{Text: stickersText, Unique: "toggle_stickers"}
	hundoText := getTranslation("💯 Disable 100% IV Notifications", user.Language)
	if !user.HundoIV {
		hundoText = getTranslation("💯 Enable 100% IV Notifications", user.Language)
	}
	btnToogleHundoIV := telebot.InlineButton{Text: hundoText, Unique: "toggle_hundo_iv"}
	zeroText := getTranslation("🚫 Disable 0% IV Notifications", user.Language)
	if !user.ZeroIV {
		zeroText = getTranslation("🚫 Enable 0% IV Notifications", user.Language)
	}
	btnToogleZeroIV := telebot.InlineButton{Text: zeroText, Unique: "toggle_zero_iv"}
	cleanupText := getTranslation("🗑️ Keep Expired Notifications", user.Language)
	if !user.Cleanup {
		cleanupText = getTranslation("🗑️ Remove Expired Notifications", user.Language)
	}
	btnToggleCleanup := telebot.InlineButton{Text: cleanupText, Unique: "toggle_cleanup"}
	btnClose := telebot.InlineButton{Text: getTranslation("Close", user.Language), Unique: "close"}

	// Settings message
	settingsMessage := fmt.Sprintf(
		getTranslation("⚙️ *Your Settings:*", user.Language)+"\n"+
			"----------------------------------------------\n"+
			getTranslation("🌍 *Language:* %s", user.Language)+"\n"+
			getTranslation("📍 *Location:* %.5f, %.5f", user.Language)+"\n"+
			getTranslation("📏 *Maximal Distance:* %dm", user.Language)+"\n"+
			getTranslation("✨ *Minimal IV:* %d%%", user.Language)+"\n"+
			getTranslation("🔢 *Minimal Level:* %d", user.Language)+"\n"+
			getTranslation("🔔 *Notifications:* %s", user.Language)+"\n"+
			getTranslation("🎭 *Pokémon Stickers:* %s", user.Language)+"\n"+
			getTranslation("💯 *100%% IV Notifications:* %s", user.Language)+"\n"+
			getTranslation("🚫 *0%% IV Notifications:* %s", user.Language)+"\n"+
			getTranslation("🗑️ *Cleanup Expired Notifications:* %s", user.Language)+"\n\n"+
			getTranslation("Use the buttons below to update your settings", user.Language),
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
		btnBroadcast := telebot.InlineButton{Text: getTranslation("📢 Broadcast Message", user.Language), Unique: "broadcast"}
		btnListUsers := telebot.InlineButton{Text: getTranslation("📋 List Users", user.Language), Unique: "list_users"}
		btnImpersonateUser := telebot.InlineButton{Text: getTranslation("👤 Impersonate User", user.Language), Unique: "impersonate_user"}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnBroadcast, btnListUsers, btnImpersonateUser})
	}

	return settingsMessage, &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}
}

func getUserID(c telebot.Context) int64 {
	userID := c.Sender().ID
	language := users.All[userID].Language
	if adminID, ok := botAdmins[userID]; ok && adminID != userID {
		c.Send(getTranslation("🔒 You are impersonating another user", language))
		return adminID
	}
	return userID
}

func setupBotHandlers(bot *telebot.Bot) {

	// /subscribe <pokemon_name> [min_iv]
	bot.Handle("/subscribe", func(c telebot.Context) error {
		userID := getUserID(c)
		language := users.All[userID].Language

		args := c.Args()
		if len(args) < 1 {
			return c.Send(getTranslation("ℹ️ Usage: /subscribe <pokemon-name> [min-iv] [min-level] [max-distance]", language))
		}

		pokemonName := args[0]
		pokemonID, err := getPokemonID(pokemonName)
		if err != nil {
			return c.Send(fmt.Sprintf(getTranslation("❌ Can't find Pokedex # for Pokémon: %s", language), pokemonName))
		}

		minIV := int(0)
		minLevel := int(0)
		maxDistance := int(0)
		if len(args) > 1 {
			minIV, err = strconv.Atoi(args[1])
			if err != nil || minIV < 0 || minIV > 100 {
				return c.Send(getTranslation("❌ Invalid input! Please enter a valid IV percentage (0-100)", language))
			}
		}
		if len(args) > 2 {
			minLevel, err = strconv.Atoi(args[2])
			if err != nil || minLevel < 0 || minLevel > 40 {
				return c.Send(getTranslation("❌ Invalid input! Please enter a valid level (0-40)", language))
			}
		}
		if len(args) > 3 {
			maxDistance, err = strconv.Atoi(args[3])
			if err != nil || maxDistance < 0 {
				return c.Send(getTranslation("❌ Invalid input! Please enter a valid distance (in m)", language))
			}
		}

		addSubscription(userID, pokemonID, minIV, minLevel, maxDistance)

		user := getUserPreferences(userID)
		return c.Send(fmt.Sprintf(getTranslation("✅ Subscribed to %s alerts (Min IV: %d%%, Min Level: %d, Max Distance: %dm)", language),
			getPokemonName(pokemonID, user.Language),
			minIV, minLevel, maxDistance,
		))
	})

	// /list
	bot.Handle("/list", func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))

		var text strings.Builder
		text.WriteString(getTranslation("📋 *Your Pokémon Subscriptions:*", user.Language) + "\n\n")
		if user.HundoIV {
			text.WriteString(fmt.Sprintf(getTranslation("🔹 *All* (Min IV: 100%%, Min Level: 0, Max Distance: %dm)", user.Language)+"\n", user.MaxDistance))
		}
		if user.ZeroIV {
			text.WriteString(fmt.Sprintf(getTranslation("🔹 *All* (Max IV: 0%%, Min Level: 0, Max Distance: %dm", user.Language)+"\n", user.MaxDistance))
		}
		c.Send(text.String(), telebot.ModeMarkdown)
		text.Reset()

		var subs []Subscription
		dbConfig.Where("user_id = ?", user.ID).Order("pokemon_id").Find(&subs)

		if len(subs) == 0 {
			return c.Send(getTranslation("🔹 You have no specific Pokémon subscriptions", user.Language))
		}

		for _, sub := range subs {
			entry :=
				fmt.Sprintf(getTranslation("🔹 %s (Min IV: %d%%, Min Level: %d, Max Distance: %dm)", user.Language)+"\n",
					getPokemonName(sub.PokemonID, user.Language),
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
		userID := getUserID(c)
		language := users.All[userID].Language

		args := c.Args()
		if len(args) < 1 {
			return c.Send(getTranslation("ℹ️ Usage: /unsubscribe <pokemon-name>", language))
		}

		pokemonName := args[0]
		pokemonID, err := getPokemonID(pokemonName)
		if err != nil {
			return c.Send(fmt.Sprintf(getTranslation("❌ Can't find Pokedex # for Pokémon: %s", language), pokemonName))
		}

		dbConfig.Where("user_id = ? AND pokemon_id = ?", userID, pokemonID).Delete(&Subscription{})

		getActiveSubscriptions()

		user := getUserPreferences(userID)

		return c.Send(fmt.Sprintf(getTranslation("✅ Unsubscribed from %s alerts", language), getPokemonName(pokemonID, user.Language)))
	})

	bot.Handle(telebot.OnLocation, func(c telebot.Context) error {
		userID := getUserID(c)
		language := users.All[userID].Language
		location := c.Message().Location

		updateUserPreference(userID, "Latitude", location.Lat)
		updateUserPreference(userID, "Longitude", location.Lng)

		return c.Send(getTranslation("📍 Location updated! Your preferences will now consider this", language))
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
			"👋 Welcome to the PoGo Notification Bot!"+"\n\n"+
				"ℹ️ Language detected: *%s*"+"\n"+
				"ℹ️ Send me your 📍 *location* to enable distance-based notifications"+"\n"+
				"ℹ️ Use /settings to update your preferences"+"\n"+
				"ℹ️ Use /subscribe <pokemon-name> [min-iv] [min-level] [max-distance] to get notified about specific Pokémon",
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
		language := users.All[userID].Language
		if _, ok := botAdmins[userID]; !ok {
			return c.Send(getTranslation("❌ You are not authorized to use this command", language))
		}
		if botAdmins[userID] == userID {
			return c.Send(getTranslation("🔒 You are not impersonating another user", language), telebot.ModeMarkdown)
		}
		botAdmins[userID] = userID
		return c.Send(getTranslation("🔒 You are now back as yourself", language))
	})

	bot.Handle(&telebot.InlineButton{Unique: "close"}, func(c telebot.Context) error {
		userID := c.Sender().ID
		language := users.All[userID].Language
		return c.Edit(getTranslation("✅ Settings closed", language))
	})

	bot.Handle(&telebot.InlineButton{Unique: "add_subscription"}, func(c telebot.Context) error {
		userID := getUserID(c)
		language := users.All[userID].Language
		userStates[c.Sender().ID] = "add_subscription"
		return c.Edit(getTranslation("📣 Enter the Pokémon name you want to subscribe to:", language))
	})

	bot.Handle(&telebot.InlineButton{Unique: "list_subscriptions"}, func(c telebot.Context) error {
		c.Delete()
		return bot.Trigger("/list", c)
	})

	bot.Handle(&telebot.InlineButton{Unique: "clear_subscriptions"}, func(c telebot.Context) error {
		userID := getUserID(c)
		language := users.All[userID].Language
		dbConfig.Where("user_id = ?", userID).Delete(&Subscription{})
		getActiveSubscriptions()
		return c.Edit(getTranslation("🗑️ All Pokémon subscriptions cleared", language))
	})

	bot.Handle(&telebot.InlineButton{Unique: "toggle_notifications"}, func(c telebot.Context) error {
		user := getUserPreferences(getUserID(c))
		user.Notify = !user.Notify
		updateUserPreference(user.ID, "Notify", user.Notify)
		getActiveSubscriptions()
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
		userID := c.Sender().ID
		language := users.All[userID].Language
		btnEn := telebot.InlineButton{Text: "🇬🇧 English", Unique: "set_lang_en"}
		btnDe := telebot.InlineButton{Text: "🇩🇪 Deutsch", Unique: "set_lang_de"}
		return c.Edit(getTranslation("🌍 *Select a language:*", language), &telebot.ReplyMarkup{
			InlineKeyboard: [][]telebot.InlineButton{{btnEn, btnDe}},
		}, telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_lang_en"}, func(c telebot.Context) error {
		updateUserPreference(getUserID(c), "Language", "en")
		return c.Edit("✅ Language set to *English*", telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_lang_de"}, func(c telebot.Context) error {
		updateUserPreference(getUserID(c), "Language", "de")
		return c.Edit("✅ Sprache auf *Deutsch* gestellt", telebot.ModeMarkdown)
	})

	bot.Handle(&telebot.InlineButton{Unique: "update_location"}, func(c telebot.Context) error {
		c.Delete()
		userID := c.Sender().ID
		language := users.All[userID].Language
		// Prompt user to send location
		btnShareLocation := telebot.ReplyButton{
			Text:     getTranslation("📍 Send Location", language),
			Location: true,
		}
		return c.Send(getTranslation("📍 Please send your current location:", language), &telebot.ReplyMarkup{
			ReplyKeyboard:  [][]telebot.ReplyButton{{btnShareLocation}},
			ResizeKeyboard: true,
		})
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_distance"}, func(c telebot.Context) error {
		userID := c.Sender().ID
		language := users.All[userID].Language
		userStates[userID] = "set_distance"
		return c.Edit(getTranslation("📏 Enter the maximal distance (in m):", language))
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_min_iv"}, func(c telebot.Context) error {
		userID := c.Sender().ID
		language := users.All[userID].Language
		userStates[userID] = "set_min_iv"
		return c.Edit(getTranslation("✨ Enter the minimal IV percentage (0-100):", language))
	})

	bot.Handle(&telebot.InlineButton{Unique: "set_min_level"}, func(c telebot.Context) error {
		userID := c.Sender().ID
		language := users.All[userID].Language
		userStates[userID] = "set_min_level"
		return c.Edit(getTranslation("🔢 Enter the minimal Pokémon level (1-40):", language))
	})

	bot.Handle(&telebot.InlineButton{Unique: "broadcast"}, func(c telebot.Context) error {
		userID := c.Sender().ID
		language := users.All[userID].Language
		if _, ok := botAdmins[userID]; !ok {
			return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
		}
		userStates[userID] = "broadcast"
		return c.Edit("📢 Enter the message you want to broadcast:")
	})

	bot.Handle(&telebot.InlineButton{Unique: "list_users"}, func(c telebot.Context) error {
		c.Delete()
		userID := c.Sender().ID
		language := users.All[userID].Language
		if _, ok := botAdmins[userID]; !ok {
			return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
		}

		var text strings.Builder
		c.Send(fmt.Sprintf("📋 *All Users:* %d\n\n", len(users.All)), telebot.ModeMarkdown)

		for _, user := range users.All {
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
		userID := c.Sender().ID
		language := users.All[userID].Language
		if _, ok := botAdmins[c.Sender().ID]; !ok {
			return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
		}
		userStates[c.Sender().ID] = "impersonate_user"
		return c.Edit(getTranslation("👤 Enter the user ID you want to impersonate:", language))
	})

	// Handle location input
	bot.Handle(telebot.OnLocation, func(c telebot.Context) error {
		userID := getUserID(c)
		language := users.All[userID].Language
		location := c.Message().Location
		// Update user location in the database
		updateUserPreference(userID, "Latitude", location.Lat)
		updateUserPreference(userID, "Longitude", location.Lng)
		return c.Send(getTranslation("✅ Location updated", language))
	})

	// Handle text input
	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		userID := c.Sender().ID
		language := users.All[userID].Language
		if userStates[userID] == "add_subscription" {
			pokemonName := c.Text()
			pokemonID, err := getPokemonID(pokemonName)
			if err != nil {
				return c.Send(fmt.Sprintf("❌ Can't find Pokedex # for Pokémon: %s", pokemonName))
			}

			userStates[userID] = fmt.Sprintf("add_subscription_iv_%d", pokemonID)

			return c.Send(fmt.Sprintf("📣 Subscribing to %s alerts. Please enter the minimal IV percentage (0-100):",
				getPokemonName(pokemonID, language),
			))
		}
		if strings.HasPrefix(userStates[userID], "add_subscription_iv") {
			pokemonID, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[3])

			// Parse user input
			var minIV int
			_, err := fmt.Sscanf(c.Text(), "%d", &minIV)
			if err != nil || minIV < 0 || minIV > 100 {
				return c.Send(getTranslation("❌ Invalid input! Please enter a valid IV percentage (0-100)", language))
			}

			userStates[userID] = fmt.Sprintf("add_subscription_level_%d_%d", pokemonID, minIV)

			return c.Send(fmt.Sprintf("✨ Minimal IV set to %d%%. Please enter the minimal Pokémon level (0-40):", minIV))
		}
		if strings.HasPrefix(userStates[userID], "add_subscription_level") {
			pokemonID, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[3])
			minIV, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[4])

			// Parse user input
			var minLevel int
			_, err := fmt.Sscanf(c.Text(), "%d", &minLevel)
			if err != nil || minLevel < 0 || minLevel > 40 {
				return c.Send(getTranslation("❌ Invalid input! Please enter a valid level (0-40)", language))
			}

			userStates[userID] = fmt.Sprintf("add_subscription_distance_%d_%d_%d", pokemonID, minIV, minLevel)

			return c.Send(fmt.Sprintf("🔢 Minimal level set to %d. Please enter the maximal distance (in m):", minLevel))
		}
		if strings.HasPrefix(userStates[userID], "add_subscription_distance") {
			pokemonID, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[3])
			minIV, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[4])
			minLevel, _ := strconv.Atoi(strings.Split(userStates[userID], "_")[5])

			// Parse user input
			var maxDistance int
			_, err := fmt.Sscanf(c.Text(), "%d", &maxDistance)
			if err != nil || maxDistance < 0 {
				return c.Send(getTranslation("❌ Invalid input! Please enter a valid distance (in m)", language))
			}

			// Subscribe user to Pokémon
			addSubscription(getUserID(c), pokemonID, minIV, minLevel, maxDistance)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf(getTranslation("✅ Subscribed to %s alerts (Min IV: %d%%, Min Level: %d, Max Distance: %dm)", language),
				getPokemonName(pokemonID, language),
				minIV, minLevel, maxDistance,
			))
		}
		if userStates[userID] == "set_distance" {
			// Parse user input
			var maxDistance int
			_, err := fmt.Sscanf(c.Text(), "%d", &maxDistance)
			if err != nil || maxDistance < 0 {
				return c.Send(getTranslation("❌ Invalid input! Please enter a valid distance (in m)", language))
			}

			// Update max distance in the database
			updateUserPreference(getUserID(c), "MaxDistance", maxDistance)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf(getTranslation("✅ Maximal distance updated to %dm", language), maxDistance))
		}
		if userStates[userID] == "set_min_iv" {
			// Parse user input
			var minIV int
			_, err := fmt.Sscanf(c.Text(), "%d", &minIV)
			if err != nil || minIV < 0 || minIV > 100 {
				return c.Send(getTranslation("❌ Invalid input! Please enter a valid IV percentage (0-100)", language))
			}

			// Update min IV in the database
			updateUserPreference(getUserID(c), "MinIV", minIV)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf(getTranslation("✅ Minimal IV updated to %d%%", language), minIV))
		}
		if userStates[userID] == "set_min_level" {
			// Parse user input
			var minLevel int
			_, err := fmt.Sscanf(c.Text(), "%d", &minLevel)
			if err != nil || minLevel < 0 || minLevel > 40 {
				return c.Send(getTranslation("❌ Invalid input! Please enter a valid level (0-40)", language))
			}

			// Update min IV in the database
			updateUserPreference(getUserID(c), "MinLevel", minLevel)

			userStates[userID] = ""

			return c.Send(fmt.Sprintf(getTranslation("✅ Minimal Level updated to %d", language), minLevel))
		}
		if userStates[userID] == "broadcast" {
			if _, ok := botAdmins[userID]; !ok {
				return c.Send(getTranslation("❌ You are not authorized to use this command", language))
			}

			message := c.Text()
			for _, user := range users.All {
				if user.Notify {
					bot.Send(&telebot.User{ID: user.ID}, message, telebot.ModeMarkdown)
				}
			}

			userStates[userID] = ""

			return c.Send(getTranslation("📢 Broadcast sent to all users", language))
		}
		if userStates[userID] == "impersonate_user" {
			if _, ok := botAdmins[userID]; !ok {
				return c.Send(getTranslation("❌ You are not authorized to use this command", language))
			}

			impersonatedUserID, err := strconv.Atoi(c.Text())
			if err != nil {
				return c.Send(getTranslation("❌ Invalid user ID", language))
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
	var encounters []EncounterData
	if err := dbScanner.Where("iv IS NOT NULL AND updated > ? AND expire_timestamp > ?", lastCheck, lastCheck).Find(&encounters).Error; err != nil {
		log.Printf("❌ Failed to fetch Pokémon encounters: %v", err)
	} else {
		encounterGauge.Set(float64(len(encounters)))
		log.Printf("✅ Found %d Pokémon", len(encounters))
		filteredAndSendEncounters(bot, users, encounters)
	}
}

func filteredAndSendEncounters(bot *telebot.Bot, users FilteredUsers, encounters []EncounterData) {
	// Match encounters with subscriptions
	for _, encounter := range encounters {
		// Check for 100% IV Pokémon
		if *encounter.IV == 100 {
			for _, user := range users.HundoIV {
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
			for _, user := range users.ZeroIV {
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
		for _, user := range users.Channels {
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
				user := users.All[sub.UserID]

				// Determine effective subscription limits by falling back to user defaults if needed
				effectiveMinIV := sub.MinIV
				if effectiveMinIV == 0 {
					effectiveMinIV = user.MinIV
				}
				effectiveMinLevel := sub.MinLevel
				if effectiveMinLevel == 0 {
					effectiveMinLevel = user.MinLevel
				}
				effectiveMaxDistance := sub.MaxDistance
				if effectiveMaxDistance == 0 {
					effectiveMaxDistance = user.MaxDistance
				}

				// Validate encounter IV against required minimum IV
				if effectiveMinIV > 0 && *encounter.IV < float32(effectiveMinIV) {
					log.Printf("🔍 Skipping encounter: IV %.2f is below required %d%%", *encounter.IV, effectiveMinIV)
					continue
				}

				// Validate encounter level against required minimum level
				if effectiveMinLevel > 0 && *encounter.Level < effectiveMinLevel {
					log.Printf("🔍 Skipping encounter: Level %d is below required %d", *encounter.Level, effectiveMinLevel)
					continue
				}

				// If user's location is set, check if the encounter is within allowed distance
				if user.Latitude != 0 && user.Longitude != 0 && effectiveMaxDistance > 0 {
					distance := haversine(float64(user.Latitude), float64(user.Longitude), float64(encounter.Lat), float64(encounter.Lon))
					if distance > float64(effectiveMaxDistance) {
						log.Printf("🔍 Skipping encounter: Distance %.0fm exceeds allowed %dm", distance, effectiveMaxDistance)
						continue
					}
				}
				sendEncounterNotification(bot, user, encounter)
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
			user := users.All[message.ChatID]
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

	cleanupCounter.Add(float64(deletedMessagesCount))
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
	customRegistry.MustRegister(cleanupCounter)
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
	log.Println("🚀 Starting PoGo Notification Bot")

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

	// Load masterfile
	loadMasterFile("masterfile.json")

	// Load translations
	loadTranslationFile("translations.json")

	// Load Pokémon name mappings
	loadPokemonNameMappings()

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
