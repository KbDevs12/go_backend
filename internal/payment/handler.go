package payment

import (
	"fmt"
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/config"
	"backend/internal/database"
	"backend/pkg/qris"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type PaymentDetail struct {
	BookingID   string    `json:"booking_id"`
	TotalPrice  int64     `json:"total_price"`
	QRISPayload string    `json:"qris_payload"`
	ExpiredAt   time.Time `json:"expired_at"`
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

	var totalPrice int64
	var status string
	err := database.Pool.QueryRow(c.Request.Context(),
		`SELECT total_price, status FROM bookings WHERE id = $1 AND user_id = $2`,
		bookingID, claims.UserID,
	).Scan(&totalPrice, &status)
	if err != nil {
		response.NotFound(c, "booking not found")
		return
	}
	if status != "pending_payment" {
		response.BadRequest(c, fmt.Sprintf("payment not required — booking status is '%s'", status))
		return
	}

	staticQRIS := config.App.QRISStatic
	if staticQRIS == "" {
		response.InternalError(c, "QRIS not configured — contact administrator")
		return
	}

	dynamicQRIS := qris.ConvertToDynamic(staticQRIS, fmt.Sprintf("%d", totalPrice))
	if dynamicQRIS == "" {
		response.InternalError(c, "failed to generate dynamic QRIS")
		return
	}

	_, _ = database.Pool.Exec(c.Request.Context(),
		`UPDATE bookings SET qris_payload = $1, payment_requested_at = NOW() WHERE id = $2`,
		dynamicQRIS, bookingID)

	response.OK(c, "payment QR generated", PaymentDetail{
		BookingID:   bookingID,
		TotalPrice:  totalPrice,
		QRISPayload: dynamicQRIS,
		ExpiredAt:   time.Now().Add(30 * time.Minute),
	})
}

// AdminConfirmPayment godoc
// POST /api/v1/admin/payments/:booking_id/confirm
func AdminConfirmPayment(c *gin.Context) {
	bookingID := c.Param("booking_id")

	result, err := database.Pool.Exec(c.Request.Context(), `
		UPDATE bookings
		   SET status = 'paid', paid_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'pending_payment'
	`, bookingID)
	if err != nil || result.RowsAffected() == 0 {
		response.BadRequest(c, "booking not found or already processed")
		return
	}

	response.OK(c, "payment confirmed", gin.H{
		"booking_id": bookingID,
		"status":     "paid",
	})
}

// AdminRejectPayment godoc
// POST /api/v1/admin/payments/:booking_id/reject
func AdminRejectPayment(c *gin.Context) {
	bookingID := c.Param("booking_id")

	result, err := database.Pool.Exec(c.Request.Context(), `
		UPDATE bookings
		   SET status = 'cancelled', updated_at = NOW()
		WHERE id = $1 AND status = 'pending_payment'
	`, bookingID)
	if err != nil || result.RowsAffected() == 0 {
		response.BadRequest(c, "booking not found or already processed")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "payment rejected — booking cancelled", "data": gin.H{"booking_id": bookingID, "status": "cancelled"}})
}
