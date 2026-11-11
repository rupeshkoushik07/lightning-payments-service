# ln-payment-go

A small Go service that sits next to two Lightning (LND) nodes: one receives ("alice"), one pays ("bob").
It creates invoices, pays invoices, and keeps a correct record of every payment, including when things fail.

Status: work in progress. The pieces below compile; they have not yet been run against Postgres or real nodes.

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
| `internal/httpapi` | `POST /payments` |
| `internal/worker` | sender (pays) and tracker (resolves unknown outcomes) |

## Hardening

SQL injection, vulnerability checks, backups and stress testing: see [docs/hardening.md](docs/hardening.md).

## Test

```bash
go test ./...
```
