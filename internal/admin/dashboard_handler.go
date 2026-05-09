package admin

import (
	"time"

	"backend/internal/database"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type RevenueChartPoint struct {
	Date    string `json:"date"`
	Revenue int64  `json:"revenue"`
}

type RecentPendingBooking struct {
	BookingID    string    `json:"booking_id"`
	Date         string    `json:"date"`
	StartTime    string    `json:"start_time"`
	EndTime      string    `json:"end_time"`
	CustomerName string    `json:"customer_name"`
	FieldName    string    `json:"field_name"`
	Amount       int64     `json:"amount"`
	BookedAt     time.Time `json:"booked_at"`
}

// Dashboard godoc
// GET /api/v1/admin/dashboard
func Dashboard(c *gin.Context) {
	today := time.Now().Format("2006-01-02")
	ctx := c.Request.Context()

	var totalToday, pendingToday, paidToday int
	var revenueToday int64
	_ = database.Pool.QueryRow(ctx, `
        SELECT
            COUNT(*),
            COUNT(*) FILTER (WHERE booking_status = 'pending_payment'),
            COUNT(*) FILTER (WHERE payment_status IN ('paid','confirmed')),
            COALESCE(SUM(amount) FILTER (WHERE payment_status IN ('paid','confirmed')), 0)
        FROM booking_detail
        WHERE date::text = $1
    `, today).Scan(&totalToday, &pendingToday, &paidToday, &revenueToday)

	var totalUsers int
	_ = database.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&totalUsers)

	chartRows, err := database.Pool.Query(ctx, `
        SELECT
            generate_series::date::text AS date,
            COALESCE(SUM(bd.amount) FILTER (WHERE bd.payment_status IN ('paid','confirmed')), 0) AS revenue
        FROM generate_series(
            CURRENT_DATE - INTERVAL '6 days',
            CURRENT_DATE,
            INTERVAL '1 day'
        )
        LEFT JOIN booking_detail bd ON bd.date = generate_series::date
        GROUP BY generate_series::date
        ORDER BY generate_series::date ASC
    `)
	if err != nil {
		response.InternalError(c, "gagal mengambil data chart")
		return
	}
	defer chartRows.Close()

	var revenueChart []RevenueChartPoint
	for chartRows.Next() {
		var p RevenueChartPoint
		if err := chartRows.Scan(&p.Date, &p.Revenue); err != nil {
			continue
		}
		revenueChart = append(revenueChart, p)
	}
	if revenueChart == nil {
		revenueChart = []RevenueChartPoint{}
	}

	pendingRows, err := database.Pool.Query(ctx, `
        SELECT
            booking_id, date::text, start_time::text, end_time::text,
            customer_name, field_name, COALESCE(amount, 0), booked_at
        FROM booking_detail
        WHERE booking_status = 'pending_payment'
        ORDER BY booked_at DESC
        LIMIT 5
    `)
	if err != nil {
		response.InternalError(c, "gagal mengambil booking pending")
		return
	}
	defer pendingRows.Close()

	var recentPending []RecentPendingBooking
	for pendingRows.Next() {
		var b RecentPendingBooking
		if err := pendingRows.Scan(
			&b.BookingID, &b.Date, &b.StartTime, &b.EndTime,
			&b.CustomerName, &b.FieldName, &b.Amount, &b.BookedAt,
		); err != nil {
			continue
		}
		recentPending = append(recentPending, b)
	}
	if recentPending == nil {
		recentPending = []RecentPendingBooking{}
	}

	response.OK(c, "data dashboard berhasil diambil", gin.H{
		"today":           today,
		"total_bookings":  totalToday,
		"pending_payment": pendingToday,
		"paid_bookings":   paidToday,
		"revenue_today":   revenueToday,
		"total_users":     totalUsers,
		"revenue_chart":   revenueChart,
		"recent_pending":  recentPending,
	})
}
