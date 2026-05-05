package auth

import (
	"errors"
	"time"

	"backend/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   string `json:"user_id"`
	FireUID  string `json:"fire_uid"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	LastSeen int64  `json:"last_seen"`
	jwt.RegisteredClaims
}

func GenerateToken(userID, fireUID, email, role string) (string, error) {
	now := time.Now()
	expiry := now.Add(time.Duration(config.App.JWTExpiryHours) * time.Hour)

	claims := Claims{
		UserID:   userID,
		FireUID:  fireUID,
		Email:    email,
		Role:     role,
		LastSeen: now.Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiry),
			Issuer:    "futsal-cahaya",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.App.JWTSecret))
}

func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.App.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
