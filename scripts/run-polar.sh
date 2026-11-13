#!/usr/bin/env bash
# Runs the server against a Polar network on this machine.
# Usage: scripts/run-polar.sh                                   (Polar network 1, nodes alice and bob)
#        POLAR_NET=2 RECEIVER_NODE=carol PAYER_NODE=dave \
#        ALICE_HOST=127.0.0.1:10003 BOB_HOST=127.0.0.1:10004 scripts/run-polar.sh
set -euo pipefail

# Local play-money demo: no passwords anywhere. The sign-in page shows one
# button per user (alice, bob, admin). The server only allows this while it
# listens on this machine.
export DEMO_LOGIN=true

# ---- Your Polar network. Every value can be overridden; nothing here is secret.
# Find yours in Polar: click a node -> "Connect" tab -> GRPC Host, TLS Cert, macaroons.
NET="${POLAR_NET:-1}"                   # the number in Polar's network list
RECEIVER="${RECEIVER_NODE:-alice}"      # the node that creates invoices
PAYER="${PAYER_NODE:-bob}"              # the node that pays
LND="${POLAR_LND_DIR:-$HOME/.polar/networks/$NET/volumes/lnd}"

# A throwaway Postgres in Docker, on its own port. Started once, reused after.
# Bring your own instead with DATABASE_URL=postgres://... scripts/run-polar.sh
if [ -z "${DATABASE_URL:-}" ]; then
  if ! docker ps --format '{{.Names}}' | grep -qx lnpay-pg; then
    docker start lnpay-pg >/dev/null 2>&1 || docker run -d --name lnpay-pg \
      -e POSTGRES_USER=lnpay -e POSTGRES_PASSWORD=dev-only -e POSTGRES_DB=lnpay \
      -p 127.0.0.1:55432:5432 postgres:16 >/dev/null
    sleep 3
  fi
  export DATABASE_URL="postgres://lnpay:dev-only@127.0.0.1:55432/lnpay"
fi

# Receiver: the invoice macaroon cannot spend.
export ALICE_HOST="${ALICE_HOST:-127.0.0.1:10001}"
export ALICE_TLS_CERT="${ALICE_TLS_CERT:-$LND/$RECEIVER/tls.cert}"
export ALICE_MACAROON="${ALICE_MACAROON:-$LND/$RECEIVER/data/chain/bitcoin/regtest/invoice.macaroon}"

# Payer. Polar's admin macaroon for now; a baked send-only key is planned.
export BOB_HOST="${BOB_HOST:-127.0.0.1:10002}"
export BOB_TLS_CERT="${BOB_TLS_CERT:-$LND/$PAYER/tls.cert}"
export BOB_MACAROON="${BOB_MACAROON:-$LND/$PAYER/data/chain/bitcoin/regtest/admin.macaroon}"

# Fail early with a clear message instead of a gRPC error later.
missing=0
for f in "$ALICE_TLS_CERT" "$ALICE_MACAROON" "$BOB_TLS_CERT" "$BOB_MACAROON"; do
  [ -f "$f" ] || { echo "not found: $f" >&2; missing=1; }
done
if [ "$missing" = 1 ]; then
  echo "Check the network number and node names in Polar, then set POLAR_NET, RECEIVER_NODE, PAYER_NODE" >&2
  echo "(or ALICE_*/BOB_* directly). See README: Use your own Polar network." >&2
  exit 1
fi
echo "receiver: $RECEIVER at $ALICE_HOST    payer: $PAYER at $BOB_HOST"

exec go run ./cmd/server
