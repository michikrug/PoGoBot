package main

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/telebot.v3"
)

// Helper function to get user ID, handling admin impersonation
func getUserID(c telebot.Context) int64 {
	userID := c.Sender().ID
	if adminID, ok := botAdmins[userID]; ok && adminID != userID {
		language := userCache.All[userID].Language
		c.Send(getTranslation("🔒 You are impersonating another user", language))
		return adminID
	}
	return userID
}

// Helper function to toggle a boolean user preference field and refresh settings
func toggleUserPreference(c telebot.Context, field string, toggle func(user *User) bool) error {
	user := getUserPreferences(getUserID(c))
	newVal := toggle(&user)
	updateUserPreference(user.ID, field, newVal)
	settingsMessage, replyMarkup := buildSettings(user)
	return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

// Build the settings UI with interactive buttons
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
		chat, _ := bot.ChatByID(user.ID)
		// Settings message
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
			user.ID, chat.Title, user.Language, user.MinIV, user.MinLevel,
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
		// Admin-only buttons
		btnBroadcast := telebot.InlineButton{Text: getTranslation("📢 Broadcast Message", user.Language), Unique: "broadcast"}
		btnListChannels := telebot.InlineButton{Text: getTranslation("📋 List Channels", user.Language), Unique: "list_channels"}
		btnListUsers := telebot.InlineButton{Text: getTranslation("📋 List Users", user.Language), Unique: "list_users"}
		btnImpersonateUser := telebot.InlineButton{Text: getTranslation("👤 Impersonate User", user.Language), Unique: "impersonate_user"}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnBroadcast, btnImpersonateUser}, []telebot.InlineButton{btnListUsers, btnListChannels})
	}

	return settingsMessage, &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}
}

// Command Handlers - extracted from setupBotHandlers for better organization

// handleSubscribe handles the /subscribe command
func handleSubscribe(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language

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
}

// handleList handles the /list command
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

	subs := getUserSubscriptions(user.ID)

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
}

// handleUnsubscribe handles the /unsubscribe command
func handleUnsubscribe(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language

	args := c.Args()
	if len(args) < 1 {
		return c.Send(getTranslation("ℹ️ Usage: /unsubscribe <pokemon-name>", language))
	}

	pokemonName := args[0]
	pokemonID, err := getPokemonID(pokemonName)
	if err != nil {
		return c.Send(fmt.Sprintf(getTranslation("❌ Can't find Pokedex # for Pokémon: %s", language), pokemonName))
	}

	deleteSubscription(userID, pokemonID)
	getActiveSubscriptions()

	user := getUserPreferences(userID)
	return c.Send(fmt.Sprintf(getTranslation("✅ Unsubscribed from %s alerts", language), getPokemonName(pokemonID, user.Language)))
}

// handleLocate handles the /locate command
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
		text := fmt.Sprintf(getTranslation("🔍 Found %d gyms matching your search:", language), len(gyms))
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

		return c.Send(text, &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}, telebot.ModeMarkdown)
	}
	gym := gyms[0]
	return c.Send(&telebot.Venue{Location: telebot.Location{Lat: float32(gym.Lat), Lng: float32(gym.Lon)}, Title: *gym.Name})
}

// handleStart handles the /start command
func handleStart(c telebot.Context) error {
	user := getUserPreferences(getUserID(c))

	lang := c.Sender().LanguageCode // Auto-detect Telegram locale
	if lang != "en" && lang != "de" {
		lang = "en"
	}
	updateUserPreference(user.ID, "Language", lang)

	// Welcome message
	startMessage := fmt.Sprintf(
		getTranslation("👋 Welcome to the PoGo Notification Bot!", lang)+"\n\n"+
			getTranslation("ℹ️ Language detected: *%s*", lang)+"\n"+
			getTranslation("ℹ️ Use /settings to update your preferences", lang)+"\n"+
			getTranslation("ℹ️ Use /subscribe <pokemon-name> [min-iv] [min-level] [max-distance] to get notified about specific Pokémon", lang)+"\n"+
			getTranslation("ℹ️ Send me your 📍 location to enable distance-based notifications", lang),
		lang,
	)

	return c.Send(startMessage)
}

