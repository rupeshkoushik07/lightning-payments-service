# Hardening plan

What protects this service beyond the happy path. "Done" means it is in the code today.

## SQL injection
- Done: every query uses placeholders (`$1, $2`). User input is sent to Postgres as data, never glued into the SQL text.
- Done: request bodies are size-limited and the invoice is decoded and validated before it reaches the database.
- Rule for reviews: no `fmt.Sprintf` or string `+` to build SQL.

## Vulnerabilities
- Done: CI runs `go vet`, tests with the race detector, and `govulncheck` (known CVEs in our dependencies that our code actually calls).
- Done: secrets (macaroons, certificates, `.env`) are git-ignored; keys are read from files at startup and never logged.
- Done: least-privilege macaroons; macaroon is refused on an unencrypted connection.
- To do: dependency update bot; container image scan when we add a Dockerfile.

## Data backup
Two different things hold state, and they are backed up differently.
- Postgres (our records): nightly full backup plus continuous WAL archiving, so we can restore to any minute (point-in-time recovery). Test the restore, not just the backup.
- LND node (the money): the 24-word seed, written down once and kept offline, plus the static channel backup file (`channel.backup`), copied off the machine every time a channel opens or closes.
- Never restore an old copy of LND's channel database: publishing an old channel state looks like cheating and the node can lose its funds. That is why the static channel backup exists.
- After any restore, the reconciler compares our rows with the node's list by payment hash.

## Stress testing
- To do, once the service runs end to end: a load test (k6 or `hey`) against `POST /payments` and `GET /payments/{id}`.
- What to measure: p50/p99 response time, error share, database connections in use, age of the oldest IN_FLIGHT payment.
- What to try: 50 identical requests at once for one invoice (exactly one row must win), node stopped mid-test (rows stay IN_FLIGHT, then resolve), database slow.
- Expected limit: one LND node pays through its own channels, so the node, not our Go code, is the ceiling.
