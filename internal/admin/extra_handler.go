package admin

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend/internal/auth"
	"backend/internal/database"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PaymentSummary struct {
	PaymentID      string     `json:"id"`
	BookingID      string     `json:"booking_id"`
	Amount         int64      `json:"amount"`
	QRCode         string     `json:"qr_code,omitempty"`
	QRType         string     `json:"qr_type"`
	PaymentStatus  string     `json:"payment_status"`
	PaidAt         *time.Time `json:"paid_at,omitempty"`
	ConfirmedAt    *time.Time `json:"confirmed_at,omitempty"`
	ConfirmedBy    *string    `json:"confirmed_by,omitempty"`
	ProofImageURL  string     `json:"proof_image_url,omitempty"`
	ProofObjectKey string     `json:"proof_object_key,omitempty"`
	ProofNote      string     `json:"proof_note,omitempty"`
	SubmittedAt    *time.Time `json:"submitted_at,omitempty"`
	BookingStatus  string     `json:"booking_status"`
	Date           string     `json:"date"`
	StartTime      string     `json:"start_time"`
	EndTime        string     `json:"end_time"`
	CustomerName   string     `json:"customer_name"`
	CustomerEmail  string     `json:"customer_email"`
	FieldName      string     `json:"field_name"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

type UserDetailRow struct {
	ID             string     `json:"id"`
	FirebaseUID    string     `json:"firebase_uid"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	Phone          string     `json:"phone"`
	EmailVerified  bool       `json:"email_verified"`
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	TotalBookings  int        `json:"total_bookings"`
	TotalSpent     int64      `json:"total_spent"`
}

type UpdateUserRequest struct {
	Name          *string `json:"name"`
	Phone         *string `json:"phone"`
	EmailVerified *bool   `json:"email_verified"`
}

type CreateAdminBookingRequest struct {
	UserID    string `json:"user_id" binding:"required"`
	FieldID   string `json:"field_id" binding:"required"`
	Date      string `json:"date" binding:"required"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
	MarkPaid  bool   `json:"mark_paid"`
}

type UpdateBookingStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type RangeReportResponse struct {
	PeriodStart       string              `json:"period_start"`
	PeriodEnd         string              `json:"period_end"`
	TotalBookings     int                 `json:"total_bookings"`
	PendingBookings   int                 `json:"pending_bookings"`
	PaidBookings      int                 `json:"paid_bookings"`
	CancelledBookings int                 `json:"cancelled_bookings"`
	CompletedBookings int                 `json:"completed_bookings"`
	TotalRevenue      int64               `json:"total_revenue"`
	RevenueChart      []RevenueChartPoint `json:"revenue_chart"`
}

// GET /api/v1/admin/payments?status=pending&date=2026-05-10&q=john&page=1&limit=20
func ListPayments(c *gin.Context) {
	status := c.Query("status")
	date := c.Query("date")
	q := strings.TrimSpace(c.Query("q"))
	page, limit, offset := pagination(c)

	rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT
            p.id::text, b.id::text, p.amount, COALESCE(p.qr_code,''), p.qr_type,
            p.status, p.paid_at, p.confirmed_at, p.confirmed_by::text,
            COALESCE(p.proof_image_url,''), COALESCE(p.proof_object_key,''), COALESCE(p.proof_note,''), p.submitted_at,
            b.status, b.date::text, b.start_time::text, b.end_time::text,
            u.name, u.email, f.name, p.created_at, p.updated_at,
            COUNT(*) OVER() AS total_count
        FROM payments p
        JOIN bookings b ON b.id = p.booking_id
        JOIN users u ON u.id = b.user_id
        JOIN fields f ON f.id = b.field_id
        WHERE ($1 = '' OR p.status = $1)
          AND ($2 = '' OR b.date::text = $2)
          AND ($3 = '' OR u.name ILIKE '%' || $3 || '%' OR u.email ILIKE '%' || $3 || '%' OR f.name ILIKE '%' || $3 || '%')
        ORDER BY p.created_at DESC
        LIMIT $4 OFFSET $5
    `, status, date, q, limit, offset)
	if err != nil {
		response.InternalError(c, "gagal mengambil data pembayaran")
		return
	}
	defer rows.Close()

	var payments []PaymentSummary
	total := 0
	for rows.Next() {
		var p PaymentSummary
		var totalCount int
		if err := rows.Scan(
			&p.PaymentID, &p.BookingID, &p.Amount, &p.QRCode, &p.QRType,
			&p.PaymentStatus, &p.PaidAt, &p.ConfirmedAt, &p.ConfirmedBy,
			&p.ProofImageURL, &p.ProofObjectKey, &p.ProofNote, &p.SubmittedAt,
			&p.BookingStatus, &p.Date, &p.StartTime, &p.EndTime,
			&p.CustomerName, &p.CustomerEmail, &p.FieldName, &p.CreatedAt, &p.UpdatedAt,
			&totalCount,
		); err != nil {
			continue
		}
		total = totalCount
		payments = append(payments, p)
	}
	if payments == nil {
		payments = []PaymentSummary{}
	}

	setPaginationHeaders(c, page, limit, total)
	response.OK(c, "data pembayaran berhasil diambil", payments)
}

