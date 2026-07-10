package database

import (
	"context"
	"log"

	"backend/internal/config"

	"github.com/jackc/pgx/v4/pgxpool"
)

var Pool *pgxpool.Pool

func Connect() {
	poolConfig, err := pgxpool.ParseConfig(config.App.DatabaseURL)
	if err != nil {
		log.Fatalf("[Database] Failed to parse database URL: %v", err)
	}
	poolConfig.ConnConfig.PreferSimpleProtocol = true

	Pool, err = pgxpool.ConnectConfig(context.Background(), poolConfig)
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
