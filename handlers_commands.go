package main

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/telebot.v3"
)

// ── Shared helpers ────────────────────────────────────────────────────────────

// getUserID returns the effective user ID, handling admin impersonation.
func getUserID(c telebot.Context) int64 {
	userID := c.Sender().ID
	if impersonatedID, ok := botAdmins[userID]; ok && impersonatedID != userID {
		language := userCache.All[userID].Language
		c.Send(getTranslation("🔒 You are impersonating another user", language))
		return impersonatedID
	}
	return userID
}

// toggleUserPreference flips a boolean user preference and refreshes the settings UI.
func toggleUserPreference(c telebot.Context, field string, toggle func(user *User) bool) error {
	user := getUserPreferences(getUserID(c))
	newValue := toggle(&user)
	updateUserPreference(user.ID, field, newValue)
	settingsMessage, replyMarkup := buildSettings(user)
	return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

// buildSettings constructs the settings message and inline keyboard for a user.
func buildSettings(user User) (string, *telebot.ReplyMarkup) {
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

	pvpText := getTranslation("🏅 Disable Top PVP Notifications", user.Language)
	if !user.TopPVP {
		pvpText = getTranslation("🏅 Enable Top PVP Notifications", user.Language)
	}
	btnToogleTopPVP := telebot.InlineButton{Text: pvpText, Unique: "toggle_top_pvp"}

	cleanupText := getTranslation("🗑️ Keep Expired Notifications", user.Language)
	if !user.Cleanup {
		cleanupText = getTranslation("🗑️ Remove Expired Notifications", user.Language)
	}
	btnToggleCleanup := telebot.InlineButton{Text: cleanupText, Unique: "toggle_cleanup"}
	btnClose := telebot.InlineButton{Text: getTranslation("Close", user.Language), Unique: "close"}

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
			getTranslation("🏅 *Top PVP Notifications:* %s", user.Language)+"\n"+
			getTranslation("🗑️ *Cleanup Expired Notifications:* %s", user.Language)+"\n\n"+
			getTranslation("Use the buttons below to update the settings", user.Language),
		user.Language, user.Latitude, user.Longitude,
		user.MaxDistance, user.MinIV, user.MinLevel,
		boolToEmoji(user.Notify), boolToEmoji(user.Stickers),
		boolToEmoji(user.HundoIV), boolToEmoji(user.ZeroIV),
		boolToEmoji(user.TopPVP), boolToEmoji(user.Cleanup),
	)

	if strings.HasPrefix(strconv.FormatInt(user.ID, 10), "-100") {
		chatInfo, _ := bot.ChatByID(user.ID)
		settingsMessage = fmt.Sprintf(
			getTranslation("⚙️ *Channel Settings:*", user.Language)+"\n"+
				"----------------------------------------------\n"+
				getTranslation("#️⃣ *Channel ID:* %d", user.Language)+"\n"+
				getTranslation("#️⃣ *Channel Name:* %s", user.Language)+"\n"+
				getTranslation("🌍 *Language:* %s", user.Language)+"\n"+
				getTranslation("✨ *Minimal IV:* %d%%", user.Language)+"\n"+
				getTranslation("🔢 *Minimal Level:* %d", user.Language)+"\n"+
				getTranslation("🔔 *Notifications:* %s", user.Language)+"\n"+
				getTranslation("🎭 *Pokémon Stickers:* %s", user.Language)+"\n"+
				getTranslation("💯 *100%% IV Notifications:* %s", user.Language)+"\n"+
				getTranslation("🚫 *0%% IV Notifications:* %s", user.Language)+"\n"+
				getTranslation("🏅 *Top PVP Notifications:* %s", user.Language)+"\n"+
				getTranslation("🗑️ *Cleanup Expired Notifications:* %s", user.Language)+"\n\n"+
				getTranslation("Use the buttons below to update the settings", user.Language),
			user.ID, chatInfo.Title, user.Language, user.MinIV, user.MinLevel,
			boolToEmoji(user.Notify), boolToEmoji(user.Stickers),
			boolToEmoji(user.HundoIV), boolToEmoji(user.ZeroIV),
			boolToEmoji(user.TopPVP), boolToEmoji(user.Cleanup),
		)
	}

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
		{btnToogleTopPVP},
		{btnToggleCleanup},
		{btnClose},
	}

	if strings.HasPrefix(strconv.FormatInt(user.ID, 10), "-100") {
		btnReset := telebot.InlineButton{Text: getTranslation("🔄 Reset", user.Language), Unique: "reset"}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnReset})
	} else if _, ok := botAdmins[user.ID]; ok {
		btnBroadcast := telebot.InlineButton{Text: getTranslation("📢 Broadcast Message", user.Language), Unique: "broadcast"}
		btnListChannels := telebot.InlineButton{Text: getTranslation("📋 List Channels", user.Language), Unique: "list_channels"}
		btnListUsers := telebot.InlineButton{Text: getTranslation("📋 List Users", user.Language), Unique: "list_users"}
		btnImpersonateUser := telebot.InlineButton{Text: getTranslation("👤 Impersonate User", user.Language), Unique: "impersonate_user"}
		inlineKeyboard = append(inlineKeyboard,
			[]telebot.InlineButton{btnBroadcast, btnImpersonateUser},
			[]telebot.InlineButton{btnListUsers, btnListChannels},
		)
	}

	return settingsMessage, &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}
}

