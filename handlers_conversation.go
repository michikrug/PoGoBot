package main

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/telebot.v3"
)

// ── Validation helpers ────────────────────────────────────────────────────────

func validateIntInRange(text string, min, max int, errorMsg string) (int, error) {
	var parsedValue int
	_, err := fmt.Sscanf(text, "%d", &parsedValue)
	if err != nil || parsedValue < min || parsedValue > max {
		return 0, fmt.Errorf("%s", errorMsg)
	}
	return parsedValue, nil
}

func isConversationCancelled(text string) bool {
	return strings.ToLower(text) == "abbruch" || strings.ToLower(text) == "cancel"
}

func clearConversationState(userID int64) {
	userConversationStates[userID] = ""
}

// ── Conversation state handlers ───────────────────────────────────────────────

func handleCancelConversation(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	clearConversationState(userID)
	return c.Send(getTranslation("❌ Aborted", language))
}

func handleAddSubscriptionPokemon(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	pokemonName := c.Text()

	pokemonID, err := getPokemonID(pokemonName)
	if err != nil {
		return c.Send(fmt.Sprintf(getTranslation("❌ Can't find Pokedex # for Pokémon: %s", language), pokemonName))
	}

	userConversationStates[userID] = fmt.Sprintf("add_subscription_iv_%d", pokemonID)

	return c.Send(fmt.Sprintf(getTranslation("📣 Subscribing to %s alerts. Please enter the minimal IV percentage (0-100):", language),
		getPokemonName(pokemonID, language),
	))
}

func handleAddSubscriptionIV(c telebot.Context, pokemonID int) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language

	minIV, err := validateIntInRange(c.Text(), 0, 100, "❌ Invalid input! Please enter a valid IV percentage (0-100)")
	if err != nil {
		return c.Send(getTranslation(err.Error(), language))
	}

	userConversationStates[userID] = fmt.Sprintf("add_subscription_level_%d_%d", pokemonID, minIV)

	return c.Send(fmt.Sprintf(getTranslation("✨ Minimal IV set to %d%%. Please enter the minimal Pokémon level (0-40):", language), minIV))
}

func handleAddSubscriptionLevel(c telebot.Context, pokemonID, minIV int) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language

	minLevel, err := validateIntInRange(c.Text(), 0, 40, "❌ Invalid input! Please enter a valid level (0-40)")
	if err != nil {
		return c.Send(getTranslation(err.Error(), language))
	}

	userConversationStates[userID] = fmt.Sprintf("add_subscription_distance_%d_%d_%d", pokemonID, minIV, minLevel)

	return c.Send(fmt.Sprintf(getTranslation("🔢 Minimal level set to %d. Please enter the maximal distance (in m):", language), minLevel))
}

func handleAddSubscriptionDistance(c telebot.Context, pokemonID, minIV, minLevel int) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language

	maxDistance, err := validateIntInRange(c.Text(), 0, 999999, "❌ Invalid input! Please enter a valid distance (in m)")
	if err != nil {
		return c.Send(getTranslation(err.Error(), language))
	}

	// Subscribe user to Pokémon
	addSubscription(getUserID(c), pokemonID, minIV, minLevel, maxDistance)
	clearConversationState(userID)

	return c.Send(fmt.Sprintf(getTranslation("✅ Subscribed to %s alerts (Min IV: %d%%, Min Level: %d, Max Distance: %dm)", language),
		getPokemonName(pokemonID, language),
		minIV, minLevel, maxDistance,
	))
}

func handleSetDistanceInput(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language

	maxDistance, err := validateIntInRange(c.Text(), 0, 999999, "❌ Invalid input! Please enter a valid distance (in m)")
	if err != nil {
		return c.Send(getTranslation(err.Error(), language))
	}

	// Update max distance in the database
	updateUserPreference(getUserID(c), "MaxDistance", maxDistance)
	clearConversationState(userID)

	return c.Send(fmt.Sprintf(getTranslation("✅ Maximal distance updated to %dm", language), maxDistance))
}

