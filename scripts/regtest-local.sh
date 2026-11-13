#!/usr/bin/env bash
# A private Lightning network without Docker or Polar: one bitcoind and two
# LND nodes (alice receives, bob pays), all on 127.0.0.1, play money only.
# Usage: BIN=/path/with/bitcoind-and-lnd scripts/regtest-local.sh up|down
set -euo pipefail
BIN="${BIN:?set BIN to the folder holding bitcoind, bitcoin-cli, lnd, lncli}"
DIR="${REGTEST_DIR:-$HOME/.lnpay-regtest}"
BTC=(-regtest -datadir="$DIR/bitcoin" -rpcuser=dev -rpcpassword=dev-only -rpcport=18443)

cli()  { "$BIN/bitcoin-cli" "${BTC[@]}" "$@"; }
ln()   { local n=$1; shift; "$BIN/lncli" --network=regtest --lnddir="$DIR/$n" --rpcserver="127.0.0.1:$(port "$n")" "$@"; }
port() { [ "$1" = alice ] && echo 10001 || echo 10002; }
mine() { cli generatetoaddress "$1" "$(cli -rpcwallet=miner getnewaddress)" >/dev/null; }
wait_for() { for _ in $(seq 60); do "$@" >/dev/null 2>&1 && return 0; sleep 1; done; echo "timed out: $*" >&2; return 1; }

start_lnd() { # name grpc rest p2p
  mkdir -p "$DIR/$1"
  nohup "$BIN/lnd" --lnddir="$DIR/$1" --bitcoin.active --bitcoin.regtest --bitcoin.node=bitcoind \
    --bitcoind.rpchost=127.0.0.1:18443 --bitcoind.rpcuser=dev --bitcoind.rpcpass=dev-only \
    --bitcoind.zmqpubrawblock=tcp://127.0.0.1:28332 --bitcoind.zmqpubrawtx=tcp://127.0.0.1:28333 \
    --rpclisten=127.0.0.1:$2 --restlisten=127.0.0.1:$3 --listen=127.0.0.1:$4 \
    --noseedbackup --alias="$1" >"$DIR/$1.log" 2>&1 &
}

case "${1:-up}" in
up)
  mkdir -p "$DIR/bitcoin"
  "$BIN/bitcoind" "${BTC[@]}" -daemon -server -txindex -fallbackfee=0.0002 -rpcbind=127.0.0.1 -rpcallowip=127.0.0.1 \
    -zmqpubrawblock=tcp://127.0.0.1:28332 -zmqpubrawtx=tcp://127.0.0.1:28333 -listen=0
  wait_for cli getblockchaininfo
  cli createwallet miner >/dev/null 2>&1 || cli loadwallet miner >/dev/null 2>&1 || true
  mine 101                                   # coinbase coins need 100 blocks to be spendable
  start_lnd alice 10001 18081 19735
  start_lnd bob   10002 18082 19736
  wait_for ln alice getinfo; wait_for ln bob getinfo
  cli -rpcwallet=miner sendtoaddress "$(ln bob newaddress p2wkh | grep -o 'bcrt1[a-z0-9]*')" 1 >/dev/null
  mine 6
  wait_for sh -c "'$BIN/lncli' --network=regtest --lnddir='$DIR/bob' --rpcserver=127.0.0.1:10002 walletbalance | grep -q '\"confirmed_balance\": *\"[1-9]'"
  ALICE_KEY=$(ln alice getinfo | grep -o '"identity_pubkey": *"[0-9a-f]*"' | grep -o '[0-9a-f]\{66\}')
  ln bob connect "$ALICE_KEY@127.0.0.1:19735" >/dev/null || true
  ln bob openchannel --node_key="$ALICE_KEY" --local_amt=5000000 >/dev/null
  mine 6
  wait_for sh -c "'$BIN/lncli' --network=regtest --lnddir='$DIR/bob' --rpcserver=127.0.0.1:10002 listchannels | grep -q '\"active\": *true'"
  echo "ready: bob -> alice channel of 5,000,000 sats"
  echo "alice: 127.0.0.1:10001  cert $DIR/alice/tls.cert  macaroon $DIR/alice/data/chain/bitcoin/regtest/invoice.macaroon"
  echo "bob:   127.0.0.1:10002  cert $DIR/bob/tls.cert    macaroon $DIR/bob/data/chain/bitcoin/regtest/admin.macaroon"
  ;;
down)
  ln alice stop >/dev/null 2>&1 || true; ln bob stop >/dev/null 2>&1 || true
  cli stop >/dev/null 2>&1 || true
  ;;
esac
