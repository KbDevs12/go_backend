package user

import (
	"time"

	"backend/internal/auth"
	"backend/internal/database"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type User struct {
	ID             string     `json:"id"`
	FirebaseUID    string     `json:"firebase_uid"`
	Email          string     `json:"email"`
	DisplayName    string     `json:"display_name"`
	PhoneNumber    string     `json:"phone_number,omitempty"`
	Role           string     `json:"role"`
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// GetProfile godoc
// GET /api/v1/user/profile
func GetProfile(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var u User
	err := database.Pool.QueryRow(c.Request.Context(),
		`SELECT id, firebase_uid, email, display_name, COALESCE(phone_number,''), role,
		        last_login_at, last_activity_at, created_at
		 FROM users WHERE id = $1`, claims.UserID,
	).Scan(&u.ID, &u.FirebaseUID, &u.Email, &u.DisplayName, &u.PhoneNumber,
		&u.Role, &u.LastLoginAt, &u.LastActivityAt, &u.CreatedAt)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	response.OK(c, "profile retrieved", u)
}

type UpdateProfileRequest struct {
	DisplayName string `json:"display_name"`
	PhoneNumber string `json:"phone_number"`
}

// UpdateProfile godoc
// PATCH /api/v1/user/profile
func UpdateProfile(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	_, err := database.Pool.Exec(c.Request.Context(),
		`UPDATE users SET display_name = $1, phone_number = $2 WHERE id = $3`,
		req.DisplayName, req.PhoneNumber, claims.UserID)
	if err != nil {
		response.InternalError(c, "failed to update profile")
		return
	}

	response.OK(c, "profile updated", nil)
}
