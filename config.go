package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/tikinang/discord-ppa/ppa"
)

func init() {
	if err := godotenv.Load(); err != nil {
		slog.Debug("No .env file found, using environment variables")
	}
}

func SetupLogging() {
	var lvl slog.Level
	switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
	case "DEBUG":
		lvl = slog.LevelDebug
	case "WARN", "WARNING":
		lvl = slog.LevelWarn
	case "ERROR":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
}

type AppConfig struct {
	PPA ppa.Config

	DiscordDownloadURL  string
	DiscordPollInterval time.Duration

	PostmanDownloadURL  string
	PostmanPollInterval time.Duration

	ZCLIGithubRepo   string
	ZCLIPollInterval time.Duration
}

func LoadConfig() (*AppConfig, error) {
	cfg := &AppConfig{
		PPA: ppa.Config{
			GPGPrivateKey: os.Getenv("GPG_PRIVATE_KEY"),
			S3Endpoint:    os.Getenv("S3_ENDPOINT"),
			S3Bucket:      os.Getenv("S3_BUCKET"),
			S3AccessKey:   os.Getenv("S3_ACCESS_KEY"),
			S3SecretKey:   os.Getenv("S3_SECRET_KEY"),
			S3Region:      getEnv("S3_REGION", "us-east-1"),
			ListenAddr:    getEnv("LISTEN_ADDR", ":8080"),
			Origin:        getEnv("ORIGIN", "ppa.matejpavlicek.cz"),
			Label:         getEnv("LABEL", "PPA"),
			Maintainer:    getEnv("MAINTAINER", "PPA <ppa@matejpavlicek.cz>"),
		},
		DiscordDownloadURL: getEnv("DISCORD_DOWNLOAD_URL", ""),
		PostmanDownloadURL: getEnv("POSTMAN_DOWNLOAD_URL", ""),
		ZCLIGithubRepo:     getEnv("ZCLI_GITHUB_REPO", "zeropsio/zcli"),
	}

	var err error

	cfg.DiscordPollInterval, err = parseDuration("DISCORD_POLL_INTERVAL", "1h")
	if err != nil {
		return nil, err
	}

	cfg.PostmanPollInterval, err = parseDuration("POSTMAN_POLL_INTERVAL", "6h")
	if err != nil {
		return nil, err
	}

	cfg.ZCLIPollInterval, err = parseDuration("ZCLI_POLL_INTERVAL", "1h")
	if err != nil {
		return nil, err
	}

	if cfg.PPA.GPGPrivateKey == "" {
		return nil, fmt.Errorf("GPG_PRIVATE_KEY is required")
	}
	if cfg.PPA.S3Endpoint == "" {
		return nil, fmt.Errorf("S3_ENDPOINT is required")
	}
	if cfg.PPA.S3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is required")
	}
	if cfg.PPA.S3AccessKey == "" {
		return nil, fmt.Errorf("S3_ACCESS_KEY is required")
	}
	if cfg.PPA.S3SecretKey == "" {
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

func parseDuration(envKey, fallback string) (time.Duration, error) {
	raw := getEnv(envKey, fallback)
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", envKey, raw, err)
	}
	return d, nil
}
