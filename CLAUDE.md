# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go app (`github.com/tikinang/discord-ppa`) that acts as a Debian APT repository for Discord's Linux `.deb` package. Polls Discord's download URL for new versions, stores `.deb` files in S3, generates GPG-signed APT metadata, and serves everything over HTTP. Deployed to Zerops at `ppa.matejpavlicek.cz`.

## Architecture

- **Poller** — background goroutine that checks Discord for new `.deb`, parses it, uploads to S3, regenerates signed repo metadata
- **HTTP server** — serves APT repo files by proxying from S3, plus `/key.gpg` and index page

All code is in a single `main` package — no sub-packages.

## File Structure

| File | Purpose |
|---|---|
| `main.go` | Entrypoint, wires config + poller + server, graceful shutdown |
| `config.go` | Env var parsing into Config struct, `.env` file support via godotenv |
| `poller.go` | Background polling loop, new-version detection via ETag, retry with Retry-After |
| `deb.go` | `.deb` ar archive parsing (control.tar.gz → control fields) |
| `repo.go` | APT metadata generation (Packages, Packages.gz, Release with hashes) |
| `gpg.go` | GPG key loading, clearsign (InRelease) + detached sign (Release.gpg) |
| `s3.go` | S3 client wrapper (upload, download, list, GetObject) |
| `server.go` | HTTP handler (proxy S3 objects, serve /key.gpg, index page) |
| `zerops.yml` | Zerops deployment config |

## Build & Run Commands

- **Build**: `go build -o discord-ppa`
- **Run**: `go run .`
- **Test**: `go test ./...`
- **Format**: `gofmt -w .`

## Environment Variables

Required: `GPG_PRIVATE_KEY`, `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`

Optional: `DOWNLOAD_URL` (default: Discord download API), `POLL_INTERVAL` (default: `1h`), `S3_REGION` (default: `us-east-1`), `LISTEN_ADDR` (default: `:8080`)

Supports `.env` file (gitignored) for local development.

## Key Dependencies

- `github.com/ProtonMail/go-crypto/openpgp` — GPG signing
- `github.com/aws/aws-sdk-go-v2/service/s3` — S3 operations
- `github.com/blakesmith/ar` — .deb ar archive parsing
- `github.com/joho/godotenv` — .env file loading
