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
| `internal/httpapi` | the HTTP endpoints |
| `internal/lnd` | all calls to the nodes; local invoice decoding |
| `cmd/server` | the program: API plus workers |
| `internal/worker` | sender, tracker, invoice listener |

## Run against Polar

Needs Go, Docker, and a started Polar network with two LND nodes (alice, bob) and a channel from bob to alice.

```bash
scripts/run-polar.sh
```

Then, in another terminal:

```bash
# alice asks to be paid 1,000 sats
curl -s -X POST localhost:8080/invoices -d '{"amount_sat":1000,"memo":"t-shirt"}'

# bob pays the invoice text from the answer above
curl -s -X POST localhost:8080/payments -d '{"invoice":"lnbcrt..."}'

# poll the results
curl -s localhost:8080/payments/1
curl -s localhost:8080/invoices/<payment_hash>
```

| Endpoint | Does |
|---|---|
| `POST /invoices` | create an invoice on alice |
| `GET /invoices/{hash}` | OPEN, PAID or EXPIRED |
| `POST /payments` | accept an invoice to pay from bob; answers 202 |
| `GET /payments/{id}` | PENDING, IN_FLIGHT, SUCCEEDED (with preimage) or FAILED |
| `POST /admin/payouts` | `{"paused": true}` stops new payouts |

## Hardening

SQL injection, vulnerability checks, backups and stress testing: see [docs/hardening.md](docs/hardening.md).

## Test

```bash
go test ./...
```