// ── Command handlers ──────────────────────────────────────────────────────────

func handleStart(c telebot.Context) error {
	user := getUserPreferences(getUserID(c))

	detectedLanguage := c.Sender().LanguageCode
	if detectedLanguage != "en" && detectedLanguage != "de" {
		detectedLanguage = "en"
	}
	updateUserPreference(user.ID, "Language", detectedLanguage)

	startMessage := fmt.Sprintf(
		getTranslation("👋 Welcome to the PoGo Notification Bot!", detectedLanguage)+"\n\n"+
			getTranslation("ℹ️ Language detected: *%s*", detectedLanguage)+"\n"+
			getTranslation("ℹ️ Use /settings to update your preferences", detectedLanguage)+"\n"+
			getTranslation("ℹ️ Use /subscribe <pokemon-name> [min-iv] [min-level] [max-distance] to get notified about specific Pokémon", detectedLanguage)+"\n"+
			getTranslation("ℹ️ Send me your 📍 location to enable distance-based notifications", detectedLanguage),
		detectedLanguage,
	)

	return c.Send(startMessage)
}

func handleHelp(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	helpMessage := getTranslation("🤖 PoGo Notification Bot Commands:", language) + "\n\n" +
		getTranslation("🔔 /settings - Update your preferences", language) + "\n" +
		getTranslation("📋 /list - List your Pokémon subscriptions", language) + "\n" +
		getTranslation("📣 /subscribe <pokemon-name> [min-iv] [min-level] [max-distance] - Subscribe to Pokémon alerts", language) + "\n" +
		getTranslation("🚫 /unsubscribe <pokemon-name> - Unsubscribe from Pokémon alerts", language)
	return c.Send(helpMessage, telebot.ModeMarkdown)
}

