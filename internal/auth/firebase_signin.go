package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type firebaseSignInResponse struct {
	IDToken      string `json:"idToken"`
	Email        string `json:"email"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    string `json:"expiresIn"`
	LocalID      string `json:"localId"`
	Error        *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func signInWithEmailPassword(ctx context.Context, apiKey, email, password string) (string, error) {
	endpoint := fmt.Sprintf(
		"https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=%s",
		apiKey,
	)

	body, _ := json.Marshal(map[string]interface{}{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var result firebaseSignInResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}

	if result.Error != nil {
		return "", errors.New(result.Error.Message)
	}

	return result.IDToken, nil
}
