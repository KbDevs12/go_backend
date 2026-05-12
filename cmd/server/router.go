package main

import (
	"net/http"

	"backend/internal/admin"
	"backend/internal/auth"
	"backend/internal/booking"
	"backend/internal/middleware"
	"backend/internal/payment"
	"backend/internal/user"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func NewRouter() http.Handler {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"X-Total-Count"},
		AllowCredentials: false,
	}))

	// ── Health ────────────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// ── Public ────────────────────────────────────────────────────
	r.POST("/api/v1/auth/login", auth.Login)
	r.POST("/api/v1/auth/admin-login", auth.AdminLogin)
	r.GET("/api/v1/fields/availability", booking.GetAvailability)

	// ── Customer (Bearer required) ────────────────────────────────
	customer := r.Group("/api/v1", middleware.Auth())
	{
		customer.POST("/auth/refresh", auth.RefreshLastSeen)

		customer.GET("/user/profile", user.GetProfile)
		customer.PATCH("/user/profile", user.UpdateProfile)
		customer.GET("/user/notifications", user.GetNotifications)
		customer.POST("/user/fcm-token", user.UpsertFCMToken)
		customer.DELETE("/user/fcm-token", user.DeleteFCMToken)

		customer.POST("/bookings", booking.Create)
		customer.GET("/bookings", booking.ListMine)
		customer.GET("/bookings/:id", booking.GetByID)
		customer.PATCH("/bookings/:id/cancel", booking.Cancel)

		customer.GET("/payments/:booking_id/qr", payment.GetPaymentQR)
	}

	// ── Admin (Bearer + admin role required) ──────────────────────
	adm := r.Group("/api/v1/admin", middleware.Auth(), middleware.AdminOnly())
	{
		adm.GET("/dashboard", admin.Dashboard)
		adm.GET("/bookings", admin.ListBookings)
		adm.POST("/bookings", admin.CreateBooking)
		adm.GET("/bookings/:id", admin.GetBookingDetail)
		adm.PATCH("/bookings/:id/status", admin.UpdateBookingStatus)
		adm.GET("/payments", admin.ListPayments)
		adm.GET("/payments/:booking_id", admin.GetPaymentDetail)
		adm.POST("/payments/:booking_id/confirm", payment.AdminConfirmPayment)
		adm.POST("/payments/:booking_id/reject", payment.AdminRejectPayment)
		adm.GET("/reports/daily", admin.DailyReport)
		adm.GET("/reports/range", admin.RangeReport)
		adm.GET("/notifications", admin.ListNotifications)
		adm.GET("/users", admin.ListUsers)
		adm.GET("/users/:id", admin.GetUserDetail)
		adm.PATCH("/users/:id", admin.UpdateUser)
		adm.DELETE("/users/:id", admin.DeleteUser)
		adm.GET("/fields", admin.ListFields)
		adm.POST("/fields", admin.CreateField)
		adm.GET("/fields/:id", admin.GetFieldDetail)
		adm.PATCH("/fields/:id", admin.UpdateField)
		adm.DELETE("/fields/:id", admin.DeleteField)
		adm.PUT("/fields/:id/schedules", admin.UpsertSchedule)
		adm.GET("/fields/:id/schedules", admin.ListSchedules)
		adm.DELETE("/fields/:id/schedules/:date", admin.DeleteSchedule)
	}

	// ── Superadmin (Bearer + superadmin role required) ──────────────────────
	superAdm := r.Group("/api/v1/admin", middleware.Auth(), middleware.SuperAdminOnly())
	{
		superAdm.GET("/admins", admin.ListAdmins)
		superAdm.POST("/admins", admin.CreateAdmin)
		superAdm.PATCH("/admins/:id", admin.UpdateAdmin)
		superAdm.DELETE("/admins/:id", admin.DeleteAdmin)
	}

	return r
}