// handleSettings handles the /settings command
func handleSettings(c telebot.Context) error {
	userID := getUserID(c)
	user := getUserPreferences(userID)
	settingsMessage, replyMarkup := buildSettings(user)
	return c.Send(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

// handleHelp handles the /help command
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

// handleReset handles the /reset command (admin function for impersonation)
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

// handleLocationMessage handles location messages sent by users
func handleLocationMessage(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language
	location := c.Message().Location

	updateUserPreference(userID, "Latitude", location.Lat)
	updateUserPreference(userID, "Longitude", location.Lng)

	return c.Send(getTranslation("📍 Location updated! Your preferences will now consider this", language))
}

// Callback Handlers

// handleLocateGymCallback handles the inline button callback for gym selection
func handleLocateGymCallback(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language
	gymID := c.Callback().Data
	if gymID == "" {
		return c.Send(getTranslation("❌ Invalid Gym ID", language))
	}
	gym := getGymByID(gymID)
	c.Delete()
	return c.Send(&telebot.Venue{Location: telebot.Location{Lat: float32(gym.Lat), Lng: float32(gym.Lon)}, Title: *gym.Name})
}

// UI Control Callbacks

// handleCloseCallback handles the close button
func handleCloseCallback(c telebot.Context) error {
	return c.Delete()
}

// handleResetCallback handles the reset button callback
func handleResetCallback(c telebot.Context) error {
	c.Delete()
	return bot.Trigger("/reset", c)
}

// Subscription Management Callbacks

// handleAddSubscriptionCallback handles the add subscription button
func handleAddSubscriptionCallback(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language
	userConversationStates[c.Sender().ID] = "add_subscription"
	return c.Edit(getTranslation("📣 Enter the Pokémon name you want to subscribe to:", language))
}

// handleListSubscriptionsCallback handles the list subscriptions button
func handleListSubscriptionsCallback(c telebot.Context) error {
	c.Delete()
	return bot.Trigger("/list", c)
}

// handleClearSubscriptionsCallback handles the clear all subscriptions button
func handleClearSubscriptionsCallback(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language
	deleteAllUserSubscriptions(userID)
	getActiveSubscriptions()
	return c.Edit(getTranslation("🗑️ All Pokémon subscriptions cleared", language))
}

// Settings Toggle Callbacks

// handleToggleNotificationsCallback handles the toggle notifications button
func handleToggleNotificationsCallback(c telebot.Context) error {
	return toggleUserPreference(c, "Notify", func(user *User) bool {
		return !user.Notify
	})
}

// handleToggleStickersCallback handles the toggle stickers button
func handleToggleStickersCallback(c telebot.Context) error {
	return toggleUserPreference(c, "Stickers", func(user *User) bool {
		return !user.Stickers
	})
}

// handleToggleHundoIVCallback handles the toggle 100% IV button
func handleToggleHundoIVCallback(c telebot.Context) error {
	return toggleUserPreference(c, "HundoIV", func(user *User) bool {
		return !user.HundoIV
	})
}

// handleToggleZeroIVCallback handles the toggle 0% IV button
func handleToggleZeroIVCallback(c telebot.Context) error {
	return toggleUserPreference(c, "ZeroIV", func(user *User) bool {
		return !user.ZeroIV
	})
}

// handleToggleTopPVPCallback handles the toggle top PVP button
func handleToggleTopPVPCallback(c telebot.Context) error {
	return toggleUserPreference(c, "TopPVP", func(user *User) bool {
		return !user.TopPVP
	})
}

// handleToggleCleanupCallback handles the toggle cleanup button
func handleToggleCleanupCallback(c telebot.Context) error {
	return toggleUserPreference(c, "Cleanup", func(user *User) bool {
		return !user.Cleanup
	})
}

// Language Settings Callbacks

// handleChangeLangCallback handles the change language button
func handleChangeLangCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	btnEn := telebot.InlineButton{Text: "🇬🇧 English", Unique: "set_lang_en"}
	btnDe := telebot.InlineButton{Text: "🇩🇪 Deutsch", Unique: "set_lang_de"}
	return c.Edit(getTranslation("🌍 *Select a language:*", language), &telebot.ReplyMarkup{
		InlineKeyboard: [][]telebot.InlineButton{{btnEn, btnDe}},
	}, telebot.ModeMarkdown)
}

// handleSetLangEnCallback handles setting language to English
func handleSetLangEnCallback(c telebot.Context) error {
	updateUserPreference(getUserID(c), "Language", "en")
	user := getUserPreferences(getUserID(c))
	settingsMessage, replyMarkup := buildSettings(user)
	return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

// handleSetLangDeCallback handles setting language to German
func handleSetLangDeCallback(c telebot.Context) error {
	updateUserPreference(getUserID(c), "Language", "de")
	user := getUserPreferences(getUserID(c))
	settingsMessage, replyMarkup := buildSettings(user)
	return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

// Location and Settings Update Callbacks

// handleUpdateLocationCallback handles the update location button
func handleUpdateLocationCallback(c telebot.Context) error {
	c.Delete()
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	// Prompt user to send location
	btnShareLocation := telebot.ReplyButton{
		Text:     getTranslation("📍 Send Location", language),
		Location: true,
	}
	return c.Send(getTranslation("📍 Please send your current location:", language), &telebot.ReplyMarkup{
		ReplyKeyboard:  [][]telebot.ReplyButton{{btnShareLocation}},
		ResizeKeyboard: true,
	})
}

// handleSetDistanceCallback handles the set distance button
func handleSetDistanceCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	userConversationStates[userID] = "set_distance"
	return c.Edit(getTranslation("📏 Enter the maximal distance (in m):", language))
}

// handleSetMinIVCallback handles the set minimum IV button
func handleSetMinIVCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	userConversationStates[userID] = "set_min_iv"
	return c.Edit(getTranslation("✨ Enter the minimal IV percentage (0-100):", language))
}

// handleSetMinLevelCallback handles the set minimal level button
func handleSetMinLevelCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	userConversationStates[userID] = "set_min_level"
	return c.Edit(getTranslation("🔢 Enter the minimal Pokémon level (1-40):", language))
}

