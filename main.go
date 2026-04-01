package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/telebot.v3"
)

var (
	bot                    *telebot.Bot
	botAdmins              map[int64]int64
	userConversationStates map[int64]string
	userCache              FilteredUsers
	activeSubscriptions    map[int][]Subscription
)

func main() {
	log.Println("🚀 Starting PoGo Notification Bot")

	// Initialize the entire application
	initializeApplication()

	// Setup bot handlers and background processes.
	setupBotHandlers()
	startNotificationProcessing()

	// Initialize and start metrics server.
	initMetrics()
	startMetricsServer()

	// Use a context with cancellation for graceful shutdown.
	shutdownCtx, stop := context.WithCancel(context.Background())
	defer stop()

	// Listen for termination signals.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("🛑 Caught signal %v: shutting down", sig)
		bot.Stop()
		// Shutdown the metrics server gracefully.
		shutdownMetricsServerWithContext(shutdownCtx)
		stop() // Cancel the shutdown context
		os.Exit(0)
	}()

	// Start the bot.
	bot.Start()
}
