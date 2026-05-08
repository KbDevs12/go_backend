package main

import (
	"context"
	"log"

	"backend/internal/config"
	"backend/pkg/firebase"

	firebaseAuth "firebase.google.com/go/v4/auth"
)

func main() {
	config.Load()
	firebase.Init()
	ctx := context.Background()

	uid := "8abI9T7bsUS9KKlTi2h9pV8Xdc82"

	params := (&firebaseAuth.UserToUpdate{}).EmailVerified(true)
	u, err := firebase.AuthClient.UpdateUser(ctx, uid, params)
	if err != nil {
		log.Fatalf("gagal update: %v", err)
	}

	log.Printf("berhasil: %s emailVerified=%v", u.Email, u.EmailVerified)
}
