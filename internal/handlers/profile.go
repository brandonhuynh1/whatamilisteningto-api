package handlers

import (
	"net/http"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/models"
	"github.com/brandonhuynh1/whatamilisteningto-api/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// RegisterProfileHandlers registers all profile-related routes
func RegisterProfileHandlers(r *gin.Engine, profileService *services.ProfileService, userService *services.UserService, logger zerolog.Logger) {
	handler := &profileHandler{
		profileService: profileService,
		userService:    userService,
		logger:         logger.With().Str("handler", "profile").Logger(),
	}

	// Public routes
	r.GET("/profile/:profileURL", handler.getPublicProfile)

	// Protected routes
	profile := r.Group("/api/profile")
	profile.Use(authMiddleware(userService))
	{
		profile.GET("", handler.getProfile)
		profile.PUT("", handler.updateProfile)
		profile.PUT("/settings", handler.updateSettings)
	}
}

type profileHandler struct {
	profileService *services.ProfileService
	userService    *services.UserService
	logger         zerolog.Logger
}

// getPublicProfile returns the public profile for a given URL
func (h *profileHandler) getPublicProfile(c *gin.Context) {
	profileURL := c.Param("profileURL")

	// Get user by profile URL
	user, err := h.userService.GetUserByProfileURL(c.Request.Context(), profileURL)
	if err != nil {
		h.logger.Error().Err(err).Str("profileURL", profileURL).Msg("Profile not found")
		c.HTML(http.StatusNotFound, "404.html", gin.H{
			"error": "Profile not found",
		})
		return
	}

	// If user is not active or not sharing
	if !user.IsActive || !user.IsSharingEnabled {
		c.HTML(http.StatusNotFound, "profile_unavailable.html", gin.H{
			"username": user.DisplayName,
		})
		return
	}

	// Record the visit
	visitorIP := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	referrer := c.GetHeader("Referer")

	var visitorUserID *string
	loggedInUserID, _ := c.Cookie("user_id")
	if loggedInUserID != "" && loggedInUserID != user.ID {
		visitorUserID = &loggedInUserID
	}

	visitID, err := h.userService.RecordProfileVisit(
		c.Request.Context(),
		user.ID,
		visitorIP,
		userAgent,
		referrer,
		visitorUserID,
	)

	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to record profile visit")
	} else {
		// Set the visit ID cookie for WebSocket authentication
		c.SetCookie("visit_id", visitID, 0, "/", "", false, false)
	}

	// Get profile data
	profileResponse, err := h.profileService.GetProfileResponse(c.Request.Context(), user, h.userService)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get profile data")
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Failed to load profile data",
		})
		return
	}

	// Render profile page
	c.HTML(http.StatusOK, "profile.html", gin.H{
		"profile": profileResponse,
	})
}

// getProfile returns the authenticated user's profile
func (h *profileHandler) getProfile(c *gin.Context) {
	userID := c.GetString("user_id")

	profile, err := h.profileService.GetProfile(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Str("userID", userID).Msg("Failed to get profile")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get profile"})
		return
	}

	c.JSON(http.StatusOK, profile)
}

// updateProfile updates the authenticated user's profile
func (h *profileHandler) updateProfile(c *gin.Context) {
	userID := c.GetString("user_id")

	var profileUpdates models.Profile
	if err := c.ShouldBindJSON(&profileUpdates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err := h.profileService.UpdateProfile(c.Request.Context(), userID, profileUpdates)
	if err != nil {
		h.logger.Error().Err(err).Str("userID", userID).Msg("Failed to update profile")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// updateSettings updates the user's sharing settings
func (h *profileHandler) updateSettings(c *gin.Context) {
	userID := c.GetString("user_id")

	var settings struct {
		IsSharingEnabled bool `json:"isSharingEnabled"`
	}

	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err := h.userService.UpdateUserSettings(c.Request.Context(), userID, settings.IsSharingEnabled)
	if err != nil {
		h.logger.Error().Err(err).Str("userID", userID).Msg("Failed to update settings")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
