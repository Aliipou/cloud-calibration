package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aliipou/cloud-calibration/internal/api"
	"github.com/aliipou/cloud-calibration/internal/calibration"
	"github.com/aliipou/cloud-calibration/internal/store"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	// Bootstrap logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialise logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	// Configuration from environment
	dsn := getenv("DATABASE_URL", "postgres://caluser:calpass@localhost:5432/calibration")
	port := getenv("HTTP_PORT", "8080")

	logger.Info("starting cloud-calibration API",
		zap.String("port", port),
	)

	// Connect to PostgreSQL
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.New(ctx, dsn)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()
	logger.Info("connected to PostgreSQL")

	// Build service layer
	svc := calibration.NewService(db)

	// Set up Gin
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(
		gin.Recovery(),
		requestLogger(logger),
	)

	handler := api.NewHandler(db, svc)
	api.RegisterRoutes(router, handler)

	// HTTP server with graceful shutdown
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
	case err := <-serverErr:
		logger.Error("server error", zap.Error(err))
	}

	// Graceful shutdown with 30s deadline
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	} else {
		logger.Info("server shutdown complete")
	}
}

// getenv returns the value of an environment variable or the provided default.
func getenv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// requestLogger returns a Gin middleware that logs each request using zap.
func requestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		)
	}
}
