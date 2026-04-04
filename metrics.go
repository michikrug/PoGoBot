package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus metrics variables
var (
	customRegistry       = prometheus.NewRegistry()
	notificationsCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bot_notifications_total",
			Help: "Total number of notifications triggered",
		},
	)
	messagesCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bot_messages_total",
			Help: "Total number of messages sent",
		},
	)
	cleanupCounter = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_cleanup_total",
			Help: "Total number of expired messages cleaned up",
		},
	)
	encounterGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_encounters_count",
			Help: "Total number of Pokémon encounters retrieved",
		},
	)
	usersGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_users_count",
			Help: "Total number of users subscribed to notifications",
		},
	)
	subscriptionGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_subscription_count",
			Help: "Total number of Pokémon subscriptions",
		},
	)
	activeSubscriptionGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bot_subscription_active_count",
			Help: "Total number of active Pokémon subscriptions",
		},
	)
)

// initMetrics initializes and registers all Prometheus metrics
func initMetrics() {
	customRegistry.MustRegister(notificationsCounter)
	customRegistry.MustRegister(messagesCounter)
	customRegistry.MustRegister(encounterGauge)
	customRegistry.MustRegister(cleanupCounter)
	customRegistry.MustRegister(usersGauge)
	customRegistry.MustRegister(subscriptionGauge)
	customRegistry.MustRegister(activeSubscriptionGauge)
}

// metricsServer holds the HTTP server instance for shutdown
var metricsServer *http.Server

// startMetricsServer starts the Prometheus metrics HTTP server
func startMetricsServer() {
	metricsServer = &http.Server{Addr: ":9001"}
	http.Handle("/metrics", promhttp.HandlerFor(customRegistry, promhttp.HandlerOpts{}))

	go func() {
		log.Println("🚀 Prometheus metrics available at /metrics")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ HTTP server error: %v", err)
		}
	}()
}

// shutdownMetricsServer gracefully shuts down the metrics HTTP server,
// allowing up to 5 seconds for in-flight requests to complete.
func shutdownMetricsServer() {
	if metricsServer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := metricsServer.Shutdown(ctx); err != nil {
		log.Printf("❌ HTTP server shutdown failed: %v", err)
		return
	}
	log.Println("✅ Metrics server shut down gracefully")
}
