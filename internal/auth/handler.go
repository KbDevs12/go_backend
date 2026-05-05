package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/futsalcahaya/backend/internal/database"
	"github.com/futsalcahaya/backend/pkg/firebase"
	"github.com/futsalcahaya/backend/pkg/response"
)

// LoginRequest is the payload sent by the Flutter app after Firebase sign-in.
// The app signs in with Firebase, gets an ID token, and sends it here.
type LoginRequest struct {
	FirebaseIDToken string `json:"firebase_id_token"`
}

// LoginResponse carries the Bearer token and basic user info.
type LoginResponse struct {
	AccessToken          string `json:"access_token"`
	TokenType            string `json:"token_type"`
	ExpiresIn            int    `json:"expires_in"`             // seconds
	InactivityLogoutDays int    `json:"inactivity_logout_days"` // hint to mobile
	UserID               string `json:"user_id"`
	Email                string `json:"email"`
	DisplayName          string `json:"display_name"`
	Role                 string `json:"role"`
}

// Login godoc
// POST /api/v1/auth/login
// Verifies the Firebase ID token, checks email verification,
// upserts the user in our DB, and returns a Bearer JWT.
func Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid request body")
		return
	}
	if req.FirebaseIDToken == "" {
		response.BadRequest(w, "firebase_id_token is required")
		return
	}

	ctx := r.Context()

	// 1. Verify the Firebase ID token
	fireToken, err := firebase.VerifyIDToken(ctx, req.FirebaseIDToken)
	if err != nil {
		response.Unauthorized(w, "invalid or expired firebase token")
		return
	}

	// 2. Fetch full Firebase user record to check email verification
	fireUser, err := firebase.GetUser(ctx, fireToken.UID)
	if err != nil {
		response.InternalError(w, "failed to fetch firebase user")
		return
	}
	if !fireUser.EmailVerified {
		response.Forbidden(w, "email not verified — please verify your email before logging in")
		return
	}

	// 3. Upsert user in our PostgreSQL DB and retrieve role + UUID
	userID, role, displayName, err := upsertUser(ctx, fireUser.UID, fireUser.Email, fireUser.DisplayName)
	if err != nil {
		response.InternalError(w, "failed to process user account")
		return
	}

	// 4. Issue Bearer JWT (valid 24h; last_seen embedded for mobile inactivity check)
	from, _ := GetConfig()
	token, err := GenerateToken(userID, fireUser.UID, fireUser.Email, role)
	if err != nil {
		response.InternalError(w, "failed to generate token")
		return
	}

	response.OK(w, "login successful", LoginResponse{
		AccessToken:          token,
		TokenType:            "Bearer",
		ExpiresIn:            from.JWTExpiryHours * 3600,
		InactivityLogoutDays: from.InactivityLogoutDays,
		UserID:               userID,
		Email:                fireUser.Email,
		DisplayName:          displayName,
		Role:                 role,
	})
}

// upsertUser inserts a new user or updates last_login timestamp for returning users.
// Returns the internal UUID, role, and display name.
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

// GetConfig is a thin adapter so the handler can read app config without importing internal/config directly.
func GetConfig() (*authConfig, error) {
	return &authConfig{
		JWTExpiryHours:       24,
		InactivityLogoutDays: 30,
	}, nil
}

type authConfig struct {
	JWTExpiryHours       int
	InactivityLogoutDays int
}

// RefreshLastSeen godoc
// POST /api/v1/auth/refresh
// Clients call this on each app foreground to slide the last_seen timestamp.
// The mobile app uses last_seen to determine if 30 days of inactivity have passed.
func RefreshLastSeen(w http.ResponseWriter, r *http.Request) {
	// Claims already validated by AuthMiddleware; retrieve from context
	claims, ok := r.Context().Value(ClaimsKey{}).(*Claims)
	if !ok {
		response.Unauthorized(w, "missing auth context")
		return
	}

	// Update last_activity_at in DB
	_, err := database.Pool.Exec(r.Context(),
		`UPDATE users SET last_activity_at = NOW() WHERE id = $1`, claims.UserID)
	if err != nil {
		response.InternalError(w, "failed to update activity")
		return
	}

	// Issue a fresh token with updated last_seen
	token, err := GenerateToken(claims.UserID, claims.FireUID, claims.Email, claims.Role)
	if err != nil {
		response.InternalError(w, "failed to refresh token")
		return
	}

	response.OK(w, "token refreshed", map[string]interface{}{
		"access_token":           token,
		"token_type":             "Bearer",
		"last_activity_at":       time.Now().UTC(),
		"inactivity_logout_days": 30,
	})
}
