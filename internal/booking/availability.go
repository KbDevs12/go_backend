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
	PricePerHour int64      `json:"price_per_hour"`
	Date         string     `json:"date"`
	BookedSlots  []TimeSlot `json:"booked_slots"`
	OpenHour     string     `json:"open_hour"`
	CloseHour    string     `json:"close_hour"`
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
		response.BadRequest(c, "invalid date format — use YYYY-MM-DD")
		return
	}

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT f.id, f.name, f.price_per_hour, f.open_hour::text, f.close_hour::text,
		       COALESCE(b.start_time::text, ''), COALESCE(b.end_time::text, '')
		FROM fields f
		LEFT JOIN bookings b ON b.field_id = f.id
		                     AND b.date::text = $1
		                     AND b.status NOT IN ('cancelled')
		WHERE f.is_active = true
		ORDER BY f.name, b.start_time
	`, date)
	if err != nil {
		response.InternalError(c, "failed to fetch availability")
		return
	}
	defer rows.Close()

	fieldMap := make(map[string]*FieldAvailability)
	var fieldOrder []string

	for rows.Next() {
		var fid, fname, openH, closeH, startT, endT string
		var price int64
		if err := rows.Scan(&fid, &fname, &price, &openH, &closeH, &startT, &endT); err != nil {
			continue
		}
		if _, exists := fieldMap[fid]; !exists {
			fieldMap[fid] = &FieldAvailability{
				FieldID:      fid,
				FieldName:    fname,
				PricePerHour: price,
				Date:         date,
				OpenHour:     openH,
				CloseHour:    closeH,
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

	response.OK(c, "availability retrieved", result)
}
