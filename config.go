package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}
}

type Config struct {
	DownloadURL   string
	PollInterval  time.Duration
	GPGPrivateKey string
	S3Endpoint    string
	S3Bucket      string
	S3AccessKey   string
	S3SecretKey   string
	S3Region      string
	ListenAddr    string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		DownloadURL:   getEnv("DOWNLOAD_URL", "https://discord.com/api/download?platform=linux&format=deb"),
		GPGPrivateKey: os.Getenv("GPG_PRIVATE_KEY"),
		S3Endpoint:    os.Getenv("S3_ENDPOINT"),
		S3Bucket:      os.Getenv("S3_BUCKET"),
		S3AccessKey:   os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:   os.Getenv("S3_SECRET_KEY"),
		S3Region:      getEnv("S3_REGION", "us-east-1"),
		ListenAddr:    getEnv("LISTEN_ADDR", ":8080"),
	}

	interval := getEnv("POLL_INTERVAL", "1h")
	d, err := time.ParseDuration(interval)
	if err != nil {
		return nil, fmt.Errorf("invalid POLL_INTERVAL %q: %w", interval, err)
	}
	cfg.PollInterval = d

	if cfg.GPGPrivateKey == "" {
		return nil, fmt.Errorf("GPG_PRIVATE_KEY is required")
	}
	if cfg.S3Endpoint == "" {
		return nil, fmt.Errorf("S3_ENDPOINT is required")
	}
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is required")
	}
	if cfg.S3AccessKey == "" {
		return nil, fmt.Errorf("S3_ACCESS_KEY is required")
	}
	if cfg.S3SecretKey == "" {
		return nil, fmt.Errorf("S3_SECRET_KEY is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