// GET /api/v1/admin/payments/:booking_id
func GetPaymentDetail(c *gin.Context) {
	bookingID := c.Param("booking_id")

	var p PaymentSummary
	err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT
            p.id::text, b.id::text, p.amount, COALESCE(p.qr_code,''), p.qr_type,
            p.status, p.paid_at, p.confirmed_at, p.confirmed_by::text,
            COALESCE(p.proof_image_url,''), COALESCE(p.proof_object_key,''), COALESCE(p.proof_note,''), p.submitted_at,
            b.status, b.date::text, b.start_time::text, b.end_time::text,
            u.name, u.email, f.name, p.created_at, p.updated_at
        FROM payments p
        JOIN bookings b ON b.id = p.booking_id
        JOIN users u ON u.id = b.user_id
        JOIN fields f ON f.id = b.field_id
        WHERE b.id = $1
    `, bookingID).Scan(
		&p.PaymentID, &p.BookingID, &p.Amount, &p.QRCode, &p.QRType,
		&p.PaymentStatus, &p.PaidAt, &p.ConfirmedAt, &p.ConfirmedBy,
		&p.ProofImageURL, &p.ProofObjectKey, &p.ProofNote, &p.SubmittedAt,
		&p.BookingStatus, &p.Date, &p.StartTime, &p.EndTime,
		&p.CustomerName, &p.CustomerEmail, &p.FieldName, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		response.NotFound(c, "data pembayaran tidak ditemukan")
		return
	}

	response.OK(c, "detail pembayaran berhasil diambil", p)
}

// POST /api/v1/admin/bookings
func CreateBooking(c *gin.Context) {
	var req CreateAdminBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "user_id, field_id, date, start_time, end_time wajib diisi")
		return
	}

	if err := validateDateAndTime(req.Date, req.StartTime, req.EndTime); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	ctx := c.Request.Context()

	var userExists bool
	_ = database.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, req.UserID).Scan(&userExists)
	if !userExists {
		response.NotFound(c, "user tidak ditemukan")
		return
	}

	var pricePerHour int64
	var fieldName string
	err := database.Pool.QueryRow(ctx, `
        SELECT name, price_per_hour
        FROM fields
        WHERE id = $1 AND is_available = true
    `, req.FieldID).Scan(&fieldName, &pricePerHour)
	if err != nil {
		response.NotFound(c, "lapangan tidak ditemukan atau tidak tersedia")
		return
	}

	var isClosed bool
	_ = database.Pool.QueryRow(ctx, `
        SELECT COALESCE(is_closed, false)
        FROM schedules
        WHERE field_id = $1 AND date = $2
    `, req.FieldID, req.Date).Scan(&isClosed)
	if isClosed {
		response.BadRequest(c, "lapangan tutup pada tanggal tersebut")
		return
	}

	var conflictCount int
	_ = database.Pool.QueryRow(ctx, `
        SELECT COUNT(*) FROM bookings
        WHERE field_id = $1
          AND date = $2
          AND status NOT IN ('cancelled')
          AND (start_time, end_time) OVERLAPS ($3::time, $4::time)
    `, req.FieldID, req.Date, req.StartTime, req.EndTime).Scan(&conflictCount)
	if conflictCount > 0 {
		response.Conflict(c, "slot waktu yang dipilih sudah tidak tersedia")
		return
	}

	start, _ := time.Parse("15:04", req.StartTime)
	end, _ := time.Parse("15:04", req.EndTime)
	durationHrs := end.Sub(start).Hours()
	totalPrice := int64(durationHrs * float64(pricePerHour))

	bookingStatus := "pending_payment"
	paymentStatus := "pending"
	if req.MarkPaid {
		bookingStatus = "paid"
		paymentStatus = "confirmed"
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		response.InternalError(c, "gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	bookingID := uuid.NewString()
	err = tx.QueryRow(ctx, `
        INSERT INTO bookings (id, user_id, field_id, date, start_time, end_time, duration_hrs, status, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
        RETURNING id
    `, bookingID, req.UserID, req.FieldID, req.Date, req.StartTime, req.EndTime, durationHrs, bookingStatus).Scan(&bookingID)
	if err != nil {
		response.InternalError(c, "gagal membuat booking")
		return
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO payments (id, booking_id, amount, qr_type, status, paid_at, confirmed_at, created_at, updated_at)
        VALUES (gen_random_uuid(), $1, $2, 'dynamic', $3,
                CASE WHEN $3 = 'confirmed' THEN NOW() ELSE NULL END,
                CASE WHEN $3 = 'confirmed' THEN NOW() ELSE NULL END,
                NOW(), NOW())
    `, bookingID, totalPrice, paymentStatus)
	if err != nil {
		response.InternalError(c, "gagal membuat pembayaran")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		response.InternalError(c, "gagal menyimpan booking")
		return
	}

	response.Created(c, fmt.Sprintf("booking admin berhasil dibuat untuk %s", fieldName), gin.H{
		"id":             bookingID,
		"user_id":        req.UserID,
		"field_id":       req.FieldID,
		"date":           req.Date,
		"start_time":     req.StartTime,
		"end_time":       req.EndTime,
		"duration_hrs":   durationHrs,
		"total_price":    totalPrice,
		"status":         bookingStatus,
		"payment_status": paymentStatus,
	})
}

