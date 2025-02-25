package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/config"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/database"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/models"
	"github.com/brandonhuynh1/whatamilisteningto-api/pkg/spotify"
	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// SpotifyService handles interaction with the Spotify API
type SpotifyService struct {
	spotifyClient *spotify.Client
	redis         *database.RedisClient
	logger        zerolog.Logger
}

// NewSpotifyService creates a new Spotify service
func NewSpotifyService(cfg config.SpotifyConfig, redis *database.RedisClient, logger zerolog.Logger) *SpotifyService {
	return &SpotifyService{
		spotifyClient: spotify.NewClient(cfg.ClientID, cfg.ClientSecret, cfg.RedirectURI),
		redis:         redis,
		logger:        logger.With().Str("service", "spotify").Logger(),
	}
}

// GetAuthURL returns the Spotify authorization URL
func (s *SpotifyService) GetAuthURL(state string) string {
	return s.spotifyClient.GetAuthURL(state, []string{
		"user-read-private",
		"user-read-email",
		"user-read-currently-playing",
	})
}

// ExchangeCodeForToken exchanges an authorization code for tokens
func (s *SpotifyService) ExchangeCodeForToken(ctx context.Context, code string) (*spotify.TokenResponse, error) {
	return s.spotifyClient.ExchangeCodeForToken(ctx, code)
}

// RefreshAccessToken refreshes an access token
func (s *SpotifyService) RefreshAccessToken(ctx context.Context, refreshToken string) (*spotify.TokenResponse, error) {
	return s.spotifyClient.RefreshAccessToken(ctx, refreshToken)
}

// GetUserProfile gets a user's Spotify profile
func (s *SpotifyService) GetUserProfile(ctx context.Context, accessToken string) (string, string, string, error) {
	profile, err := s.spotifyClient.GetUserProfile(ctx, accessToken)
	if err != nil {
		return "", "", "", err
	}

	spotifyID, _ := profile["id"].(string)
	email, _ := profile["email"].(string)
	displayName, _ := profile["display_name"].(string)

	return spotifyID, email, displayName, nil
}

// GetCurrentlyPlayingTrack gets the user's currently playing track
func (s *SpotifyService) GetCurrentlyPlayingTrack(ctx context.Context, accessToken string) (*models.SpotifyCurrentlyPlaying, error) {
	result, err := s.spotifyClient.GetCurrentlyPlaying(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	// If nothing is playing
	if result == nil {
		return &models.SpotifyCurrentlyPlaying{
			IsPlaying: false,
		}, nil
	}

	// Extract track information
	isPlaying, _ := result["is_playing"].(bool)

	item, ok := result["item"].(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid response format")
	}

	trackID, _ := item["id"].(string)
	trackName, _ := item["name"].(string)
	trackURL, _ := item["external_urls"].(map[string]interface{})["spotify"].(string)
	durationMs, _ := item["duration_ms"].(float64)
	progressMs, _ := result["progress_ms"].(float64)

	// Extract album information
	album, ok := item["album"].(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid album format")
	}

	albumName, _ := album["name"].(string)

	// Get album art (use the second-to-last image for medium size)
	var albumArtURL string
	if images, ok := album["images"].([]interface{}); ok && len(images) > 0 {
		imageIdx := 1 // medium size
		if len(images) == 1 {
			imageIdx = 0
		}
		if image, ok := images[imageIdx].(map[string]interface{}); ok {
			albumArtURL, _ = image["url"].(string)
		}
	}

	// Extract artist information
	var artistName string
	if artists, ok := item["artists"].([]interface{}); ok && len(artists) > 0 {
		if artist, ok := artists[0].(map[string]interface{}); ok {
			artistName, _ = artist["name"].(string)
		}
	}

	return &models.SpotifyCurrentlyPlaying{
		IsPlaying:   isPlaying,
		TrackID:     trackID,
		TrackName:   trackName,
		ArtistName:  artistName,
		AlbumName:   albumName,
		AlbumArtURL: albumArtURL,
		TrackURL:    trackURL,
		DurationMs:  int(durationMs),
		ProgressMs:  int(progressMs),
	}, nil
}

// CacheCurrentlyPlaying caches the currently playing track in Redis
func (s *SpotifyService) CacheCurrentlyPlaying(ctx context.Context, userID string, track *models.SpotifyCurrentlyPlaying) error {
	// Convert track to JSON
	trackJSON, err := json.Marshal(track)
	if err != nil {
		return err
	}

	// Store in Redis with 2-minute expiration
	key := fmt.Sprintf("track:current:%s", userID)
	return s.redis.Set(ctx, key, trackJSON, 2*time.Minute)
}

// GetCachedCurrentlyPlaying gets a cached currently playing track from Redis
func (s *SpotifyService) GetCachedCurrentlyPlaying(ctx context.Context, userID string) (*models.SpotifyCurrentlyPlaying, error) {
	key := fmt.Sprintf("track:current:%s", userID)
	trackJSON, err := s.redis.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var track models.SpotifyCurrentlyPlaying
	if err := json.Unmarshal([]byte(trackJSON), &track); err != nil {
		return nil, err
	}

	return &track, nil
}

// NotifyTrackChange publishes a track change to Redis pub/sub
func (s *SpotifyService) NotifyTrackChange(ctx context.Context, userID string, track *models.SpotifyCurrentlyPlaying) error {
	// Convert track to JSON
	trackJSON, err := json.Marshal(track)
	if err != nil {
		return err
	}

	// Publish to channel for this user
	channel := fmt.Sprintf("track:updates:%s", userID)
	return s.redis.Publish(ctx, channel, trackJSON)
}

// SubscribeToTrackUpdates subscribes to track updates for a user
func (s *SpotifyService) SubscribeToTrackUpdates(ctx context.Context, userID string) *redis.PubSub {
	channel := fmt.Sprintf("track:updates:%s", userID)
	return s.redis.Subscribe(ctx, channel)
}

// GetTrackHistory gets a user's track history
func (s *SpotifyService) GetTrackHistory(ctx context.Context, userID string, limit int) ([]models.Track, error) {
	var tracks []models.Track
	query := `
		SELECT * FROM tracks 
		WHERE user_id = $1 
		ORDER BY played_at DESC 
		LIMIT $2
	`

	db, ok := ctx.Value("db").(*sqlx.DB)
	if !ok {
		return nil, errors.New("database connection not found in context")
	}

	err := db.SelectContext(ctx, &tracks, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get track history: %w", err)
	}

	return tracks, nil
}
