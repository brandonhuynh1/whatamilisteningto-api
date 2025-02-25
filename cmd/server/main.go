package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/config"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/database"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/handlers"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/services"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Initialize logger
	logger := utils.NewLogger()
	logger.Info().Msg("Starting Music Sharing App")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Set Gin mode
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		logger.Info().Msg("Running in development mode")
	}

	// Initialize database connections
	logger.Info().Msg("Connecting to PostgreSQL")
	db, err := database.NewPostgresConnection(cfg.Database)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}
	defer db.Close()

	// Run database migrations
	logger.Info().Msg("Running database migrations")
	if err := database.RunMigrations(db); err != nil {
		logger.Fatal().Err(err).Msg("Failed to run database migrations")
	}

	// Initialize Redis
	logger.Info().Msg("Connecting to Redis")
	redisClient, err := database.NewRedisClient(cfg.Redis)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer redisClient.Close()

	// Initialize services
	userService := services.NewUserService(db, redisClient, logger)
	spotifyService := services.NewSpotifyService(cfg.Spotify, redisClient, logger)
	profileService := services.NewProfileService(db, redisClient, spotifyService, logger)

	// Initialize router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(utils.LoggerMiddleware(logger))

	// Register routes
	logger.Info().Msg("Registering routes")
	handlers.RegisterAuthHandlers(router, userService, spotifyService, logger)
	handlers.RegisterProfileHandlers(router, profileService, userService, logger)
	handlers.RegisterTrackHandlers(router, spotifyService, userService, logger)

	// Serve static files
	router.Static("/static", "./web/static")
	router.LoadHTMLGlob("./web/templates/*")

	// Setup server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutSeconds) * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Info().Msgf("Starting server on port %d", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("Shutting down server...")

	// Create a deadline for server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.GracefulShutdownSeconds)*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	logger.Info().Msg("Server exiting")
}
