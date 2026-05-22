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

	start, end, err := validateBookingDateAndTime(req.Date, req.StartTime, req.EndTime)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	ctx := c.Request.Context()

	var pricePerHour int64
	var fieldName, openTime, closeTime string
	var isClosed bool
	err = database.Pool.QueryRow(ctx, `
		SELECT
			f.name,
			f.price_per_hour,
			COALESCE(s.open_time::text, '08:00:00'),
			COALESCE(s.close_time::text, '23:00:00'),
			COALESCE(s.is_closed, false)
		FROM fields f
		LEFT JOIN LATERAL (
			SELECT open_time, close_time, is_closed
			FROM schedules
			WHERE field_id = f.id AND date = $2
			ORDER BY created_at DESC
			LIMIT 1
		) s ON true
		WHERE f.id = $1 AND f.is_available = true
	`, req.FieldID, req.Date).Scan(&fieldName, &pricePerHour, &openTime, &closeTime, &isClosed)
	if err != nil {
		response.NotFound(c, "lapangan tidak ditemukan atau tidak tersedia")
		return
	}

	if isClosed {
		response.BadRequest(c, "lapangan tutup pada tanggal tersebut")
		return
	}

	if err := validateBookingInsideSchedule(start, end, openTime, closeTime); err != nil {
		response.BadRequest(c, err.Error())
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

	durationHrs := end.Sub(start).Hours()
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

func validateBookingDateAndTime(date, startTime, endTime string) (time.Time, time.Time, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("format tanggal tidak valid — gunakan YYYY-MM-DD")
	}
	start, err := parseClock(startTime)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("format start_time tidak valid — gunakan HH:MM")
	}
	end, err := parseClock(endTime)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("format end_time tidak valid — gunakan HH:MM")
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end_time harus setelah start_time")
	}
	return start, end, nil
}

func validateBookingInsideSchedule(start, end time.Time, openTime, closeTime string) error {
	open, err := parseClock(openTime)
	if err != nil {
		return fmt.Errorf("jadwal buka lapangan tidak valid")
	}
	close, err := parseClock(closeTime)
	if err != nil {
		return fmt.Errorf("jadwal tutup lapangan tidak valid")
	}
	if start.Before(open) || end.After(close) {
		return fmt.Errorf("jam booking harus berada di antara %s-%s", formatClock(open), formatClock(close))
	}
	return nil
}

func parseClock(value string) (time.Time, error) {
	if parsed, err := time.Parse("15:04", value); err == nil {
		return parsed, nil
	}
	return time.Parse("15:04:05", value)
}

func formatClock(value time.Time) string {
	return value.Format("15:04")
}
