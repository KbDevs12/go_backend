package database

import (
	"context"
	"log"

	"backend/internal/config"

	"github.com/jackc/pgx/v4/pgxpool"
)

var Pool *pgxpool.Pool

func Connect() {
	var err error
	Pool, err = pgxpool.Connect(context.Background(), config.App.DatabaseURL)
	if err != nil {
		log.Fatalf("[Database] Failed to connect to database: %v", err)
	}

	if err = Pool.Ping(context.Background()); err != nil {
		log.Fatalf("[Database] Failed to ping database: %v", err)
	}

	log.Println("[Database] Successfully connected to database")
}

func Close() {
	if Pool != nil {
		Pool.Close()
		log.Println("[Database] Database connection closed")
	}
}
