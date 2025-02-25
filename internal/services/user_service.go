package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/database"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// UserService handles user-related operations
type UserService struct {
	db     *sqlx.DB
	redis  *database.RedisClient
	logger zerolog.Logger
}

// NewUserService creates a new user service
func NewUserService(db *sqlx.DB, redis *database.RedisClient, logger zerolog.Logger) *UserService {
	return &UserService{
		db:     db,
		redis:  redis,
		logger: logger.With().Str("service", "user").Logger(),
	}
}

// CreateOrUpdateUser creates a new user or updates an existing one
func (s *UserService) CreateOrUpdateUser(ctx context.Context, spotifyID, email, displayName string, accessToken, refreshToken string, expiresIn int) (*models.User, error) {
	// Check if user exists
	var user models.User
	err := s.db.GetContext(ctx, &user, "SELECT * FROM users WHERE spotify_id = $1", spotifyID)

	if err != nil {
		// User doesn't exist, create new user
		newUser := models.User{
			ID:                  uuid.New().String(),
			SpotifyID:           spotifyID,
			Email:               email,
			DisplayName:         displayName,
			ProfileURL:          s.generateProfileURL(displayName),
			SpotifyAccessToken:  accessToken,
			SpotifyRefreshToken: refreshToken,
			TokenExpiresAt:      time.Now().Add(time.Duration(expiresIn) * time.Second),
			IsActive:            true,
			IsSharingEnabled:    true,
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		}

		_, err := s.db.NamedExecContext(ctx, `
			INSERT INTO users (
				id, spotify_id, email, display_name, profile_url, 
				spotify_access_token, spotify_refresh_token, token_expires_at,
				is_active, is_sharing_enabled, created_at, updated_at
			) VALUES (
				:id, :spotify_id, :email, :display_name, :profile_url,
				:spotify_access_token, :spotify_refresh_token, :token_expires_at,
				:is_active, :is_sharing_enabled, :created_at, :updated_at
			)
		`, newUser)

		if err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}

		// Create default profile for the new user
		profile := models.Profile{
			ID:              uuid.New().String(),
			UserID:          newUser.ID,
			Theme:           "default",
			BackgroundColor: "#121212",
			TextColor:       "#FFFFFF",
			AnimationStyle:  "fade",
			ShowStats:       true,
			ShowHistory:     true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		_, err = s.db.NamedExecContext(ctx, `
			INSERT INTO profiles (
				id, user_id, theme, background_color, text_color,
				custom_message, show_stats, show_history, animation_style,
				created_at, updated_at
			) VALUES (
				:id, :user_id, :theme, :background_color, :text_color,
				:custom_message, :show_stats, :show_history, :animation_style,
				:created_at, :updated_at
			)
		`, profile)

		if err != nil {
			return nil, fmt.Errorf("failed to create profile: %w", err)
		}

		return &newUser, nil
	}

	// User exists, update tokens
	user.SpotifyAccessToken = accessToken
	user.SpotifyRefreshToken = refreshToken
	user.TokenExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	user.UpdatedAt = time.Now()

	_, err = s.db.NamedExecContext(ctx, `
		UPDATE users SET
			spotify_access_token = :spotify_access_token,
			spotify_refresh_token = :spotify_refresh_token,
			token_expires_at = :token_expires_at,
			updated_at = :updated_at
		WHERE id = :id
	`, user)

	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return &user, nil
}

// GetUserByID gets a user by ID
func (s *UserService) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	err := s.db.GetContext(ctx, &user, "SELECT * FROM users WHERE id = $1", id)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// GetUserByProfileURL gets a user by profile URL
func (s *UserService) GetUserByProfileURL(ctx context.Context, profileURL string) (*models.User, error) {
	var user models.User
	err := s.db.GetContext(ctx, &user, "SELECT * FROM users WHERE profile_url = $1", profileURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get user by profile URL: %w", err)
	}
	return &user, nil
}

// UpdateUserSettings updates a user's settings
func (s *UserService) UpdateUserSettings(ctx context.Context, userID string, isSharingEnabled bool) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE users SET is_sharing_enabled = $1, updated_at = $2 WHERE id = $3",
		isSharingEnabled, time.Now(), userID)

	if err != nil {
		return fmt.Errorf("failed to update user settings: %w", err)
	}

	return nil
}

// IsTokenExpired checks if a user's token is expired or about to expire
func (s *UserService) IsTokenExpired(user *models.User) bool {
	// Consider token expired if it expires in less than 5 minutes
	return user.TokenExpiresAt.Before(time.Now().Add(5 * time.Minute))
}

