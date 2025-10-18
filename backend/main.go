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

	// Initialize authentication service
	initAuthService()
	slog.Info("Auth service initialized")

	// Setup chi router
	r := chi.NewRouter()

	// Apply middleware stack
	r.Use(requestIDMiddleware)
	r.Use(recoveryMiddleware)
	r.Use(corsMiddleware)
	r.Use(contentTypeMiddleware)

	// ============================================================================
	// Frontend & Static Asset Routes
	// ============================================================================
	// IMPORTANT: Static assets must be registered BEFORE API routes to prevent
	// the catch-all SPA handler from intercepting API requests

	frontendHandler := serveFrontend()

	// Static assets (public, no auth required)
	// These are served directly from the embedded filesystem with long cache TTLs
	r.Handle("/assets/*", frontendHandler)
	r.Handle("/favicon.ico", frontendHandler)
	r.Handle("/icon-*.png", frontendHandler) // All icon sizes
	r.Handle("/logo.png", frontendHandler)
	r.Handle("/manifest.json", frontendHandler)
	r.Handle("/og-image.png", frontendHandler)

	// ============================================================================
	// Health Check Routes (K8s liveness/readiness)
	// ============================================================================
	r.Get("/healthz", healthzHandler)
	r.Get("/readyz", readyzHandler)
	r.Get("/health", healthHandler)

	// Register API routes
	// Public endpoints (no auth required)
	registerAuthRoutes(r) // POST /api/v1/auth/signup, /api/v1/auth/login

	// Protected endpoints (require valid JWT)
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware) // Apply auth middleware to this group

		registerAccountRoutes(r)     // All /api/v1/accounts/* routes
		registerUserRoutes(r)        // All /api/v1/users/* routes
		registerAccountUserRoutes(r) // All /api/v1/account_users/* routes
	})

	slog.Info("Routes registered")

	// ============================================================================
	// SPA Catch-All Handler (must be LAST)
	// ============================================================================
	// Serve index.html for all remaining routes to enable React Router
	// React will handle:
	//   - Public routes: /, /login, /register (inventory without auth)
	//   - Protected routes: /dashboard, /assets, /settings (redirects to login)
	r.HandleFunc("/*", spaHandler)

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
