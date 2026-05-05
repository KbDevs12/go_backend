package firebase

import (
	"context"
	"log"

	"backend/internal/config"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

var AuthClient *auth.Client

func Init() {
	opt := option.WithCredentialsFile(config.App.FirebaseCredPath)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("[Firebase] Failed to initialize Firebase app: %v", err)
	}

	AuthClient, err = app.Auth(context.Background())
	if err != nil {
		log.Fatalf("[Firebase] Failed to initialize Firebase Auth client: %v", err)
	}

	log.Println("[Firebase] Firebase initialized successfully")
}

func VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	return AuthClient.VerifyIDToken(ctx, idToken)
}

func GetUser(ctx context.Context, uid string) (*auth.UserRecord, error) {
	return AuthClient.GetUser(ctx, uid)
}
