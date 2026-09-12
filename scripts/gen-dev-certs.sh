#!/usr/bin/env bash
# Generates a throwaway self-signed TLS certificate for local HTTPS via
# Nginx (deployments/nginx/nginx.conf). Never use this output in production —
# a real deployment terminates TLS with a certificate from a real CA (or
# ACME/Let's Encrypt), which is a deployment-environment concern, not
# something this repo can generate for you.
set -euo pipefail

CERT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/deployments/nginx/certs"
mkdir -p "$CERT_DIR"

if [[ -f "$CERT_DIR/dev.crt" && -f "$CERT_DIR/dev.key" ]]; then
  echo "Dev certs already exist at $CERT_DIR — remove them first to regenerate."
  exit 0
fi

openssl req -x509 -nodes -newkey rsa:2048 \
  -keyout "$CERT_DIR/dev.key" \
  -out "$CERT_DIR/dev.crt" \
  -days 365 \
  -subj "/CN=localhost/O=GearShare Dev" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

echo "Generated self-signed dev cert at $CERT_DIR/dev.crt (+ dev.key)."
echo "Your browser will warn about it being untrusted — that's expected for local dev."
