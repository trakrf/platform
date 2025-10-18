package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	version   = "dev" // Overridden at build time via -ldflags
	startTime time.Time
)

func main() {
	startTime = time.Now()
	// Setup structured JSON logging to stdout (12-factor)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Config from environment (12-factor)
	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}

	// Initialize database connection pool
	ctx := context.Background()
	if err := initDB(ctx); err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	slog.Info("Database connection pool initialized")

	// Initialize repositories
	initAccountRepo()
	initUserRepo()
	initAccountUserRepo()
	slog.Info("Repositories initialized")

	// Setup chi router
	r := chi.NewRouter()

	// Apply middleware stack
	r.Use(requestIDMiddleware)
	r.Use(recoveryMiddleware)
	r.Use(corsMiddleware)
	r.Use(contentTypeMiddleware)

	// Register health check routes (K8s liveness/readiness)
	r.Get("/healthz", healthzHandler)
	r.Get("/readyz", readyzHandler)
	r.Get("/health", healthHandler)

	// Register API routes
	registerAccountRoutes(r)
	registerUserRoutes(r)
	registerAccountUserRoutes(r)

	slog.Info("Routes registered")

	// HTTP server with timeouts
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("Server starting", "port", port, "version", version)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown (Railway/K8s requirement)
	slog.Info("Shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Shutdown error", "error", err)
	}

	// Close database connection pool
	closeDB()
	slog.Info("Database connection pool closed")

	slog.Info("Server stopped")
}
