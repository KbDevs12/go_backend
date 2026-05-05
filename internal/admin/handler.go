package admin

import (
	"time"

	"backend/internal/database"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type BookingSummary struct {
	ID            string     `json:"id"`
	CustomerName  string     `json:"customer_name"`
	CustomerEmail string     `json:"customer_email"`
	FieldName     string     `json:"field_name"`
	Date          string     `json:"date"`
	StartTime     string     `json:"start_time"`
	EndTime       string     `json:"end_time"`
	TotalPrice    int64      `json:"total_price"`
	Status        string     `json:"status"`
	QRISPayload   string     `json:"qris_payload,omitempty"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ListBookings godoc
// GET /api/v1/admin/bookings?status=pending_payment&date=2024-07-01
func ListBookings(c *gin.Context) {
	status := c.Query("status")
	date := c.Query("date")

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT b.id, u.display_name, u.email, f.name,
		       b.date::text, b.start_time::text, b.end_time::text,
		       b.total_price, b.status,
		       COALESCE(b.qris_payload,''), b.paid_at, b.created_at
		FROM bookings b
		JOIN users  u ON u.id = b.user_id
		JOIN fields f ON f.id = b.field_id
		WHERE ($1 = '' OR b.status = $1)
		  AND ($2 = '' OR b.date::text = $2)
		ORDER BY b.created_at DESC
		LIMIT 200
	`, status, date)
	if err != nil {
		response.InternalError(c, "failed to fetch bookings")
		return
	}
	defer rows.Close()

	var bookings []BookingSummary
	for rows.Next() {
		var b BookingSummary
		if err := rows.Scan(&b.ID, &b.CustomerName, &b.CustomerEmail, &b.FieldName,
			&b.Date, &b.StartTime, &b.EndTime,
			&b.TotalPrice, &b.Status, &b.QRISPayload, &b.PaidAt, &b.CreatedAt); err != nil {
			continue
		}
		bookings = append(bookings, b)
	}

	// react-admin needs total count in X-Total-Count header
	c.Header("X-Total-Count", "200")
	c.Header("Access-Control-Expose-Headers", "X-Total-Count")

	response.OK(c, "bookings retrieved", bookings)
}

// GetBookingDetail godoc
// GET /api/v1/admin/bookings/:id
func GetBookingDetail(c *gin.Context) {
	id := c.Param("id")

	var b BookingSummary
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT b.id, u.display_name, u.email, f.name,
		       b.date::text, b.start_time::text, b.end_time::text,
		       b.total_price, b.status,
		       COALESCE(b.qris_payload,''), b.paid_at, b.created_at
		FROM bookings b
		JOIN users  u ON u.id = b.user_id
		JOIN fields f ON f.id = b.field_id
		WHERE b.id = $1
	`, id).Scan(&b.ID, &b.CustomerName, &b.CustomerEmail, &b.FieldName,
		&b.Date, &b.StartTime, &b.EndTime,
		&b.TotalPrice, &b.Status, &b.QRISPayload, &b.PaidAt, &b.CreatedAt)
	if err != nil {
		response.NotFound(c, "booking not found")
		return
	}

	response.OK(c, "booking retrieved", b)
}

// DailyReport godoc
// GET /api/v1/admin/reports/daily?date=2024-07-01
func DailyReport(c *gin.Context) {
	date := c.Query("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	var totalRevenue int64
	var totalBookings, paidBookings int

	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT
		  COUNT(*),
		  COUNT(*) FILTER (WHERE status IN ('paid','confirmed','completed')),
		  COALESCE(SUM(total_price) FILTER (WHERE status IN ('paid','confirmed','completed')), 0)
		FROM bookings
		WHERE date::text = $1
	`, date).Scan(&totalBookings, &paidBookings, &totalRevenue)
	if err != nil {
		response.InternalError(c, "failed to generate report")
		return
	}

	response.OK(c, "daily report", gin.H{
		"date":           date,
		"total_bookings": totalBookings,
		"paid_bookings":  paidBookings,
		"total_revenue":  totalRevenue,
	})
}

// ListUsers godoc
// GET /api/v1/admin/users
func ListUsers(c *gin.Context) {
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id, email, display_name, COALESCE(phone_number,''), role, last_login_at, created_at
		FROM users
		ORDER BY created_at DESC
		LIMIT 200
	`)
	if err != nil {
		response.InternalError(c, "failed to fetch users")
		return
	}
	defer rows.Close()

	type UserRow struct {
		ID          string     `json:"id"`
		Email       string     `json:"email"`
		DisplayName string     `json:"display_name"`
		PhoneNumber string     `json:"phone_number"`
		Role        string     `json:"role"`
		LastLoginAt *time.Time `json:"last_login_at"`
		CreatedAt   time.Time  `json:"created_at"`
	}

	var users []UserRow
	for rows.Next() {
		var u UserRow
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PhoneNumber,
			&u.Role, &u.LastLoginAt, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	c.Header("X-Total-Count", "200")
	c.Header("Access-Control-Expose-Headers", "X-Total-Count")
	response.OK(c, "users retrieved", users)
}

// ListFields godoc
// GET /api/v1/admin/fields
func ListFields(c *gin.Context) {
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id, name, COALESCE(description,''), price_per_hour,
		       open_hour::text, close_hour::text, is_active, created_at
		FROM fields ORDER BY name
	`)
	if err != nil {
		response.InternalError(c, "failed to fetch fields")
		return
	}
	defer rows.Close()

	type FieldRow struct {
		ID           string    `json:"id"`
		Name         string    `json:"name"`
		Description  string    `json:"description"`
		PricePerHour int64     `json:"price_per_hour"`
		OpenHour     string    `json:"open_hour"`
		CloseHour    string    `json:"close_hour"`
		IsActive     bool      `json:"is_active"`
		CreatedAt    time.Time `json:"created_at"`
	}

	var fields []FieldRow
	for rows.Next() {
		var f FieldRow
		if err := rows.Scan(&f.ID, &f.Name, &f.Description, &f.PricePerHour,
			&f.OpenHour, &f.CloseHour, &f.IsActive, &f.CreatedAt); err != nil {
			continue
		}
		fields = append(fields, f)
	}

	c.Header("X-Total-Count", "200")
	c.Header("Access-Control-Expose-Headers", "X-Total-Count")
	response.OK(c, "fields retrieved", fields)
}
