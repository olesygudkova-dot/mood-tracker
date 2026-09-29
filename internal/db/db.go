package db

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	"mood-tracker/internal/config"
)

func Connect(cfg *config.Config) *pgxpool.Pool {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName,
	)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("db ping error: %v", err)
	}

	log.Println("db connected")
	return pool
}