package admin

import (
	"time"

	"backend/internal/database"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

// BookingSummary menggunakan view booking_detail dari database
type BookingSummary struct {
	BookingID     string     `json:"id"`
	Date          string     `json:"date"`
	StartTime     string     `json:"start_time"`
	EndTime       string     `json:"end_time"`
	DurationHrs   float64    `json:"duration_hrs"`
	BookingStatus string     `json:"booking_status"`
	BookedAt      time.Time  `json:"booked_at"`
	UserID        string     `json:"user_id"`
	CustomerName  string     `json:"customer_name"`
	CustomerEmail string     `json:"customer_email"`
	CustomerPhone string     `json:"customer_phone"`
	FieldID       string     `json:"field_id"`
	FieldName     string     `json:"field_name"`
	FieldType     string     `json:"field_type"`
	PaymentID     string     `json:"payment_id"`
	Amount        int64      `json:"amount"`
	QRCode        string     `json:"qr_code,omitempty"`
	QRType        string     `json:"qr_type"`
	PaymentStatus string     `json:"payment_status"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	ConfirmedAt   *time.Time `json:"confirmed_at,omitempty"`
}

// ListBookings godoc
// GET /api/v1/admin/bookings?status=pending_payment&date=2024-07-01
func ListBookings(c *gin.Context) {
	bookingStatus := c.Query("status")
	date := c.Query("date")

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT booking_id, date::text, start_time::text, end_time::text,
		       duration_hrs, booking_status, booked_at,
		       user_id, customer_name, customer_email, COALESCE(customer_phone,''),
		       field_id, field_name, field_type,
		       COALESCE(payment_id::text,''), COALESCE(amount,0),
		       COALESCE(qr_code,''), COALESCE(qr_type,'dynamic'),
		       COALESCE(payment_status,'pending'),
		       paid_at, confirmed_at
		FROM booking_detail
		WHERE ($1 = '' OR booking_status = $1)
		  AND ($2 = '' OR date::text = $2)
		ORDER BY booked_at DESC
		LIMIT 200
	`, bookingStatus, date)
	if err != nil {
		response.InternalError(c, "gagal mengambil data pemesanan")
		return
	}
	defer rows.Close()

	var bookings []BookingSummary
	for rows.Next() {
		var b BookingSummary
		if err := rows.Scan(
			&b.BookingID, &b.Date, &b.StartTime, &b.EndTime,
			&b.DurationHrs, &b.BookingStatus, &b.BookedAt,
			&b.UserID, &b.CustomerName, &b.CustomerEmail, &b.CustomerPhone,
			&b.FieldID, &b.FieldName, &b.FieldType,
			&b.PaymentID, &b.Amount, &b.QRCode, &b.QRType,
			&b.PaymentStatus, &b.PaidAt, &b.ConfirmedAt,
		); err != nil {
			continue
		}
		bookings = append(bookings, b)
	}

	if bookings == nil {
		bookings = []BookingSummary{}
	}

	c.Header("X-Total-Count", "200")
	c.Header("Access-Control-Expose-Headers", "X-Total-Count")
	response.OK(c, "data pemesanan berhasil diambil", bookings)
}

