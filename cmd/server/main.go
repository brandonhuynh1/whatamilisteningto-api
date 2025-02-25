Let's finish implementing the User Service. We'll place this in the internal/services directory.
powershellCopy@"
package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/database"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/models"
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
			ID:                 uuid.New().String(),
			SpotifyID:          spotifyID,
			Email:              email,
			DisplayName:        displayName,
			ProfileURL:         s.generateProfileURL(displayName),
			SpotifyAccessToken: accessToken,
			SpotifyRefreshToken: refreshToken,
			TokenExpiresAt:     time.Now().Add(time.Duration(expiresIn) * time.Second),
			IsActive:           true,
			IsSharingEnabled:   true,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
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
"@ | Out-File -FilePath internal\services\user_service.go -Encoding utf8
Now, let's implement the Profile Service to handle profile customization:
powershellCopy@"
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/database"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/models"
)

// ProfileService handles profile-related operations
type ProfileService struct {
	db             *sqlx.DB
	redis          *database.RedisClient
	spotifyService *SpotifyService
	logger         zerolog.Logger
}

// NewProfileService creates a new profile service
func NewProfileService(db *sqlx.DB, redis *database.RedisClient, spotifyService *SpotifyService, logger zerolog.Logger) *ProfileService {
	return &ProfileService{
		db:             db,
		redis:          redis,
		spotifyService: spotifyService,
		logger:         logger.With().Str("service", "profile").Logger(),
	}
}

// GetProfile gets a user's profile
func (s *ProfileService) GetProfile(ctx context.Context, userID string) (*models.Profile, error) {
	var profile models.Profile
	err := s.db.GetContext(ctx, &profile, "SELECT * FROM profiles WHERE user_id = $1", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}
	return &profile, nil
}

// UpdateProfile updates a user's profile
func (s *ProfileService) UpdateProfile(ctx context.Context, userID string, updates models.Profile) error {
	// Get the current profile
	currentProfile, err := s.GetProfile(ctx, userID)
	if err != nil {
		return err
	}
	
	// Update only the fields that can be changed
	currentProfile.Theme = updates.Theme
	currentProfile.BackgroundColor = updates.BackgroundColor
	currentProfile.TextColor = updates.TextColor
	currentProfile.CustomMessage = updates.CustomMessage
	currentProfile.ShowStats = updates.ShowStats
	currentProfile.ShowHistory = updates.ShowHistory
	currentProfile.AnimationStyle = updates.AnimationStyle
	currentProfile.UpdatedAt = time.Now()
	
	// Save the updated profile
	_, err = s.db.NamedExecContext(ctx, `
		UPDATE profiles SET
			theme = :theme,
			background_color = :background_color,
			text_color = :text_color,
			custom_message = :custom_message,
			show_stats = :show_stats,
			show_history = :show_history,
			animation_style = :animation_style,
			updated_at = :updated_at
		WHERE id = :id
	`, currentProfile)
	
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}
	
	return nil
}

