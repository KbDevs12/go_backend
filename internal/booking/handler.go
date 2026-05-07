package booking

import (
	"fmt"
	"time"

	"backend/internal/auth"
	"backend/internal/database"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	StatusPendingPayment = "pending_payment"
	StatusPaid           = "paid"
	StatusConfirmed      = "confirmed"
	StatusCancelled      = "cancelled"
	StatusCompleted      = "completed"
)

type Booking struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	FieldID     string     `json:"field_id"`
	Date        string     `json:"date"`
	StartTime   string     `json:"start_time"`
	EndTime     string     `json:"end_time"`
	DurationHrs float64    `json:"duration_hrs"`
	TotalPrice  int64      `json:"total_price"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

type CreateBookingRequest struct {
	FieldID   string `json:"field_id"  binding:"required"`
	Date      string `json:"date"       binding:"required"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time"   binding:"required"`
}

// Create godoc
// POST /api/v1/bookings
func Create(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req CreateBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "field_id, date, start_time, end_time wajib diisi")
		return
	}

	ctx := c.Request.Context()

	var pricePerHour int64
	var fieldName string
	err := database.Pool.QueryRow(ctx,
		`SELECT name, price_per_hour FROM fields WHERE id = $1 AND is_available = true`,
		req.FieldID,
	).Scan(&fieldName, &pricePerHour)
	if err != nil {
		response.NotFound(c, "lapangan tidak ditemukan atau tidak tersedia")
		return
	}

	var isClosed bool
	_ = database.Pool.QueryRow(ctx,
		`SELECT COALESCE(is_closed, false) FROM schedules WHERE field_id = $1 AND date = $2`,
		req.FieldID, req.Date,
	).Scan(&isClosed)
	if isClosed {
		response.BadRequest(c, "lapangan tutup pada tanggal tersebut")
		return
	}

	var conflictCount int
	_ = database.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM bookings
		WHERE field_id  = $1
		  AND date      = $2
		  AND status   NOT IN ('cancelled')
		  AND (start_time, end_time) OVERLAPS ($3::time, $4::time)
	`, req.FieldID, req.Date, req.StartTime, req.EndTime).Scan(&conflictCount)
	if conflictCount > 0 {
		response.Conflict(c, "slot waktu yang dipilih sudah tidak tersedia")
		return
	}

	layout := "15:04"
	start, _ := time.Parse(layout, req.StartTime)
	end, _ := time.Parse(layout, req.EndTime)
	durationHrs := end.Sub(start).Hours()
	if durationHrs <= 0 {
		response.BadRequest(c, "end_time harus setelah start_time")
		return
	}
	totalPrice := int64(durationHrs * float64(pricePerHour))

	bookingID := uuid.NewString()
	_, err = database.Pool.Exec(ctx, `
		INSERT INTO bookings
		  (id, user_id, field_id, date, start_time, end_time, duration_hrs, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`, bookingID, claims.UserID, req.FieldID, req.Date,
		req.StartTime, req.EndTime, durationHrs, StatusPendingPayment)
	if err != nil {
		response.InternalError(c, "gagal menyimpan pemesanan")
		return
	}

	_, err = database.Pool.Exec(ctx, `
		INSERT INTO payments (id, booking_id, amount, qr_type, status, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'dynamic', 'pending', NOW())
	`, bookingID, totalPrice)
	if err != nil {
		response.InternalError(c, "gagal membuat record pembayaran")
		return
	}

	response.Created(c, fmt.Sprintf("pemesanan berhasil dibuat untuk %s", fieldName), Booking{
		ID:          bookingID,
		UserID:      claims.UserID,
		FieldID:     req.FieldID,
		Date:        req.Date,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		DurationHrs: durationHrs,
		TotalPrice:  totalPrice,
		Status:      StatusPendingPayment,
		CreatedAt:   time.Now(),
	})
}

// ListMine godoc
// GET /api/v1/bookings
func ListMine(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT b.id, b.user_id, b.field_id, b.date::text,
		       b.start_time::text, b.end_time::text,
		       b.duration_hrs, p.amount, b.status, b.created_at
		FROM bookings b
		JOIN payments p ON p.booking_id = b.id
		WHERE b.user_id = $1
		ORDER BY b.created_at DESC
		LIMIT 50
	`, claims.UserID)
	if err != nil {
		response.InternalError(c, "gagal mengambil data pemesanan")
		return
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.UserID, &b.FieldID, &b.Date,
			&b.StartTime, &b.EndTime, &b.DurationHrs, &b.TotalPrice,
			&b.Status, &b.CreatedAt); err != nil {
			continue
		}
		bookings = append(bookings, b)
	}

	if bookings == nil {
		bookings = []Booking{}
	}

	response.OK(c, "data pemesanan berhasil diambil", bookings)
}

// GetByID godoc
// GET /api/v1/bookings/:id
func GetByID(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	bookingID := c.Param("id")
	var b Booking
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT b.id, b.user_id, b.field_id, b.date::text,
		       b.start_time::text, b.end_time::text,
		       b.duration_hrs, p.amount, b.status, b.created_at
		FROM bookings b
		JOIN payments p ON p.booking_id = b.id
		WHERE b.id = $1 AND b.user_id = $2
	`, bookingID, claims.UserID).Scan(
		&b.ID, &b.UserID, &b.FieldID, &b.Date,
		&b.StartTime, &b.EndTime, &b.DurationHrs, &b.TotalPrice,
		&b.Status, &b.CreatedAt)
	if err != nil {
		response.NotFound(c, "pemesanan tidak ditemukan")
		return
	}

	response.OK(c, "detail pemesanan berhasil diambil", b)
}

// Cancel godoc
// PATCH /api/v1/bookings/:id/cancel
func Cancel(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	bookingID := c.Param("id")

	result, err := database.Pool.Exec(c.Request.Context(), `
		UPDATE bookings SET status = 'cancelled', updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND status = 'pending_payment'
	`, bookingID, claims.UserID)
	if err != nil || result.RowsAffected() == 0 {
		response.BadRequest(c, "pemesanan tidak dapat dibatalkan (tidak ditemukan atau sudah diproses)")
		return
	}

	_, _ = database.Pool.Exec(c.Request.Context(), `
		UPDATE payments SET status = 'rejected', updated_at = NOW()
		WHERE booking_id = $1
	`, bookingID)

	response.OK(c, "pemesanan berhasil dibatalkan", nil)
}
