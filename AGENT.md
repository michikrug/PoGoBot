# Agent Guide — PoGoBot

This document is intended for AI agents working on this codebase. Read it before making any changes.

---

## Overview

PoGoBot is a Telegram bot written in **Go** that notifies users about Pokémon encounters. It polls a read-only scanner database (Golbat / RDM schema) every 30 seconds and dispatches personalised Telegram messages based on per-user preferences and subscriptions.

---

## Architecture

### Data flow

```txt
Telegram long-polling  ──►  telebot handlers  ──►  data.go wrappers  ──►  botDB (MySQL, read/write)
                                                                      ──►  scannerDB (MySQL, read-only)

Background goroutine (30 s tick):
  NotificationService.cleanupMessages()
      └─► botDB.GetExpiredEncountersWithMessages()
          └─► sender.Delete() + botDB.DeleteMessage/Encounter()
  NotificationService.processEncounters()
      └─► scannerDB.GetRecentEncounters()
          └─► filterAndSendEncounters()
              └─► sendEncounterNotification() ──► sender.Send() + botDB.SaveMessage/Encounter()
```

### Component map

| File | Responsibility |
| --- | --- |
| `main.go` | Global variable declarations, startup wiring, signal/shutdown goroutine |
| `config.go` | Ordered initialisation: env → DBs → static files → bot → notification service |
| `models.go` | All GORM model structs and JSON data structs — no logic |
| `interfaces.go` | `BotDB`, `ScannerDB`, `BotSender` interface definitions + `telegramBotSender` adapter |
| `database.go` | GORM implementations of `BotDB` and `ScannerDB` |
| `data.go` | Thin wrapper functions over `botDB`; in-memory cache management |
| `game_data.go` | Pokémon/move name cache, `Translator` type, `getTranslation()` |
| `notification_service.go` | Core notification loop, filtering, deduplication, message dispatch, cleanup |
| `handlers_commands.go` | Slash command handlers, `setupBotHandlers()`, settings UI builder |
| `handlers_callbacks.go` | Inline button callback handlers |
| `handlers_conversation.go` | Multi-step text-input conversation state machine |
| `utils.go` | `haversine`, `withinDistance`, `boolToEmoji`, emoji lookup maps |
| `metrics.go` | Prometheus metric definitions, HTTP server on `:9001` |
| `mocks_test.go` | `testify/mock` implementations of all three interfaces |
| `data_test.go` | Shared test fixtures (`testGameData`, `testTranslations`) |

### Dependency injection

`NotificationService` is constructed with explicit `BotDB`, `ScannerDB`, and `BotSender` arguments — this is the only place DI is used. Command handlers reach the DB through the package-level `botDB`/`scannerDB` globals via `data.go` wrapper functions.

### Concurrency

- Telegram bot runs in the main goroutine (`bot.Start()` blocks).
- Notification loop runs in a single background goroutine, ticker-driven (`time.After(30s)`).
- Shutdown is coordinated by closing a `chan struct{}` (`stopChannel`).
- There is no mutex on the in-memory caches — telebot serialises handler calls internally; only the notification goroutine reads the caches while only Telegram handler goroutines write them.

### In-memory caches

`data.go` maintains two caches rebuilt on every preference write:

- `userCache` — four pre-segmented slices produced by `getUsersByFilters()`
- `activeSubscriptions` — `map[pokemonID][]Subscription` produced by `getActiveSubscriptions()`

The notification loop reads exclusively from these caches, never from the DB directly.

---

## Tech Stack

| | |
|---|---|
| **Language** | Go 1.26 |
| **Bot API** | `gopkg.in/telebot.v3` (long-polling) |
| **ORM** | `gorm.io/gorm` + MySQL driver (SQLite in tests only) |
| **Config** | `github.com/joho/godotenv` + `os.Getenv` |
| **Metrics** | `github.com/prometheus/client_golang` on `:9001` |
| **Testing** | `github.com/stretchr/testify` (assert, require, mock) |
| **Deployment** | Docker, multi-stage Alpine build, CGO disabled |

---

## Package Structure

Everything is **`package main`** in the repository root — a single flat package. There are no sub-packages except `migration/`, which is a standalone CLI tool not imported by the bot.

Do not introduce sub-packages or split the package without a compelling reason. The flat structure is intentional.

---

## Code Style

### Naming

- Unexported: `camelCase` — `handleStart`, `getUserPreferences`, `botDB`
- Exported: `PascalCase` — `NotificationService`, `BotDB`, `User`
- Test functions: `TestFunctionName_Condition_ExpectedBehaviour` — e.g. `TestFilterAndSendEncounters_HundoIV_NotifiesHundoUsers`

### Section dividers

Every file uses this exact visual separator to group related functions:

```go
// ── Section name ─────────────────────────────────────────────────────────────
```

Use this style when adding new sections. Do not use blank comment lines or `//---` variants.

### Imports

Standard grouping: stdlib first, then third-party. No dot imports, no aliases unless disambiguation requires it.

### Logging

Every `log.Printf` / `log.Fatalf` line uses an emoji prefix. Follow the established conventions:

