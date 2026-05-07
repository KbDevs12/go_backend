package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort           string
	AppEnv               string
	JWTSecret            string
	JWTExpiryHours       int
	InactivityLogoutDays int
	DatabaseURL          string
	FirebaseCredPath     string
	FirebaseProjectID    string
	FirebaseWebAPIKey    string
	QRISStatic           string
}

var App *Config

func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("[config] .env not found, using system environment variables")
	}

	App = &Config{
		ServerPort:           getEnv("SERVER_PORT", "8080"),
		AppEnv:               getEnv("APP_ENV", "development"),
		JWTSecret:            getEnv("JWT_SECRET", "change-me"),
		JWTExpiryHours:       getEnvInt("JWT_EXPIRY_HOURS", 24),
		InactivityLogoutDays: getEnvInt("INACTIVITY_LOGOUT_DAYS", 30),
		DatabaseURL:          getEnv("DATABASE_URL", ""),
		FirebaseCredPath:     getEnv("FIREBASE_CREDENTIALS_PATH", "./firebase.json"),
		FirebaseProjectID:    getEnv("FIREBASE_PROJECT_ID", ""),
		FirebaseWebAPIKey:    getEnv("FIREBASE_WEB_API_KEY", ""),
		QRISStatic:           getEnv("QRIS_STATIC", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}
