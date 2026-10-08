# Operations

Day-to-day running of the platform: what runs in the background, what to watch, and what to do when
something goes wrong.

## Background jobs (inside the API process)

| Job | Every | What it does |
|---|---|---|
| Notification delivery | continuous | Sends queued email and SMS; batches are claimed with `SKIP LOCKED`, so several API instances never double-send. Failures retry with backoff and are kept on the notification row. |
| Payment reconciliation | 15 s | Re-checks payments still in progress with the provider, so a lost callback never leaves money unrecorded. |
| Quote expiry | 1 min | Marks sent quotes past their validity as expired. |
| Appointment reminders | 5 min | Queues reminders the day before. |
| Upload retention | 1 h | Deletes expired private uploads and abandoned guest uploads (bytes and records). |
| Session cleanup | 1 h | Removes expired sessions. |

## What to watch

- `GET /readyz` from your uptime monitor (checks the database).
- Logs are JSON, one line per request with `request_id`, route, status and duration. Errors are
  logged at `ERROR` with the request ID the customer sees in error pages; search by it.
- `GET /metrics` request counts and latencies per route (protect it at the proxy).
- Owner > Payments filtered by "Pending" or "Customer action required" older than an hour usually
  means a provider outage or a customer who never approved; use "Check again" on the order.
- Owner > Overview lists overdue orders, unread messages and low fabric stock.

## Common tasks

**A customer says they paid but the order is unpaid.** Open the order, press "Check again" on the
payment. If the provider still reports pending or failed, the money has not reached the merchant
account. Never mark it paid by hand unless the money is in the merchant account; then use "Record a
payment" with the provider's transaction ID in the note.

**Refunds.** Send the money from the merchant account first, then record it on the payment in the
order. The order's balance and payment status update and the action is audited.

**Locked staff account.** After five failed sign-ins an account locks for 15 minutes. The owner can
also unlock it by resetting the password; a successful reset unlocks it.

**A customer asks for their data or for deletion.** Account holders can do both themselves in
Settings and privacy. For guests, find them in Owner > Customers; export by request through the
support thread and contact a developer to anonymise (orders are kept for accounting with personal
details removed).

**Change a policy or page text.** Owner > Pages and policies. Changes appear within a minute.

## Backups

- Database: daily full backups plus point-in-time recovery where available. Test a restore every
  quarter into a scratch database and run `go test ./tests/` against it.
- Object storage: enable versioning on the bucket, or replicate it.
- Configuration and secrets: kept in the host's secret manager, never in backups of the repository.

## Incidents

1. Check `/readyz`, recent deploys and provider status pages (MTN, Orange).
2. If payments are failing at the provider, turn off `online_payments` in Owner > Settings; customers
   can still order and pay at the studio, and nothing already paid is affected.
3. If a release is at fault, redeploy the previous images (see `docs/deployment.md`).
4. Write down what happened in the audit trail note or your incident log.