func handleSettings(c telebot.Context) error {
	userID := getUserID(c)
	user := getUserPreferences(userID)
	settingsMessage, replyMarkup := buildSettings(user)
	return c.Send(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

func handleSubscribe(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language

	args := c.Args()
	if len(args) < 1 {
		return c.Send(getTranslation("ℹ️ Usage: /subscribe <pokemon-name> [min-iv] [min-level] [max-distance]", language))
	}

	// Collect trailing numeric args so Pokémon names with spaces (e.g. "Jangmo O") work.
	numericArgCount := 0
	for argIndex := len(args) - 1; argIndex >= 1 && numericArgCount < 3; argIndex-- {
		if _, err := strconv.Atoi(args[argIndex]); err == nil {
			numericArgCount++
		} else {
			break
		}
	}
	nameParts := args[:len(args)-numericArgCount]
	numericArgs := args[len(args)-numericArgCount:]

	pokemonName := strings.Join(nameParts, " ")
	pokemonID, err := getPokemonID(pokemonName)
	if err != nil {
		return c.Send(fmt.Sprintf(getTranslation("❌ Can't find Pokedex # for Pokémon: %s", language), pokemonName))
	}

	minIV := 0
	minLevel := 0
	maxDistance := 0
	if len(numericArgs) > 0 {
		minIV, err = strconv.Atoi(numericArgs[0])
		if err != nil || minIV < 0 || minIV > 100 {
			return c.Send(getTranslation("❌ Invalid input! Please enter a valid IV percentage (0-100)", language))
		}
	}
	if len(numericArgs) > 1 {
		minLevel, err = strconv.Atoi(numericArgs[1])
		if err != nil || minLevel < 0 || minLevel > 40 {
			return c.Send(getTranslation("❌ Invalid input! Please enter a valid level (0-40)", language))
		}
	}
	if len(numericArgs) > 2 {
		maxDistance, err = strconv.Atoi(numericArgs[2])
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
}

func handleList(c telebot.Context) error {
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

	subscriptions := getUserSubscriptions(user.ID)

	if len(subscriptions) == 0 {
		return c.Send(getTranslation("🔹 You have no specific Pokémon subscriptions", user.Language))
	}

	for _, subscription := range subscriptions {
		entry := fmt.Sprintf(getTranslation("🔹 %s (Min IV: %d%%, Min Level: %d, Max Distance: %dm)", user.Language)+"\n",
			getPokemonName(subscription.PokemonID, user.Language),
			subscription.MinIV, subscription.MinLevel, subscription.MaxDistance,
		)
		if text.Len()+len(entry) > 4000 {
			c.Send(text.String())
			text.Reset()
		}
		text.WriteString(entry)
	}
	return c.Send(text.String())
}

func handleUnsubscribe(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language

	args := c.Args()
	if len(args) < 1 {
		return c.Send(getTranslation("ℹ️ Usage: /unsubscribe <pokemon-name>", language))
	}

	pokemonName := strings.Join(args, " ")
	pokemonID, err := getPokemonID(pokemonName)
	if err != nil {
		return c.Send(fmt.Sprintf(getTranslation("❌ Can't find Pokedex # for Pokémon: %s", language), pokemonName))
	}

	deleteSubscription(userID, pokemonID)
	getActiveSubscriptions()

	user := getUserPreferences(userID)
	return c.Send(fmt.Sprintf(getTranslation("✅ Unsubscribed from %s alerts", language), getPokemonName(pokemonID, user.Language)))
}

func handleLocate(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language

	args := c.Args()
	if len(args) < 1 {
		return c.Send(getTranslation("ℹ️ Usage: /locate <gym-name>", language))
	}

	gymName := strings.Join(args, " ")
	gyms := searchGymsByName(gymName)

	if len(gyms) == 0 {
		return c.Send(fmt.Sprintf(getTranslation("❌ Can't find gym: %s", language), gymName))
	} else if len(gyms) > 1 {
		responseText := fmt.Sprintf(getTranslation("🔍 Found %d gyms matching your search:", language), len(gyms))
		inlineKeyboard := [][]telebot.InlineButton{}
		for _, gym := range gyms {
			btnGym := telebot.InlineButton{
				Text:   *gym.Name,
				Unique: "locate_gym",
				Data:   gym.ID,
			}
			inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnGym})
		}
		btnClose := telebot.InlineButton{Text: getTranslation("Close", language), Unique: "close"}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnClose})
		return c.Send(responseText, &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}, telebot.ModeMarkdown)
	}

	foundGym := gyms[0]
	return c.Send(&telebot.Venue{Location: telebot.Location{Lat: float32(foundGym.Lat), Lng: float32(foundGym.Lon)}, Title: *foundGym.Name})
}

func handleReset(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	if _, ok := botAdmins[userID]; !ok {
		return c.Send(getTranslation("❌ You are not authorized to use this command", language))
	}
	if botAdmins[userID] == userID {
		return c.Send(getTranslation("🔒 You are not impersonating another user", language), telebot.ModeMarkdown)
	}
	botAdmins[userID] = userID
	return c.Send(getTranslation("🔒 You are now back as yourself", language))
}

