# Failure tests, run by hand

Run on 11–12 November 2026 on a local regtest network (Polar: Bitcoin Core 30.0, two LND 0.20.0 nodes, one 250,000-sat channel from bob to alice), through the web page.

| # | What was done | What happened | Rule it proves |
|---|---|---|---|
| 0 | Created a 1,000-sat invoice on alice and paid it from bob through the service | Payment `SUCCEEDED` with a preimage; invoice `PAID`. SHA-256 of the preimage equals the payment hash | the happy path; the preimage is the receipt |
| A | Paid the same invoice a second time | Refused: "this invoice is already being paid or was paid". No second row | the database allows one live payment per payment hash |
| A2 | Sent the same pay request 20 times in parallel (scripted, against a test database) | 1 accepted (202), 19 refused (409), one row | insert and let the unique index decide; no check-then-insert |
| B | Stopped bob's node, then asked to pay | Request accepted; row stayed `IN_FLIGHT`, not `FAILED`. After bob returned, the tracker asked the node, got "not found", and closed the row as `FAILED: node never received the payment request` | an error from the node call means unknown; only the node's answer closes a payment |
| B2 | Retried the same invoice after the failure | A new attempt was allowed (not "already being paid") | a failed payment may be retried; a live or succeeded one may not |
| C | Paused payouts, tried to pay, resumed, paid; paused again and restarted the service | Refused while paused; creating invoices still worked; same invoice paid after resume; still paused after restart | the pause switch is a database row checked before anything is saved |
| D | Stopped the service, paid alice's invoice directly from bob in Polar, started the service | Invoice turned `PAID` after start-up, with the time it was actually paid | the listener resumes from its saved settle index; no missed payments |
| E | Stopped bob, asked to pay (row `IN_FLIGHT`), killed the service with `kill -9`, restarted both | Row survived the kill; the tracker settled it as `FAILED` after start-up; a retry `SUCCEEDED` | the row is saved before the node is called; recovery needs only the database and the node |
| F | Pasted "hello" as an invoice; asked for a 200,000-sat invoice; pasted a genuine 150,000-sat invoice; signed in as alice and as bob | All three refused with a clear message and no row; alice has no pay action, bob has no receive action | input is checked first; "valid" and "allowed" are separate; each action names its roles |

Not run by hand: invoice expiry (needs an hour of waiting).

## An incident found while testing

After bob's node was restarted in test B, retries failed with `FAILURE_REASON_INSUFFICIENT_BALANCE` although the channel held about 241,000 sats on bob's side.

How it was found:

1. Polar showed the channel as open with funds, so the money was not the problem.
2. A health check was added to the service: how many of the paying node's channels are *active*. It showed "0 of 1 ready".
3. On the node itself, `lncli connect` answered "server is still in the process of starting".
4. Cause: at start-up LND waits until it is synced to the chain, and it treats a chain whose newest block is hours old as not synced. On regtest no block had been mined since the day before.
5. Fix: mine one block. The node finished starting, reconnected to its peer, the channel became active and payments succeeded. Auto-mining prevents it.

Two earlier guesses were wrong (the channel only needed a minute; the peer's address had changed). The node's own error message settled it.

What the service did during the incident: every attempt ended with the node's own answer, nothing was left stuck, nothing was paid twice, and each failed attempt could be retried.

Lesson: a node that answers RPC calls is not necessarily able to pay. Readiness has to be checked at the channel level, which the node panel now does.
