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
	tr := newTranslatorFor(c)
	clearConversationState(userID)
	return c.Send(tr.T("❌ Aborted"))
}

func handleAddSubscriptionPokemon(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	pokemonName := c.Text()

	pokemonID, err := getPokemonID(pokemonName)
	if err != nil {
		return c.Send(tr.Tf("❌ Can't find Pokedex # for Pokémon: %s", pokemonName))
	}

	userConversationStates[userID] = fmt.Sprintf("add_subscription_iv_%d", pokemonID)

	return c.Send(tr.Tf("📣 Subscribing to %s alerts. Please enter the minimal IV percentage (0-100):",
		tr.PokemonName(pokemonID),
	))
}

func handleAddSubscriptionIV(c telebot.Context, pokemonID int) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)

	minIV, err := validateIntInRange(c.Text(), 0, 100, tr.T("❌ Invalid input! Please enter a valid IV percentage (0-100)"))
	if err != nil {
		return c.Send(tr.T(err.Error()))
	}

	userConversationStates[userID] = fmt.Sprintf("add_subscription_level_%d_%d", pokemonID, minIV)

	return c.Send(tr.Tf("✨ Minimal IV set to %d%%. Please enter the minimal Pokémon level (0-40):", minIV))
}

func handleAddSubscriptionLevel(c telebot.Context, pokemonID, minIV int) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)

	minLevel, err := validateIntInRange(c.Text(), 0, 40, tr.T("❌ Invalid input! Please enter a valid level (0-40)"))
	if err != nil {
		return c.Send(tr.T(err.Error()))
	}

	userConversationStates[userID] = fmt.Sprintf("add_subscription_distance_%d_%d_%d", pokemonID, minIV, minLevel)

	return c.Send(tr.Tf("🔢 Minimal level set to %d. Please enter the maximal distance (in m):", minLevel))
}

func handleAddSubscriptionDistance(c telebot.Context, pokemonID, minIV, minLevel int) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)

	maxDistance, err := validateIntInRange(c.Text(), 0, 999999, tr.T("❌ Invalid input! Please enter a valid distance (in m)"))
	if err != nil {
		return c.Send(tr.T(err.Error()))
	}

	// Subscribe user to Pokémon
	addSubscription(getUserID(c), pokemonID, minIV, minLevel, maxDistance)
	clearConversationState(userID)

	return c.Send(tr.Tf("✅ Subscribed to %s alerts (Min IV: %d%%, Min Level: %d, Max Distance: %dm)",
		tr.PokemonName(pokemonID),
		minIV, minLevel, maxDistance,
	))
}

func handleSetDistanceInput(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)

	maxDistance, err := validateIntInRange(c.Text(), 0, 999999, tr.T("❌ Invalid input! Please enter a valid distance (in m)"))
	if err != nil {
		return c.Send(tr.T(err.Error()))
	}

	// Update max distance in the database
	updateUserPreference(getUserID(c), "MaxDistance", maxDistance)
	clearConversationState(userID)

	return c.Send(tr.Tf("✅ Maximal distance updated to %dm", maxDistance))
}

func handleSetMinIVInput(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)

	minIV, err := validateIntInRange(c.Text(), 0, 100, tr.T("❌ Invalid input! Please enter a valid IV percentage (0-100)"))
	if err != nil {
		return c.Send(tr.T(err.Error()))
	}

	// Update min IV in the database
	updateUserPreference(getUserID(c), "MinIV", minIV)
	clearConversationState(userID)

	if minIV == 0 {
		return c.Send(tr.T("✅ Minimal IV reset"))
	}
	return c.Send(tr.Tf("✅ Minimal IV updated to %d%%", minIV))
}

func handleSetMinLevelInput(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)

	minLevel, err := validateIntInRange(c.Text(), 0, 40, tr.T("❌ Invalid input! Please enter a valid level (0-40)"))
	if err != nil {
		return c.Send(tr.T(err.Error()))
	}

	// Update min level in the database
	updateUserPreference(getUserID(c), "MinLevel", minLevel)
	clearConversationState(userID)

	if minLevel == 0 {
		return c.Send(tr.T("✅ Minimal Level reset"))
	}
	return c.Send(tr.Tf("✅ Minimal Level updated to %d", minLevel))
}

