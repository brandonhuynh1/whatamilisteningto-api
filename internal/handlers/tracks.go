package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// RegisterTrackHandlers registers all track-related routes
func RegisterTrackHandlers(r *gin.Engine, spotifyService *services.SpotifyService, userService *services.UserService, logger zerolog.Logger) {
	handler := &trackHandler{
		spotifyService: spotifyService,
		userService:    userService,
		logger:         logger.With().Str("handler", "track").Logger(),
	}

	// WebSocket endpoint for real-time updates
	r.GET("/ws/tracks/:profileURL", handler.trackUpdatesWebSocket)

	// API endpoints
	tracks := r.Group("/api/tracks")
	tracks.Use(authMiddleware(userService))
	{
		tracks.GET("/current", handler.getCurrentTrack)
		tracks.GET("/history", handler.getTrackHistory)
		tracks.POST("/refresh", handler.refreshCurrentTrack)
	}
}

type trackHandler struct {
	spotifyService *services.SpotifyService
	userService    *services.UserService
	logger         zerolog.Logger
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now, in production you might want to restrict this
	},
}

// trackUpdatesWebSocket handles WebSocket connections for real-time track updates
func (h *trackHandler) trackUpdatesWebSocket(c *gin.Context) {
	profileURL := c.Param("profileURL")

	// Get user by profile URL
	user, err := h.userService.GetUserByProfileURL(c.Request.Context(), profileURL)
	if err != nil {
		h.logger.Error().Err(err).Str("profileURL", profileURL).Msg("Profile not found")
		c.JSON(http.StatusNotFound, gin.H{"error": "Profile not found"})
		return
	}

	// Verify that the user is active and sharing
	if !user.IsActive || !user.IsSharingEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "Profile not available"})
		return
	}

	// Validate the visitor
	visitID, err := c.Cookie("visit_id")
	if err != nil {
		h.logger.Error().Err(err).Msg("Missing visit_id cookie")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Upgrade to WebSocket connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to upgrade to WebSocket connection")
		return
	}
	defer conn.Close()

	// Subscribe to Redis channel for track updates
	ctx := c.Request.Context()
	pubsub := h.spotifyService.SubscribeToTrackUpdates(ctx, user.ID)
	defer pubsub.Close()
	ch := pubsub.Channel()

	// Send initial track data
	cachedTrack, err := h.spotifyService.GetCachedCurrentlyPlaying(ctx, user.ID)
	if err == nil && cachedTrack != nil && cachedTrack.IsPlaying {
		if err := conn.WriteJSON(cachedTrack); err != nil {
			h.logger.Error().Err(err).Msg("Failed to send initial track data")
			return
		}
	}

	// Renewal routine for visitor activity
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Renew visitor activity
				err := h.userService.RenewVisitorActivity(ctx, visitID)
				if err != nil {
					h.logger.Error().Err(err).Msg("Failed to renew visitor activity")
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Listen for messages from Redis channel
	for {
		select {
		case msg := <-ch:
			// Forward track update to the WebSocket client
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				h.logger.Error().Err(err).Msg("Failed to write to WebSocket")
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// getCurrentTrack gets the user's currently playing track
func (h *trackHandler) getCurrentTrack(c *gin.Context) {
	userID := c.GetString("user_id")

	// Get user to check if sharing is enabled
	user, err := h.userService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Str("userID", userID).Msg("Failed to get user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
		return
	}

	if !user.IsSharingEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "Music sharing is disabled"})
		return
	}

	// Check if token is expired and refresh if needed
	if h.userService.IsTokenExpired(user) {
		tokenResp, err := h.spotifyService.RefreshAccessToken(c.Request.Context(), user.SpotifyRefreshToken)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to refresh access token")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refresh Spotify access"})
			return
		}

		// Update user's token
		err = h.userService.UpdateUserToken(c.Request.Context(), user.ID, tokenResp.AccessToken, tokenResp.ExpiresIn)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to update user token")
		}

		// Update in-memory token for immediate use
		user.SpotifyAccessToken = tokenResp.AccessToken
	}

	// Try to get from cache first
	cachedTrack, err := h.spotifyService.GetCachedCurrentlyPlaying(c.Request.Context(), user.ID)
	if err == nil && cachedTrack != nil && cachedTrack.IsPlaying {
		c.JSON(http.StatusOK, cachedTrack)
		return
	}

	// Get from Spotify API
	track, err := h.spotifyService.GetCurrentlyPlayingTrack(c.Request.Context(), user.SpotifyAccessToken)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get currently playing track")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get track from Spotify"})
		return
	}

	// Cache the result
	if track.IsPlaying {
		err = h.spotifyService.CacheCurrentlyPlaying(c.Request.Context(), user.ID, track)
		if err != nil {
			h.logger.Warn().Err(err).Msg("Failed to cache currently playing track")
		}
	}

	c.JSON(http.StatusOK, track)
}

// getTrackHistory gets the user's track history
func (h *trackHandler) getTrackHistory(c *gin.Context) {
	userID := c.GetString("user_id")

	// Get limit from query parameters, default to 10
	limit := 10
	if limitParam := c.Query("limit"); limitParam != "" {
		// Convert string to int properly
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil {
			limit = parsedLimit
		}
	}

	// Get tracks from database
	tracks, err := h.spotifyService.GetTrackHistory(c.Request.Context(), userID, limit)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get track history")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get track history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tracks": tracks})
}

// refreshCurrentTrack manually refreshes the user's currently playing track
func (h *trackHandler) refreshCurrentTrack(c *gin.Context) {
	userID := c.GetString("user_id")

	// Get user to check if sharing is enabled
	user, err := h.userService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Str("userID", userID).Msg("Failed to get user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
		return
	}

	if !user.IsSharingEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "Music sharing is disabled"})
		return
	}

	// Check if token is expired and refresh if needed
	if h.userService.IsTokenExpired(user) {
		tokenResp, err := h.spotifyService.RefreshAccessToken(c.Request.Context(), user.SpotifyRefreshToken)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to refresh access token")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refresh Spotify access"})
			return
		}

		// Update user's token
		err = h.userService.UpdateUserToken(c.Request.Context(), user.ID, tokenResp.AccessToken, tokenResp.ExpiresIn)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to update user token")
		}

		// Update in-memory token for immediate use
		user.SpotifyAccessToken = tokenResp.AccessToken
	}

	// Get from Spotify API
	track, err := h.spotifyService.GetCurrentlyPlayingTrack(c.Request.Context(), user.SpotifyAccessToken)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get currently playing track")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get track from Spotify"})
		return
	}

	// Cache the result and notify subscribers
	if track.IsPlaying {
		err = h.spotifyService.CacheCurrentlyPlaying(c.Request.Context(), user.ID, track)
		if err != nil {
			h.logger.Warn().Err(err).Msg("Failed to cache currently playing track")
		}

		err = h.spotifyService.NotifyTrackChange(c.Request.Context(), user.ID, track)
		if err != nil {
			h.logger.Warn().Err(err).Msg("Failed to notify track change")
		}
	}

	c.JSON(http.StatusOK, track)
}
