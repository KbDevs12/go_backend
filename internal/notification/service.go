package notification

import (
	"backend/internal/database"
	fbpkg "backend/pkg/firebase"
	"context"
	"log"

	"firebase.google.com/go/v4/messaging"
)

type PushRequest struct {
	UserID    string
	BookingID string
	Title     string
	Message   string
	Type      string
}

func CreateAndPush(ctx context.Context, req PushRequest) error {
	_, err := database.Pool.Exec(ctx, `
	INSERT INTO notifications (id, user_id, booking_id, message, type, is_read, created_at)
	VALUES (gen_random_uuid(), $1, $2, $3, $4, false, NOW())`, req.UserID, req.BookingID, req.Message, req.Type)
	if err != nil {
		return err
	}

	go func() {
		if err := SendToUser(context.Background(), req); err != nil {
			log.Printf("[FCM] failed to push notification to user %s: %v", req.UserID, err)
		}
	}()

	return nil
}

func SendToUser(ctx context.Context, req PushRequest) error {
	if fbpkg.MessagingClient == nil {
		log.Println("[FCM] MessagingClient is nil, skip push")
		return nil
	}

	rows, err := database.Pool.Query(ctx, `
		SELECT token
		FROM user_device_tokens
		WHERE user_id = $1 AND is_active = true
	`, req.UserID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			continue
		}
		tokens = append(tokens, token)
	}

	if len(tokens) == 0 {
		return nil
	}

	data := map[string]string{
		"type":         req.Type,
		"booking_id":   req.BookingID,
		"click_action": "FLUTTER_NOTIFICATION_CLICK",
	}

	for _, token := range tokens {
		message := &messaging.Message{
			Token: token,
			Notification: &messaging.Notification{
				Title: req.Title,
				Body:  req.Message,
			},
			Data: data,
			Android: &messaging.AndroidConfig{
				Priority: "high",
				Notification: &messaging.AndroidNotification{
					ChannelID: "payment_status",
				},
			},
			APNS: &messaging.APNSConfig{
				Payload: &messaging.APNSPayload{
					Aps: &messaging.Aps{
						Sound: "default",
					},
				},
			},
		}

		if _, err := fbpkg.MessagingClient.Send(ctx, message); err != nil {
			log.Printf("[FCM] send failed for token prefix %.12s: %v", token, err)
		}
	}

	return nil
}
