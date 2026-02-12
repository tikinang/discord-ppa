# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go app (`github.com/tikinang/discord-ppa`) that acts as a Debian APT repository for multiple package sources (Discord, Postman,
zCLI). Each source implements a `ppa.Source` interface. The PPA orchestrator polls sources for new versions, stores `.deb` files
in S3, generates GPG-signed APT metadata, and serves everything over HTTP. Deployed to Zerops at `ppa.matejpavlicek.cz`.

## Architecture

- **`ppa/` sub-package** — reusable PPA library: `Source` interface, orchestrator, S3 client, GPG signing, APT metadata
  generation, HTTP server, `.deb` parsing and building
- **Root `main` package** — config loading, source implementations (Discord, Postman, zCLI), entrypoint

Each source has its own polling goroutine. A `sync.Mutex` serializes repo metadata regeneration when any source updates.

### S3 Layout

- `pool/{first-letter}/{package-name}/{package}-{version}.deb` — package files
- `meta/{source-name}/state` — last seen state per source
- `meta/{source-name}/packages-entry` — per-source Packages stanza
- `dists/stable/...` — APT repo metadata (Packages, Release, InRelease, Release.gpg)
- `key.gpg` — GPG public key

## File Structure

### `ppa/` sub-package

| File             | Purpose                                                                                                   |
|------------------|-----------------------------------------------------------------------------------------------------------|
| `ppa/source.go`  | `Source` interface definition                                                                             |
| `ppa/ppa.go`     | `PPA` struct, `Config`, `New()`, `Register()`, `Run()`, polling orchestration, repo metadata regeneration |
| `ppa/deb.go`     | `.deb` ar archive parsing (control.tar.gz → control fields)                                               |
| `ppa/debuild.go` | `.deb` builder (`BuildDeb`) — creates ar archives from control fields + file entries                      |
| `ppa/repo.go`    | APT metadata generation (Packages, Packages.gz, Release with hashes)                                      |
| `ppa/gpg.go`     | GPG key loading, clearsign (InRelease) + detached sign (Release.gpg)                                      |
| `ppa/s3.go`      | S3 client wrapper (upload, download, list, GetObject)                                                     |
| `ppa/server.go`  | HTTP handler (proxy S3 objects, serve /key.gpg, dynamic index page)                                       |
| `ppa/http.go`    | `HTTPWithRetry` utility (exported, reusable by sources)                                                   |

### Root package

| File         | Purpose                                                                 |
|--------------|-------------------------------------------------------------------------|
| `main.go`    | Entrypoint: load config, create PPA, register sources, run              |
| `config.go`  | Env var parsing, `.env` file support via godotenv                       |
| `discord.go` | Discord source: HEAD for ETag check, GET for .deb download              |
| `postman.go` | Postman source: download tar.gz, extract, build .deb via `ppa.BuildDeb` |
| `zcli.go`    | zCLI source: GitHub releases API, download .deb asset                   |
| `zerops.yml` | Zerops deployment config                                                |

## Build & Run Commands

- **Build**: `go build -o discord-ppa`
- **Run**: `go run .`
- **Test**: `go test ./...`
- **Format**: `gofmt -w .`

## Environment Variables

Required: `GPG_PRIVATE_KEY`, `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`

Optional:

- `ORIGIN` (default: `ppa.matejpavlicek.cz`), `LABEL` (default: `PPA`)
- `S3_REGION` (default: `us-east-1`), `LISTEN_ADDR` (default: `:8080`)
- `LOG_LEVEL` (default: `INFO`, options: `DEBUG`, `INFO`, `WARN`, `ERROR`)
- `DISCORD_DOWNLOAD_URL`, `DISCORD_POLL_INTERVAL` (default: `1h`)
- `POSTMAN_DOWNLOAD_URL`, `POSTMAN_POLL_INTERVAL` (default: `6h`)
- `ZCLI_GITHUB_REPO` (required to enable zCLI source), `ZCLI_POLL_INTERVAL` (default: `1h`)

Supports `.env` file (gitignored) for local development.

## Key Dependencies

- `github.com/ProtonMail/go-crypto/openpgp` — GPG signing
- `github.com/aws/aws-sdk-go-v2/service/s3` — S3 operations
- Custom `ppa/ar.go` — minimal ar archive reader/writer (replaces `blakesmith/ar` which had a panicking bug)
- `github.com/joho/godotenv` — .env file loading
