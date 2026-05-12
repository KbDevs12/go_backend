package user

import (
	"time"

	"backend/internal/auth"
	"backend/internal/database"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type User struct {
	ID            string     `json:"id"`
	FirebaseUID   string     `json:"firebase_uid"`
	Name          string     `json:"name"`
	Email         string     `json:"email"`
	Phone         string     `json:"phone,omitempty"`
	EmailVerified bool       `json:"email_verified"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
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
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT id, firebase_uid, name, email, COALESCE(phone,''),
		       email_verified, last_login_at, created_at
		FROM users WHERE id = $1
	`, claims.UserID).Scan(
		&u.ID, &u.FirebaseUID, &u.Name, &u.Email, &u.Phone,
		&u.EmailVerified, &u.LastLoginAt, &u.CreatedAt,
	)
	if err != nil {
		response.NotFound(c, "pengguna tidak ditemukan")
		return
	}

	response.OK(c, "profil berhasil diambil", u)
}

type UpdateProfileRequest struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
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
		response.BadRequest(c, "request tidak valid")
		return
	}

	_, err := database.Pool.Exec(c.Request.Context(),
		`UPDATE users SET name = $1, phone = $2 WHERE id = $3`,
		req.Name, req.Phone, claims.UserID)
	if err != nil {
		response.InternalError(c, "gagal memperbarui profil")
		return
	}

	response.OK(c, "profil berhasil diperbarui", nil)
}

// GetNotifications godoc
// GET /api/v1/user/notifications
func GetNotifications(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	type Notif struct {
		ID        string    `json:"id"`
		BookingID *string   `json:"booking_id,omitempty"`
		Message   string    `json:"message"`
		Type      string    `json:"type"`
		IsRead    bool      `json:"is_read"`
		CreatedAt time.Time `json:"created_at"`
	}

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id, booking_id, message, type, is_read, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 30
	`, claims.UserID)
	if err != nil {
		response.InternalError(c, "gagal mengambil notifikasi")
		return
	}
	defer rows.Close()

	var notifs []Notif
	for rows.Next() {
		var n Notif
		if err := rows.Scan(&n.ID, &n.BookingID, &n.Message, &n.Type, &n.IsRead, &n.CreatedAt); err != nil {
			continue
		}
		notifs = append(notifs, n)
	}

	if notifs == nil {
		notifs = []Notif{}
	}

	_, _ = database.Pool.Exec(c.Request.Context(),
		`UPDATE notifications SET is_read = true WHERE user_id = $1 AND is_read = false`,
		claims.UserID)

	response.OK(c, "notifikasi berhasil diambil", notifs)
}

type FCMTokenRequest struct {
	Token    string `json:"token" binding:"required"`
	Platform string `json:"platform"`
}

// Upsert FCMToken godoc
// POST /api/v1/user/fcm-token
func UpsertFCMToken(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req FCMTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		response.BadRequest(c, "request tidak valid")
		return
	}

	if req.Platform == "" {
		req.Platform = "unknown"
	}

	_, err := database.Pool.Exec(c.Request.Context(), `
		INSERT INTO user_device_tokens (id, user_id, token, platform, is_active, last_seen_at, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, true, NOW(), NOW(), NOW())
		ON CONFLICT (token) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			platform = EXCLUDED.platform,
			is_active = true,
			last_seen_at = NOW(),
			updated_at = NOW()
	`, claims.UserID, req.Token, req.Platform)
	if err != nil {
		response.InternalError(c, "gagal menyimpan token FCM")
		return
	}

	response.OK(c, "token FCM berhasil disimpan", nil)
}

// DeleteFCMToken godoc
// DELETE /api/v1/user/fcm-token
func DeleteFCMToken(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req FCMTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		response.BadRequest(c, "token FCM tidak valid")
		return
	}

	_, err := database.Pool.Exec(c.Request.Context(), `
		UPDATE user_device_tokens
		SET is_active = false, updated_at = NOW()
		WHERE user_id = $1 AND token = $2
	`, claims.UserID, req.Token)
	if err != nil {
		response.InternalError(c, "gagal menghapus token FCM")
		return
	}

	response.OK(c, "token FCM berhasil dinonaktifkan", nil)
}
