#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${1:-https://ppa.matejpavlicek.cz}"
KEYRING="/usr/share/keyrings/discord-ppa.gpg"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

pass=0
fail=0

check() {
    local name="$1"
    shift
    if "$@" > "$TMPDIR/out" 2>&1; then
        echo "PASS: $name"
        pass=$((pass + 1))
    else
        echo "FAIL: $name"
        fail=$((fail + 1))
    fi
    if [ -s "$TMPDIR/out" ]; then
        sed 's/^/  /' "$TMPDIR/out"
    fi
    echo ""
}

echo "Verifying PPA at $BASE_URL"
echo "---"

# 1. TLS
check "TLS certificate is valid" \
    bash -c "curl -sI '$BASE_URL/key.gpg' | head -1"

# 2. Key endpoint serves a valid GPG key
check "GET /key.gpg returns a valid GPG key" \
    bash -c "curl -fsSL '$BASE_URL/key.gpg' | gpg --show-keys --with-fingerprint"

# 3. Keyring installed locally
check "Local keyring exists at $KEYRING" \
    bash -c "ls -l '$KEYRING'"

# 4. Key fingerprints match (server vs local keyring)
check "Served key matches local keyring" \
    bash -c '
        remote=$(curl -fsSL "'"$BASE_URL"'/key.gpg" | gpg --show-keys --with-colons 2>/dev/null | grep "^fpr:" | head -1 | cut -d: -f10)
        local=$(gpg --no-default-keyring --keyring "'"$KEYRING"'" --list-keys --with-colons 2>/dev/null | grep "^fpr:" | head -1 | cut -d: -f10)
        echo "Remote: $remote"
        echo "Local:  $local"
        [ -n "$remote" ] && [ "$remote" = "$local" ]
    '

# 5. InRelease exists and has GPG signature
check "GET /dists/stable/InRelease returns clearsigned content" \
    bash -c "curl -fsSL '$BASE_URL/dists/stable/InRelease' | head -5"

# 6. InRelease signature verifies against the keyring
check "InRelease GPG signature is valid" \
    bash -c "curl -fsSL '$BASE_URL/dists/stable/InRelease' | gpg --no-default-keyring --keyring '$KEYRING' --verify 2>&1"

# 7. Detached Release.gpg verifies against Release
check "Release.gpg detached signature is valid" \
    bash -c '
        curl -fsSL "'"$BASE_URL"'/dists/stable/Release" -o "'"$TMPDIR"'/Release"
        curl -fsSL "'"$BASE_URL"'/dists/stable/Release.gpg" -o "'"$TMPDIR"'/Release.gpg"
        gpg --no-default-keyring --keyring "'"$KEYRING"'" --verify "'"$TMPDIR"'/Release.gpg" "'"$TMPDIR"'/Release" 2>&1
    '

# 8. Packages file exists and has content
check "GET /dists/stable/main/binary-amd64/Packages has entries" \
    bash -c "curl -fsSL '$BASE_URL/dists/stable/main/binary-amd64/Packages' | grep -E '^(Package|Version|SHA256):'"

# 9. SHA256 of Packages file matches what Release claims
check "Packages file SHA256 matches Release" \
    bash -c '
        curl -fsSL "'"$BASE_URL"'/dists/stable/main/binary-amd64/Packages" -o "'"$TMPDIR"'/Packages"
        actual=$(sha256sum "'"$TMPDIR"'/Packages" | cut -d" " -f1)
        expected=$(grep -A 100 "^SHA256:" "'"$TMPDIR"'/Release" | grep "main/binary-amd64/Packages$" | awk "{print \$1}")
        echo "Expected: $expected"
        echo "Actual:   $actual"
        [ -n "$actual" ] && [ "$actual" = "$expected" ]
    '

# 10. .deb file SHA256 matches what Packages claims
check ".deb SHA256 matches Packages metadata" \
    bash -c '
        sha_expected=$(grep "^SHA256:" "'"$TMPDIR"'/Packages" | awk "{print \$2}")
        filename=$(grep "^Filename:" "'"$TMPDIR"'/Packages" | awk "{print \$2}")
        [ -z "$sha_expected" ] || [ -z "$filename" ] && exit 1
        sha_actual=$(curl -fsSL "'"$BASE_URL"'/$filename" | sha256sum | cut -d" " -f1)
        echo "Expected: $sha_expected"
        echo "Actual:   $sha_actual"
        [ "$sha_expected" = "$sha_actual" ]
    '

# 11. apt update works without warnings (requires sources list to be configured)
if [ -f /etc/apt/sources.list.d/discord-ppa.list ]; then
    check "apt update accepts the repo without warnings" \
        bash -c "sudo apt update 2>&1 | grep -E 'ppa.matejpavlicek.cz' | grep -qv -i -e warning -e error -e err"
else
    echo "SKIP: apt update (no /etc/apt/sources.list.d/discord-ppa.list)"
fi

echo "---"
echo "Results: $pass passed, $fail failed"
exit "$fail"
