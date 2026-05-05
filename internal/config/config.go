package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort              string
	AppEnv                  string
	JWTSecret               string
	JWTExpirationHours      int
	InactivtyLogoutDays     int
	DatabaseURL             string
	FirebaseCredentialsPath string
	FirebaseProjectID       string
	QRISStatic              string
}

var App *Config

func load() {
	if err := godotenv.Load(); err != nil {
		log.Println("[Config] .env not found, using system environment variables")
	}

	App = &Config{
		ServerPort:              getEnv("SERVER_PORT", "8080"),
		AppEnv:                  getEnv("APP_ENV", "development"),
		JWTSecret:               getEnv("JWT_SECRET", "change-me"),
		JWTExpirationHours:      getEnvInt("JWT_EXPIRY_HOURS", 24),
		InactivtyLogoutDays:     getEnvInt("INACTIVITY_LOGOUT_DAYS", 30),
		DatabaseURL:             getEnv("DATABASE_URL", ""),
		FirebaseCredentialsPath: getEnv("FIREBASE_CREDENTIALS_PATH", "./firebase-service-account.json"),
		FirebaseProjectID:       getEnv("FIREBASE_PROJECT_ID", ""),
		QRISStatic:              getEnv("QRIS_STATIC", ""),
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
