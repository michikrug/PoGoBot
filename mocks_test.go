package main

import (
	"github.com/stretchr/testify/mock"
	"gopkg.in/telebot.v3"
)

// ── mockBotDB ─────────────────────────────────────────────────────────────────

type mockBotDB struct {
	mock.Mock
}

func (m *mockBotDB) GetUserPreferences(userID int64) User {
	args := m.Called(userID)
	return args.Get(0).(User)
}

func (m *mockBotDB) UpdateUserPreference(userID int64, field string, value interface{}) {
	m.Called(userID, field, value)
}

func (m *mockBotDB) AddSubscription(userID int64, pokemonID, minIV, minLevel, maxDistance int) {
	m.Called(userID, pokemonID, minIV, minLevel, maxDistance)
}

func (m *mockBotDB) GetUsers() []User {
	args := m.Called()
	return args.Get(0).([]User)
}

func (m *mockBotDB) GetSubscriptions() []Subscription {
	args := m.Called()
	return args.Get(0).([]Subscription)
}

func (m *mockBotDB) SaveMessage(chatID int64, messageID int, encounterID string) {
	m.Called(chatID, messageID, encounterID)
}

func (m *mockBotDB) SaveEncounter(encounterID string, expiration int64) {
	m.Called(encounterID, expiration)
}

func (m *mockBotDB) GetUserSubscriptions(userID int64) []Subscription {
	args := m.Called(userID)
	return args.Get(0).([]Subscription)
}

func (m *mockBotDB) DeleteSubscription(userID int64, pokemonID int) {
	m.Called(userID, pokemonID)
}

func (m *mockBotDB) DeleteAllUserSubscriptions(userID int64) {
	m.Called(userID)
}

func (m *mockBotDB) GetExpiredEncountersWithMessages() ([]Encounter, map[string][]Message) {
	args := m.Called()
	return args.Get(0).([]Encounter), args.Get(1).(map[string][]Message)
}

func (m *mockBotDB) DeleteMessage(message Message) {
	m.Called(message)
}

func (m *mockBotDB) DeleteEncounter(encounter Encounter) {
	m.Called(encounter)
}

// ── mockBotDB: raid subscription methods ──────────────────────────────────────

func (m *mockBotDB) AddRaidSubscription(userID int64, pokemonID, raidLevel int) {
	m.Called(userID, pokemonID, raidLevel)
}

func (m *mockBotDB) GetRaidSubscriptions() []RaidSubscription {
	args := m.Called()
	return args.Get(0).([]RaidSubscription)
}

func (m *mockBotDB) GetUserRaidSubscriptions(userID int64) []RaidSubscription {
	args := m.Called(userID)
	return args.Get(0).([]RaidSubscription)
}

func (m *mockBotDB) DeleteRaidSubscription(userID int64, pokemonID, raidLevel int) {
	m.Called(userID, pokemonID, raidLevel)
}

func (m *mockBotDB) DeleteAllUserRaidSubscriptions(userID int64) {
	m.Called(userID)
}

// ── mockScannerDB ─────────────────────────────────────────────────────────────

type mockScannerDB struct {
	mock.Mock
}

func (m *mockScannerDB) GetRecentEncounters() ([]EncounterData, error) {
	args := m.Called()
	return args.Get(0).([]EncounterData), args.Error(1)
}

func (m *mockScannerDB) SearchGymsByName(name string) []GymData {
	args := m.Called(name)
	return args.Get(0).([]GymData)
}

func (m *mockScannerDB) GetGymByID(id string) GymData {
	args := m.Called(id)
	return args.Get(0).(GymData)
}

func (m *mockScannerDB) GetActiveRaids() ([]GymData, error) {
	args := m.Called()
	return args.Get(0).([]GymData), args.Error(1)
}

// ── mockBotSender ─────────────────────────────────────────────────────────────

type mockBotSender struct {
	mock.Mock
}

func (m *mockBotSender) Send(to telebot.Recipient, what interface{}, opts ...interface{}) (*telebot.Message, error) {
	// Collect all args including variadic opts for assertion support.
	allArgs := []interface{}{to, what}
	allArgs = append(allArgs, opts...)
	args := m.Called(allArgs...)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*telebot.Message), args.Error(1)
}

func (m *mockBotSender) Delete(msg telebot.Editable) error {
	args := m.Called(msg)
	return args.Error(0)
}
