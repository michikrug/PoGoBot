package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/telebot.v3"
)

var (
	bot                    *telebot.Bot
	adminImpersonation     map[int64]int64
	userConversationStates map[int64]string
	userCache              FilteredUsers
	activeSubscriptions    map[int][]Subscription
)

func main() {
	log.Println("🚀 Starting PoGo Notification Bot")

	// Initialize the entire application
	initializeApplication()

	// stopChannel is closed on shutdown to signal background goroutines.
	stopChannel := make(chan struct{})

	// Setup bot handlers and background processes.
	setupBotHandlers()
	startNotificationProcessing(stopChannel)

	// Initialize and start metrics server.
	initMetrics()
	startMetricsServer()

	// Listen for termination signals.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("🛑 Caught signal %v: shutting down", sig)
		close(stopChannel)      // stop notification processing loop
		bot.Stop()              // unblock bot.Start() on the main goroutine
		shutdownMetricsServer() // graceful HTTP drain
	}()

	// Start the bot — blocks until bot.Stop() is called.
	bot.Start()
	log.Println("✅ Shutdown complete")
}
