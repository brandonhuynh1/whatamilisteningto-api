package database

import (
	"fmt"
	"time"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/config"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// NewPostgresConnection establishes a connection to the PostgreSQL database
func NewPostgresConnection(cfg config.DatabaseConfig) (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// RunMigrations applies database migrations to ensure the schema is up to date
func RunMigrations(db *sqlx.DB) error {
	// For simplicity, we're defining our schema initialization here
	// In a real app, you would use a migration tool like golang-migrate

	// Create users table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY,
			spotify_id VARCHAR(255) UNIQUE NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			display_name VARCHAR(255) NOT NULL,
			profile_url VARCHAR(255) UNIQUE NOT NULL,
			spotify_access_token TEXT NOT NULL,
			spotify_refresh_token TEXT NOT NULL,
			token_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			is_sharing_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	// Create profiles table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS profiles (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			theme VARCHAR(50) NOT NULL DEFAULT 'default',
			background_color VARCHAR(20) NOT NULL DEFAULT '#121212',
			text_color VARCHAR(20) NOT NULL DEFAULT '#FFFFFF',
			custom_message TEXT,
			show_stats BOOLEAN NOT NULL DEFAULT TRUE,
			show_history BOOLEAN NOT NULL DEFAULT TRUE,
			animation_style VARCHAR(50) NOT NULL DEFAULT 'fade',
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create profiles table: %w", err)
	}

	// Create tracks table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tracks (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			spotify_track_id VARCHAR(255) NOT NULL,
			name VARCHAR(255) NOT NULL,
			artist VARCHAR(255) NOT NULL,
			album VARCHAR(255) NOT NULL,
			album_art_url TEXT,
			track_url TEXT,
			duration_ms INTEGER NOT NULL,
			is_currently_playing BOOLEAN NOT NULL DEFAULT FALSE,
			played_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create tracks table: %w", err)
	}

	// Create profile_visits table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS profile_visits (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			visitor_ip VARCHAR(45),
			visitor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
			user_agent TEXT,
			referrer_url TEXT,
			started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			ended_at TIMESTAMP WITH TIME ZONE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create profile_visits table: %w", err)
	}

	// Create indexes
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS tracks_user_id_idx ON tracks(user_id);
		CREATE INDEX IF NOT EXISTS tracks_played_at_idx ON tracks(played_at);
		CREATE INDEX IF NOT EXISTS profile_visits_user_id_idx ON profile_visits(user_id);
		CREATE INDEX IF NOT EXISTS profile_visits_started_at_idx ON profile_visits(started_at);
	`)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}
