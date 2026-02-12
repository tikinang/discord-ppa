package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/tikinang/discord-ppa/ppa"
)

const defaultDiscordDownloadURL = "https://discord.com/api/download?platform=linux&format=deb"

type DiscordSource struct {
	downloadURL string
}

func NewDiscordSource(downloadURL string) *DiscordSource {
	if downloadURL == "" {
		downloadURL = defaultDiscordDownloadURL
	}
	return &DiscordSource{downloadURL: downloadURL}
}

func (d *DiscordSource) Name() string {
	return "discord"
}

func (d *DiscordSource) Description() string {
	return "Discord voice and text chat client. The official .deb is fetched directly from Discord's download API. New versions are detected via ETag changes on the download URL."
}

func (d *DiscordSource) Check(ctx context.Context) (string, error) {
	resp, err := ppa.HTTPWithRetry(ctx, d.downloadURL, "HEAD")
	if err != nil {
		return "", fmt.Errorf("HEAD request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		etag = resp.Header.Get("Content-Length")
	}
	return etag, nil
}

func (d *DiscordSource) Fetch(ctx context.Context) ([]byte, error) {
	resp, err := ppa.HTTPWithRetry(ctx, d.downloadURL, "GET")
	if err != nil {
		return nil, fmt.Errorf("downloading .deb: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d downloading .deb", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading .deb: %w", err)
	}
	return data, nil
}
