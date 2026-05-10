package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"backend/internal/database"
	"backend/internal/middleware"
	"backend/pkg/firebase"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type AdminAccountRow struct {
	ID          string     `json:"id"`
	FirebaseUID string     `json:"firebase_uid"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type CreateAdminRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Username string `json:"username" binding:"required"`
	Role     string `json:"role"`
}

type UpdateAdminRequest struct {
	Email    *string `json:"email"`
	Password *string `json:"password"`
	Username *string `json:"username"`
	Role     *string `json:"role"`
	Disabled *bool   `json:"disabled"`
}

func ListAdmins(c *gin.Context) {
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id::text, firebase_uid, username, email, role, last_login_at, created_at
		FROM admins
		ORDER BY created_at DESC
	`)
	if err != nil {
		response.InternalError(c, "gagal mengambil data admin")
		return
	}
	defer rows.Close()

	admins := []AdminAccountRow{}
	for rows.Next() {
		var a AdminAccountRow
		if err := rows.Scan(&a.ID, &a.FirebaseUID, &a.Username, &a.Email, &a.Role, &a.LastLoginAt, &a.CreatedAt); err != nil {
			continue
		}
		admins = append(admins, a)
	}

	response.OK(c, "data admin berhasil diambil", admins)
}

func CreateAdmin(c *gin.Context) {
	var req CreateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "email, password, dan username wajib diisi")
		return
	}

	req.Email = normalizeEmail(req.Email)
	req.Username = strings.TrimSpace(req.Username)
	if req.Role == "" {
		req.Role = "admin"
	}
	if !validAdminRole(req.Role) {
		response.BadRequest(c, "role harus admin atau superadmin")
		return
	}
	if req.Username == "" {
		response.BadRequest(c, "username wajib diisi")
		return
	}

	ctx := c.Request.Context()

	fireUser, err := firebase.CreateUser(ctx, req.Email, req.Password, req.Username, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "gagal membuat user Firebase",
			"error":   err.Error(),
		})
		return
	}

	var adminID string
	err = database.Pool.QueryRow(ctx, `
		INSERT INTO admins (id, firebase_uid, username, email, role, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW())
		RETURNING id::text
	`, fireUser.UID, req.Username, req.Email, req.Role).Scan(&adminID)
	if err != nil {
		_ = firebase.DeleteUser(ctx, fireUser.UID)
		response.InternalError(c, "gagal menyimpan admin ke database")
		return
	}

	response.Created(c, "admin berhasil dibuat", gin.H{
		"id":           adminID,
		"firebase_uid": fireUser.UID,
		"email":        fireUser.Email,
		"username":     req.Username,
		"role":         req.Role,
	})
}

