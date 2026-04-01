package main

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/telebot.v3"
)

// ── UI control ────────────────────────────────────────────────────────────────

func handleCloseCallback(c telebot.Context) error {
	return c.Delete()
}

func handleResetCallback(c telebot.Context) error {
	c.Delete()
	return bot.Trigger("/reset", c)
}

// ── Gym location ──────────────────────────────────────────────────────────────

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

// ── Subscription management ───────────────────────────────────────────────────

func handleAddSubscriptionCallback(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language
	userConversationStates[c.Sender().ID] = "add_subscription"
	return c.Edit(getTranslation("📣 Enter the Pokémon name you want to subscribe to:", language))
}

func handleListSubscriptionsCallback(c telebot.Context) error {
	c.Delete()
	return bot.Trigger("/list", c)
}

func handleClearSubscriptionsCallback(c telebot.Context) error {
	userID := getUserID(c)
	language := userCache.All[userID].Language
	deleteAllUserSubscriptions(userID)
	getActiveSubscriptions()
	return c.Edit(getTranslation("🗑️ All Pokémon subscriptions cleared", language))
}

// ── Settings toggle callbacks ─────────────────────────────────────────────────

func handleToggleNotificationsCallback(c telebot.Context) error {
	return toggleUserPreference(c, "Notify", func(user *User) bool { return !user.Notify })
}

func handleToggleStickersCallback(c telebot.Context) error {
	return toggleUserPreference(c, "Stickers", func(user *User) bool { return !user.Stickers })
}

func handleToggleHundoIVCallback(c telebot.Context) error {
	return toggleUserPreference(c, "HundoIV", func(user *User) bool { return !user.HundoIV })
}

func handleToggleZeroIVCallback(c telebot.Context) error {
	return toggleUserPreference(c, "ZeroIV", func(user *User) bool { return !user.ZeroIV })
}

func handleToggleTopPVPCallback(c telebot.Context) error {
	return toggleUserPreference(c, "TopPVP", func(user *User) bool { return !user.TopPVP })
}

func handleToggleCleanupCallback(c telebot.Context) error {
	return toggleUserPreference(c, "Cleanup", func(user *User) bool { return !user.Cleanup })
}

// ── Language callbacks ────────────────────────────────────────────────────────

func handleChangeLangCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	btnEnglish := telebot.InlineButton{Text: "🇬🇧 English", Unique: "set_lang_en"}
	btnDeutsch := telebot.InlineButton{Text: "🇩🇪 Deutsch", Unique: "set_lang_de"}
	return c.Edit(getTranslation("🌍 *Select a language:*", language), &telebot.ReplyMarkup{
		InlineKeyboard: [][]telebot.InlineButton{{btnEnglish, btnDeutsch}},
	}, telebot.ModeMarkdown)
}

func handleSetLangEnCallback(c telebot.Context) error {
	updateUserPreference(getUserID(c), "Language", "en")
	user := getUserPreferences(getUserID(c))
	settingsMessage, replyMarkup := buildSettings(user)
	return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

func handleSetLangDeCallback(c telebot.Context) error {
	updateUserPreference(getUserID(c), "Language", "de")
	user := getUserPreferences(getUserID(c))
	settingsMessage, replyMarkup := buildSettings(user)
	return c.Edit(settingsMessage, replyMarkup, telebot.ModeMarkdown)
}

// ── Location and numeric-setting callbacks ────────────────────────────────────

func handleUpdateLocationCallback(c telebot.Context) error {
	c.Delete()
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	btnShareLocation := telebot.ReplyButton{
		Text:     getTranslation("📍 Send Location", language),
		Location: true,
	}
	return c.Send(getTranslation("📍 Please send your current location:", language), &telebot.ReplyMarkup{
		ReplyKeyboard:  [][]telebot.ReplyButton{{btnShareLocation}},
		ResizeKeyboard: true,
	})
}

func handleSetDistanceCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	userConversationStates[userID] = "set_distance"
	return c.Edit(getTranslation("📏 Enter the maximal distance (in m):", language))
}

func handleSetMinIVCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	userConversationStates[userID] = "set_min_iv"
	return c.Edit(getTranslation("✨ Enter the minimal IV percentage (0-100):", language))
}

func handleSetMinLevelCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	userConversationStates[userID] = "set_min_level"
	return c.Edit(getTranslation("🔢 Enter the minimal Pokémon level (1-40):", language))
}

// ── Admin callbacks ───────────────────────────────────────────────────────────

func handleBroadcastCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	if _, ok := botAdmins[userID]; !ok {
		return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
	}
	userConversationStates[userID] = "broadcast"
	return c.Edit(getTranslation("📢 Enter the message you want to broadcast to all users:", language))
}

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
		chatInfo, _ := bot.ChatByID(user.ID)
		entry := fmt.Sprintf("🔹 %s %s @%s (%d) - Notify: %s\n", chatInfo.FirstName, chatInfo.LastName, chatInfo.Username, user.ID, boolToEmoji(user.Notify))
		if text.Len()+len(entry) > 4000 {
			c.Send(text.String())
			text.Reset()
		}
		text.WriteString(entry)
	}

	return c.Send(text.String())
}

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
		chatInfo, _ := bot.ChatByID(channel.ID)
		text.WriteString(fmt.Sprintf("🔹 %s @%s (%d) - Notify: %s\n", chatInfo.Title, chatInfo.Username, channel.ID, boolToEmoji(channel.Notify)))
		btnEditChannel := telebot.InlineButton{
			Text:   fmt.Sprintf(getTranslation("✏️ Edit %s", language), chatInfo.Title),
			Unique: "edit_channel",
			Data:   strconv.FormatInt(channel.ID, 10),
		}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnEditChannel})
	}
	btnClose := telebot.InlineButton{Text: getTranslation("Close", language), Unique: "close"}
	inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnClose})

	return c.Edit(text.String(), &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}, telebot.ModeMarkdown)
}

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

func handleImpersonateUserCallback(c telebot.Context) error {
	userID := c.Sender().ID
	language := userCache.All[userID].Language
	if _, ok := botAdmins[c.Sender().ID]; !ok {
		return c.Edit(getTranslation("❌ You are not authorized to use this command", language))
	}
	userConversationStates[c.Sender().ID] = "impersonate_user"
	return c.Edit(getTranslation("👤 Enter the user ID you want to impersonate:", language))
}