// GetBookingDetail godoc
// GET /api/v1/admin/bookings/:id
func GetBookingDetail(c *gin.Context) {
	id := c.Param("id")

	var b BookingSummary
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT booking_id, date::text, start_time::text, end_time::text,
		       duration_hrs, booking_status, booked_at,
		       user_id, customer_name, customer_email, COALESCE(customer_phone,''),
		       field_id, field_name, field_type,
		       COALESCE(payment_id::text,''), COALESCE(amount,0),
		       COALESCE(qr_code,''), COALESCE(qr_type,'dynamic'),
		       COALESCE(payment_status,'pending'),
		       paid_at, confirmed_at
		FROM booking_detail
		WHERE booking_id = $1
	`, id).Scan(
		&b.BookingID, &b.Date, &b.StartTime, &b.EndTime,
		&b.DurationHrs, &b.BookingStatus, &b.BookedAt,
		&b.UserID, &b.CustomerName, &b.CustomerEmail, &b.CustomerPhone,
		&b.FieldID, &b.FieldName, &b.FieldType,
		&b.PaymentID, &b.Amount, &b.QRCode, &b.QRType,
		&b.PaymentStatus, &b.PaidAt, &b.ConfirmedAt,
	)
	if err != nil {
		response.NotFound(c, "pemesanan tidak ditemukan")
		return
	}

	response.OK(c, "detail pemesanan berhasil diambil", b)
}

// DailyReport godoc
// GET /api/v1/admin/reports/daily?date=2024-07-01
func DailyReport(c *gin.Context) {
	date := c.Query("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	var totalBookings, paidBookings int
	var totalRevenue int64

	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT
		  COUNT(*),
		  COUNT(*) FILTER (WHERE payment_status IN ('paid','confirmed')),
		  COALESCE(SUM(amount) FILTER (WHERE payment_status IN ('paid','confirmed')), 0)
		FROM booking_detail
		WHERE date::text = $1
	`, date).Scan(&totalBookings, &paidBookings, &totalRevenue)
	if err != nil {
		response.InternalError(c, "gagal membuat laporan")
		return
	}

	response.OK(c, "laporan harian", gin.H{
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
		SELECT id, name, email, COALESCE(phone,''), email_verified, last_login_at, created_at
		FROM users
		ORDER BY created_at DESC
		LIMIT 200
	`)
	if err != nil {
		response.InternalError(c, "gagal mengambil data pengguna")
		return
	}
	defer rows.Close()

	type UserRow struct {
		ID            string     `json:"id"`
		Name          string     `json:"name"`
		Email         string     `json:"email"`
		Phone         string     `json:"phone"`
		EmailVerified bool       `json:"email_verified"`
		LastLoginAt   *time.Time `json:"last_login_at"`
		CreatedAt     time.Time  `json:"created_at"`
	}

	var users []UserRow
	for rows.Next() {
		var u UserRow
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Phone,
			&u.EmailVerified, &u.LastLoginAt, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	if users == nil {
		users = []UserRow{}
	}

	c.Header("X-Total-Count", "200")
	c.Header("Access-Control-Expose-Headers", "X-Total-Count")
	response.OK(c, "data pengguna berhasil diambil", users)
}

// ListFields godoc
// GET /api/v1/admin/fields
func ListFields(c *gin.Context) {
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id, name, type, COALESCE(description,''), price_per_hour, is_available, created_at
		FROM fields ORDER BY name
	`)
	if err != nil {
		response.InternalError(c, "gagal mengambil data lapangan")
		return
	}
	defer rows.Close()

	type FieldRow struct {
		ID           string    `json:"id"`
		Name         string    `json:"name"`
		Type         string    `json:"type"`
		Description  string    `json:"description"`
		PricePerHour int64     `json:"price_per_hour"`
		IsAvailable  bool      `json:"is_available"`
		CreatedAt    time.Time `json:"created_at"`
	}

	var fields []FieldRow
	for rows.Next() {
		var f FieldRow
		if err := rows.Scan(&f.ID, &f.Name, &f.Type, &f.Description,
			&f.PricePerHour, &f.IsAvailable, &f.CreatedAt); err != nil {
			continue
		}
		fields = append(fields, f)
	}

	if fields == nil {
		fields = []FieldRow{}
	}

	c.Header("X-Total-Count", "200")
	c.Header("Access-Control-Expose-Headers", "X-Total-Count")
	response.OK(c, "data lapangan berhasil diambil", fields)
}

// ListNotifications godoc
// GET /api/v1/admin/notifications — untuk monitoring notifikasi
func ListNotifications(c *gin.Context) {
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT n.id, n.user_id, u.name, n.booking_id, n.message, n.type, n.is_read, n.created_at
		FROM notifications n
		JOIN users u ON u.id = n.user_id
		ORDER BY n.created_at DESC
		LIMIT 100
	`)
	if err != nil {
		response.InternalError(c, "gagal mengambil notifikasi")
		return
	}
	defer rows.Close()

	type NotifRow struct {
		ID           string    `json:"id"`
		UserID       string    `json:"user_id"`
		CustomerName string    `json:"customer_name"`
		BookingID    *string   `json:"booking_id,omitempty"`
		Message      string    `json:"message"`
		Type         string    `json:"type"`
		IsRead       bool      `json:"is_read"`
		CreatedAt    time.Time `json:"created_at"`
	}

	var notifs []NotifRow
	for rows.Next() {
		var n NotifRow
		if err := rows.Scan(&n.ID, &n.UserID, &n.CustomerName, &n.BookingID,
			&n.Message, &n.Type, &n.IsRead, &n.CreatedAt); err != nil {
			continue
		}
		notifs = append(notifs, n)
	}

	if notifs == nil {
		notifs = []NotifRow{}
	}

	response.OK(c, "notifikasi berhasil diambil", notifs)
}
