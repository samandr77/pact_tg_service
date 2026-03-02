package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

type Config struct {
	AppID    int
	AppHash  string
	GRPCAddr string
	AppEnv   string
}

func Load() *Config {
	return &Config{
		AppID:    requireInt("APP_ID"),
		AppHash:  requireString("APP_HASH"),
		GRPCAddr: envOr("GRPC_ADDR", ":50051"),
		AppEnv:   envOr("APP_ENV", "development"),
	}
}

func (c *Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func requireString(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("обязательная переменная окружения %s не задана", key)
	}
	return v
}

func requireInt(key string) int {
	raw := requireString(key)
	v, err := strconv.Atoi(raw)
	if err != nil {
		log.Fatalf("%s", fmt.Errorf("переменная окружения %s должна быть целым числом: %w", key, err))
	}
	return v
}

func envOr(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
