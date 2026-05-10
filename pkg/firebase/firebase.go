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

func CreateUser(ctx context.Context, email, password, displayName string, emailVerified bool) (*auth.UserRecord, error) {
	params := (&auth.UserToCreate{}).
		Email(email).
		Password(password).
		DisplayName(displayName).
		EmailVerified(emailVerified)

	return AuthClient.CreateUser(ctx, params)
}

func UpdateUser(ctx context.Context, uid string, email, displayName *string, password *string, disabled *bool) (*auth.UserRecord, error) {
	params := (&auth.UserToUpdate{})

	if email != nil {
		params.Email(*email)
	}
	if displayName != nil {
		params.DisplayName(*displayName)
	}
	if password != nil && *password != "" {
		params.Password(*password)
	}
	if disabled != nil {
		params.Disabled(*disabled)
	}

	return AuthClient.UpdateUser(ctx, uid, params)
}

func DeleteUser(ctx context.Context, uid string) error {
	return AuthClient.DeleteUser(ctx, uid)
}
