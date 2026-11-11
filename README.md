# ln-payment-go

A small Go service that sits next to two Lightning (LND) nodes: one receives ("alice"), one pays ("bob").
It creates invoices, pays invoices, and keeps a correct record of every payment, including when things fail.

Status: work in progress. The server runs against Postgres (duplicates, pause, unreachable node and clean shutdown checked by hand). A full payment through real nodes is the next step.

## Design rules

- The node is the truth about money; the database holds the business record.
- Save the payment row (with the payment hash from the invoice) before telling the node to pay.
- The database refuses a second live payment for the same hash (partial unique index). No check-then-insert.
- An error from the node call means "unknown", never "failed". The row stays IN_FLIGHT.
- The tracker asks the node what happened; it never pays again. Only the node's final answer closes a payment.

## Layout

| Path | What |
|---|---|
| `migrations/001_payments.sql` | payments table and its rules |
| `internal/payment` | status values and allowed moves |
| `internal/store` | all Postgres access |
| `internal/httpapi` | the HTTP endpoints, sign-in and roles |
| `internal/web` | the browser pages |
| `internal/lnd` | all calls to the nodes; local invoice decoding |
| `cmd/server` | the program: API plus workers |
| `internal/worker` | sender, tracker, invoice listener |

## Run against Polar

Needs Go, Docker, and a started Polar network with two LND nodes (alice, bob) and a channel from bob to alice.

```bash
scripts/run-polar.sh
```

Then open http://localhost:8080 and sign in. Three demo users exist: `alice` (receives), `bob` (pays), `admin` (both, plus the pause switch). Their passwords are in `.env`, which the script creates from `.env.example`.

To switch on **Sign in with Google**, fill in `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` and `ADMIN_EMAIL` in `.env` (see `.env.example`).

Try it in two browser windows: sign in as alice in one and create an invoice, sign in as bob in the other (a private window) and pay it. Both tables update by themselves.

| Endpoint | Who | Does |
|---|---|---|
| `POST /api/login`, `POST /api/logout`, `GET /api/me` | anyone | session sign-in |
| `POST /api/invoices` | receiver, admin | create an invoice on alice |
| `GET /api/invoices`, `GET /api/invoices/{hash}` | signed in | OPEN, PAID or EXPIRED |
| `POST /api/payments` | payer, admin | accept an invoice to pay from bob; answers 202 |
| `GET /api/payments`, `GET /api/payments/{id}` | signed in | PENDING, IN_FLIGHT, SUCCEEDED (with preimage) or FAILED |
| `GET/POST /api/admin/payouts` | admin | read or flip the pause switch |

## Hardening

SQL injection, vulnerability checks, backups and stress testing: see [docs/hardening.md](docs/hardening.md).

## Test

```bash
go test ./...
```
