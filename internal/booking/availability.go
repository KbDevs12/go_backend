package booking

import (
	"time"

	"backend/internal/database"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type FieldAvailability struct {
	FieldID      string     `json:"field_id"`
	FieldName    string     `json:"field_name"`
	FieldType    string     `json:"field_type"`
	PricePerHour int64      `json:"price_per_hour"`
	Date         string     `json:"date"`
	OpenTime     string     `json:"open_time"`
	CloseTime    string     `json:"close_time"`
	IsAvailable  bool       `json:"is_available"`
	IsClosed     bool       `json:"is_closed"`
	BookedSlots  []TimeSlot `json:"booked_slots"`
}

type TimeSlot struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// GetAvailability godoc
// GET /api/v1/fields/availability?date=2024-07-01
func GetAvailability(c *gin.Context) {
	date := c.Query("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	if _, err := time.Parse("2006-01-02", date); err != nil {
		response.BadRequest(c, "format tanggal tidak valid — gunakan YYYY-MM-DD")
		return
	}

	// Join fields + schedules + bookings
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT
		  f.id, f.name, f.type, f.price_per_hour, f.is_available,
		  COALESCE(s.open_time::text,  '08:00'),
		  COALESCE(s.close_time::text, '23:00'),
		  COALESCE(s.is_closed, false),
		  COALESCE(b.start_time::text, ''),
		  COALESCE(b.end_time::text,   '')
		FROM fields f
		LEFT JOIN schedules s ON s.field_id = f.id AND s.date = $1
		LEFT JOIN bookings  b ON b.field_id = f.id AND b.date = $1
		                      AND b.status NOT IN ('cancelled')
		WHERE f.is_available = true
		ORDER BY f.name, b.start_time
	`, date)
	if err != nil {
		response.InternalError(c, "gagal mengambil data ketersediaan")
		return
	}
	defer rows.Close()

	fieldMap := make(map[string]*FieldAvailability)
	var fieldOrder []string

	for rows.Next() {
		var fid, fname, ftype, openT, closeT, startT, endT string
		var price int64
		var isAvailable, isClosed bool
		if err := rows.Scan(&fid, &fname, &ftype, &price, &isAvailable,
			&openT, &closeT, &isClosed, &startT, &endT); err != nil {
			continue
		}
		if _, exists := fieldMap[fid]; !exists {
			fieldMap[fid] = &FieldAvailability{
				FieldID:      fid,
				FieldName:    fname,
				FieldType:    ftype,
				PricePerHour: price,
				Date:         date,
				OpenTime:     openT,
				CloseTime:    closeT,
				IsAvailable:  isAvailable,
				IsClosed:     isClosed,
				BookedSlots:  []TimeSlot{},
			}
			fieldOrder = append(fieldOrder, fid)
		}
		if startT != "" {
			fieldMap[fid].BookedSlots = append(fieldMap[fid].BookedSlots, TimeSlot{
				StartTime: startT,
				EndTime:   endT,
			})
		}
	}

	result := make([]FieldAvailability, 0, len(fieldOrder))
	for _, fid := range fieldOrder {
		result = append(result, *fieldMap[fid])
	}

	response.OK(c, "data ketersediaan berhasil diambil", result)
}