// PATCH /api/v1/admin/bookings/:id/status
func UpdateBookingStatus(c *gin.Context) {
	bookingID := c.Param("id")

	var req UpdateBookingStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "status wajib diisi")
		return
	}

	allowed := map[string]bool{
		"pending_payment": true,
		"paid":            true,
		"confirmed":       true,
		"cancelled":       true,
		"completed":       true,
	}
	if !allowed[req.Status] {
		response.BadRequest(c, "status booking tidak valid")
		return
	}

	ctx := c.Request.Context()
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		response.InternalError(c, "gagal memulai transaksi")
		return
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
        UPDATE bookings SET status = $1, updated_at = NOW()
        WHERE id = $2
    `, req.Status, bookingID)
	if err != nil {
		response.InternalError(c, "gagal mengupdate status booking")
		return
	}
	if result.RowsAffected() == 0 {
		response.NotFound(c, "booking tidak ditemukan")
		return
	}

	switch req.Status {
	case "cancelled":
		_, _ = tx.Exec(ctx, `UPDATE payments SET status = 'rejected', updated_at = NOW() WHERE booking_id = $1 AND status IN ('pending','awaiting_verification')`, bookingID)
	case "paid", "confirmed", "completed":
		_, _ = tx.Exec(ctx, `
            UPDATE payments
               SET status = CASE WHEN status IN ('pending','awaiting_verification') THEN 'confirmed' ELSE status END,
                   paid_at = COALESCE(paid_at, NOW()),
                   confirmed_at = COALESCE(confirmed_at, NOW()),
                   updated_at = NOW()
             WHERE booking_id = $1
        `, bookingID)
	}

	if err := tx.Commit(ctx); err != nil {
		response.InternalError(c, "gagal menyimpan status booking")
		return
	}

	response.OK(c, "status booking berhasil diupdate", gin.H{"booking_id": bookingID, "status": req.Status})
}

// GET /api/v1/admin/users/:id
func GetUserDetail(c *gin.Context) {
	userID := c.Param("id")

	var u UserDetailRow
	err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT
            u.id::text, u.firebase_uid, u.name, u.email, COALESCE(u.phone,''),
            u.email_verified, u.last_login_at, u.last_activity_at, u.created_at,
            COUNT(b.id)::int AS total_bookings,
            COALESCE(SUM(p.amount) FILTER (WHERE p.status IN ('paid','confirmed')), 0)::bigint AS total_spent
        FROM users u
        LEFT JOIN bookings b ON b.user_id = u.id
        LEFT JOIN payments p ON p.booking_id = b.id
        WHERE u.id = $1
        GROUP BY u.id
    `, userID).Scan(
		&u.ID, &u.FirebaseUID, &u.Name, &u.Email, &u.Phone,
		&u.EmailVerified, &u.LastLoginAt, &u.LastActivityAt, &u.CreatedAt,
		&u.TotalBookings, &u.TotalSpent,
	)
	if err != nil {
		response.NotFound(c, "user tidak ditemukan")
		return
	}

	response.OK(c, "detail user berhasil diambil", u)
}

