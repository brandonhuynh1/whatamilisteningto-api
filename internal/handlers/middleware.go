package handlers

import (
	"net/http"

	"github.com/brandonhuynh1/whatamilisteningto-api/internal/services"
	"github.com/gin-gonic/gin"
)

// authMiddleware checks if the user is authenticated
func authMiddleware(userService *services.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := c.Cookie("user_id")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		user, err := userService.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.SetCookie("user_id", "", -1, "/", "", false, true)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication"})
			c.Abort()
			return
		}

		// Store user ID in context for handlers to use
		c.Set("user_id", user.ID)
		c.Next()
	}
}
