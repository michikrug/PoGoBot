# 🐾 Pokémon Notification Bot

## 📖 Overview

A **Telegram bot** written in Go that notifies users about Pokémon encounters based on their preferences. The bot polls a MySQL database (Golbat / RDM schema) every 30 seconds and sends personalized notifications as private Telegram messages.

## ✨ Features

- 📨 **Personalized Pokémon alerts** – Subscribe by Pokémon name with optional IV, level, and distance filters.
- 💯 **100% / 0% IV and Top PVP alerts** – Opt-in global alerts for perfect IV, zero IV, or top-3 PVP ranked Pokémon.
- 📍 **Location-based filtering** – Share a location to restrict alerts to a configurable radius.
- 🌍 **Multi-language support** – Pokémon and move names in English and German; auto-detected from Telegram.
- 🗺️ **Map-only mode** – Receive encounters as a Telegram Venue pin instead of text + location messages.
- 🎭 **Sticker support** – Optionally include a Pokémon sticker with each notification.
- 🗑️ **Auto-cleanup** – Optionally delete expired notification messages when a spawn despawns.
- 🔒 **Admin impersonation** – Admins can act on behalf of any user for support purposes.
- 📊 **Prometheus metrics** – Exposes operational metrics on `:9001/metrics`.
- 🛑 **Graceful shutdown** – SIGINT/SIGTERM stops the bot, drains the metrics server, and cancels the notification loop cleanly.

## 🔧 Requirements

- Go 1.21+
- Two MySQL-compatible databases:
  - **Bot DB** – stores users, subscriptions, tracked messages, and encounter state.
  - **Scanner DB** – a read-only Golbat / RDM database that provides live encounter data.

## 🚀 Installation & Setup

### 1. Clone the repository

```sh
git clone https://github.com/michikrug/PoGoBot.git
cd PoGoBot
```

### 2. Configure environment variables

Copy `example.env` to `.env` and fill in the values:

```sh
cp example.env .env
```

| Variable | Required | Description |
|---|---|---|
| `BOT_TOKEN` | ✅ | Telegram bot token from @BotFather |
| `BOT_ADMINS` | ✅ | Comma-separated Telegram user IDs with admin access |
| `BOT_DB_USER` | ✅ | Bot database username |
| `BOT_DB_PASS` | ✅ | Bot database password |
| `BOT_DB_NAME` | ✅ | Bot database name |
| `BOT_DB_HOST` | ✅ | Bot database host (e.g. `localhost:3306`) |
| `SCANNER_DB_USER` | ✅ | Scanner database username |
| `SCANNER_DB_PASS` | ✅ | Scanner database password |
| `SCANNER_DB_NAME` | ✅ | Scanner database name |
| `SCANNER_DB_HOST` | ✅ | Scanner database host |
| `BOT_TIMEZONE` | ➖ | IANA timezone name for expire times (e.g. `Europe/Berlin`). Defaults to the system local timezone; falls back to UTC if invalid. |

### 3. Run the bot

```sh
go run .
```

### 4. Run with Docker

```sh
docker build -t pogobot .
docker run --env-file .env pogobot
```

## 🤖 Commands

| Command | Description |
|---|---|
| `/start` | Register with the bot and auto-detect language |
| `/help` | Show available commands |
| `/settings` | Open the interactive settings menu |
| `/list` | List all active Pokémon subscriptions |
| `/subscribe <name> [min-iv] [min-level] [max-distance]` | Subscribe to alerts for a specific Pokémon |
| `/unsubscribe <name>` | Remove a Pokémon subscription |
| `/locate <gym-name>` | Find a gym by name and send its location |
| `/wo <gym-name>` | Alias for `/locate` |

### ⚙️ Settings menu

The `/settings` command opens an inline keyboard with the following toggles and inputs:

- 🌍 Language (English / German)
- 📍 Home location (via shared Telegram location)
- 📏 Max distance, min IV, min level
- 🔔 Enable / disable all notifications
- 🎭 Show / hide Pokémon stickers
- 💯 100% IV, 🚫 0% IV, and 🏅 Top PVP global alerts
- 🗺️ Map-only mode (Venue pin instead of text message)
- 🗑️ Auto-cleanup of expired notifications

Channel and admin-specific settings are shown automatically when the command is issued from a channel or by an admin.

## 🔔 Notification loop

The bot checks for new encounters every **30 seconds**. Each cycle:

1. 🗑️ Expired encounters are detected and their Telegram messages are deleted (if cleanup is enabled for the user).
2. 🔍 Fresh encounters from the scanner database are fetched and matched against all active subscriptions and global alert filters.
3. 📨 Matched users are notified; rate-limited users and already-notified encounter/user pairs are skipped.

## 📊 Prometheus metrics

Metrics are exposed at `http://localhost:9001/metrics` using a dedicated custom registry.

| Metric | Type | Description |
| --- | --- | --- |
| `bot_notifications_total` | Counter | Encounter notifications dispatched |
| `bot_messages_total` | Counter | Individual Telegram messages sent |
| `bot_cleanup_total` | Gauge | Expired messages deleted |
| `bot_encounters_count` | Gauge | Pokémon encounters fetched in the last cycle |
| `bot_users_count` | Gauge | Users loaded into the active cache |
| `bot_subscription_count` | Gauge | Total subscriptions loaded |
| `bot_subscription_active_count` | Gauge | Subscriptions currently active |

## 🌍 Translation generation

Translated Pokémon names, form names, move names, and bot UI strings are stored
in `translations.json` and `masterfile.json`. Both files are committed to the
repository and copied into the Docker image at build time. They are regenerated
by running:

```sh
python3 build_translations.py
```

The script requires Python 3.10+ and no third-party packages. It fetches two
upstream sources over HTTPS and writes both output files in-place:

| Source | What it provides |
| --- | --- |
| [WatWowMap/Masterfile-Generator](https://github.com/WatWowMap/Masterfile-Generator) `master-latest-react-map.json` | Pokémon, form, move, and item data with numeric IDs and English names → saved as `masterfile.json` |
| [WatWowMap/pogo-translations](https://github.com/WatWowMap/pogo-translations) `{lang}.json` | Translated names keyed by `poke_<id>`, `form_<id>`, `move_<id>`, `item_<id>` |

Bot UI strings are maintained by hand in `bot_strings.json` and merged in as a final step.

### Options

| Flag | Default | Description |
| --- | --- | --- |
| `--lang CODE` | `de` | Language code to build |
| `--masterfile FILE` | `masterfile.json` | Path to write the masterfile |
| `--masterfile-url URL` | upstream URL | Override the masterfile download URL |
| `--translations FILE` | `translations.json` | Path to write translations |
| `--bot-strings FILE` | `bot_strings.json` | Path to the hand-maintained bot UI strings |
| `--dry-run` | — | Print report only; do not write any files |
| `--check` | — | Exit 1 if bot UI keys are missing or `bot_strings.json` is out of sync with Go source (useful for CI) |

### Adding or updating a translation

- **Game data** (Pokémon / form / move / item names): re-run the script; it
  always fetches the latest upstream data.
- **Bot UI strings**: edit `bot_strings.json` directly, then re-run the script.
  The `--check` flag will report any keys present in Go source but missing from
  `bot_strings.json`, and vice-versa.

## 🤝 Contributing

Pull requests are welcome. Please follow the existing code structure and include tests for any new logic.

## 📄 License

This project is licensed under the GNU General Public License v3. See the [LICENSE](LICENSE) file for details.
