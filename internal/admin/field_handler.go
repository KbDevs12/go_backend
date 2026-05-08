package admin

import (
	"fmt"
	"time"

	"backend/internal/database"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type FieldRow struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Description  string    `json:"description"`
	PricePerHour int64     `json:"price_per_hour"`
	IsAvailable  bool      `json:"is_available"`
	CreatedAt    time.Time `json:"created_at"`
}

type CreateFieldRequest struct {
	Name         string `json:"name"          binding:"required"`
	Type         string `json:"type"          binding:"required"`
	Description  string `json:"description"`
	PricePerHour int64  `json:"price_per_hour" binding:"required,min=1"`
}

type UpdateFieldRequest struct {
	Name         *string `json:"name"`
	Type         *string `json:"type"`
	Description  *string `json:"description"`
	PricePerHour *int64  `json:"price_per_hour"`
	IsAvailable  *bool   `json:"is_available"`
}

type UpsertScheduleRequest struct {
	Date      string `json:"date"       binding:"required"`
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
	IsClosed  bool   `json:"is_closed"`
}

type ScheduleRow struct {
	ID        string    `json:"id"`
	FieldID   string    `json:"field_id"`
	Date      string    `json:"date"`
	OpenTime  string    `json:"open_time"`
	CloseTime string    `json:"close_time"`
	IsClosed  bool      `json:"is_closed"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── Field Handlers ──────────────────────────────────────────────────────────

// CreateField godoc
// POST /api/v1/admin/fields
func CreateField(c *gin.Context) {
	var req CreateFieldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "name, type, dan price_per_hour wajib diisi")
		return
	}

	id := uuid.NewString()
	var field FieldRow
	err := database.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO fields (id, name, type, description, price_per_hour, is_available, created_at)
		VALUES ($1, $2, $3, $4, $5, true, NOW())
		RETURNING id, name, type, COALESCE(description,''), price_per_hour, is_available, created_at
	`, id, req.Name, req.Type, req.Description, req.PricePerHour).Scan(
		&field.ID, &field.Name, &field.Type, &field.Description,
		&field.PricePerHour, &field.IsAvailable, &field.CreatedAt,
	)
	if err != nil {
		response.InternalError(c, "gagal membuat lapangan")
		return
	}

	response.Created(c, "lapangan berhasil dibuat", field)
}

// UpdateField godoc
// PATCH /api/v1/admin/fields/:id
func UpdateField(c *gin.Context) {
	fieldID := c.Param("id")

	var req UpdateFieldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "request tidak valid")
		return
	}

	var exists bool
	_ = database.Pool.QueryRow(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM fields WHERE id = $1)`, fieldID,
	).Scan(&exists)
	if !exists {
		response.NotFound(c, "lapangan tidak ditemukan")
		return
	}

	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, "name = $"+itoa(argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Type != nil {
		setClauses = append(setClauses, "type = $"+itoa(argIdx))
		args = append(args, *req.Type)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, "description = $"+itoa(argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.PricePerHour != nil {
		if *req.PricePerHour < 1 {
			response.BadRequest(c, "price_per_hour harus lebih dari 0")
			return
		}
		setClauses = append(setClauses, "price_per_hour = $"+itoa(argIdx))
		args = append(args, *req.PricePerHour)
		argIdx++
	}
	if req.IsAvailable != nil {
		setClauses = append(setClauses, "is_available = $"+itoa(argIdx))
		args = append(args, *req.IsAvailable)
		argIdx++
	}

	if len(setClauses) == 0 {
		response.BadRequest(c, "tidak ada field yang diupdate")
		return
	}

	args = append(args, fieldID)
	query := "UPDATE fields SET " + joinClauses(setClauses) + " WHERE id = $" + itoa(argIdx) +
		" RETURNING id, name, type, COALESCE(description,''), price_per_hour, is_available, created_at"

	var field FieldRow
	err := database.Pool.QueryRow(c.Request.Context(), query, args...).Scan(
		&field.ID, &field.Name, &field.Type, &field.Description,
		&field.PricePerHour, &field.IsAvailable, &field.CreatedAt,
	)
	if err != nil {
		response.InternalError(c, "gagal mengupdate lapangan")
		return
	}

	response.OK(c, "lapangan berhasil diupdate", field)
}

// DeleteField godoc
// DELETE /api/v1/admin/fields/:id
func DeleteField(c *gin.Context) {
	fieldID := c.Param("id")
	ctx := c.Request.Context()

	var activeBookings int
	_ = database.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM bookings
		WHERE field_id = $1 AND status NOT IN ('cancelled', 'completed')
	`, fieldID).Scan(&activeBookings)

	if activeBookings > 0 {
		result, err := database.Pool.Exec(ctx,
			`UPDATE fields SET is_available = false WHERE id = $1`, fieldID)
		if err != nil || result.RowsAffected() == 0 {
			response.NotFound(c, "lapangan tidak ditemukan")
			return
		}
		response.OK(c, "lapangan dinonaktifkan (masih ada booking aktif)", nil)
		return
	}

	result, err := database.Pool.Exec(ctx, `DELETE FROM fields WHERE id = $1`, fieldID)
	if err != nil || result.RowsAffected() == 0 {
		response.NotFound(c, "lapangan tidak ditemukan")
		return
	}

	response.OK(c, "lapangan berhasil dihapus", nil)
}

