package payment

import (
	"fmt"
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/notification"
	"backend/pkg/qris"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type PaymentDetail struct {
	PaymentID string    `json:"payment_id"`
	BookingID string    `json:"booking_id"`
	Amount    int64     `json:"amount"`
	QRCode    string    `json:"qr_code"`
	QRType    string    `json:"qr_type"`
	Status    string    `json:"status"`
	ExpiredAt time.Time `json:"expired_at"`
}

// GetPaymentQR godoc
// GET /api/v1/payments/:booking_id/qr
func GetPaymentQR(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	bookingID := c.Param("booking_id")

	var paymentID string
	var amount int64
	var status string
	var bookingUserID string

	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT p.id, p.amount, p.status, b.user_id
		FROM payments p
		JOIN bookings b ON b.id = p.booking_id
		WHERE p.booking_id = $1
	`, bookingID).Scan(&paymentID, &amount, &status, &bookingUserID)
	if err != nil {
		response.NotFound(c, "data pembayaran tidak ditemukan")
		return
	}

	if bookingUserID != claims.UserID {
		response.Forbidden(c, "akses ditolak")
		return
	}

	if status != "pending" {
		response.BadRequest(c, fmt.Sprintf("pembayaran tidak diperlukan — status saat ini: '%s'", status))
		return
	}

	staticQRIS := config.App.QRISStatic
	if staticQRIS == "" {
		response.InternalError(c, "QRIS belum dikonfigurasi — hubungi administrator")
		return
	}

	dynamicQRIS := qris.ConvertToDynamic(staticQRIS, fmt.Sprintf("%d", amount))
	if dynamicQRIS == "" {
		response.InternalError(c, "gagal membuat QR Code dinamis")
		return
	}

	_, _ = database.Pool.Exec(c.Request.Context(), `
		UPDATE payments
		   SET qr_code = $1, qr_type = 'dynamic', qr_generated_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`, dynamicQRIS, paymentID)

	response.OK(c, "QR Code pembayaran berhasil dibuat", PaymentDetail{
		PaymentID: paymentID,
		BookingID: bookingID,
		Amount:    amount,
		QRCode:    dynamicQRIS,
		QRType:    "dynamic",
		Status:    "pending",
		ExpiredAt: time.Now().Add(30 * time.Minute),
	})
}

// AdminConfirmPayment godoc
// POST /api/v1/admin/payments/:booking_id/confirm
func AdminConfirmPayment(c *gin.Context) {
	claims, _ := c.MustGet("claims").(*auth.Claims)
	bookingID := c.Param("booking_id")

	var adminID string
	err := database.Pool.QueryRow(c.Request.Context(),
		`SELECT id FROM admins WHERE firebase_uid = $1`, claims.FireUID,
	).Scan(&adminID)
	if err != nil {
		response.Forbidden(c, "admin tidak ditemukan di database")
		return
	}

	result, err := database.Pool.Exec(c.Request.Context(), `
		UPDATE payments
		   SET status       = 'confirmed',
		       confirmed_by = $1,
		       confirmed_at = NOW(),
		       paid_at      = NOW(),
		       updated_at   = NOW()
		WHERE booking_id = $2 AND status = 'pending'
	`, adminID, bookingID)
	if err != nil || result.RowsAffected() == 0 {
		response.BadRequest(c, "data pembayaran tidak ditemukan atau sudah diproses")
		return
	}

	_, _ = database.Pool.Exec(c.Request.Context(), `
		UPDATE bookings SET status = 'paid', updated_at = NOW() WHERE id = $1
	`, bookingID)

	var userID string
	if err := database.Pool.QueryRow(c.Request.Context(),
		`SELECT user_id FROM bookings WHERE id = $1`, bookingID,
	).Scan(&userID); err == nil {
		_ = notification.CreateAndPush(c.Request.Context(), notification.PushRequest{
			UserID:    userID,
			BookingID: bookingID,
			Title:     "Pembayaran dikonfirmasi",
			Message:   "Pembayaran pemesanan kamu telah dikonfirmasi oleh admin ✓",
			Type:      "payment_confirmed",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "pembayaran dikonfirmasi",
		"data":    gin.H{"booking_id": bookingID, "status": "paid"},
	})
}

// AdminRejectPayment godoc
// POST /api/v1/admin/payments/:booking_id/reject
func AdminRejectPayment(c *gin.Context) {
	bookingID := c.Param("booking_id")

	result, err := database.Pool.Exec(c.Request.Context(), `
		UPDATE payments SET status = 'rejected', updated_at = NOW()
		WHERE booking_id = $1 AND status = 'pending'
	`, bookingID)
	if err != nil || result.RowsAffected() == 0 {
		response.BadRequest(c, "data pembayaran tidak ditemukan atau sudah diproses")
		return
	}

	_, _ = database.Pool.Exec(c.Request.Context(), `
		UPDATE bookings SET status = 'cancelled', updated_at = NOW() WHERE id = $1
	`, bookingID)

	var userID string
	if err := database.Pool.QueryRow(c.Request.Context(),
		`SELECT user_id FROM bookings WHERE id = $1`, bookingID,
	).Scan(&userID); err == nil {
		_ = notification.CreateAndPush(c.Request.Context(), notification.PushRequest{
			UserID:    userID,
			BookingID: bookingID,
			Title:     "Pembayaran ditolak",
			Message:   "Pembayaran pemesanan kamu ditolak. Silakan hubungi admin.",
			Type:      "payment_rejected",
		})
	}

	response.OK(c, "pembayaran ditolak — pemesanan dibatalkan", gin.H{
		"booking_id": bookingID,
		"status":     "cancelled",
	})
}
