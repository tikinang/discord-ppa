package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tikinang/discord-ppa/ppa"
)

type ZCLISource struct {
	githubRepo string // "owner/repo" format
}

func NewZCLISource(githubRepo string) *ZCLISource {
	return &ZCLISource{githubRepo: githubRepo}
}

func (z *ZCLISource) Name() string {
	return "zcli"
}

func (z *ZCLISource) Description() string {
	return "Zerops CLI for managing Zerops projects and services. Installs to /usr/local/bin/zcli. The .deb is downloaded directly from GitHub releases of " + z.githubRepo + ". New versions are detected via the GitHub latest release API."
}

func (z *ZCLISource) Check(ctx context.Context) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", z.githubRepo)
	resp, err := ppa.HTTPWithRetry(ctx, url, "GET")
	if err != nil {
		return "", fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decoding GitHub release: %w", err)
	}

	return release.TagName, nil
}

func (z *ZCLISource) Fetch(ctx context.Context) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", z.githubRepo)
	resp, err := ppa.HTTPWithRetry(ctx, url, "GET")
	if err != nil {
		return nil, fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding GitHub release: %w", err)
	}

	// Find the amd64 .deb asset
	var debURL, fallbackURL string
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.BrowserDownloadURL, "_amd64.deb") {
			debURL = asset.BrowserDownloadURL
			break
		}
		if strings.HasSuffix(asset.BrowserDownloadURL, ".deb") && fallbackURL == "" {
			fallbackURL = asset.BrowserDownloadURL
		}
	}
	if debURL == "" {
		debURL = fallbackURL
	}
	if debURL == "" {
		return nil, fmt.Errorf("no .deb asset found in release %s", release.TagName)
	}

	debResp, err := ppa.HTTPWithRetry(ctx, debURL, "GET")
	if err != nil {
		return nil, fmt.Errorf("downloading .deb: %w", err)
	}
	defer debResp.Body.Close()

	if debResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d downloading .deb", debResp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(debResp.Body, 512*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading .deb: %w", err)
	}
	return data, nil
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}
