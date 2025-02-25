package services

import (
	"context"
	"fmt"
	"time"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/database"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/models"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
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
					UserID:             user.ID,
					SpotifyTrackID:     spotifyTrack.TrackID,
					Name:               spotifyTrack.TrackName,
					Artist:             spotifyTrack.ArtistName,
					Album:              spotifyTrack.AlbumName,
					AlbumArtURL:        spotifyTrack.AlbumArtURL,
					TrackURL:           spotifyTrack.TrackURL,
					DurationMs:         spotifyTrack.DurationMs,
					IsCurrentlyPlaying: true,
					PlayedAt:           time.Now(),
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
			UserID:             user.ID,
			SpotifyTrackID:     cachedTrack.TrackID,
			Name:               cachedTrack.TrackName,
			Artist:             cachedTrack.ArtistName,
			Album:              cachedTrack.AlbumName,
			AlbumArtURL:        cachedTrack.AlbumArtURL,
			TrackURL:           cachedTrack.TrackURL,
			DurationMs:         cachedTrack.DurationMs,
			IsCurrentlyPlaying: true,
			PlayedAt:           time.Now(), // Approximate time
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