func handleBroadcastInput(c telebot.Context) error {
	if unauthorized, err := requireAdmin(c, c.Send); unauthorized {
		return err
	}
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	message := c.Text()
	for _, user := range userCache.All {
		if user.Notify {
			bot.Send(&telebot.User{ID: user.ID}, message, telebot.ModeMarkdown)
		}
	}

	clearConversationState(userID)

	return c.Send(tr.T("📢 Broadcast sent to all users"))
}

func handleImpersonateUserInput(c telebot.Context) error {
	if unauthorized, err := requireAdmin(c, c.Send); unauthorized {
		return err
	}
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	impersonatedUserID, err := strconv.Atoi(c.Text())
	if err != nil {
		return c.Send(tr.T("❌ Invalid user ID"))
	}

	clearConversationState(userID)

	adminImpersonation[userID] = int64(impersonatedUserID)
	user := getUserPreferences(int64(impersonatedUserID))
	settingsMessage, replyMarkup := buildSettings(user)

	return c.Send(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

// ── Raid subscription wizard ──────────────────────────────────────────────────

func handleAddRaidSubscriptionPokemon(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	name := strings.TrimSpace(c.Text())

	if strings.ToLower(name) == "all" || strings.ToLower(name) == "alle" {
		userConversationStates[userID] = "add_raid_subscription_level_0"
		return c.Send(tr.Tf("⚔️ Subscribing to raids. Enter the raid level (1-19):"))
	}

	pokemonID, err := getPokemonID(name)
	if err != nil {
		return c.Send(tr.Tf("❌ Can't find Pokedex # for Pokémon: %s", name))
	}

	userConversationStates[userID] = fmt.Sprintf("add_raid_subscription_level_%d", pokemonID)
	return c.Send(tr.Tf("⚔️ Subscribing to %s raids. Enter the raid level (1-19, or 0 for any):",
		tr.PokemonName(pokemonID),
	))
}

func handleAddRaidSubscriptionLevel(c telebot.Context, pokemonID int) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)

	raidLevel, err := validateIntInRange(c.Text(), 0, 19, tr.T("❌ Invalid raid level! Please enter a level between 1 and 19"))
	if err != nil {
		return c.Send(tr.T(err.Error()))
	}

	if pokemonID == 0 {
		// "all" path — raidLevel must be > 0; creates a level-only subscription (pkm=0, level=N).
		if raidLevel == 0 {
			return c.Send(tr.T("❌ To subscribe to all raids regardless of level, use /settings → ") + tr.T("⚔️ Enable Notifications for all Raids"))
		}
		addRaidSubscription(getUserID(c), 0, raidLevel)
		clearConversationState(userID)
		return c.Send(tr.Tf("✅ Subscribed to all level %d raids", raidLevel))
	}

	addRaidSubscription(getUserID(c), pokemonID, raidLevel)
	clearConversationState(userID)
	if raidLevel == 0 {
		return c.Send(tr.Tf("✅ Subscribed to %s raids (any level)", tr.PokemonName(pokemonID)))
	}
	return c.Send(tr.Tf("✅ Subscribed to %s raids (Level: %d)", tr.PokemonName(pokemonID), raidLevel))
}

func handleSetRaidMinLevelInput(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)

	raidLevel, err := validateIntInRange(c.Text(), 0, 19, tr.T("❌ Invalid raid level! Please enter a level between 0 and 19"))
	if err != nil {
		return c.Send(tr.T(err.Error()))
	}

	updateUserPreference(getUserID(c), "RaidMinLevel", raidLevel)
	clearConversationState(userID)
	if raidLevel == 0 {
		return c.Send(tr.T("🔢 Raid Minimal Level reset"))
	}
	return c.Send(tr.Tf("🔢 Raid Minimal Level updated to %d", raidLevel))
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

	case conversationState == "add_raid_subscription":
		return handleAddRaidSubscriptionPokemon(c)

	case strings.HasPrefix(conversationState, "add_raid_subscription_level"):
		parts := strings.Split(conversationState, "_")
		pokemonID, _ := strconv.Atoi(parts[len(parts)-1])
		return handleAddRaidSubscriptionLevel(c, pokemonID)

	case conversationState == "set_raid_min_level":
		return handleSetRaidMinLevelInput(c)
	}

	return nil
}