func handleSetMinIVInput(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language

	minIV, err := validateIntInRange(c.Text(), 0, 100, "❌ Invalid input! Please enter a valid IV percentage (0-100)")
	if err != nil {
		return c.Send(getTranslation(err.Error(), language))
	}

	// Update min IV in the database
	updateUserPreference(getUserID(c), "MinIV", minIV)
	clearConversationState(userID)

	return c.Send(fmt.Sprintf(getTranslation("✅ Minimal IV updated to %d%%", language), minIV))
}

func handleSetMinLevelInput(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language

	minLevel, err := validateIntInRange(c.Text(), 0, 40, "❌ Invalid input! Please enter a valid level (0-40)")
	if err != nil {
		return c.Send(getTranslation(err.Error(), language))
	}

	// Update min level in the database
	updateUserPreference(getUserID(c), "MinLevel", minLevel)
	clearConversationState(userID)

	return c.Send(fmt.Sprintf(getTranslation("✅ Minimal Level updated to %d", language), minLevel))
}

func handleBroadcastInput(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language

	if _, ok := botAdmins[userID]; !ok {
		return c.Send(getTranslation("❌ You are not authorized to use this command", language))
	}

	message := c.Text()
	for _, user := range userCache.All {
		if user.Notify {
			bot.Send(&telebot.User{ID: user.ID}, message, telebot.ModeMarkdown)
		}
	}

	clearConversationState(userID)

	return c.Send(getTranslation("📢 Broadcast sent to all users", language))
}

func handleImpersonateUserInput(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language

	if _, ok := botAdmins[userID]; !ok {
		return c.Send(getTranslation("❌ You are not authorized to use this command", language))
	}

	impersonatedUserID, err := strconv.Atoi(c.Text())
	if err != nil {
		return c.Send(getTranslation("❌ Invalid user ID", language))
	}

	clearConversationState(userID)

	botAdmins[userID] = int64(impersonatedUserID)
	user := getUserPreferences(int64(impersonatedUserID))
	settingsMessage, replyMarkup := buildSettings(user)

	return c.Send(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

func handleTextInput(c telebot.Context) error {
	userID := c.Sender().ID
	conversationState := userConversationStates[userID]

	// Handle conversation cancellation
	if conversationState != "" && isConversationCancelled(c.Text()) {
		return handleCancelConversation(c)
	}

	// Route to appropriate state handler
	switch {
	case conversationState == "add_subscription":
		return handleAddSubscriptionPokemon(c)

	case strings.HasPrefix(conversationState, "add_subscription_iv"):
		pokemonID, _ := strconv.Atoi(strings.Split(conversationState, "_")[3])
		return handleAddSubscriptionIV(c, pokemonID)

	case strings.HasPrefix(conversationState, "add_subscription_level"):
		parts := strings.Split(conversationState, "_")
		pokemonID, _ := strconv.Atoi(parts[3])
		minIV, _ := strconv.Atoi(parts[4])
		return handleAddSubscriptionLevel(c, pokemonID, minIV)

	case strings.HasPrefix(conversationState, "add_subscription_distance"):
		parts := strings.Split(conversationState, "_")
		pokemonID, _ := strconv.Atoi(parts[3])
		minIV, _ := strconv.Atoi(parts[4])
		minLevel, _ := strconv.Atoi(parts[5])
		return handleAddSubscriptionDistance(c, pokemonID, minIV, minLevel)

	case conversationState == "set_distance":
		return handleSetDistanceInput(c)

	case conversationState == "set_min_iv":
		return handleSetMinIVInput(c)

	case conversationState == "set_min_level":
		return handleSetMinLevelInput(c)

	case conversationState == "broadcast":
		return handleBroadcastInput(c)

	case conversationState == "impersonate_user":
		return handleImpersonateUserInput(c)
	}

	return nil
}