// UpdateUserToken updates a user's Spotify access token
func (s *UserService) UpdateUserToken(ctx context.Context, userID, accessToken string, expiresIn int) error {
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
	_, err := s.db.ExecContext(ctx,
		"UPDATE users SET spotify_access_token = $1, token_expires_at = $2, updated_at = $3 WHERE id = $4",
		accessToken, expiresAt, time.Now(), userID)

	if err != nil {
		return fmt.Errorf("failed to update user token: %w", err)
	}

	return nil
}

// GetActiveUserCount gets the count of currently active viewers for a profile
func (s *UserService) GetActiveUserCount(ctx context.Context, userID string) (int, error) {
	key := fmt.Sprintf("visitors:%s", userID)
	count, err := s.redis.GetSetSize(ctx, key)
	if err != nil {
		return 0, fmt.Errorf("failed to get active viewer count: %w", err)
	}

	return int(count), nil
}

// RecordProfileVisit records a new profile visit
func (s *UserService) RecordProfileVisit(ctx context.Context, userID string, visitorIP, userAgent, referrerURL string, visitorUserID *string) (string, error) {
	// Create a new profile visit record
	visitID := uuid.New().String()
	visit := models.ProfileVisit{
		ID:            visitID,
		UserID:        userID,
		VisitorIP:     visitorIP,
		VisitorUserID: visitorUserID,
		UserAgent:     userAgent,
		ReferrerURL:   referrerURL,
		StartedAt:     time.Now(),
	}

	_, err := s.db.NamedExecContext(ctx, `
		INSERT INTO profile_visits (
			id, user_id, visitor_ip, visitor_user_id, user_agent, referrer_url, started_at
		) VALUES (
			:id, :user_id, :visitor_ip, :visitor_user_id, :user_agent, :referrer_url, :started_at
		)
	`, visit)

	if err != nil {
		return "", fmt.Errorf("failed to record profile visit: %w", err)
	}

	// Add to active visitors set with 5-minute expiration
	visitorKey := fmt.Sprintf("visitor:%s", visitID)
	err = s.redis.Set(ctx, visitorKey, "1", 5*time.Minute)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to set visitor key in Redis")
	}

	// Add to active visitors set for this profile
	activeVisitorsKey := fmt.Sprintf("visitors:%s", userID)
	err = s.redis.AddToSet(ctx, activeVisitorsKey, visitID)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to add to active visitors set")
	}

	return visitID, nil
}

// EndProfileVisit marks a profile visit as ended
func (s *UserService) EndProfileVisit(ctx context.Context, visitID string) error {
	// Get the visit to find the user ID
	var visit models.ProfileVisit
	err := s.db.GetContext(ctx, &visit, "SELECT * FROM profile_visits WHERE id = $1", visitID)
	if err != nil {
		return fmt.Errorf("failed to get profile visit: %w", err)
	}

	// Update the visit end time
	now := time.Now()
	_, err = s.db.ExecContext(ctx,
		"UPDATE profile_visits SET ended_at = $1 WHERE id = $2",
		now, visitID)

	if err != nil {
		return fmt.Errorf("failed to update profile visit: %w", err)
	}

	// Remove from active visitors set
	activeVisitorsKey := fmt.Sprintf("visitors:%s", visit.UserID)
	err = s.redis.RemoveFromSet(ctx, activeVisitorsKey, visitID)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to remove from active visitors set")
	}

	// Delete visitor key
	visitorKey := fmt.Sprintf("visitor:%s", visitID)
	err = s.redis.Delete(ctx, visitorKey)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to delete visitor key")
	}

	return nil
}

// RenewVisitorActivity renews a visitor's activity timeout
func (s *UserService) RenewVisitorActivity(ctx context.Context, visitID string) error {
	// Set visitor key with new 5-minute expiration
	visitorKey := fmt.Sprintf("visitor:%s", visitID)
	return s.redis.Set(ctx, visitorKey, "1", 5*time.Minute)
}

// generateProfileURL creates a unique profile URL from a display name
func (s *UserService) generateProfileURL(displayName string) string {
	// Convert to lowercase
	urlBase := strings.ToLower(displayName)

	// Replace spaces with hyphens and remove special characters
	urlBase = strings.ReplaceAll(urlBase, " ", "-")
	urlBase = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return -1
	}, urlBase)

	// Check if URL already exists, if so, add a random suffix
	var count int
	err := s.db.Get(&count, "SELECT COUNT(*) FROM users WHERE profile_url = $1", urlBase)
	if err != nil || count > 0 {
		// Add a random suffix (last 6 chars of a UUID)
		suffix := uuid.New().String()
		suffix = suffix[len(suffix)-6:]
		urlBase = urlBase + "-" + suffix
	}

	return urlBase
}
