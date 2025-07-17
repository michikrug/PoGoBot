package main

// User represents a bot user with their preferences
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
	TopPVP      bool    `gorm:"not null;default:false"`
	MinIV       int     `gorm:"not null;default:0;type:tinyint(3)"`
	MinLevel    int     `gorm:"not null;default:0;type:tinyint(2)"`
}

// FilteredUsers contains users organized by notification type
type FilteredUsers struct {
	All      map[int64]User
	HundoIV  []User
	ZeroIV   []User
	TopPVP   []User
	Channels []User
}

// Subscription represents a user's subscription to a specific Pokémon
type Subscription struct {
	UserID      int64 `gorm:"primaryKey;autoIncrement:false"`
	PokemonID   int   `gorm:"primaryKey;autoIncrement:false;type=smallint(5)"`
	MinIV       int   `gorm:"not null;default:0;type:tinyint(3)"`
	MinLevel    int   `gorm:"not null;default:0;type:tinyint(2)"`
	MaxDistance int   `gorm:"not null;default:0;type:mediumint(6)"`
}

// Encounter represents a tracked Pokémon encounter
type Encounter struct {
	ID         string `gorm:"primaryKey;autoIncrement:false;type:varchar(25)"`
	Expiration int    `gorm:"index;not null;type:int(10)"`
}

// Message represents a sent Telegram message for cleanup purposes
type Message struct {
	ChatID      int64  `gorm:"primaryKey;autoIncrement:false"`
	MessageID   int    `gorm:"primaryKey;autoIncrement:false"`
	EncounterID string `gorm:"index;not null;type:varchar(25)"`
}

// EncounterData represents the complete Pokémon encounter data from the scanner database
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
	PVPData                 PVP `gorm:"-"`
}

// GymData represents gym information for location services
type GymData struct {
	ID                     string
	Lat                    float64
	Lon                    float64
	Name                   *string
	Url                    *string
	LastModifiedTimestamp  *int
	RaidEndTimestamp       *int
	RaidSpawnTimestamp     *int
	RaidBattleTimestamp    *int
	Updated                int64
	RaidPokemonID          *int
	GuardingPokemonID      *int
	GuardingPokemonDisplay *string
	AvailableSlots         *int
	TeamID                 *int
	RaidLevel              *int
	Enabled                *int
	ExRaidEligible         *int
	InBattle               *int
	RaidPokemonMove1       *int
	RaidPokemonMove2       *int
	RaidPokemonForm        *int
	RaidPokemonAlignment   *int
	RaidPokemonCp          *int
	RaidIsExclusive        *int
	CellID                 *int
	Deleted                bool
	TotalCp                *int
	FirstSeenTimestamp     int64
	RaidPokemonGender      *int
	SponsorID              *int
	PartnerID              *string
	RaidPokemonCostume     *int
	RaidPokemonEvolution   *int
	ArScanEligible         *int
	PowerUpLevel           *int
	PowerUpPoints          *int
	PowerUpEndTimestamp    *int
	Description            *string
}

// MasterFile contains all game data from the master file
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

// Pokemon represents a Pokémon species with all its metadata
type Pokemon struct {
	Name          string          `json:"name"`
	PokedexID     int             `json:"pokedexId"`
	DefaultFormID int             `json:"defaultFormId"`
	Types         []int           `json:"types"`
	QuickMoves    []int           `json:"quickMoves"`
	ChargedMoves  []int           `json:"chargedMoves"`
	GenID         int             `json:"genId"`
	Generation    string          `json:"generation"`
	Forms         map[string]Form `json:"forms"`
	Height        float64         `json:"height"`
	Weight        float64         `json:"weight"`
	Family        int             `json:"family"`
	Legendary     bool            `json:"legendary"`
	Mythical      bool            `json:"mythical"`
	UltraBeast    bool            `json:"ultraBeast"`
}

// Form represents a Pokémon form variant
type Form struct {
	Name      string `json:"name"`
	IsCostume bool   `json:"isCostume,omitempty"`
}

// Move represents a Pokémon move
type Move struct {
	Name string `json:"name"`
}

// Weather represents weather conditions
type Weather struct {
	Name  string `json:"name"`
	Types []int  `json:"types"`
}

// PokemonEntry represents a PVP ranking entry
type PokemonEntry struct {
	Pokemon    int     `json:"pokemon"`
	Form       int     `json:"form,omitempty"`
	Cap        float64 `json:"cap,omitempty"`
	Value      float64 `json:"value,omitempty"`
	Level      float64 `json:"level"`
	CP         int     `json:"cp,omitempty"`
	Percentage float64 `json:"percentage"`
	Rank       int16   `json:"rank"`
	Capped     bool    `json:"capped,omitempty"`
	Evolution  int     `json:"evolution,omitempty"`
}

// PVP represents PVP rankings for different leagues
type PVP map[string][]PokemonEntry

// TableName returns the table name for EncounterData
func (EncounterData) TableName() string {
	return "pokemon"
}

// TableName returns the table name for GymData
func (GymData) TableName() string {
	return "gym"
}