// PATCH /api/v1/admin/users/:id
func UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "request tidak valid")
		return
	}

	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.Name))
		argIdx++
	}
	if req.Phone != nil {
		setClauses = append(setClauses, fmt.Sprintf("phone = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.Phone))
		argIdx++
	}
	if req.EmailVerified != nil {
		setClauses = append(setClauses, fmt.Sprintf("email_verified = $%d", argIdx))
		args = append(args, *req.EmailVerified)
		argIdx++
	}

	if len(setClauses) == 0 {
		response.BadRequest(c, "tidak ada field yang diupdate")
		return
	}

	args = append(args, userID)
	query := fmt.Sprintf(`
        UPDATE users SET %s
        WHERE id = $%d
        RETURNING id::text, firebase_uid, name, email, COALESCE(phone,''), email_verified, last_login_at, last_activity_at, created_at
    `, strings.Join(setClauses, ", "), argIdx)

	var u UserDetailRow
	err := database.Pool.QueryRow(c.Request.Context(), query, args...).Scan(
		&u.ID, &u.FirebaseUID, &u.Name, &u.Email, &u.Phone,
		&u.EmailVerified, &u.LastLoginAt, &u.LastActivityAt, &u.CreatedAt,
	)
	if err != nil {
		response.NotFound(c, "user tidak ditemukan")
		return
	}

	response.OK(c, "user berhasil diupdate", u)
}

// DELETE /api/v1/admin/users/:id
func DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	var exists bool
	_ = database.Pool.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)
	`, userID).Scan(&exists)
	if !exists {
		response.NotFound(c, "user tidak ditemukan")
		return
	}

	var totalBookings int
	_ = database.Pool.QueryRow(c.Request.Context(), `
		SELECT COUNT(*) FROM bookings WHERE user_id = $1
	`, userID).Scan(&totalBookings)
	if totalBookings > 0 {
		response.BadRequest(c, "user tidak bisa dihapus karena sudah memiliki riwayat booking")
		return
	}

	result, err := database.Pool.Exec(c.Request.Context(), `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		response.InternalError(c, "gagal menghapus user")
		return
	}
	if result.RowsAffected() == 0 {
		response.NotFound(c, "user tidak ditemukan")
		return
	}

	response.OK(c, "user berhasil dihapus", nil)
}

