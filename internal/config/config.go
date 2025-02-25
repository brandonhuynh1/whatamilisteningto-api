package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the application
type Config struct {
	Environment string
	Server      ServerConfig
	Database    DatabaseConfig
	Redis       RedisConfig
	Spotify     SpotifyConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port                    int
	ReadTimeoutSeconds      int
	WriteTimeoutSeconds     int
	IdleTimeoutSeconds      int
	GracefulShutdownSeconds int
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// SpotifyConfig holds Spotify API configuration
type SpotifyConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	return &Config{
		Environment: getEnv("APP_ENV", "development"),
		Server: ServerConfig{
			Port:                    getEnvAsInt("SERVER_PORT", 8080),
			ReadTimeoutSeconds:      getEnvAsInt("SERVER_READ_TIMEOUT", 10),
			WriteTimeoutSeconds:     getEnvAsInt("SERVER_WRITE_TIMEOUT", 10),
			IdleTimeoutSeconds:      getEnvAsInt("SERVER_IDLE_TIMEOUT", 60),
			GracefulShutdownSeconds: getEnvAsInt("SERVER_SHUTDOWN_TIMEOUT", 30),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvAsInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			DBName:   getEnv("DB_NAME", "music_sharing"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvAsInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
		Spotify: SpotifyConfig{
			ClientID:     getEnv("SPOTIFY_CLIENT_ID", ""),
			ClientSecret: getEnv("SPOTIFY_CLIENT_SECRET", ""),
			RedirectURI:  getEnv("SPOTIFY_REDIRECT_URI", "http://localhost:8080/auth/spotify/callback"),
			Scopes:       strings.Split(getEnv("SPOTIFY_SCOPES", "user-read-private user-read-email user-read-currently-playing"), " "),
		},
	}, nil
}

// Helper functions for reading environment variables
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}
