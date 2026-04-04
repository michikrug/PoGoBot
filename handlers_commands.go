package main

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/telebot.v3"
)

// ── Shared helpers ────────────────────────────────────────────────────────────

// newTranslatorFor returns a Translator for the sender of c, falling back to
// "en" if the sender is not yet present in the user cache.
func newTranslatorFor(c telebot.Context) Translator {
	return newTranslator(userFromCache(c.Sender().ID).Language)
}

// getUserID returns the effective user ID, handling admin impersonation.
func getUserID(c telebot.Context) int64 {
	userID := c.Sender().ID
	if impersonatedID, ok := adminImpersonation[userID]; ok {
		c.Send(newTranslatorFor(c).T("🔒 You are impersonating another user"))
		return impersonatedID
	}
	return userID
}

// isAdmin reports whether the sender is a registered bot admin.
func isAdmin(c telebot.Context) bool {
	_, ok := appConfig.Admins[c.Sender().ID]
	return ok
}

// requireAdmin checks that the sender is an admin. If not, it calls reply with
// an error message and returns (true, err). Pass c.Send for command/conversation
// handlers and c.Edit for inline-button callbacks.
func requireAdmin(c telebot.Context, reply func(interface{}, ...interface{}) error) (bool, error) {
	if !isAdmin(c) {
		return true, reply(newTranslatorFor(c).T("❌ You are not authorized to use this command"))
	}
	return false, nil
}

