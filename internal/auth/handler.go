package auth

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/pkg/firebase"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4"
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
		log.Printf("[login] user upsert failed email=%s firebase_uid=%s err=%v", fireUser.Email, fireUser.UID, err)
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

func upsertUser(ctx context.Context, fireUID, email, name string) (string, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	name = strings.TrimSpace(name)
	if name == "" && email != "" {
		name = strings.Split(email, "@")[0]
	}

	// User lama bisa sudah ada di tabel users dari seed/admin panel, tetapi
	// firebase_uid kosong atau berbeda. Kalau langsung INSERT ON CONFLICT
	// (firebase_uid), login customer bisa gagal karena email sudah dipakai.
	// Maka cari dulu berdasarkan firebase_uid ATAU email, lalu sinkronkan UID.
	var (
		userID             string
		userName           string
		currentFirebaseUID string
	)

	err := database.Pool.QueryRow(ctx, `
		SELECT
			id::text,
			COALESCE(NULLIF(TRIM(name), ''), NULLIF(TRIM(email), ''), $3) AS name,
			COALESCE(firebase_uid, '') AS firebase_uid
		FROM users
		WHERE COALESCE(firebase_uid, '') = $1
		   OR LOWER(TRIM(COALESCE(email, ''))) = LOWER(TRIM($2))
		ORDER BY CASE WHEN COALESCE(firebase_uid, '') = $1 THEN 0 ELSE 1 END
		LIMIT 1
	`, fireUID, email, name).Scan(&userID, &userName, &currentFirebaseUID)
	if err == nil {
		_, err = database.Pool.Exec(ctx, `
			UPDATE users
			SET firebase_uid = $1,
			    email = $2,
			    name = CASE
			        WHEN TRIM(COALESCE(name, '')) = '' THEN $3
			        ELSE name
			    END,
			    email_verified = true,
			    last_login_at = NOW()
			WHERE id = $4
		`, fireUID, email, name, userID)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(userName) == "" {
			userName = name
		}
		if strings.TrimSpace(currentFirebaseUID) != fireUID {
			log.Printf("[login] synced user firebase_uid email=%s user_id=%s firebase_uid=%s", email, userID, fireUID)
		}
		return userID, userName, nil
	}
	if err != pgx.ErrNoRows {
		return "", "", err
	}

	const query = `
		INSERT INTO users (firebase_uid, name, email, email_verified, last_login_at, created_at)
		VALUES ($1, $2, $3, true, NOW(), NOW())
		RETURNING id::text, name
	`
	err = database.Pool.QueryRow(ctx, query, fireUID, name, email).Scan(&userID, &userName)
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

type AdminLoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func normalizeAdminRole(role string) string {
	normalized := strings.ToLower(strings.TrimSpace(role))
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	return normalized
}

// AdminLogin godoc
// POST /api/v1/auth/admin-login
func AdminLogin(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email dan password wajib diisi")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
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

	var (
		adminID            string
		adminName          string
		role               string
		currentFirebaseUID string
		adminEmail         string
	)

	// Admin lama dari dashboard web kadang sudah ada di tabel admins,
	// tapi firebase_uid kosong/berbeda, email tersimpan dengan spasi/huruf besar,
	// atau username kosong. Karena password tetap divalidasi Firebase, fallback
	// by email/username aman dipakai untuk menyambungkan ulang row admins.
	err = database.Pool.QueryRow(ctx, `
		SELECT
			id::text,
			COALESCE(NULLIF(TRIM(username), ''), NULLIF(TRIM(email), ''), 'Admin') AS admin_name,
			COALESCE(TRIM(role), '') AS role,
			COALESCE(firebase_uid, '') AS firebase_uid,
			COALESCE(TRIM(email), '') AS email
		FROM admins
		WHERE COALESCE(firebase_uid, '') = $1
		   OR LOWER(TRIM(COALESCE(email, ''))) = LOWER(TRIM($2))
		   OR LOWER(TRIM(COALESCE(username, ''))) = LOWER(TRIM($2))
		ORDER BY CASE WHEN COALESCE(firebase_uid, '') = $1 THEN 0 ELSE 1 END
		LIMIT 1
	`, fireToken.UID, req.Email).Scan(&adminID, &adminName, &role, &currentFirebaseUID, &adminEmail)
	if err != nil {
		log.Printf("[admin-login] admin lookup failed email=%s firebase_uid=%s err=%v", req.Email, fireToken.UID, err)
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "akun ini bukan admin"})
		return
	}

	role = normalizeAdminRole(role)
	if role != "admin" && role != "superadmin" {
		log.Printf("[admin-login] invalid admin role email=%s admin_id=%s role=%q", req.Email, adminID, role)
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "role admin tidak valid"})
		return
	}

	if strings.TrimSpace(currentFirebaseUID) != fireToken.UID || strings.ToLower(strings.TrimSpace(adminEmail)) != req.Email {
		_, err = database.Pool.Exec(ctx, `
			UPDATE admins
			SET firebase_uid = $1, email = $2
			WHERE id = $3
		`, fireToken.UID, req.Email, adminID)
		if err != nil {
			log.Printf("[admin-login] failed to sync admin firebase uid email=%s admin_id=%s firebase_uid=%s err=%v", req.Email, adminID, fireToken.UID, err)
			response.InternalError(c, "gagal menyinkronkan data admin")
			return
		}
	}

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
