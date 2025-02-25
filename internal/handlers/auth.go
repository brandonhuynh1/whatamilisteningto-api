package handlers

import (
	"net/http"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// RegisterAuthHandlers registers all auth-related routes
func RegisterAuthHandlers(r *gin.Engine, userService *services.UserService, spotifyService *services.SpotifyService, logger zerolog.Logger) {
	handler := &authHandler{
		userService:    userService,
		spotifyService: spotifyService,
		logger:         logger.With().Str("handler", "auth").Logger(),
	}

	auth := r.Group("/auth")
	{
		auth.GET("/spotify", handler.initiateSpotifyAuth)
		auth.GET("/spotify/callback", handler.handleSpotifyCallback)
		auth.GET("/logout", handler.logout)
		auth.GET("/status", handler.checkAuthStatus)
	}
}

type authHandler struct {
	userService    *services.UserService
	spotifyService *services.SpotifyService
	logger         zerolog.Logger
}

// initiateSpotifyAuth redirects to Spotify's auth page
func (h *authHandler) initiateSpotifyAuth(c *gin.Context) {
	// Generate a random state for security
	state := uuid.New().String()

	// Store state in cookie for validation later
	c.SetCookie("spotify_auth_state", state, 60*15, "/", "", false, true)

	// Redirect to Spotify login
	authURL := h.spotifyService.GetAuthURL(state)
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// handleSpotifyCallback processes Spotify's auth callback
func (h *authHandler) handleSpotifyCallback(c *gin.Context) {
	// Get code and state from query params
	code := c.Query("code")
	state := c.Query("state")

	// Get stored state from cookie
	storedState, err := c.Cookie("spotify_auth_state")
	if err != nil || state != storedState {
		h.logger.Error().Err(err).Str("provided_state", state).Str("stored_state", storedState).Msg("State validation failed")
		c.JSON(http.StatusBadRequest, gin.H{"error": "State validation failed"})
		return
	}

	// Exchange code for tokens
	tokenResponse, err := h.spotifyService.ExchangeCodeForToken(c.Request.Context(), code)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to exchange code for token")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to authenticate with Spotify"})
		return
	}

	// Get user info from Spotify
	spotifyID, email, displayName, err := h.spotifyService.GetUserProfile(c.Request.Context(), tokenResponse.AccessToken)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get user profile from Spotify")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user profile"})
		return
	}

	// Create or update user
	user, err := h.userService.CreateOrUpdateUser(
		c.Request.Context(),
		spotifyID,
		email,
		displayName,
		tokenResponse.AccessToken,
		tokenResponse.RefreshToken,
		tokenResponse.ExpiresIn,
	)

	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to create/update user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process user data"})
		return
	}

	// Create session for user
	c.SetCookie("user_id", user.ID, 3600*24*30, "/", "", false, true)

	// Redirect to user's profile
	c.Redirect(http.StatusTemporaryRedirect, "/profile/"+user.ProfileURL)
}

// logout logs the user out
func (h *authHandler) logout(c *gin.Context) {
	// Clear cookies
	c.SetCookie("user_id", "", -1, "/", "", false, true)

	// Redirect to home page
	c.Redirect(http.StatusTemporaryRedirect, "/")
}

// checkAuthStatus checks if the user is authenticated
func (h *authHandler) checkAuthStatus(c *gin.Context) {
	userID, err := c.Cookie("user_id")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
		return
	}

	user, err := h.userService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.SetCookie("user_id", "", -1, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"user": gin.H{
			"id":          user.ID,
			"displayName": user.DisplayName,
			"profileUrl":  user.ProfileURL,
			"isSharing":   user.IsSharingEnabled,
		},
	})
}
