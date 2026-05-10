package auth

import (
	"context"
	"log"
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
	Name                 string `json:"name"`
	Role                 string `json:"role"`
}

// Login godoc
// POST /api/v1/auth/login
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "firebase_id_token wajib diisi")
		return
	}

	ctx := c.Request.Context()

	fireToken, err := firebase.VerifyIDToken(ctx, req.FirebaseIDToken)
	if err != nil {
		response.Unauthorized(c, "token Firebase tidak valid atau sudah kadaluarsa")
		return
	}

	fireUser, err := firebase.GetUser(ctx, fireToken.UID)
	if err != nil {
		response.InternalError(c, "gagal mengambil data user dari Firebase")
		return
	}
	if !fireUser.EmailVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "email belum diverifikasi — silakan cek inbox email kamu",
		})
		return
	}

	userID, name, err := upsertUser(ctx, fireUser.UID, fireUser.Email, fireUser.DisplayName)
	if err != nil {
		response.InternalError(c, "gagal memproses akun pengguna")
		return
	}

	token, err := GenerateToken(userID, fireUser.UID, fireUser.Email, "customer")
	if err != nil {
		response.InternalError(c, "gagal membuat token")
		return
	}

	response.OK(c, "login berhasil", LoginResponse{
		AccessToken:          token,
		TokenType:            "Bearer",
		ExpiresIn:            config.App.JWTExpiryHours * 3600,
		InactivityLogoutDays: config.App.InactivityLogoutDays,
		UserID:               userID,
		Email:                fireUser.Email,
		Name:                 name,
		Role:                 "customer",
	})
}

// upsertUser — sesuai tabel users di class diagram (name, email, phone, email_verified)
func upsertUser(ctx context.Context, fireUID, email, name string) (string, string, error) {
	const query = `
		INSERT INTO users (firebase_uid, name, email, email_verified, last_login_at, created_at)
		VALUES ($1, $2, $3, true, NOW(), NOW())
		ON CONFLICT (firebase_uid) DO UPDATE
		  SET last_login_at  = NOW(),
		      email          = EXCLUDED.email,
		      name           = EXCLUDED.name,
		      email_verified = true
		RETURNING id, name
	`
	var userID, userName string
	err := database.Pool.QueryRow(ctx, query, fireUID, name, email).Scan(&userID, &userName)
	return userID, userName, err
}

// RefreshLastSeen godoc
// POST /api/v1/auth/refresh
func RefreshLastSeen(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*Claims)
	if !ok {
		response.Unauthorized(c, "missing auth context")
		return
	}

	if claims.Role != "customer" {
		_, err := database.Pool.Exec(c.Request.Context(),
			`UPDATE admins SET last_login_at = NOW() WHERE id = $1`, claims.UserID)
		if err != nil {
			response.InternalError(c, "gagal memperbarui aktivitas")
			return
		}
	} else {
		_, err := database.Pool.Exec(c.Request.Context(),
			`UPDATE users SET last_activity_at = NOW() WHERE id = $1`, claims.UserID)
		if err != nil {
			response.InternalError(c, "gagal memperbarui aktivitas")
			return
		}
	}

	token, err := GenerateToken(claims.UserID, claims.FireUID, claims.Email, claims.Role)
	if err != nil {
		response.InternalError(c, "gagal memperbarui token")
		return
	}

	response.OK(c, "token diperbarui", gin.H{
		"access_token":           token,
		"token_type":             "Bearer",
		"last_activity_at":       time.Now().UTC(),
		"inactivity_logout_days": config.App.InactivityLogoutDays,
	})
}

// AdminLoginRequest untuk panel web admin
type AdminLoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// AdminLogin godoc
// POST /api/v1/auth/admin-login
func AdminLogin(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email dan password wajib diisi")
		return
	}

	ctx := c.Request.Context()

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

	log.Printf("[admin-login] firebase sign-in success email=%s token_len=%d", req.Email, len(fireIDToken))

	fireToken, err := firebase.VerifyIDToken(ctx, fireIDToken)
	if err != nil {
		response.Unauthorized(c, "verifikasi token gagal")
		return
	}

	fireUser, err := firebase.GetUser(ctx, fireToken.UID)
	if err != nil {
		log.Printf("[admin-login] VerifyIDToken failed email=%s err=%v", req.Email, err)
		response.InternalError(c, "gagal mengambil data user")
		return
	}

	if !fireUser.EmailVerified {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "email belum diverifikasi"})
		return
	}

	// Cek di tabel admins (tabel terpisah sesuai class diagram)
	var adminID, adminName, role string
	err = database.Pool.QueryRow(ctx,
		`SELECT id, username, role FROM admins WHERE firebase_uid = $1`, fireToken.UID,
	).Scan(&adminID, &adminName, &role)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "akun ini bukan admin"})
		return
	}

	// Update last_login_at di admins
	_, _ = database.Pool.Exec(ctx, `UPDATE admins SET last_login_at = NOW() WHERE id = $1`, adminID)

	token, err := GenerateToken(adminID, fireToken.UID, req.Email, role)
	if err != nil {
		response.InternalError(c, "gagal membuat token")
		return
	}

	response.OK(c, "login admin berhasil", LoginResponse{
		AccessToken:          token,
		TokenType:            "Bearer",
		ExpiresIn:            config.App.JWTExpiryHours * 3600,
		InactivityLogoutDays: config.App.InactivityLogoutDays,
		UserID:               adminID,
		Email:                req.Email,
		Name:                 adminName,
		Role:                 role,
	})
}