func UpdateAdmin(c *gin.Context) {
	adminID := c.Param("id")

	var req UpdateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "request tidak valid")
		return
	}

	if req.Role != nil {
		*req.Role = strings.TrimSpace(*req.Role)
		if !validAdminRole(*req.Role) {
			response.BadRequest(c, "role harus admin atau superadmin")
			return
		}
	}

	ctx := c.Request.Context()

	var current AdminAccountRow
	err := database.Pool.QueryRow(ctx, `
		SELECT id::text, firebase_uid, username, email, role, last_login_at, created_at
		FROM admins
		WHERE id = $1
	`, adminID).Scan(&current.ID, &current.FirebaseUID, &current.Username, &current.Email, &current.Role, &current.LastLoginAt, &current.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(c, "admin tidak ditemukan")
		return
	}
	if err != nil {
		response.InternalError(c, "gagal mengambil data admin")
		return
	}

	claims, _ := middleware.ClaimsFrom(c)
	if claims != nil && claims.UserID == adminID && req.Role != nil && *req.Role != "superadmin" {
		response.BadRequest(c, "tidak bisa menurunkan role akun sendiri")
		return
	}

	if current.Role == "superadmin" && req.Role != nil && *req.Role != "superadmin" {
		ok, err := hasMoreThanOneSuperAdmin(ctx)
		if err != nil {
			response.InternalError(c, "gagal memvalidasi superadmin")
			return
		}
		if !ok {
			response.BadRequest(c, "tidak bisa menurunkan role superadmin terakhir")
			return
		}
	}

	var emailForFirebase *string
	if req.Email != nil {
		normalized := normalizeEmail(*req.Email)
		if normalized == "" {
			response.BadRequest(c, "email tidak valid")
			return
		}
		req.Email = &normalized
		emailForFirebase = req.Email
	}

	var usernameForFirebase *string
	if req.Username != nil {
		trimmed := strings.TrimSpace(*req.Username)
		if trimmed == "" {
			response.BadRequest(c, "username wajib diisi")
			return
		}
		req.Username = &trimmed
		usernameForFirebase = req.Username
	}

	if req.Password != nil {
		password := strings.TrimSpace(*req.Password)
		if password != "" && len(password) < 6 {
			response.BadRequest(c, "password minimal 6 karakter")
			return
		}
		req.Password = &password
	}

	if emailForFirebase != nil || usernameForFirebase != nil || req.Password != nil || req.Disabled != nil {
		_, err := firebase.UpdateUser(ctx, current.FirebaseUID, emailForFirebase, usernameForFirebase, req.Password, req.Disabled)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "gagal mengubah user Firebase",
				"error":   err.Error(),
			})
			return
		}
	}

	if req.Email != nil {
		_, err = database.Pool.Exec(ctx, `UPDATE admins SET email = $2 WHERE id = $1`, adminID, *req.Email)
		if err != nil {
			response.InternalError(c, "gagal mengubah email admin")
			return
		}
	}
	if req.Username != nil {
		_, err = database.Pool.Exec(ctx, `UPDATE admins SET username = $2 WHERE id = $1`, adminID, *req.Username)
		if err != nil {
			response.InternalError(c, "gagal mengubah username admin")
			return
		}
	}
	if req.Role != nil {
		_, err = database.Pool.Exec(ctx, `UPDATE admins SET role = $2 WHERE id = $1`, adminID, *req.Role)
		if err != nil {
			response.InternalError(c, "gagal mengubah role admin")
			return
		}
	}

	response.OK(c, "admin berhasil diubah", gin.H{"id": adminID})
}

func DeleteAdmin(c *gin.Context) {
	adminID := c.Param("id")
	ctx := c.Request.Context()

	claims, _ := middleware.ClaimsFrom(c)
	if claims != nil && claims.UserID == adminID {
		response.BadRequest(c, "tidak bisa menghapus akun sendiri")
		return
	}

	var firebaseUID, role string
	err := database.Pool.QueryRow(ctx, `
		SELECT firebase_uid, role
		FROM admins
		WHERE id = $1
	`, adminID).Scan(&firebaseUID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(c, "admin tidak ditemukan")
		return
	}
	if err != nil {
		response.InternalError(c, "gagal mengambil data admin")
		return
	}

	if role == "superadmin" {
		ok, err := hasMoreThanOneSuperAdmin(ctx)
		if err != nil {
			response.InternalError(c, "gagal memvalidasi superadmin")
			return
		}
		if !ok {
			response.BadRequest(c, "tidak bisa menghapus superadmin terakhir")
			return
		}
	}

	if err := firebase.DeleteUser(ctx, firebaseUID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "gagal menghapus admin dari Firebase",
			"error":   err.Error(),
		})
		return
	}

	_, err = database.Pool.Exec(ctx, `DELETE FROM admins WHERE id = $1`, adminID)
	if err != nil {
		response.InternalError(c, "Firebase user sudah terhapus, tapi gagal menghapus admin dari database")
		return
	}

	response.OK(c, "admin berhasil dihapus", gin.H{"id": adminID})
}

func validAdminRole(role string) bool {
	return role == "admin" || role == "superadmin"
}

func normalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func hasMoreThanOneSuperAdmin(ctx context.Context) (bool, error) {
	var count int
	err := database.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM admins WHERE role = 'superadmin'`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 1, nil
}
