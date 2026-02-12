package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tikinang/discord-ppa/ppa"
)

func main() {
	SetupLogging()

	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	p, err := ppa.New(cfg.PPA)
	if err != nil {
		slog.Error("PPA init error", "error", err)
		os.Exit(1)
	}

	// Handle subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "delete":
			if len(os.Args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: %s delete <source-name>\n", os.Args[0])
				os.Exit(1)
			}
			ctx := context.Background()
			for _, name := range os.Args[2:] {
				if err := p.DeleteSource(ctx, name); err != nil {
					slog.Error("Error deleting source", "source", name, "error", err)
					os.Exit(1)
				}
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\nUsage: %s [delete <source-name>]\n", os.Args[1], os.Args[0])
			os.Exit(1)
		}
	}

	if cfg.DiscordPollInterval > 0 {
		p.Register(ppa.SourceRegistration{
			Source:       NewDiscordSource(cfg.DiscordDownloadURL),
			PollInterval: cfg.DiscordPollInterval,
		})
	}

	if cfg.PostmanPollInterval > 0 {
		p.Register(ppa.SourceRegistration{
			Source:       NewPostmanSource(cfg.PostmanDownloadURL, cfg.PPA.Maintainer),
			PollInterval: cfg.PostmanPollInterval,
		})
	}

	if cfg.ZCLIGithubRepo != "" && cfg.ZCLIPollInterval > 0 {
		p.Register(ppa.SourceRegistration{
			Source:       NewZCLISource(cfg.ZCLIGithubRepo),
			PollInterval: cfg.ZCLIPollInterval,
		})
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := p.Run(ctx); err != nil {
		slog.Error("PPA error", "error", err)
		os.Exit(1)
	}
}
