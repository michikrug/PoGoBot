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

// DBConfig holds the connection parameters for a single database.
type DBConfig struct {
	User string
	Pass string
	Name string
	Host string
}

// DSN returns the MySQL DSN string for the given database configuration.
func (c DBConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User, c.Pass, c.Host, c.Name)
}

// Config holds all application configuration
type Config struct {
	// Database configurations
	BotDB     DBConfig
	ScannerDB DBConfig

	// Bot configuration
	BotToken string
	Admins   map[int64]struct{}

	// Application settings
	Timezone *time.Location
}

// Global configuration instance
var appConfig *Config

// Static data loaded from files
var (
	gameData     MasterFile
	translations map[string]map[string]string
)

// initConfig initializes the application configuration
func initConfig() {
	appConfig = &Config{}

	// Load environment variables from .env file, if available
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found, using system environment variables")
	}

	// Load and validate environment variables
	loadEnvironmentVariables()

	// Configure bot administrators
	configureBotAdmins()

	// Set timezone
	configureTimezone()
}

// loadEnvironmentVariables loads and validates required environment variables
func loadEnvironmentVariables() {
	// Check required environment variables
	requiredVars := []string{
		"BOT_TOKEN", "BOT_ADMINS", "BOT_DB_USER", "BOT_DB_PASS", "BOT_DB_NAME", "BOT_DB_HOST",
		"SCANNER_DB_USER", "SCANNER_DB_PASS", "SCANNER_DB_NAME", "SCANNER_DB_HOST",
	}
	checkEnvVars(requiredVars)

	// Load bot database configuration
	appConfig.BotDB.User = os.Getenv("BOT_DB_USER")
	appConfig.BotDB.Pass = os.Getenv("BOT_DB_PASS")
	appConfig.BotDB.Name = os.Getenv("BOT_DB_NAME")
	appConfig.BotDB.Host = os.Getenv("BOT_DB_HOST")

	// Load scanner database configuration
	appConfig.ScannerDB.User = os.Getenv("SCANNER_DB_USER")
	appConfig.ScannerDB.Pass = os.Getenv("SCANNER_DB_PASS")
	appConfig.ScannerDB.Name = os.Getenv("SCANNER_DB_NAME")
	appConfig.ScannerDB.Host = os.Getenv("SCANNER_DB_HOST")

	// Load bot token
	appConfig.BotToken = os.Getenv("BOT_TOKEN")
}

// configureBotAdmins parses and configures bot administrators
func configureBotAdmins() {
	appConfig.Admins = make(map[int64]struct{})
	for _, admin := range strings.Split(os.Getenv("BOT_ADMINS"), ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(admin), 10, 64)
		if err != nil {
			log.Fatalf("❌ Invalid admin ID: %v", err)
		}
		appConfig.Admins[id] = struct{}{}
	}
	log.Printf("✅ Configured %d bot administrators", len(appConfig.Admins))
}

// configureTimezone sets up the application timezone.
// BOT_TIMEZONE may be set to any IANA timezone name (e.g. "Europe/Berlin").
// Falls back to UTC if the value is invalid.
func configureTimezone() {
	tz := os.Getenv("BOT_TIMEZONE")
	if tz == "" {
		tz = "Local"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		log.Printf("⚠️ Unknown timezone %q, falling back to UTC: %v", tz, err)
		loc = time.UTC
	}
	appConfig.Timezone = loc
	log.Printf("✅ Timezone set to: %s", appConfig.Timezone.String())
}

// initDatabases initializes both bot and scanner database connections
func initDatabases() {
	db := openDatabase(appConfig.BotDB)
	if err := db.AutoMigrate(&User{}, &Subscription{}, &RaidSubscription{}, &Message{}, &Encounter{}); err != nil {
		log.Fatalf("❌ Failed to auto-migrate bot database: %v", err)
	}
	botDB = &gormBotDB{db: db}
	log.Println("✅ Connected to bot database")

	scanDB := openDatabase(appConfig.ScannerDB)
	scannerDB = &gormScannerDB{db: scanDB}
	log.Println("✅ Connected to scanner database")
}

// openDatabase opens a GORM MySQL connection for the given DBConfig.
// Calls log.Fatalf on failure.
func openDatabase(cfg DBConfig) *gorm.DB {
	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database (%s): %v", cfg.Name, err)
	}
	return db
}

// initBot initializes the Telegram bot
func initBot() {
	pref := telebot.Settings{
		Token:  appConfig.BotToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	var err error
	bot, err = telebot.NewBot(pref)
	if err != nil {
		log.Fatalf("❌ Failed to initialize bot: %v", err)
	}
	log.Println("✅ Bot initialized successfully")
}

// loadStaticFiles loads all required static files
func loadStaticFiles() {
	if err := loadMasterFile("masterfile.json"); err != nil {
		log.Fatalf("❌ Unable to load masterfile: %v", err)
	}
	if err := loadTranslationFile("translations.json"); err != nil {
		log.Fatalf("❌ Unable to load translations: %v", err)
	}
}

// loadMasterFile loads the Pokémon master file
func loadMasterFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read masterfile (%s): %w", filename, err)
	}

	if err := json.Unmarshal(data, &gameData); err != nil {
		return fmt.Errorf("failed to unmarshal JSON from (%s): %w", filename, err)
	}

	log.Printf("✅ Loaded Master File: %d Pokémon & %d Moves", len(gameData.Pokemon), len(gameData.Moves))
	return nil
}

// loadTranslationFile loads the translation file
func loadTranslationFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read translation file %s: %w", filename, err)
	}

	if err = json.Unmarshal(data, &translations); err != nil {
		return fmt.Errorf("failed to unmarshal JSON from %s: %w", filename, err)
	}

	for lang, translations := range translations {
		log.Printf("✅ Loaded %d translations for language: %s", len(translations), lang)
	}
	return nil
}

// loadPokemonNameMappings creates a mapping from Pokémon names to their IDs
func loadPokemonNameMappings() {
	pokemonNameCache = make(map[string]int)

	for _, pokemon := range gameData.Pokemon {
		pokemonNameCache[strings.ToLower(pokemon.Name)] = pokemon.PokedexID
		for _, translations := range translations {
			if translation, exists := translations[pokemon.Name]; exists {
				pokemonNameCache[strings.ToLower(translation)] = pokemon.PokedexID
			}
		}
	}

	log.Printf("✅ Loaded %d Pokémon Name to ID mappings", len(pokemonNameCache))
}

// initializeApplication performs all application initialization
func initializeApplication() {
	log.Println("🚀 Starting PoGoBot initialization...")

	// Initialize configuration
	initConfig()

	// Initialize state maps
	userConversationStates = make(map[int64]string)
	adminImpersonation = make(map[int64]int64)

	// Load static files
	loadStaticFiles()
	loadPokemonNameMappings()

	// Initialize databases — sets the global botDB and scannerDB vars.
	initDatabases()
	// Warm the in-memory user and subscription caches used by the notification loop.
	getUsersByFilters()
	getActiveSubscriptions(botDB)
	getRaidActiveSubscriptions(botDB)

	// Initialize bot
	initBot()

	// Wire up the global notification service
	notificationService = newNotificationService(
		botDB,
		scannerDB,
		&telegramBotSender{bot: bot},
		&gameData,
		translations,
		appConfig.Timezone,
		notificationsCounter,
		messagesCounter,
		cleanupCounter,
		encounterGauge,
		raidNotificationsCounter,
		raidEncounterGauge,
	)

	log.Println("✅ PoGoBot initialization completed successfully")
}