// UpsertSchedule godoc
// PUT /api/v1/admin/fields/:id/schedules

func UpsertSchedule(c *gin.Context) {
	fieldID := c.Param("id")

	var req UpsertScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "date wajib diisi (format YYYY-MM-DD)")
		return
	}

	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		response.BadRequest(c, "format tanggal tidak valid — gunakan YYYY-MM-DD")
		return
	}

	var exists bool
	_ = database.Pool.QueryRow(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM fields WHERE id = $1)`, fieldID,
	).Scan(&exists)
	if !exists {
		response.NotFound(c, "lapangan tidak ditemukan")
		return
	}

	if req.OpenTime != "" {
		if _, err := time.Parse("15:04", req.OpenTime); err != nil {
			response.BadRequest(c, "format open_time tidak valid — gunakan HH:MM")
			return
		}
	}
	if req.CloseTime != "" {
		if _, err := time.Parse("15:04", req.CloseTime); err != nil {
			response.BadRequest(c, "format close_time tidak valid — gunakan HH:MM")
			return
		}
	}
	if req.OpenTime != "" && req.CloseTime != "" {
		open, _ := time.Parse("15:04", req.OpenTime)
		close, _ := time.Parse("15:04", req.CloseTime)
		if !close.After(open) {
			response.BadRequest(c, "close_time harus setelah open_time")
			return
		}
	}

	openTime := req.OpenTime
	if openTime == "" {
		openTime = "08:00"
	}
	closeTime := req.CloseTime
	if closeTime == "" {
		closeTime = "23:00"
	}

	scheduleID := uuid.NewString()
	var sched ScheduleRow
	err := database.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO schedules (id, field_id, date, open_time, close_time, is_closed, created_at)
		VALUES ($1, $2, $3, $4::time, $5::time, $6, NOW())
		ON CONFLICT (field_id, date) DO UPDATE
		  SET open_time  = EXCLUDED.open_time,
		      close_time = EXCLUDED.close_time,
		      is_closed  = EXCLUDED.is_closed
		RETURNING id, field_id, date::text, open_time::text, close_time::text, is_closed, created_at
	`, scheduleID, fieldID, req.Date, openTime, closeTime, req.IsClosed).Scan(
		&sched.ID, &sched.FieldID, &sched.Date,
		&sched.OpenTime, &sched.CloseTime, &sched.IsClosed, &sched.CreatedAt,
	)
	if err != nil {
		response.InternalError(c, "gagal menyimpan jadwal")
		return
	}

	msg := "jadwal berhasil disimpan"
	if sched.IsClosed {
		msg = "lapangan ditandai tutup pada " + req.Date
	}
	response.OK(c, msg, sched)
}

// ListSchedules godoc
// GET /api/v1/admin/fields/:id/schedules?from=2024-07-01&to=2024-07-31
func ListSchedules(c *gin.Context) {
	fieldID := c.Param("id")
	from := c.Query("from")
	to := c.Query("to")

	now := time.Now()
	if from == "" {
		from = now.Format("2006-01-") + "01"
	}
	if to == "" {
		firstOfNext := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		to = firstOfNext.AddDate(0, 0, -1).Format("2006-01-02")
	}

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id, field_id, date::text, open_time::text, close_time::text, is_closed, created_at
		FROM schedules
		WHERE field_id = $1 AND date BETWEEN $2 AND $3
		ORDER BY date
	`, fieldID, from, to)
	if err != nil {
		response.InternalError(c, "gagal mengambil jadwal")
		return
	}
	defer rows.Close()

	var schedules []ScheduleRow
	for rows.Next() {
		var s ScheduleRow
		if err := rows.Scan(&s.ID, &s.FieldID, &s.Date,
			&s.OpenTime, &s.CloseTime, &s.IsClosed, &s.CreatedAt); err != nil {
			continue
		}
		schedules = append(schedules, s)
	}

	if schedules == nil {
		schedules = []ScheduleRow{}
	}

	response.OK(c, "jadwal berhasil diambil", schedules)
}

// DeleteSchedule godoc
// DELETE /api/v1/admin/fields/:id/schedules/:date
//
// Hapus override jadwal — lapangan akan kembali ke jam default (08:00-23:00).
func DeleteSchedule(c *gin.Context) {
	fieldID := c.Param("id")
	date := c.Param("date")

	result, err := database.Pool.Exec(c.Request.Context(),
		`DELETE FROM schedules WHERE field_id = $1 AND date = $2`, fieldID, date)
	if err != nil || result.RowsAffected() == 0 {
		response.NotFound(c, "jadwal tidak ditemukan")
		return
	}

	response.OK(c, "jadwal dihapus — lapangan kembali ke jam default", nil)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func joinClauses(clauses []string) string {
	result := ""
	for i, c := range clauses {
		if i > 0 {
			result += ", "
		}
		result += c
	}
	return result
}
