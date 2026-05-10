package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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

	body, err := json.Marshal(map[string]interface{}{
		"email":             email,
		"password":          password,
		"returnSecureToken": true,
	})
	if err != nil {
		log.Printf("[auth] failed to marshal firebase sign-in request: %v", err)
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		log.Printf("[auth] failed to create firebase sign-in request: %v", err)
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	log.Printf("[auth] firebase sign-in request started email=%s", email)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[auth] firebase sign-in request failed email=%s err=%v", email, err)
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[auth] failed to read firebase response body email=%s status=%d err=%v", email, resp.StatusCode, err)
		return "", err
	}

	log.Printf(
		"[auth] firebase sign-in response email=%s status=%d body=%s",
		email,
		resp.StatusCode,
		string(raw),
	)

	var result firebaseSignInResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		log.Printf("[auth] failed to unmarshal firebase response email=%s status=%d err=%v body=%s",
			email,
			resp.StatusCode,
			err,
			string(raw),
		)
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if result.Error != nil {
			log.Printf("[auth] firebase sign-in returned error email=%s status=%d message=%s",
				email,
				resp.StatusCode,
				result.Error.Message,
			)
			return "", errors.New(result.Error.Message)
		}

		log.Printf("[auth] firebase sign-in returned non-2xx email=%s status=%d body=%s",
			email,
			resp.StatusCode,
			string(raw),
		)
		return "", fmt.Errorf("firebase sign-in failed with status %d", resp.StatusCode)
	}

	if result.Error != nil {
		log.Printf("[auth] firebase sign-in returned firebase error email=%s message=%s",
			email,
			result.Error.Message,
		)
		return "", errors.New(result.Error.Message)
	}

	if result.IDToken == "" {
		log.Printf("[auth] firebase sign-in success response but idToken is empty email=%s status=%d body=%s",
			email,
			resp.StatusCode,
			string(raw),
		)
		return "", errors.New("firebase sign-in response missing idToken")
	}

	log.Printf("[auth] firebase sign-in success email=%s localId=%s expiresIn=%s",
		result.Email,
		result.LocalID,
		result.ExpiresIn,
	)

	return result.IDToken, nil
}
