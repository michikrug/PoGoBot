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
	tr := newTranslatorFor(c)
	gymID := c.Callback().Data
	if gymID == "" {
		return c.Send(tr.T("❌ Invalid Gym ID"))
	}
	gym := getGymByID(gymID)
	c.Delete()
	return c.Send(&telebot.Venue{Location: telebot.Location{Lat: float32(gym.Lat), Lng: float32(gym.Lon)}, Title: *gym.Name})
}

// ── Subscription management ───────────────────────────────────────────────────

func handleAddSubscriptionCallback(c telebot.Context) error {
	tr := newTranslatorFor(c)
	userConversationStates[c.Sender().ID] = "add_subscription"
	return c.Edit(tr.T("📣 Enter the Pokémon name you want to subscribe to:"))
}

func handleListSubscriptionsCallback(c telebot.Context) error {
	c.Delete()
	return bot.Trigger("/list", c)
}

func handleClearSubscriptionsCallback(c telebot.Context) error {
	userID := getUserID(c)
	tr := newTranslatorFor(c)
	deleteAllUserSubscriptions(userID)
	getActiveSubscriptions(botDB)
	return c.Edit(tr.T("🗑️ All Pokémon subscriptions cleared"))
}

// ── Raid subscription callbacks ───────────────────────────────────────────────

func handleToggleAllRaidsCallback(c telebot.Context) error {
	return toggleUserPreference(c, "AllRaids", func(user *User) bool { return !user.AllRaids })
}

func handleSetRaidMinLevelCallback(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	userConversationStates[userID] = "set_raid_min_level"
	return c.Edit(tr.T("🔢 Enter the minimal raid level (1-19):"))
}

func handleAddRaidSubscriptionCallback(c telebot.Context) error {
	tr := newTranslatorFor(c)
	userConversationStates[c.Sender().ID] = "add_raid_subscription"
	return c.Edit(tr.T("⚔️ Enter the Pokémon name (or 'all') to subscribe to raids:"))
}

func handleListRaidSubscriptionsCallback(c telebot.Context) error {
	c.Delete()
	return bot.Trigger("/raidlist", c)
}

func handleClearRaidSubscriptionsCallback(c telebot.Context) error {
	userID := getUserID(c)
	tr := newTranslatorFor(c)
	deleteAllUserRaidSubscriptions(userID)
	return c.Edit(tr.T("🗑️ All Raid subscriptions cleared"))
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
	tr := newTranslatorFor(c)
	btnEnglish := telebot.InlineButton{Text: "🇬🇧 English", Unique: "set_lang_en"}
	btnDeutsch := telebot.InlineButton{Text: "🇩🇪 Deutsch", Unique: "set_lang_de"}
	return c.Edit(tr.T("🌍 *Select a language:*"), &telebot.ReplyMarkup{
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
	tr := newTranslatorFor(c)
	btnShareLocation := telebot.ReplyButton{
		Text:     tr.T("📍 Send Location"),
		Location: true,
	}
	return c.Send(tr.T("📍 Please send your current location:"), &telebot.ReplyMarkup{
		ReplyKeyboard:  [][]telebot.ReplyButton{{btnShareLocation}},
		ResizeKeyboard: true,
	})
}

func handleSetDistanceCallback(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	userConversationStates[userID] = "set_distance"
	return c.Edit(tr.T("📏 Enter the maximal distance (in m):"))
}

func handleSetMinIVCallback(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	userConversationStates[userID] = "set_min_iv"
	return c.Edit(tr.T("✨ Enter the minimal IV percentage (0-100):"))
}

func handleSetMinLevelCallback(c telebot.Context) error {
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	userConversationStates[userID] = "set_min_level"
	return c.Edit(tr.T("🔢 Enter the minimal Pokémon level (1-40):"))
}

// ── Admin callbacks ───────────────────────────────────────────────────────────

func handleBroadcastCallback(c telebot.Context) error {
	if unauthorized, err := requireAdmin(c, c.Edit); unauthorized {
		return err
	}
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	userConversationStates[userID] = "broadcast"
	return c.Edit(tr.T("📢 Enter the message you want to broadcast to all users:"))
}

func handleListUsersCallback(c telebot.Context) error {
	c.Delete()
	if unauthorized, err := requireAdmin(c, c.Edit); unauthorized {
		return err
	}
	tr := newTranslatorFor(c)
	var text strings.Builder
	c.Send(tr.Tf("📋 *All Users:* %d", len(userCache.All))+"\n\n", telebot.ModeMarkdown)

	for _, user := range userCache.All {
		if isChannelID(user.ID) {
			continue
		}
		chatInfo, err := bot.ChatByID(user.ID)
		var entry string
		if err != nil || chatInfo == nil {
			entry = fmt.Sprintf("🔹 (unknown) (%d) - Notify: %s\n", user.ID, boolToEmoji(user.Notify))
		} else {
			entry = fmt.Sprintf("🔹 %s %s @%s (%d) - Notify: %s\n", chatInfo.FirstName, chatInfo.LastName, chatInfo.Username, user.ID, boolToEmoji(user.Notify))
		}
		if text.Len()+len(entry) > 4000 {
			c.Send(text.String())
			text.Reset()
		}
		text.WriteString(entry)
	}

	return c.Send(text.String())
}

func handleListChannelsCallback(c telebot.Context) error {
	if unauthorized, err := requireAdmin(c, c.Edit); unauthorized {
		return err
	}
	tr := newTranslatorFor(c)

	var text strings.Builder
	text.WriteString(tr.Tf("📋 *All Channels:* %d", len(userCache.Channels)) + "\n\n")

	inlineKeyboard := [][]telebot.InlineButton{}
	for _, channel := range userCache.Channels {
		chatInfo, err := bot.ChatByID(channel.ID)
		var title, username string
		if err != nil || chatInfo == nil {
			title = fmt.Sprintf("(%d)", channel.ID)
			username = ""
		} else {
			title = chatInfo.Title
			username = chatInfo.Username
		}
		text.WriteString(fmt.Sprintf("🔹 %s @%s (%d) - Notify: %s\n", title, username, channel.ID, boolToEmoji(channel.Notify)))
		btnEditChannel := telebot.InlineButton{
			Text:   tr.Tf("✏️ Edit %s", title),
			Unique: "edit_channel",
			Data:   strconv.FormatInt(channel.ID, 10),
		}
		inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnEditChannel})
	}
	btnClose := telebot.InlineButton{Text: tr.T("Close"), Unique: "close"}
	inlineKeyboard = append(inlineKeyboard, []telebot.InlineButton{btnClose})

	return c.Edit(text.String(), &telebot.ReplyMarkup{InlineKeyboard: inlineKeyboard}, telebot.ModeMarkdown)
}

func handleEditChannelCallback(c telebot.Context) error {
	if unauthorized, err := requireAdmin(c, c.Edit); unauthorized {
		return err
	}
	userID := c.Sender().ID

	channelID, _ := strconv.ParseInt(c.Callback().Data, 10, 64)
	adminImpersonation[userID] = channelID
	c.Delete()
	return bot.Trigger("/settings", c)
}

func handleImpersonateUserCallback(c telebot.Context) error {
	if unauthorized, err := requireAdmin(c, c.Edit); unauthorized {
		return err
	}
	userID := c.Sender().ID
	tr := newTranslatorFor(c)
	userConversationStates[userID] = "impersonate_user"
	return c.Edit(tr.T("👤 Enter the user ID you want to impersonate:"))
}
