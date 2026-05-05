package middleware

import (
	"net/http"
	"strings"
	"time"

	"backend/internal/auth"
	"backend/internal/config"

	"github.com/gin-gonic/gin"
)

const ClaimsContextKey = "claims"

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "authorization header missing or malformed",
			})
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := auth.ParseToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "invalid or expired token",
			})
			return
		}

		threshold := time.Duration(config.App.InactivityLogoutDays) * 24 * time.Hour
		if time.Since(time.Unix(claims.LastSeen, 0)) > threshold {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "session expired due to inactivity — please log in again",
			})
			return
		}

		c.Set(ClaimsContextKey, claims)
		c.Next()
	}
}

func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := c.MustGet(ClaimsContextKey).(*auth.Claims)
		if !ok || claims.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "admin access required",
			})
			return
		}
		c.Next()
	}
}

func ClaimsFrom(c *gin.Context) (*auth.Claims, bool) {
	val, exists := c.Get(ClaimsContextKey)
	if !exists {
		return nil, false
	}
	claims, ok := val.(*auth.Claims)
	return claims, ok
}