// toggleUserPreference flips a boolean user preference and refreshes the settings UI.
func toggleUserPreference(c telebot.Context, field string, toggle func(user *User) bool) error {
	user := getUserPreferences(getUserID(c))
	newValue := toggle(&user)
	updateUserPreference(user.ID, field, newValue)
	updated := userFromCache(getUserID(c))
	settingsMessage, replyMarkup := buildSettings(updated)
	return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

// buildSettings constructs the settings message and inline keyboard for a user.
func buildSettings(user User) (string, *telebot.ReplyMarkup) {
	tr := newTranslator(user.Language)
	btnChangeLanguage := telebot.InlineButton{Text: tr.T("🌍 Change Language"), Unique: "change_lang"}
	btnUpdateLocation := telebot.InlineButton{Text: tr.T("📍 Update Location"), Unique: "update_location"}
	btnSetDistance := telebot.InlineButton{Text: tr.T("📏 Set Maximal Distance"), Unique: "set_distance"}
	btnSetMinIV := telebot.InlineButton{Text: tr.T("✨ Set Minimal IV"), Unique: "set_min_iv"}
	btnSetMinLevel := telebot.InlineButton{Text: tr.T("🔢 Set Minimal Level"), Unique: "set_min_level"}
	btnAddSubscription := telebot.InlineButton{Text: tr.T("📣 Add Pokémon Subscription"), Unique: "add_subscription"}
	btnListSubscriptions := telebot.InlineButton{Text: tr.T("📋 List all Pokémon Subscriptions"), Unique: "list_subscriptions"}
	btnClearSubscriptions := telebot.InlineButton{Text: tr.T("🗑️ Clear all Pokémon Subscriptions"), Unique: "clear_subscriptions"}

	allRaidsText := tr.T("⚔️ Disable Notifications for all Raids")
	if !user.AllRaids {
		allRaidsText = tr.T("⚔️ Enable Notifications for all Raids")
	}
	btnToggleAllRaids := telebot.InlineButton{Text: allRaidsText, Unique: "toggle_all_raids"}
	btnSetRaidMinLevel := telebot.InlineButton{Text: tr.T("🔢 Set Minimal Level for all Raids"), Unique: "set_raid_min_level"}
	btnAddRaidSubscription := telebot.InlineButton{Text: tr.T("⚔️ Add Raid Subscription"), Unique: "add_raid_subscription"}
	btnListRaidSubscriptions := telebot.InlineButton{Text: tr.T("📋 List all Raid Subscriptions"), Unique: "list_raid_subscriptions"}
	btnClearRaidSubscriptions := telebot.InlineButton{Text: tr.T("🗑️ Clear all Raid Subscriptions"), Unique: "clear_raid_subscriptions"}

	notificationsText := tr.T("🔔 Disable all Notifications")
	if !user.Notify {
		notificationsText = tr.T("🔕 Enable all Notifications")
	}
	btnToggleNotifications := telebot.InlineButton{Text: notificationsText, Unique: "toggle_notifications"}

	stickersText := tr.T("🎭 Do not show Pokémon Stickers")
	if !user.Stickers {
		stickersText = tr.T("🎭 Show Pokémon Stickers")
	}
	btnToggleStickers := telebot.InlineButton{Text: stickersText, Unique: "toggle_stickers"}

	hundoText := tr.T("💯 Disable 100% IV Notifications")
	if !user.HundoIV {
		hundoText = tr.T("💯 Enable 100% IV Notifications")
	}
	btnToogleHundoIV := telebot.InlineButton{Text: hundoText, Unique: "toggle_hundo_iv"}

	zeroText := tr.T("🚫 Disable 0% IV Notifications")
	if !user.ZeroIV {
		zeroText = tr.T("🚫 Enable 0% IV Notifications")
	}
	btnToogleZeroIV := telebot.InlineButton{Text: zeroText, Unique: "toggle_zero_iv"}

	pvpText := tr.T("🏅 Disable Top PVP Notifications")
	if !user.TopPVP {
		pvpText = tr.T("🏅 Enable Top PVP Notifications")
	}
	btnToogleTopPVP := telebot.InlineButton{Text: pvpText, Unique: "toggle_top_pvp"}

	cleanupText := tr.T("🗑️ Keep Expired Notifications")
	if !user.Cleanup {
		cleanupText = tr.T("🗑️ Remove Expired Notifications")
	}
	btnToggleCleanup := telebot.InlineButton{Text: cleanupText, Unique: "toggle_cleanup"}
	btnClose := telebot.InlineButton{Text: tr.T("Close"), Unique: "close"}

	settingsMessage := fmt.Sprintf(
		tr.T("⚙️ *Your Settings:*")+"\n"+
			"----------------------------------------------\n"+
			tr.T("🌍 *Language:* %s")+"\n"+
			tr.T("📍 *Location:* %.5f, %.5f")+"\n"+
			tr.T("📏 *Maximal Distance:* %dm")+"\n"+
			tr.T("✨ *Minimal IV:* %d%%")+"\n"+
			tr.T("🔢 *Minimal Level:* %d")+"\n"+
			tr.T("🔔 *Notifications:* %s")+"\n"+
			tr.T("🎭 *Pokémon Stickers:* %s")+"\n"+
			tr.T("💯 *100%% IV Notifications:* %s")+"\n"+
			tr.T("🚫 *0%% IV Notifications:* %s")+"\n"+
			tr.T("🏅 *Top PVP Notifications:* %s")+"\n"+
			tr.T("⚔️ *Raid Subscriptions:* %s")+"\n"+
			tr.T("⚔️ *Raid Minimal Level:* %d")+"\n"+
			tr.T("🗑️ *Cleanup Expired Notifications:* %s")+"\n\n"+
			tr.T("Use the buttons below to update the settings"),
		user.Language, user.Latitude, user.Longitude,
		user.MaxDistance, user.MinIV, user.MinLevel,
		boolToEmoji(user.Notify), boolToEmoji(user.Stickers),
		boolToEmoji(user.HundoIV), boolToEmoji(user.ZeroIV),
		boolToEmoji(user.TopPVP), boolToEmoji(user.AllRaids),
		user.RaidMinLevel, boolToEmoji(user.Cleanup),
	)

	if isChannelID(user.ID) {
		chatInfo, _ := bot.ChatByID(user.ID)
		settingsMessage = fmt.Sprintf(
			tr.T("⚙️ *Channel Settings:*")+"\n"+
				"----------------------------------------------\n"+
				tr.T("#️⃣ *Channel ID:* %d")+"\n"+
				tr.T("#️⃣ *Channel Name:* %s")+"\n"+
				tr.T("🌍 *Language:* %s")+"\n"+
				tr.T("✨ *Minimal IV:* %d%%")+"\n"+
				tr.T("🔢 *Minimal Level:* %d")+"\n"+
				tr.T("🔔 *Notifications:* %s")+"\n"+
				tr.T("🎭 *Pokémon Stickers:* %s")+"\n"+
				tr.T("💯 *100%% IV Notifications:* %s")+"\n"+
				tr.T("🚫 *0%% IV Notifications:* %s")+"\n"+
				tr.T("🏅 *Top PVP Notifications:* %s")+"\n"+
				tr.T("⚔️ *Raid Subscriptions:* %s")+"\n"+
				tr.T("⚔️ *Raid Minimal Level:* %d")+"\n"+
				tr.T("🗑️ *Cleanup Expired Notifications:* %s")+"\n\n"+
				tr.T("Use the buttons below to update the settings"),
			user.ID, chatInfo.Title, user.Language, user.MinIV, user.MinLevel,
			boolToEmoji(user.Notify), boolToEmoji(user.Stickers),
			boolToEmoji(user.HundoIV), boolToEmoji(user.ZeroIV),
			boolToEmoji(user.TopPVP), boolToEmoji(user.AllRaids),
			user.RaidMinLevel, boolToEmoji(user.Cleanup),
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
		{btnToggleAllRaids},
		{btnSetRaidMinLevel},
		{btnAddRaidSubscription},
		{btnListRaidSubscriptions},
		{btnClearRaidSubscriptions},
		{btnToggleNotifications},
		{btnToggleStickers},
		{btnToogleHundoIV},
		{btnToogleZeroIV},
		{btnToogleTopPVP},
		{btnToggleCleanup},
		{btnClose},
	}

	if isChannelID(user.ID) {
		btnReset := telebot.InlineButton{Text: tr.T("🔄 Reset"), Unique: "reset"}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnReset})
	} else if _, ok := appConfig.Admins[user.ID]; ok {
		btnBroadcast := telebot.InlineButton{Text: tr.T("📢 Broadcast Message"), Unique: "broadcast"}
		btnListChannels := telebot.InlineButton{Text: tr.T("📋 List Channels"), Unique: "list_channels"}
		btnListUsers := telebot.InlineButton{Text: tr.T("📋 List Users"), Unique: "list_users"}
		btnImpersonateUser := telebot.InlineButton{Text: tr.T("👤 Impersonate User"), Unique: "impersonate_user"}
		inlineKeyboard = append(inlineKeyboard,
			[]telebot.InlineButton{btnBroadcast, btnImpersonateUser},
			[]telebot.InlineButton{btnListUsers, btnListChannels},
		)
	}

	return settingsMessage, &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}
}