| Emoji | Meaning |
| --- | --- |
| `✅` | Success |
| `❌` | Error / failure |
| `⚠️` | Warning |
| `🚀` | Startup |
| `🛑` | Shutdown |
| `🔔` | Notification |
| `📋` | Listing / enumeration |

Use `log.Fatalf` for unrecoverable startup conditions. Do not bubble errors back up to `main`.

### Receiver style

All interface implementations and service types use **pointer receivers** (`*gormBotDB`, `*NotificationService`, etc.).

### Nil-pointer safety

All pointer fields on `EncounterData` (`*int`, `*float32`, `*string`, `*bool`) must be nil-checked before use. Provide safe fallbacks (e.g. `generateNotificationTitle` produces a simpler format when IV fields are nil).

### String building

Use `strings.Builder` for multi-line message construction. Do not concatenate strings with `+` in loops.

### GORM patterns

- `FirstOrCreate` for user get-or-create
- `Save` for full upserts (subscriptions, encounters)
- `db.Model(&T{}).Where(...).Update(field, value)` for single-field updates
- `db.Where(...).Delete(...)` for deletions
- `AutoMigrate` at startup for the bot DB only; never migrate the scanner DB
- Nullable scanner fields use pointer types; `gorm:"-"` to exclude computed fields from queries

---

## Database

### Two MySQL connections

**Bot DB** (read/write) — managed by the bot:

| Table | Model | Notes |
| --- | --- | --- |
| `users` | `User` | Auto-migrated by GORM |
| `subscriptions` | `Subscription` | Composite PK: `(user_id, pokemon_id)` |
| `encounters` | `Encounter` | Indexed on `expiration` |
| `messages` | `Message` | Composite PK `(chat_id, message_id)`, indexed on `encounter_id` |

**Scanner DB** (read-only) — Golbat / RDM schema:

| Table | Model | Mapped via |
| --- | --- | --- |
| `pokemon` | `EncounterData` | `TableName()` method |
| `gym` | `GymData` | `TableName()` method |

Never add write operations to `gormScannerDB`.

---

## Testing

### Structure

- All tests are in **`package main`** (white-box) in `*_test.go` files.
- Shared fixtures live in `data_test.go` (`testGameData()`, `testTranslations()`).
- All interface mocks are in `mocks_test.go`.

### Test naming

```go
TestFunctionName_Condition_ExpectedBehaviour
```

### Database tests

Use `newTestBotDB(t)` and `newTestScannerDB(t)` helpers. These open an in-memory SQLite DB and call `AutoMigrate`. Tests set up fixtures by calling `db.db.Create(...)` directly.

### Running tests

```sh
go test ./...          # all tests
go test -v ./...       # verbose
go test -cover ./...   # with coverage
```

Tests must pass before any commit. Do not commit `coverage.html` or `coverage.out`.

### What to test

Write tests for any new logic that involves:

- Filtering or matching encounters
- Database reads/writes
- Message generation
- State transitions in the conversation machine

Pure string-building helpers and trivial wrappers do not require tests, but complex conditional logic always does.

---

## Adding a New Command

1. Write a handler function `handleXxx(c telebot.Context) error` in `handlers_commands.go`.
2. Register it in `setupBotHandlers()` in the same file: `bot.Handle("/xxx", handleXxx)`.
3. Add the command to `commands.txt` for BotFather registration.
4. If the command needs multi-step user input, add a state constant and a handler branch in `handlers_conversation.go`.

## Adding a New Inline Button

1. Define the button as a package-level `telebot.InlineButton` with a descriptive `Unique` string.
2. Register the handler in `setupBotHandlers()`: `bot.Handle(&myButton, handleMyButton)`.
3. Add the button to `buildSettings()` if it belongs in the settings menu.

## Adding a New Subscription Filter

1. Add the field to the `User` struct in `models.go` with a GORM column tag.
2. Update the `getUsersByFilters()` cache rebuild in `data.go` if the field affects segmentation.
3. Add the corresponding filter logic in `filterAndSendEncounters()` in `notification_service.go`.
4. Expose the toggle in `buildSettings()` and register a callback in `handlers_callbacks.go`.
5. Write a test in `notifications_test.go` covering the new filter path.

## Adding a New Metric

Add the metric definition to `metrics.go` alongside the existing ones, using the same custom registry (`prometheus.NewRegistry()`). Do not use the default global registry.

---

## Conversation State Machine

State strings are defined in `handlers_callbacks.go` (setting state) and consumed in `handlers_conversation.go` (dispatching). Intermediate wizard values are **encoded into the state string itself** — e.g. `"add_subscription_level_25_80"` encodes pokémon ID 25 and minimum IV 80. Parse them back with `strings.Split(state, "_")`.

This is an established pattern — follow it for any new multi-step flows.

---

## Key Constraints

- **Do not add new package-level globals** without a clear reason. If a value is only needed inside `NotificationService`, add it as a struct field.
- **Do not call the DB interfaces directly from handlers.** Always go through the `data.go` wrapper functions so the caches stay consistent.
- **Do not import the scanner DB schema** into bot DB migrations. The scanner DB is owned externally.
- **Static files (`masterfile.json`, `translations.json`) are read at startup** from the working directory. Their paths are hardcoded; do not parameterise them via config unless you also update the Dockerfile.
