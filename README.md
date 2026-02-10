# Discord PPA

Unofficial APT repository for [Discord](https://discord.com/) on Linux.

A Go app that polls Discord's download URL for new `.deb` releases, stores them in S3, generates GPG-signed APT metadata, and serves the repository over HTTPS. Deployed at **[ppa.matejpavlicek.cz](https://ppa.matejpavlicek.cz)**.

## Install Discord

```bash
# 1. Download the signing key
curl -fsSL https://ppa.matejpavlicek.cz/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/discord-ppa.gpg

# 2. Add the repository
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/discord-ppa.gpg] https://ppa.matejpavlicek.cz stable main" | sudo tee /etc/apt/sources.list.d/discord-ppa.list

# 3. Update and install
sudo apt update
sudo apt install discord
```

Updates are delivered automatically via `apt upgrade` whenever Discord publishes a new version.

## How It Works

1. A background poller checks Discord's download URL periodically (ETag-based change detection)
2. When a new version is found, the `.deb` is downloaded, parsed, and uploaded to S3
3. APT metadata (`Packages`, `Release`, `InRelease`, `Release.gpg`) is regenerated and GPG-signed
4. An HTTP server proxies repository files from S3 to apt clients

## Self-Hosting

### Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `GPG_PRIVATE_KEY` | yes | | Armored PGP private key (no passphrase) |
| `S3_ENDPOINT` | yes | | S3-compatible endpoint |
| `S3_BUCKET` | yes | | Bucket name |
| `S3_ACCESS_KEY` | yes | | |
| `S3_SECRET_KEY` | yes | | |
| `S3_REGION` | no | `us-east-1` | |
| `DOWNLOAD_URL` | no | Discord API | URL to poll for `.deb` |
| `POLL_INTERVAL` | no | `1h` | Go duration string |
| `LISTEN_ADDR` | no | `:8080` | HTTP listen address |

A `.env` file in the working directory is loaded automatically.

### Build and Run

```bash
go build -o discord-ppa
./discord-ppa
```

### Generate a GPG Key

```bash
gpg --batch --gen-key <<EOF
%no-protection
Key-Type: RSA
Key-Length: 4096
Name-Real: Discord PPA
Name-Email: you@example.com
Expire-Date: 0
EOF

gpg --armor --export-secret-keys you@example.com
```

Set the output as `GPG_PRIVATE_KEY`.

### Verify

```bash
./verify.sh                        # against production
./verify.sh http://localhost:8080   # against local instance
```

## Powered by Zerops

This project runs on [Zerops](https://zerops.io) ❤️ — a dev-first cloud platform that handles infrastructure so you can focus on code.

## License

MIT
