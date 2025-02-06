# Pokémon Notification Bot

## Overview

This is a **Telegram bot** written in Go that notifies users about Pokémon encounters based on their preferences. The bot fetches Pokémon encounters from a MySQL database (Golbat / RDM Schema) and allows users to configure filters like IV, level, and distance. Notifications are sent as private messages.

## Features

- 📨 **Personalized Pokémon Alerts** – Users can subscribe to Pokémon notifications based on ID, IV, level, and distance.
- 🌍 **Multi-Language Support** – Pokémon names and move names are displayed based on user language settings (currently supports English and German).
- 📍 **Location-Based Filtering** – Users can share their location to receive alerts for Pokémon within a specified radius.
- 🛠 **Flexible Configuration** – Users can adjust settings via `/settings`, including notification preferences, sticker usage, and language.
- 📊 **Prometheus Metrics** – The bot exposes Prometheus metrics to monitor performance and activity.
- 🗑️ **Auto Cleanup** – Optionally deletes expired notifications.
- 🔔 **Support for 100% and 0% IV Pokémon Alerts** – Users can opt-in for alerts on perfect or worst IV Pokémon.

## Installation & Setup

### **1. Clone the Repository**

```sh
git clone https://github.com/michikrug/PoGoBot.git
cd PoGoBot
```

### **2. Configure Environment Variables**

Create a `.env` file and define the required variables:

```sh
BOT_TOKEN=your-telegram-bot-token
BOT_ADMINS=12345678,87654321
BOT_DB_USER=dbuser
BOT_DB_PASS=dbpassword
BOT_DB_NAME=bot_database
BOT_DB_HOST=localhost
SCANNER_DB_USER=scanner_db_user
SCANNER_DB_PASS=scanner_db_password
SCANNER_DB_NAME=scanner_database
SCANNER_DB_HOST=localhost
```

### **3. Run the Bot**

```sh
go run main.go
```

### **4. Run with Docker**

Build and run the bot in a Docker container:

```sh
docker build -t pogobot .
docker run --env-file .env pogobot
```

## Commands

| Command          | Description |
|-----------------|-------------|
| `/start`        | Starts the bot and sets default settings |
| `/help`         | Show help information |
| `/settings`     | Open settings to adjust preferences |
| `/list`         | List all subscriptions |
| `/subscribe <pokemon_name> [min-iv] [min-level] [max-distance]` | Subscribe to Pokémon alerts |
| `/unsubscribe <pokemon_name>` | Unsubscribe from Pokémon alerts |

## Prometheus Metrics

The bot exposes metrics at:

```sh
http://localhost:9001/metrics
```

### **Available Metrics:**

- `bot_notifications_total` – Total number of notifications sent.
- `bot_messages_total` – Total number of messages sent.
- `bot_cleanup_total` – Number of expired messages cleaned up.
- `bot_encounters_count` – Number of Pokémon encounters retrieved.
- `bot_users_count` – Number of users subscribed to notifications.
- `bot_subscription_count` – Total number of subscriptions.
- `bot_subscription_active_count` – Active Pokémon subscriptions.

## Contributing

Pull requests are welcome! Please follow the existing code structure and submit any improvements.

## License

This project is licensed under the **GNU General Public License (GPL)**. See `LICENSE` for details.