// GET /api/v1/admin/fields/:id
func GetFieldDetail(c *gin.Context) {
	fieldID := c.Param("id")

	var f FieldRow
	err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id::text, name, type, COALESCE(description,''), price_per_hour, is_available, created_at
        FROM fields
        WHERE id = $1
    `, fieldID).Scan(&f.ID, &f.Name, &f.Type, &f.Description, &f.PricePerHour, &f.IsAvailable, &f.CreatedAt)
	if err != nil {
		response.NotFound(c, "lapangan tidak ditemukan")
		return
	}

	response.OK(c, "detail lapangan berhasil diambil", f)
}

// GET /api/v1/admin/reports/range?from=2026-05-01&to=2026-05-31
func RangeReport(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")
	if from == "" || to == "" {
		now := time.Now()
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		to = now.Format("2006-01-02")
	}

	startDate, err := time.Parse("2006-01-02", from)
	if err != nil {
		response.BadRequest(c, "format from tidak valid — gunakan YYYY-MM-DD")
		return
	}
	endDate, err := time.Parse("2006-01-02", to)
	if err != nil {
		response.BadRequest(c, "format to tidak valid — gunakan YYYY-MM-DD")
		return
	}
	if endDate.Before(startDate) {
		response.BadRequest(c, "to harus setelah atau sama dengan from")
		return
	}

	ctx := c.Request.Context()
	result := RangeReportResponse{PeriodStart: from, PeriodEnd: to}

	err = database.Pool.QueryRow(ctx, `
        SELECT
            COUNT(*)::int,
            COUNT(*) FILTER (WHERE booking_status = 'pending_payment')::int,
            COUNT(*) FILTER (WHERE payment_status IN ('paid','confirmed'))::int,
            COUNT(*) FILTER (WHERE booking_status = 'cancelled')::int,
            COUNT(*) FILTER (WHERE booking_status = 'completed')::int,
            COALESCE(SUM(amount) FILTER (WHERE payment_status IN ('paid','confirmed')), 0)::bigint
        FROM booking_detail
        WHERE date BETWEEN $1 AND $2
    `, from, to).Scan(
		&result.TotalBookings,
		&result.PendingBookings,
		&result.PaidBookings,
		&result.CancelledBookings,
		&result.CompletedBookings,
		&result.TotalRevenue,
	)
	if err != nil {
		response.InternalError(c, "gagal membuat laporan")
		return
	}

	rows, err := database.Pool.Query(ctx, `
        SELECT gs::date::text AS date,
               COALESCE(SUM(bd.amount) FILTER (WHERE bd.payment_status IN ('paid','confirmed')), 0)::bigint AS revenue
        FROM generate_series($1::date, $2::date, INTERVAL '1 day') gs
        LEFT JOIN booking_detail bd ON bd.date = gs::date
        GROUP BY gs::date
        ORDER BY gs::date ASC
    `, from, to)
	if err != nil {
		response.InternalError(c, "gagal mengambil chart laporan")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var p RevenueChartPoint
		if err := rows.Scan(&p.Date, &p.Revenue); err != nil {
			continue
		}
		result.RevenueChart = append(result.RevenueChart, p)
	}
	if result.RevenueChart == nil {
		result.RevenueChart = []RevenueChartPoint{}
	}

	response.OK(c, "laporan range berhasil diambil", result)
}

func validateDateAndTime(date, startTime, endTime string) error {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("format tanggal tidak valid — gunakan YYYY-MM-DD")
	}
	start, err := time.Parse("15:04", startTime)
	if err != nil {
		return fmt.Errorf("format start_time tidak valid — gunakan HH:MM")
	}
	end, err := time.Parse("15:04", endTime)
	if err != nil {
		return fmt.Errorf("format end_time tidak valid — gunakan HH:MM")
	}
	if !end.After(start) {
		return fmt.Errorf("end_time harus setelah start_time")
	}
	return nil
}

func pagination(c *gin.Context) (page int, limit int, offset int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset = (page - 1) * limit
	return page, limit, offset
}

func setPaginationHeaders(c *gin.Context, page, limit, total int) {
	totalPages := 0
	if limit > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(limit)))
	}
	c.Header("X-Total-Count", strconv.Itoa(total))
	c.Header("X-Page", strconv.Itoa(page))
	c.Header("X-Limit", strconv.Itoa(limit))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.Header("Access-Control-Expose-Headers", "X-Total-Count, X-Page, X-Limit, X-Total-Pages")
}

func RequireSuperAdmin(c *gin.Context) bool {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok || claims.Role != "superadmin" {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "superadmin access required"})
		return false
	}
	return true
}