// ── Command handlers ──────────────────────────────────────────────────────────

func handleStart(c telebot.Context) error {
	userID := getUserID(c)

	detectedLanguage := c.Sender().LanguageCode
	if detectedLanguage != "en" && detectedLanguage != "de" {
		detectedLanguage = "en"
	}
	updateUserPreference(userID, "Language", detectedLanguage)

	tr := newTranslator(detectedLanguage)
	startMessage := fmt.Sprintf(
		tr.T("👋 Welcome to the PoGo Notification Bot!")+"\n\n"+
			tr.T("ℹ️ Language detected: *%s*")+"\n"+
			tr.T("ℹ️ Use /settings to update your preferences")+"\n"+
			tr.T("ℹ️ Use /subscribe <pokemon-name> [min-iv] [min-level] [max-distance] to get notified about specific Pokémon")+"\n"+
			tr.T("ℹ️ Send me your 📍 location to enable distance-based notifications"),
		detectedLanguage,
	)

	return c.Send(startMessage)
}

func handleHelp(c telebot.Context) error {
	tr := newTranslatorFor(c)
	helpMessage := tr.T("🤖 PoGo Notification Bot Commands:") + "\n\n" +
		tr.T("🔔 /settings - Update your preferences") + "\n" +
		tr.T("📋 /list - List your Pokémon subscriptions") + "\n" +
		tr.T("📣 /subscribe <pokemon-name> [min-iv] [min-level] [max-distance] - Subscribe to Pokémon alerts") + "\n" +
		tr.T("🚫 /unsubscribe <pokemon-name> - Unsubscribe from Pokémon alerts") + "\n" +
		tr.T("⚔️ /raidsubscribe - Subscribe to raid alerts") + "\n" +
		tr.T("⚔️ /raidlist - List your raid subscriptions") + "\n" +
		tr.T("🚫 /raidunsubscribe - Unsubscribe from raid alerts")
	return c.Send(helpMessage, telebot.ModeMarkdown)
}

