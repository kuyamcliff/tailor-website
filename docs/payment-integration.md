# Payment integration: MTN MoMo and Orange Money

Customers pay deposits and balances with Mobile Money from their order page. The browser never
decides whether a payment succeeded: the API asks the provider, and only a verified provider status
moves money on an order.

## How a payment works

1. The customer chooses MTN MoMo or Orange Money, enters their number and presses Pay.
2. `POST /api/v1/payments/intent` (with an `Idempotency-Key`) creates one payment for the order.
   A partial unique index allows only one active payment per order, so double clicks and retries
   cannot charge twice.
3. The API calls the provider: MTN `requesttopay`, or Orange `mp/init` then `mp/pay`. The customer
   approves on their phone with their PIN. We never see or store the PIN.
4. The provider calls back `POST|PUT /api/v1/payments/{provider}/webhook?ref=...&sig=...`. The URL
   is signed with an HMAC of our reference (`*_CALLBACK_SECRET`). Callback bodies are treated as a
   hint only: each event is stored once (unique provider and event key), then the API **re-reads
   the status from the provider** before changing anything.
5. The order page polls `GET /api/v1/payments/{id}`; each read also re-checks the provider while
   the payment is in progress.
6. A reconciliation job runs every 15 seconds for payments still in progress, so a lost callback
   cannot leave money unrecorded. A payment that expires and is later approved by the provider is
   still recorded (late approval).
7. When a payment succeeds, the order's paid amount and payment status update in the same
   transaction, the stage moves on if a deposit was due, and the customer and staff are notified.

Refunds are recorded by staff in the dashboard after the money has been sent back from the
merchant account; the provider APIs used here do not initiate refunds.

## MTN MoMo (Collection API)

Endpoints used (base URL and target environment come from MTN onboarding):

```
POST {MTN_MOMO_API_URL}/collection/token/                  Basic auth (API user, API key)
POST {MTN_MOMO_API_URL}/collection/v1_0/requesttopay       X-Reference-Id: our payment reference
GET  {MTN_MOMO_API_URL}/collection/v1_0/requesttopay/{ref} PENDING | SUCCESSFUL | FAILED
```

Variables: `MTN_MOMO_API_URL`, `MTN_MOMO_TARGET_ENVIRONMENT` (`sandbox`, then the market value
MTN gives you, for example `mtncameroon`), `MTN_MOMO_SUBSCRIPTION_KEY`, `MTN_MOMO_API_USER`,
`MTN_MOMO_API_KEY`, `MTN_MOMO_CALLBACK_SECRET`.

Sandbox checklist:
1. Create a sandbox user and key with the MTN developer portal (Collection product).
2. Set the variables, `PAYMENTS_DEV_SIMULATOR=false`, restart the API.
3. Owner > Settings > Features: turn on `payments_mtn` and `online_payments`. A switch refuses to
   turn on and explains why if credentials are missing.
4. Pay a test order with the sandbox test numbers MTN provides; confirm success, failure and
   timeout paths, and that a refreshed order page shows the right state.
5. Check Owner > Payments: each attempt, its provider reference and final status.

## Orange Money (merchant payment Core API)

Endpoints used (paths under the base URL from Orange onboarding; the version defaults to `1.0.2`
and can be changed with `ORANGE_MONEY_API_VERSION`):

```
POST {ORANGE_MONEY_API_URL}/token                                  client credentials
POST {ORANGE_MONEY_API_URL}/omcoreapis/{version}/mp/init           returns a payToken
POST {ORANGE_MONEY_API_URL}/omcoreapis/{version}/mp/pay            prompts the customer
GET  {ORANGE_MONEY_API_URL}/omcoreapis/{version}/mp/paymentstatus/{payToken}
```

Variables: `ORANGE_MONEY_API_URL`, `ORANGE_MONEY_API_VERSION`, `ORANGE_MONEY_CLIENT_ID`,
`ORANGE_MONEY_CLIENT_SECRET`, `ORANGE_MONEY_AUTH_TOKEN`, `ORANGE_MONEY_CHANNEL_MSISDN`,
`ORANGE_MONEY_PIN`, `ORANGE_MONEY_CALLBACK_SECRET`. Turn on `payments_orange` when ready.

Customers who see no prompt can dial `#150*50#` to approve pending payments; the payment panel says
so.

## Status

Both adapters follow the providers' published contracts and are covered by the simulator and
integration tests, but **neither has run against a real sandbox or production account yet**,
because no credentials were available. Expect to adjust field names or headers once the merchant
accounts are approved; the adapters keep those details in `backend/internal/payments/mtn.go` and
`orange.go`.

## Development simulator

`PAYMENTS_DEV_SIMULATOR=true` replaces both providers with a simulator (refused when
`APP_ENV=production`). The phone number's last four digits pick the outcome: `0001` success,
`0002` pending, `0003` failed, `0004` provider unavailable, `0005` duplicate callback, `0006` no
callback (resolved by reconciliation), `0007` cancelled, `0008` expired (the provider reports a timeout). Simulated payments carry
`simulated: true`, are labelled "Test" in every screen and document, and are excluded from revenue.

## Security notes

- Provider secrets live only in the API's environment. The browser never receives them.
- Every amount comes from the order on the server; the client only chooses deposit, balance or full.
- Provider events are append-only and deduplicated; payment status changes are audited.
