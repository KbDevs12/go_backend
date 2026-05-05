package auth

import (
	"context"
	"net/http"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/pkg/firebase"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	FirebaseIDToken string `json:"firebase_id_token" binding:"required"`
}

type LoginResponse struct {
	AccessToken          string `json:"access_token"`
	TokenType            string `json:"token_type"`
	ExpiresIn            int    `json:"expires_in"`
	InactivityLogoutDays int    `json:"inactivity_logout_days"`
	UserID               string `json:"user_id"`
	Email                string `json:"email"`
	DisplayName          string `json:"display_name"`
	Role                 string `json:"role"`
}

// Login godoc
// POST /api/v1/auth/login
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "firebase_id_token is required")
		return
	}

	ctx := c.Request.Context()

	fireToken, err := firebase.VerifyIDToken(ctx, req.FirebaseIDToken)
	if err != nil {
		response.Unauthorized(c, "invalid or expired firebase token")
		return
	}

	fireUser, err := firebase.GetUser(ctx, fireToken.UID)
	if err != nil {
		response.InternalError(c, "failed to fetch firebase user")
		return
	}
	if !fireUser.EmailVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "email not verified — please verify your email before logging in",
		})
		return
	}

	userID, role, displayName, err := upsertUser(ctx, fireUser.UID, fireUser.Email, fireUser.DisplayName)
	if err != nil {
		response.InternalError(c, "failed to process user account")
		return
	}

	token, err := GenerateToken(userID, fireUser.UID, fireUser.Email, role)
	if err != nil {
		response.InternalError(c, "failed to generate token")
		return
	}

	response.OK(c, "login successful", LoginResponse{
		AccessToken:          token,
		TokenType:            "Bearer",
		ExpiresIn:            config.App.JWTExpiryHours * 3600,
		InactivityLogoutDays: config.App.InactivityLogoutDays,
		UserID:               userID,
		Email:                fireUser.Email,
		DisplayName:          displayName,
		Role:                 role,
	})
}

func upsertUser(ctx context.Context, fireUID, email, displayName string) (string, string, string, error) {
	const query = `
		INSERT INTO users (firebase_uid, email, display_name, role, last_login_at, created_at)
		VALUES ($1, $2, $3, 'customer', NOW(), NOW())
		ON CONFLICT (firebase_uid) DO UPDATE
		  SET last_login_at = NOW(),
		      email         = EXCLUDED.email,
		      display_name  = EXCLUDED.display_name
		RETURNING id, role, display_name
	`
	var userID, role, name string
	err := database.Pool.QueryRow(ctx, query, fireUID, email, displayName).Scan(&userID, &role, &name)
	return userID, role, name, err
}

// RefreshLastSeen godoc
// POST /api/v1/auth/refresh
func RefreshLastSeen(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*Claims)
	if !ok {
		response.Unauthorized(c, "missing auth context")
		return
	}

	_, err := database.Pool.Exec(c.Request.Context(),
		`UPDATE users SET last_activity_at = NOW() WHERE id = $1`, claims.UserID)
	if err != nil {
		response.InternalError(c, "failed to update activity")
		return
	}

	token, err := GenerateToken(claims.UserID, claims.FireUID, claims.Email, claims.Role)
	if err != nil {
		response.InternalError(c, "failed to refresh token")
		return
	}

	response.OK(c, "token refreshed", gin.H{
		"access_token":           token,
		"token_type":             "Bearer",
		"last_activity_at":       time.Now().UTC(),
		"inactivity_logout_days": config.App.InactivityLogoutDays,
	})
}

// AdminLoginRequest untuk panel web admin — menerima email + password
// lalu diverifikasi ke Firebase melalui REST API Firebase Auth.
type AdminLoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// AdminLogin godoc
// POST /api/v1/auth/admin-login
// Digunakan khusus oleh React Admin panel.
// Backend verifikasi email+password ke Firebase REST API,
// lalu cek role = 'admin' sebelum menerbitkan Bearer token.
func AdminLogin(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email dan password wajib diisi")
		return
	}

	ctx := c.Request.Context()

	// Verifikasi email+password ke Firebase Auth REST API
	firebaseAPIKey := config.App.FirebaseWebAPIKey
	if firebaseAPIKey == "" {
		response.InternalError(c, "Firebase Web API Key belum dikonfigurasi")
		return
	}

	fireIDToken, err := signInWithEmailPassword(ctx, firebaseAPIKey, req.Email, req.Password)
	if err != nil {
		response.Unauthorized(c, "email atau password salah")
		return
	}

	// Verifikasi token & ambil data user
	fireToken, err := firebase.VerifyIDToken(ctx, fireIDToken)
	if err != nil {
		response.Unauthorized(c, "verifikasi token gagal")
		return
	}

	fireUser, err := firebase.GetUser(ctx, fireToken.UID)
	if err != nil {
		response.InternalError(c, "gagal mengambil data user")
		return
	}

	if !fireUser.EmailVerified {
		c.JSON(403, gin.H{"success": false, "message": "email belum diverifikasi"})
		return
	}

	// Cek role di database
	userID, role, displayName, err := upsertUser(ctx, fireUser.UID, fireUser.Email, fireUser.DisplayName)
	if err != nil {
		response.InternalError(c, "gagal memproses akun")
		return
	}

	if role != "admin" {
		c.JSON(403, gin.H{"success": false, "message": "akun ini bukan admin"})
		return
	}

	token, err := GenerateToken(userID, fireUser.UID, fireUser.Email, role)
	if err != nil {
		response.InternalError(c, "gagal membuat token")
		return
	}

	response.OK(c, "login admin berhasil", LoginResponse{
		AccessToken:          token,
		TokenType:            "Bearer",
		ExpiresIn:            config.App.JWTExpiryHours * 3600,
		InactivityLogoutDays: config.App.InactivityLogoutDays,
		UserID:               userID,
		Email:                fireUser.Email,
		DisplayName:          displayName,
		Role:                 role,
	})
}
