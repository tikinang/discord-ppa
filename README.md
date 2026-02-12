# PPA

Unofficial APT repository serving Discord, Postman, and zCLI on Linux.

A Go app that polls multiple upstream sources for new `.deb` releases, stores them in S3, generates GPG-signed APT metadata, and serves the repository over HTTPS. Deployed at **[ppa.matejpavlicek.cz](https://ppa.matejpavlicek.cz)**.

## Install

```bash
# 1. Download the signing key
curl -fsSL https://ppa.matejpavlicek.cz/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/ppa.gpg

# 2. Add the repository
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/ppa.gpg] https://ppa.matejpavlicek.cz stable main" | sudo tee /etc/apt/sources.list.d/matej-pavlicek-ppa.list

# 3. Update and install
sudo apt update
sudo apt install discord postman   # install whichever packages you need
```

Updates are delivered automatically via `apt upgrade`.

## How It Works

1. Each source (Discord, Postman, zCLI) has its own polling goroutine that checks for new upstream versions
2. When a new version is found, the `.deb` is downloaded (or built from a tar.gz), parsed, and uploaded to S3
3. APT metadata (`Packages`, `Release`, `InRelease`, `Release.gpg`) is regenerated and GPG-signed
4. An HTTP server proxies repository files from S3 to apt clients

## Self-Hosting

### Environment Variables

| Variable                 | Required | Default                   | Description                             |
|--------------------------|----------|---------------------------|-----------------------------------------|
| `GPG_PRIVATE_KEY`        | yes      |                           | Armored PGP private key (no passphrase) |
| `S3_ENDPOINT`            | yes      |                           | S3-compatible endpoint                  |
| `S3_BUCKET`              | yes      |                           | Bucket name                             |
| `S3_ACCESS_KEY`          | yes      |                           |                                         |
| `S3_SECRET_KEY`          | yes      |                           |                                         |
| `S3_REGION`              | no       | `us-east-1`               |                                         |
| `LISTEN_ADDR`            | no       | `:8080`                   | HTTP listen address                     |
| `ORIGIN`                 | no       | `ppa.matejpavlicek.cz`    | APT Release Origin field                |
| `LABEL`                  | no       | `PPA`                     | APT Release Label field                 |
| `DISCORD_DOWNLOAD_URL`   | no       | Discord API               | URL to poll for Discord `.deb`          |
| `DISCORD_POLL_INTERVAL`  | no       | `1h`                      | Go duration string                      |
| `POSTMAN_DOWNLOAD_URL`   | no       | `dl.pstmn.io/...`         | URL to poll for Postman tar.gz          |
| `POSTMAN_POLL_INTERVAL`  | no       | `6h`                      | Go duration string                      |
| `ZCLI_GITHUB_REPO`       | no       |                           | GitHub `owner/repo` (enables zCLI)      |
| `ZCLI_POLL_INTERVAL`     | no       | `1h`                      | Go duration string                      |

A `.env` file in the working directory is loaded automatically.

### Build and Run

```bash
go build -o discord-ppa
./discord-ppa
```

### Verify

```bash
./verify.sh                        # against production
./verify.sh http://localhost:8080   # against local instance
```

## Local Development

### 1. Generate a test GPG key

Create a throwaway key with no passphrase (required by the app):

```bash
gpg --batch --gen-key <<EOF
%no-protection
Key-Type: RSA
Key-Length: 4096
Name-Real: PPA Dev
Name-Email: dev@localhost
Expire-Date: 1y
EOF
```

Export the private key in armored form:

```bash
gpg --armor --export-secret-keys dev@localhost
```

Copy the entire output (including the `-----BEGIN PGP PRIVATE KEY BLOCK-----` and `-----END PGP PRIVATE KEY BLOCK-----` lines).

### 2. Create a `.env` file

The app loads `.env` automatically (it is gitignored). Assuming your S3 storage is provided by Zerops, grab the credentials from the Zerops dashboard and create `.env`:

```bash
cat > .env <<'EOF'
GPG_PRIVATE_KEY=-----BEGIN PGP PRIVATE KEY BLOCK-----
<paste your full armored key here>
-----END PGP PRIVATE KEY BLOCK-----

S3_ENDPOINT=<your-zerops-s3-endpoint>
S3_BUCKET=<your-zerops-bucket-name>
S3_ACCESS_KEY=<your-zerops-access-key>
S3_SECRET_KEY=<your-zerops-secret-key>

LISTEN_ADDR=:8080

# Shorter intervals for development
DISCORD_POLL_INTERVAL=5m
POSTMAN_POLL_INTERVAL=10m

# Disable zCLI locally (omit ZCLI_GITHUB_REPO)
EOF
```

### 3. Run

```bash
go run .
```

The server starts at `http://localhost:8080`. On first run each source will poll immediately, so you should see Discord and Postman packages appear in S3 within a few minutes.

### 4. Test the APT repo locally

```bash
# Fetch the public key from your local server
curl -fsSL http://localhost:8080/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/ppa-dev.gpg

# Add a local sources entry
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/ppa-dev.gpg] http://localhost:8080 stable main" | sudo tee /etc/apt/sources.list.d/ppa-dev.list

# Verify
sudo apt update
apt list --upgradable
```

### 5. Clean up the test GPG key

When you are done, remove the key from your local keyring:

```bash
gpg --delete-secret-and-public-keys dev@localhost
```

And remove the local APT config:

```bash
sudo rm /usr/share/keyrings/ppa-dev.gpg /etc/apt/sources.list.d/ppa-dev.list
```

## Powered by Zerops

This project runs on [Zerops](https://zerops.io) ❤️ — a dev-first cloud platform that handles infrastructure so you can focus on code.

## License

MIT
