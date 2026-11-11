# Hardening plan

What protects this service beyond the happy path. "Done" means it is in the code today.

## SQL injection
- Done: every query uses placeholders (`$1, $2`). User input is sent to Postgres as data, never glued into the SQL text.
- Done: request bodies are size-limited and the invoice is decoded and validated before it reaches the database.
- Rule for reviews: no `fmt.Sprintf` or string `+` to build SQL.

## Security checks in CI
Four jobs run on every push.

| Job | Kind | Tool | Looks at | Finds |
|---|---|---|---|---|
| build and test | - | `go vet`, `go test -race` | our code, running | broken code, data races |
| static analysis | SAST | `staticcheck`, `gosec` | our code, without running it | bugs, SQL built from strings, weak crypto, unchecked errors, hardcoded secrets |
| dependency scan | SCA | `govulncheck` | Go itself and the libraries we import | known vulnerabilities (CVEs) that our code actually reaches |
| secret scan | - | `gitleaks` | every commit in history | keys or passwords committed by mistake |

Also: Dependabot opens a pull request weekly when a library or a CI action has a newer version.

Other protections already in the code: secrets are git-ignored, read from files at startup and never logged; least-privilege macaroons; the macaroon is refused on an unencrypted connection.

To do: container image scan when we add a Dockerfile. CodeQL once the repository is public (code scanning is free for public repositories).

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
