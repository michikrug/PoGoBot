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

// Config holds all application configuration
type Config struct {
	// Database configurations
	botDB struct {
		User string
		Pass string
		Name string
		Host string
	}
	scannerDB struct {
		User string
		Pass string
		Name string
		Host string
	}

	// Bot configuration
	BotToken string
	Admins   map[int64]int64

	// Application settings
	Timezone *time.Location
}

// Global configuration instance
var appConfig *Config

// Static data loaded from files
var (
	gameData     MasterFile
	translations map[string]map[string]string
	timezone     *time.Location // Local timezone
)

// notificationService is the singleton used by the production application.
// Tests create their own NotificationService with mocked dependencies instead.
var notificationService *NotificationService

// startNotificationProcessing starts the background goroutine.
func startNotificationProcessing() {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			notificationService.cleanupMessages(userCache)
			notificationService.processEncounters(userCache, activeSubscriptions)
		}
	}()
}

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
	appConfig.botDB.User = os.Getenv("BOT_DB_USER")
	appConfig.botDB.Pass = os.Getenv("BOT_DB_PASS")
	appConfig.botDB.Name = os.Getenv("BOT_DB_NAME")
	appConfig.botDB.Host = os.Getenv("BOT_DB_HOST")

	// Load scanner database configuration
	appConfig.scannerDB.User = os.Getenv("SCANNER_DB_USER")
	appConfig.scannerDB.Pass = os.Getenv("SCANNER_DB_PASS")
	appConfig.scannerDB.Name = os.Getenv("SCANNER_DB_NAME")
	appConfig.scannerDB.Host = os.Getenv("SCANNER_DB_HOST")

	// Load bot token
	appConfig.BotToken = os.Getenv("BOT_TOKEN")
}

// configureBotAdmins parses and configures bot administrators
func configureBotAdmins() {
	appConfig.Admins = make(map[int64]int64)
	for _, admin := range strings.Split(os.Getenv("BOT_ADMINS"), ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(admin), 10, 64)
		if err != nil {
			log.Fatalf("❌ Invalid admin ID: %v", err)
		}
		appConfig.Admins[id] = id
	}
	log.Printf("✅ Configured %d bot administrators", len(appConfig.Admins))
}

// configureTimezone sets up the application timezone
func configureTimezone() {
	var err error
	if appConfig.Timezone, err = time.LoadLocation("Local"); err != nil {
		log.Printf("❌ Failed to load local timezone: %v", err)
		appConfig.Timezone = time.UTC
	}
	log.Printf("✅ Timezone set to: %s", appConfig.Timezone.String())
}

// initDatabases initializes both bot and scanner database connections
func initDatabases() {
	initBotDatabase()
	initScannerDatabase()
}

// initBotDatabase initializes the bot database connection
func initBotDatabase() {
	configDSN := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		appConfig.botDB.User, appConfig.botDB.Pass, appConfig.botDB.Host, appConfig.botDB.Name)

	db, err := gorm.Open(mysql.Open(configDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to bot database: %v", err)
	}
	log.Println("✅ Connected to bot database")

	// Auto-migrate database schema
	db.AutoMigrate(&User{}, &Subscription{}, &Message{}, &Encounter{})

	botDB = &gormBotDB{db: db}
}

// initScannerDatabase initializes the scanner database connection
func initScannerDatabase() {
	scannerDSN := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		appConfig.scannerDB.User, appConfig.scannerDB.Pass, appConfig.scannerDB.Host, appConfig.scannerDB.Name)

	db, err := gorm.Open(mysql.Open(scannerDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Failed to connect to scanner database: %v", err)
	}
	log.Println("✅ Connected to scanner database")

	scannerDB = &gormScannerDB{db: db}
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

	// Load static files
	loadStaticFiles()
	loadPokemonNameMappings()

	// Initialize databases — sets botDB and scannerDB
	initDatabases()
	getUsersByFilters()
	getActiveSubscriptions(botDB)

	// Initialize bot
	initBot()

	// Update global variables that depend on config
	botAdmins = appConfig.Admins
	timezone = appConfig.Timezone

	// Wire up the global notification service
	notificationService = newNotificationService(
		botDB,
		scannerDB,
		&telegramBotSender{bot: bot},
		&gameData,
		translations,
		timezone,
		notificationsCounter,
		messagesCounter,
		cleanupCounter,
		encounterGauge,
	)

	log.Println("✅ PoGoBot initialization completed successfully")
}
