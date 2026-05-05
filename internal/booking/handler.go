package booking

import (
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
	QRISPayload string     `json:"qris_payload,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

type CreateBookingRequest struct {
	FieldID   string `json:"field_id"   binding:"required"`
	Date      string `json:"date"        binding:"required"`
	StartTime string `json:"start_time"  binding:"required"`
	EndTime   string `json:"end_time"    binding:"required"`
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
		response.BadRequest(c, "field_id, date, start_time, end_time are required")
		return
	}

	ctx := c.Request.Context()

	var pricePerHour int64
	var fieldName string
	err := database.Pool.QueryRow(ctx,
		`SELECT name, price_per_hour FROM fields WHERE id = $1 AND is_active = true`,
		req.FieldID,
	).Scan(&fieldName, &pricePerHour)
	if err != nil {
		response.NotFound(c, "field not found or inactive")
		return
	}

	var conflictCount int
	_ = database.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM bookings
		WHERE field_id   = $1
		  AND date       = $2
		  AND status NOT IN ('cancelled')
		  AND (start_time, end_time) OVERLAPS ($3::time, $4::time)
	`, req.FieldID, req.Date, req.StartTime, req.EndTime).Scan(&conflictCount)
	if conflictCount > 0 {
		response.Conflict(c, "the selected time slot is not available")
		return
	}

	layout := "15:04"
	start, _ := time.Parse(layout, req.StartTime)
	end, _ := time.Parse(layout, req.EndTime)
	durationHrs := end.Sub(start).Hours()
	if durationHrs <= 0 {
		response.BadRequest(c, "end_time must be after start_time")
		return
	}
	totalPrice := int64(durationHrs * float64(pricePerHour))

	bookingID := uuid.NewString()
	_, err = database.Pool.Exec(ctx, `
		INSERT INTO bookings (id, user_id, field_id, date, start_time, end_time, duration_hrs, total_price, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`, bookingID, claims.UserID, req.FieldID, req.Date, req.StartTime, req.EndTime, durationHrs, totalPrice, StatusPendingPayment)
	if err != nil {
		response.InternalError(c, "failed to create booking")
		return
	}

	response.Created(c, "booking created — proceed to payment", Booking{
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
		SELECT b.id, b.user_id, b.field_id, b.date::text, b.start_time::text, b.end_time::text,
		       b.duration_hrs, b.total_price, b.status, b.created_at
		FROM bookings b
		WHERE b.user_id = $1
		ORDER BY b.created_at DESC
		LIMIT 50
	`, claims.UserID)
	if err != nil {
		response.InternalError(c, "failed to fetch bookings")
		return
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.UserID, &b.FieldID, &b.Date, &b.StartTime, &b.EndTime,
			&b.DurationHrs, &b.TotalPrice, &b.Status, &b.CreatedAt); err != nil {
			continue
		}
		bookings = append(bookings, b)
	}

	response.OK(c, "bookings retrieved", bookings)
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
		SELECT id, user_id, field_id, date::text, start_time::text, end_time::text,
		       duration_hrs, total_price, status, created_at
		FROM bookings WHERE id = $1 AND user_id = $2
	`, bookingID, claims.UserID).Scan(
		&b.ID, &b.UserID, &b.FieldID, &b.Date, &b.StartTime, &b.EndTime,
		&b.DurationHrs, &b.TotalPrice, &b.Status, &b.CreatedAt)
	if err != nil {
		response.NotFound(c, "booking not found")
		return
	}

	response.OK(c, "booking retrieved", b)
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
		response.BadRequest(c, "booking cannot be cancelled (not found or already processed)")
		return
	}

	response.OK(c, "booking cancelled", nil)
}
