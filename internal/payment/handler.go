package payment

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"backend/internal/auth"
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/notification"
	"backend/internal/storage"
	"backend/pkg/qris"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type PaymentDetail struct {
	PaymentID      string     `json:"payment_id"`
	BookingID      string     `json:"booking_id"`
	Amount         int64      `json:"amount"`
	QRCode         string     `json:"qr_code"`
	QRType         string     `json:"qr_type"`
	Status         string     `json:"status"`
	ProofImageURL  string     `json:"proof_image_url,omitempty"`
	ProofObjectKey string     `json:"proof_object_key,omitempty"`
	ProofNote      string     `json:"proof_note,omitempty"`
	SubmittedAt    *time.Time `json:"submitted_at,omitempty"`
	ExpiredAt      time.Time  `json:"expired_at"`
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

const maxPaymentProofSize = 5 << 20 // 5MB

// SubmitPaymentProof godoc
// POST /api/v1/payments/:booking_id/proof
// multipart/form-data fields:
//   - proof_image / proof: image file jpeg/png/webp, max 5MB
//   - note: optional text note
func SubmitPaymentProof(c *gin.Context) {
	claims, ok := c.MustGet("claims").(*auth.Claims)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	bookingID := c.Param("booking_id")
	ctx := c.Request.Context()

	var paymentID string
	var status string
	var bookingUserID string
	err := database.Pool.QueryRow(ctx, `
		SELECT p.id, p.status, b.user_id
		FROM payments p
		JOIN bookings b ON b.id = p.booking_id
		WHERE p.booking_id = $1
	`, bookingID).Scan(&paymentID, &status, &bookingUserID)
	if err != nil {
		response.NotFound(c, "data pembayaran tidak ditemukan")
		return
	}

	if bookingUserID != claims.UserID {
		response.Forbidden(c, "akses ditolak")
		return
	}

	if status != "pending" && status != "awaiting_verification" {
		response.BadRequest(c, fmt.Sprintf("bukti pembayaran tidak dapat dikirim — status saat ini: '%s'", status))
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPaymentProofSize+1024)
	fileHeader, err := c.FormFile("proof_image")
	if err != nil {
		fileHeader, err = c.FormFile("proof")
	}
	if err != nil {
		response.BadRequest(c, "file bukti pembayaran wajib diunggah pada field 'proof_image'")
		return
	}
	if fileHeader.Size > maxPaymentProofSize {
		response.BadRequest(c, "ukuran file maksimal 5MB")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		response.BadRequest(c, "gagal membaca file bukti pembayaran")
		return
	}
	defer file.Close()

	body, err := io.ReadAll(io.LimitReader(file, maxPaymentProofSize+1))
	if err != nil {
		response.BadRequest(c, "gagal membaca file bukti pembayaran")
		return
	}
	if len(body) == 0 {
		response.BadRequest(c, "file bukti pembayaran kosong")
		return
	}
	if len(body) > maxPaymentProofSize {
		response.BadRequest(c, "ukuran file maksimal 5MB")
		return
	}

	contentType := http.DetectContentType(body)
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		response.BadRequest(c, "format bukti pembayaran harus JPG, PNG, atau WEBP")
		return
	}

	uploaded, err := storage.UploadPaymentProof(ctx, fileHeader.Filename, contentType, body)
	if err != nil {
		response.InternalError(c, "gagal mengunggah bukti pembayaran: "+err.Error())
		return
	}

	note := strings.TrimSpace(c.PostForm("note"))
	if len(note) > 500 {
		note = note[:500]
	}

	result, err := database.Pool.Exec(ctx, `
		UPDATE payments
		   SET status = 'awaiting_verification',
		       proof_image_url = $1,
		       proof_object_key = $2,
		       proof_note = $3,
		       submitted_at = NOW(),
		       updated_at = NOW()
		 WHERE id = $4 AND status IN ('pending', 'awaiting_verification')
	`, uploaded.PublicURL, uploaded.ObjectKey, note, paymentID)
	if err != nil || result.RowsAffected() == 0 {
		response.BadRequest(c, "pembayaran tidak dapat dikirim untuk verifikasi")
		return
	}

	response.OK(c, "bukti pembayaran berhasil dikirim untuk diverifikasi admin", gin.H{
		"booking_id":       bookingID,
		"payment_id":       paymentID,
		"payment_status":   "awaiting_verification",
		"proof_image_url":  uploaded.PublicURL,
		"proof_object_key": uploaded.ObjectKey,
		"proof_note":       note,
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
		WHERE booking_id = $2 AND status IN ('pending', 'awaiting_verification')
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
		WHERE booking_id = $1 AND status IN ('pending', 'awaiting_verification')
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