func handleSettings(c telebot.Context) error {
	userID := getUserID(c)
	user := userFromCache(userID)
	settingsMessage, replyMarkup := buildSettings(user)
	return c.Send(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

func handleSubscribe(c telebot.Context) error {
	userID := getUserID(c)
	tr := newTranslatorFor(c)

	args := c.Args()
	if len(args) < 1 {
		return c.Send(tr.T("ℹ️ Usage: /subscribe <pokemon-name> [min-iv] [min-level] [max-distance]"))
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
		return c.Send(tr.Tf("❌ Can't find Pokedex # for Pokémon: %s", pokemonName))
	}

	minIV := 0
	minLevel := 0
	maxDistance := 0
	if len(numericArgs) > 0 {
		minIV, err = strconv.Atoi(numericArgs[0])
		if err != nil || minIV < 0 || minIV > 100 {
			return c.Send(tr.T("❌ Invalid input! Please enter a valid IV percentage (0-100)"))
		}
	}
	if len(numericArgs) > 1 {
		minLevel, err = strconv.Atoi(numericArgs[1])
		if err != nil || minLevel < 0 || minLevel > 40 {
			return c.Send(tr.T("❌ Invalid input! Please enter a valid level (0-40)"))
		}
	}
	if len(numericArgs) > 2 {
		maxDistance, err = strconv.Atoi(numericArgs[2])
		if err != nil || maxDistance < 0 {
			return c.Send(tr.T("❌ Invalid input! Please enter a valid distance (in m)"))
		}
	}

	addSubscription(userID, pokemonID, minIV, minLevel, maxDistance)

	return c.Send(tr.Tf("✅ Subscribed to %s alerts (Min IV: %d%%, Min Level: %d, Max Distance: %dm)",
		tr.PokemonName(pokemonID),
		minIV, minLevel, maxDistance,
	))
}

func handleList(c telebot.Context) error {
	user := userFromCache(getUserID(c))
	tr := newTranslator(user.Language)

	var text strings.Builder
	text.WriteString(tr.T("📋 *Your Pokémon Subscriptions:*") + "\n\n")
	if user.HundoIV {
		text.WriteString(tr.Tf("🔹 *All* (Min IV: 100%%, Min Level: 0, Max Distance: %dm)", user.MaxDistance) + "\n")
	}
	if user.ZeroIV {
		text.WriteString(tr.Tf("🔹 *All* (Max IV: 0%%, Min Level: 0, Max Distance: %dm", user.MaxDistance) + "\n")
	}
	c.Send(text.String(), telebot.ModeMarkdown)
	text.Reset()

	subscriptions := getUserSubscriptions(user.ID)

	if len(subscriptions) == 0 {
		return c.Send(tr.T("🔹 You have no specific Pokémon subscriptions"))
	}

	for _, subscription := range subscriptions {
		entry := tr.Tf("🔹 %s (Min IV: %d%%, Min Level: %d, Max Distance: %dm)",
			tr.PokemonName(subscription.PokemonID),
			subscription.MinIV, subscription.MinLevel, subscription.MaxDistance,
		) + "\n"
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
	tr := newTranslatorFor(c)

	args := c.Args()
	if len(args) < 1 {
		return c.Send(tr.T("ℹ️ Usage: /unsubscribe <pokemon-name>"))
	}

	pokemonName := strings.Join(args, " ")
	pokemonID, err := getPokemonID(pokemonName)
	if err != nil {
		return c.Send(tr.Tf("❌ Can't find Pokedex # for Pokémon: %s", pokemonName))
	}

	deleteSubscription(userID, pokemonID)
	getActiveSubscriptions(botDB)

	return c.Send(tr.Tf("✅ Unsubscribed from %s alerts", tr.PokemonName(pokemonID)))
}

func handleRaidSubscribe(c telebot.Context) error {
	userID := getUserID(c)
	tr := newTranslatorFor(c)

	args := c.Args()
	if len(args) < 1 {
		return c.Send(tr.T("ℹ️ Usage: /raidsubscribe <pokemon-name|all> [raid-level]"))
	}

	// Last arg is optional raid level (numeric); everything before is the name.
	raidLevel := 0
	nameArgs := args
	if len(args) >= 2 {
		if lvl, err := strconv.Atoi(args[len(args)-1]); err == nil {
			raidLevel = lvl
			nameArgs = args[:len(args)-1]
		}
	}
	if raidLevel < 0 || raidLevel > 19 {
		return c.Send(tr.T("❌ Invalid raid level! Please enter a level between 1 and 19"))
	}

	name := strings.Join(nameArgs, " ")
	if strings.ToLower(name) == "all" || strings.ToLower(name) == "alle" {
		if raidLevel == 0 {
			return c.Send(tr.T("❌ To subscribe to all raids regardless of level, use /settings → ") + tr.T("⚔️ Enable Notifications for all Raids"))
		}
		addRaidSubscription(userID, 0, raidLevel)
		return c.Send(tr.Tf("✅ Subscribed to all level %d raids", raidLevel))
	}

	pokemonID, err := getPokemonID(name)
	if err != nil {
		return c.Send(tr.Tf("❌ Can't find Pokedex # for Pokémon: %s", name))
	}

	addRaidSubscription(userID, pokemonID, raidLevel)
	if raidLevel == 0 {
		return c.Send(tr.Tf("✅ Subscribed to %s raids (any level)", tr.PokemonName(pokemonID)))
	}
	return c.Send(tr.Tf("✅ Subscribed to %s raids (Level: %d)", tr.PokemonName(pokemonID), raidLevel))
}

func handleRaidList(c telebot.Context) error {
	user := userFromCache(getUserID(c))
	tr := newTranslator(user.Language)

	var text strings.Builder
	text.WriteString(tr.T("📋 *Your Raid Subscriptions:*") + "\n\n")
	if user.AllRaids {
		text.WriteString(tr.Tf("🔹 All raids (Min Level: %d)", user.RaidMinLevel) + "\n")
	}
	subs := getUserRaidSubscriptions(user.ID)
	if !user.AllRaids && len(subs) == 0 {
		return c.Send(tr.T("🔹 You have no raid subscriptions"), telebot.ModeMarkdown)
	}
	c.Send(text.String(), telebot.ModeMarkdown)
	text.Reset()

	for _, sub := range subs {
		var entry string
		if sub.PokemonID == 0 {
			entry = tr.Tf("🔹 Level %d raids", sub.RaidLevel) + "\n"
		} else if sub.RaidLevel == 0 {
			entry = tr.Tf("🔹 %s raids (any level)", tr.PokemonName(sub.PokemonID)) + "\n"
		} else {
			entry = tr.Tf("🔹 %s raids (Level %d only)", tr.PokemonName(sub.PokemonID), sub.RaidLevel) + "\n"
		}
		if text.Len()+len(entry) > 4000 {
			c.Send(text.String())
			text.Reset()
		}
		text.WriteString(entry)
	}
	return c.Send(text.String())
}

func handleRaidUnsubscribe(c telebot.Context) error {
	userID := getUserID(c)
	tr := newTranslatorFor(c)

	args := c.Args()
	if len(args) < 1 {
		return c.Send(tr.T("ℹ️ Usage: /raidunsubscribe <pokemon-name|all> [raid-level]"))
	}

	raidLevel := 0
	nameArgs := args
	if len(args) >= 2 {
		if lvl, err := strconv.Atoi(args[len(args)-1]); err == nil {
			raidLevel = lvl
			nameArgs = args[:len(args)-1]
		}
	}

	name := strings.Join(nameArgs, " ")
	if strings.ToLower(name) == "all" || strings.ToLower(name) == "alle" {
		updateUserPreference(userID, "AllRaids", false)
		updateUserPreference(userID, "RaidMinLevel", 0)
		return c.Send(tr.Tf("✅ Unsubscribed from level %d raids", raidLevel))
	}

	pokemonID, err := getPokemonID(name)
	if err != nil {
		return c.Send(tr.Tf("❌ Can't find Pokedex # for Pokémon: %s", name))
	}

	deleteRaidSubscription(userID, pokemonID, raidLevel)
	getRaidActiveSubscriptions(botDB)
	if raidLevel == 0 {
		return c.Send(tr.Tf("✅ Unsubscribed from %s raids (any level)", tr.PokemonName(pokemonID)))
	}
	return c.Send(tr.Tf("✅ Unsubscribed from %s raids (Level %d)", tr.PokemonName(pokemonID), raidLevel))
}

func handleLocate(c telebot.Context) error {
	tr := newTranslatorFor(c)

	args := c.Args()
	if len(args) < 1 {
		return c.Send(tr.T("ℹ️ Usage: /locate <gym-name>"))
	}

	gymName := strings.Join(args, " ")
	gyms := searchGymsByName(gymName)

	if len(gyms) == 0 {
		return c.Send(tr.Tf("❌ Can't find gym: %s", gymName))
	} else if len(gyms) > 1 {
		responseText := tr.Tf("🔍 Found %d gyms matching your search:", len(gyms))
		inlineKeyboard := [][]telebot.InlineButton{}
		for _, gym := range gyms {
			btnGym := telebot.InlineButton{
				Text:   *gym.Name,
				Unique: "locate_gym",
				Data:   gym.ID,
			}
			inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnGym})
		}
		btnClose := telebot.InlineButton{Text: tr.T("Close"), Unique: "close"}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnClose})
		return c.Send(responseText, &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}, telebot.ModeMarkdown)
	}

	foundGym := gyms[0]
	return c.Send(&telebot.Venue{Location: telebot.Location{Lat: float32(foundGym.Lat), Lng: float32(foundGym.Lon)}, Title: *foundGym.Name})
}