// Admin Callbacks

// handleBroadcastCallback handles the broadcast button (admin only)
func handleBroadcastCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	if _, ok := botAdmins[userID]; !ok {
		return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
	}
	userConversationStates[userID] = "broadcast"
	return c.Edit(getTranslation("📢 Enter the message you want to broadcast to all users:", language))
}

// handleListUsersCallback handles the list users button (admin only)
func handleListUsersCallback(c telebot.Context) error {
	c.Delete()
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	if _, ok := botAdmins[userID]; !ok {
		return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
	}

	var text strings.Builder
	c.Send(fmt.Sprintf(getTranslation("📋 *All Users:* %d", language)+"\n\n", len(userCache.All)), telebot.ModeMarkdown)

	for _, user := range userCache.All {
		if strings.HasPrefix(strconv.FormatInt(user.ID, 10), "-100") {
			continue
		}
		chat, _ := bot.ChatByID(user.ID)
		entry := fmt.Sprintf("🔹 %s %s @%s (%d) - Notify: %s\n", chat.FirstName, chat.LastName, chat.Username, user.ID, boolToEmoji(user.Notify))
		if text.Len()+len(entry) > 4000 { // Telegram message limit is 4096 bytes
			c.Send(text.String())
			text.Reset()
		}
		text.WriteString(entry)
	}

	return c.Send(text.String())
}

// handleListChannelsCallback handles the list channels button (admin only)
func handleListChannelsCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	if _, ok := botAdmins[userID]; !ok {
		return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf(getTranslation("📋 *All Channels:* %d", language)+"\n\n", len(userCache.Channels)))

	inlineKeyboard := [][]telebot.InlineButton{}
	for _, channel := range userCache.Channels {
		chat, _ := bot.ChatByID(channel.ID)
		text.WriteString(fmt.Sprintf("🔹 %s @%s (%d) - Notify: %s\n", chat.Title, chat.Username, channel.ID, boolToEmoji(channel.Notify)))
		btnEditChannel := telebot.InlineButton{
			Text:   fmt.Sprintf(getTranslation("✏️ Edit %s", language), chat.Title),
			Unique: "edit_channel",
			Data:   strconv.FormatInt(channel.ID, 10),
		}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnEditChannel})
	}
	btnClose := telebot.InlineButton{Text: getTranslation("Close", language), Unique: "close"}
	inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnClose})

	return c.Edit(text.String(), &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}, telebot.ModeMarkdown)
}

// handleEditChannelCallback handles the edit channel button (admin only)
func handleEditChannelCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	if _, ok := botAdmins[userID]; !ok {
		return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
	}

	channelID, _ := strconv.ParseInt(c.Callback().Data, 10, 64)
	botAdmins[userID] = channelID
	c.Delete()
	return bot.Trigger("/settings", c)
}

// handleImpersonateUserCallback handles the impersonate user button (admin only)
func handleImpersonateUserCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	if _, ok := botAdmins[c.Sender().ID]; !ok {
		return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
	}
	userConversationStates[c.Sender().ID] = "impersonate_user"
	return c.Edit(getTranslation("👤 Enter the user ID you want to impersonate:", language))
}

// Helper functions for validation and common patterns
func validateIntInRange(text string, min, max int, errorMsg string) (int, error) {
	var value int
	_, err := fmt.Sscanf(text, "%d", &value)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%s", errorMsg)
	}
	return value, nil
}

func isConversationCancelled(text string) bool {
	return strings.ToLower(text) == "abbruch" || strings.ToLower(text) == "cancel"
}

func clearConversationState(userID int64) {
	userConversationStates[userID] = ""
}

// Conversation state handlers
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

// handleWoCommand is an alias for the /locate command
func handleWoCommand(c telebot.Context) error {
	return bot.Trigger("/locate", c)
}

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

	// Handle text input
	bot.Handle(telebot.OnText, handleTextInput)
}
