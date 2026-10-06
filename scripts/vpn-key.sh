#!/usr/bin/env bash
# Registers a WireGuard device on your Mullvad account and writes WIREGUARD_PRIVATE_KEY and
# WIREGUARD_ADDRESSES into .env (kept if already set). Uses one of the account's 5 device slots.
#   MULLVAD_ACCOUNT=<16 digits> make vpn-key      (or you are prompted; the number is never stored)
set -euo pipefail
cd "$(dirname "$0")/.."
touch .env
if grep -q '^WIREGUARD_PRIVATE_KEY=' .env && grep -q '^WIREGUARD_ADDRESSES=' .env; then
  echo "WIREGUARD_* already set in .env: nothing to do."
  exit 0
fi
account="${MULLVAD_ACCOUNT:-}"
if [ -z "$account" ]; then
  read -rsp "Mullvad account number: " account
  echo
fi
api=https://api.mullvad.net
pem=$(openssl genpkey -algorithm X25519)
priv=$(printf '%s\n' "$pem" | openssl pkey -outform DER | tail -c 32 | base64)
pub=$(printf '%s\n' "$pem" | openssl pkey -pubout -outform DER | tail -c 32 | base64)
token=$(curl -fsS -X POST "$api/auth/v1/token" -H 'Content-Type: application/json' \
  -d "{\"account_number\":\"$account\"}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')
addr=$(curl -fsS -X POST "$api/accounts/v1/devices" -H "Authorization: Bearer $token" \
  -H 'Content-Type: application/json' -d "{\"pubkey\":\"$pub\"}" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["ipv4_address"])')
{
  echo "WIREGUARD_PRIVATE_KEY=$priv"
  echo "WIREGUARD_ADDRESSES=$addr"
} >> .env
chmod 600 .env
echo "Device registered on your Mullvad account; keys written to .env."