func handleReset(c telebot.Context) error {
	if unauthorized, err := requireAdmin(c, c.Send); unauthorized {
		return err
	}
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	if _, ok := adminImpersonation[userID]; !ok {
		return c.Send(tr.T("🔒 You are not impersonating another user"), telebot.ModeMarkdown)
	}
	delete(adminImpersonation, userID)
	return c.Send(tr.T("🔒 You are now back as yourself"))
}

// handleWoCommand is an alias for the /locate command.
func handleWoCommand(c telebot.Context) error {
	return bot.Trigger("/locate", c)
}

func handleLocationMessage(c telebot.Context) error {
	userID := getUserID(c)
	tr := newTranslatorFor(c)
	location := c.Message().Location

	updateUserPreference(userID, "Latitude", location.Lat)
	updateUserPreference(userID, "Longitude", location.Lng)

	return c.Send(tr.T("📍 Location updated! Your preferences will now consider this"))
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
	bot.Handle("/raidsubscribe", handleRaidSubscribe)
	bot.Handle("/raidlist", handleRaidList)
	bot.Handle("/raidunsubscribe", handleRaidUnsubscribe)
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
	bot.Handle(&telebot.InlineButton{Unique: "toggle_all_raids"}, handleToggleAllRaidsCallback)
	bot.Handle(&telebot.InlineButton{Unique: "set_raid_min_level"}, handleSetRaidMinLevelCallback)
	bot.Handle(&telebot.InlineButton{Unique: "add_raid_subscription"}, handleAddRaidSubscriptionCallback)
	bot.Handle(&telebot.InlineButton{Unique: "list_raid_subscriptions"}, handleListRaidSubscriptionsCallback)
	bot.Handle(&telebot.InlineButton{Unique: "clear_raid_subscriptions"}, handleClearRaidSubscriptionsCallback)
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