// GetProfileResponse gets the full profile data to show a visitor
func (s *ProfileService) GetProfileResponse(ctx context.Context, user *models.User, userService *UserService) (*models.ProfileResponse, error) {
	// Get the user's profile
	profile, err := s.GetProfile(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	
	// Get currently playing track (try cache first, then Spotify API)
	var currentTrack *models.Track
	cachedTrack, err := s.spotifyService.GetCachedCurrentlyPlaying(ctx, user.ID)
	
	// If not in cache or cache error, try Spotify API if sharing is enabled
	if err != nil || cachedTrack == nil {
		if user.IsSharingEnabled {
			// Check if token is expired and refresh if needed
			if userService.IsTokenExpired(user) {
				s.logger.Debug().Msg("Refreshing expired Spotify token")
				tokenResp, err := s.spotifyService.RefreshAccessToken(ctx, user.SpotifyRefreshToken)
				if err != nil {
					s.logger.Error().Err(err).Msg("Failed to refresh access token")
				} else {
					// Update the user's token
					err = userService.UpdateUserToken(ctx, user.ID, tokenResp.AccessToken, tokenResp.ExpiresIn)
					if err != nil {
						s.logger.Error().Err(err).Msg("Failed to update user token")
					}
					
					// Update in-memory token for immediate use
					user.SpotifyAccessToken = tokenResp.AccessToken
					user.TokenExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
				}
			}
			
			// Get currently playing from Spotify API
			spotifyTrack, err := s.spotifyService.GetCurrentlyPlayingTrack(ctx, user.SpotifyAccessToken)
			if err != nil {
				s.logger.Error().Err(err).Msg("Failed to get currently playing track")
			} else if spotifyTrack != nil && spotifyTrack.IsPlaying {
				// Cache the result
				err = s.spotifyService.CacheCurrentlyPlaying(ctx, user.ID, spotifyTrack)
				if err != nil {
					s.logger.Warn().Err(err).Msg("Failed to cache currently playing track")
				}
				
				// Convert to track model
				currentTrack = &models.Track{
					UserID:            user.ID,
					SpotifyTrackID:    spotifyTrack.TrackID,
					Name:              spotifyTrack.TrackName,
					Artist:            spotifyTrack.ArtistName,
					Album:             spotifyTrack.AlbumName,
					AlbumArtURL:       spotifyTrack.AlbumArtURL,
					TrackURL:          spotifyTrack.TrackURL,
					DurationMs:        spotifyTrack.DurationMs,
					IsCurrentlyPlaying: true,
					PlayedAt:          time.Now(),
				}
				
				// Save to track history
				s.SaveTrackToHistory(ctx, currentTrack)
				
				// Notify listeners of track change
				s.spotifyService.NotifyTrackChange(ctx, user.ID, spotifyTrack)
			}
		}
	} else if cachedTrack.IsPlaying {
		// Convert cached track to track model
		currentTrack = &models.Track{
			UserID:            user.ID,
			SpotifyTrackID:    cachedTrack.TrackID,
			Name:              cachedTrack.TrackName,
			Artist:            cachedTrack.ArtistName,
			Album:             cachedTrack.AlbumName,
			AlbumArtURL:       cachedTrack.AlbumArtURL,
			TrackURL:          cachedTrack.TrackURL,
			DurationMs:        cachedTrack.DurationMs,
			IsCurrentlyPlaying: true,
			PlayedAt:          time.Now(), // Approximate time
		}
	}
	
	// Get recent tracks if history should be shown
	var recentTracks []models.Track
	if profile.ShowHistory {
		recentTracks, err = s.GetRecentTracks(ctx, user.ID, 10)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to get recent tracks")
			recentTracks = []models.Track{} // Empty slice instead of nil
		}
	} else {
		recentTracks = []models.Track{} // Empty slice instead of nil
	}
	
	// Get active viewer count if stats should be shown
	viewerCount := 0
	if profile.ShowStats {
		count, err := userService.GetActiveUserCount(ctx, user.ID)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to get active viewer count")
		} else {
			viewerCount = count
		}
	}
	
	// Create public user info
	publicUser := models.UserPublic{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		ProfileURL:  user.ProfileURL,
	}
	
	// Create profile response
	response := &models.ProfileResponse{
		User:         publicUser,
		Profile:      *profile,
		CurrentTrack: currentTrack,
		RecentTracks: recentTracks,
		ViewerCount:  viewerCount,
	}
	
	return response, nil
}

// SaveTrackToHistory saves a track to the user's history
func (s *ProfileService) SaveTrackToHistory(ctx context.Context, track *models.Track) error {
	// Check if this track is already in history and currently playing
	var existingTrack models.Track
	err := s.db.GetContext(ctx, &existingTrack, 
		"SELECT * FROM tracks WHERE user_id = $1 AND spotify_track_id = $2 AND is_currently_playing = true",
		track.UserID, track.SpotifyTrackID)
	
	if err == nil {
		// Track exists and is currently playing, just update the played_at time
		_, err = s.db.ExecContext(ctx,
			"UPDATE tracks SET played_at = $1 WHERE id = $2",
			time.Now(), existingTrack.ID)
		
		if err != nil {
			return fmt.Errorf("failed to update track: %w", err)
		}
		
		return nil
	}
	
	// Set any currently playing tracks to not currently playing
	_, err = s.db.ExecContext(ctx,
		"UPDATE tracks SET is_currently_playing = false WHERE user_id = $1 AND is_currently_playing = true",
		track.UserID)
	
	if err != nil {
		return fmt.Errorf("failed to update currently playing tracks: %w", err)
	}
	
	// Insert the new track
	_, err = s.db.NamedExecContext(ctx, `
		INSERT INTO tracks (
			id, user_id, spotify_track_id, name, artist, album, album_art_url,
			track_url, duration_ms, is_currently_playing, played_at, created_at
		) VALUES (
			:id, :user_id, :spotify_track_id, :name, :artist, :album, :album_art_url,
			:track_url, :duration_ms, :is_currently_playing, :played_at, :created_at
		)
	`, track)
	
	if err != nil {
		return fmt.Errorf("failed to insert track: %w", err)
	}
	
	return nil
}

// GetRecentTracks gets a user's recent tracks
func (s *ProfileService) GetRecentTracks(ctx context.Context, userID string, limit int) ([]models.Track, error) {
	var tracks []models.Track
	err := s.db.SelectContext(ctx, &tracks, `
		SELECT * FROM tracks 
		WHERE user_id = $1 
		ORDER BY played_at DESC 
		LIMIT $2
	`, userID, limit)
	
	if err != nil {
		return nil, fmt.Errorf("failed to get recent tracks: %w", err)
	}
	
	return tracks, nil
}
"@ | Out-File -FilePath internal\services\profile_service.go -Encoding utf8
Now we need to implement the main server file that will tie everything together:
powershellCopymkdir -p cmd\server
@"
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

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/config"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/database"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/handlers"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/services"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/utils"
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