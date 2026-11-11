#!/usr/bin/env bash
# Runs the server against a Polar network on this machine.
# Usage: scripts/run-polar.sh            (Polar network number 1)
#        POLAR_NET=2 scripts/run-polar.sh
set -euo pipefail

NET="${POLAR_NET:-1}"
LND="$HOME/.polar/networks/$NET/volumes/lnd"

# A throwaway Postgres in Docker, on its own port. Started once, reused after.
if ! docker ps --format '{{.Names}}' | grep -qx lnpay-pg; then
  docker start lnpay-pg >/dev/null 2>&1 || docker run -d --name lnpay-pg \
    -e POSTGRES_USER=lnpay -e POSTGRES_PASSWORD=dev-only -e POSTGRES_DB=lnpay \
    -p 127.0.0.1:55432:5432 postgres:16 >/dev/null
  sleep 3
fi

export DATABASE_URL="postgres://lnpay:dev-only@127.0.0.1:55432/lnpay"

# alice receives: the invoice macaroon cannot spend.
export ALICE_HOST="127.0.0.1:10001"
export ALICE_TLS_CERT="$LND/alice/tls.cert"
export ALICE_MACAROON="$LND/alice/data/chain/bitcoin/regtest/invoice.macaroon"

# bob pays. For now this is Polar's admin macaroon; it will be replaced by a
# baked key that can only send and track payments.
export BOB_HOST="127.0.0.1:10002"
export BOB_TLS_CERT="$LND/bob/tls.cert"
export BOB_MACAROON="${BOB_MACAROON:-$LND/bob/data/chain/bitcoin/regtest/admin.macaroon}"

exec go run ./cmd/server