// handleWoCommand is an alias for the /locate command.
func handleWoCommand(c telebot.Context) error {
	return bot.Trigger("/locate", c)
}

func handleLocationMessage(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language
	location := c.Message().Location

	updateUserPreference(userID, "Latitude", location.Lat)
	updateUserPreference(userID, "Longitude", location.Lng)

	return c.Send(getTranslation("📍 Location updated! Your preferences will now consider this", language))
}

// ── Bot handler registration ──────────────────────────────────────────────────

func setupBotHandlers() {
	// Command handlers
	bot.Handle("/start", handleStart)
	bot.Handle("/help", handleHelp)
	bot.Handle("/settings", handleSettings)
	bot.Handle("/subscribe", handleSubscribe)
	bot.Handle("/list", handleList)
	bot.Handle("/unsubscribe", handleUnsubscribe)
	bot.Handle("/locate", handleLocate)
	bot.Handle("/reset", handleReset)
	bot.Handle("/wo", handleWoCommand)

	// Message handlers
	bot.Handle(telebot.OnLocation, handleLocationMessage)

	// Callback handlers
	bot.Handle(&telebot.InlineButton{Unique: "locate_gym"}, handleLocateGymCallback)
	bot.Handle(&telebot.InlineButton{Unique: "reset"}, handleResetCallback)
	bot.Handle(&telebot.InlineButton{Unique: "close"}, handleCloseCallback)
	bot.Handle(&telebot.InlineButton{Unique: "add_subscription"}, handleAddSubscriptionCallback)
	bot.Handle(&telebot.InlineButton{Unique: "list_subscriptions"}, handleListSubscriptionsCallback)
	bot.Handle(&telebot.InlineButton{Unique: "clear_subscriptions"}, handleClearSubscriptionsCallback)
	bot.Handle(&telebot.InlineButton{Unique: "toggle_notifications"}, handleToggleNotificationsCallback)
	bot.Handle(&telebot.InlineButton{Unique: "toggle_stickers"}, handleToggleStickersCallback)
	bot.Handle(&telebot.InlineButton{Unique: "toggle_hundo_iv"}, handleToggleHundoIVCallback)
	bot.Handle(&telebot.InlineButton{Unique: "toggle_zero_iv"}, handleToggleZeroIVCallback)
	bot.Handle(&telebot.InlineButton{Unique: "toggle_top_pvp"}, handleToggleTopPVPCallback)
	bot.Handle(&telebot.InlineButton{Unique: "toggle_cleanup"}, handleToggleCleanupCallback)
	bot.Handle(&telebot.InlineButton{Unique: "change_lang"}, handleChangeLangCallback)
	bot.Handle(&telebot.InlineButton{Unique: "set_lang_en"}, handleSetLangEnCallback)
	bot.Handle(&telebot.InlineButton{Unique: "set_lang_de"}, handleSetLangDeCallback)
	bot.Handle(&telebot.InlineButton{Unique: "update_location"}, handleUpdateLocationCallback)
	bot.Handle(&telebot.InlineButton{Unique: "set_distance"}, handleSetDistanceCallback)
	bot.Handle(&telebot.InlineButton{Unique: "set_min_iv"}, handleSetMinIVCallback)
	bot.Handle(&telebot.InlineButton{Unique: "set_min_level"}, handleSetMinLevelCallback)
	bot.Handle(&telebot.InlineButton{Unique: "broadcast"}, handleBroadcastCallback)
	bot.Handle(&telebot.InlineButton{Unique: "list_users"}, handleListUsersCallback)
	bot.Handle(&telebot.InlineButton{Unique: "list_channels"}, handleListChannelsCallback)
	bot.Handle(&telebot.InlineButton{Unique: "edit_channel"}, handleEditChannelCallback)
	bot.Handle(&telebot.InlineButton{Unique: "impersonate_user"}, handleImpersonateUserCallback)

	// Text input (conversation state machine)
	bot.Handle(telebot.OnText, handleTextInput)
}
