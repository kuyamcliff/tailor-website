# Working on this repository

Tailoring atelier platform: Next.js storefront and owner dashboard (`frontend/`), Go API with
PostgreSQL (`backend/`). Read `README.md` for setup and `IMPLEMENTATION_STATUS.md` for what is
finished and what blocks launch. Update `IMPLEMENTATION_STATUS.md` whenever a feature lands or a
blocker changes.

## Commands

- Backend: `go vet ./... && go test ./internal/...`; integration tests need a disposable database:
  `TEST_DATABASE_URL=postgres://atelier:atelier@localhost:5432/atelier_test go test ./tests/ -count=1`
  (the schema is dropped and recreated).
- Frontend: `npm run lint && npm run typecheck && npm test && npm run check:copy`.
- End to end: `npm run test:e2e` against a running stack with `PAYMENTS_DEV_SIMULATOR=true` and
  `seed-dev` data. Set `CHROMIUM_PATH` to use an installed Chromium. Not run in CI yet because it
  needs the full stack.
- `.claude/hooks/post-edit.sh` formats edited files (gofmt, Prettier) and blocks em dashes in
  customer-facing code and anything that looks like a secret.

## Rules that are easy to break

- **No em dashes** in any text a customer or owner can read (frontend, seed content, notification
  text). The API also rejects them in content blocks. `npm run check:copy` enforces it.
- **Never put credentials in code, fixtures, logs, screenshots or `.env.example` values.** Payment,
  email, storage and AI credentials come only from environment variables and features stay off
  until they are set.
- **Payments are confirmed only by the server after asking the provider.** Never mark an order paid
  from a browser redirect or a callback body alone. Keep idempotency keys on anything that creates
  orders, payments, requests or messages. The simulator must stay impossible in production, and
  simulated payments must stay labelled and excluded from revenue.
- **No invented content presented as real**: no fake testimonials, reviews, customer counts, stats,
  logos or portfolio work. Stock photos must stay labelled as samples (the `sample` media flag and
  footer note do this automatically from upload licence metadata). Use real brand assets from
  `frontend/public/brand` (see `SOURCES.md`) rather than approximations.
- **The 3D models are stand-ins** (`productionQuality: false`). Do not describe the studio as
  production ready until licensed production models are published; see `docs/3d-assets.md`.
- **Dates and money** on the frontend go through `lib/format.ts` and `lib/money.ts`, which assemble
  strings by hand so server and browser render identically (no hydration mismatches). Do not call
  `toLocaleString` in components.
- **Fit logic exists twice** (`backend/internal/fit` and `frontend/lib/fit.ts`). Change both and
  update `backend/internal/fit/testdata/fixtures.json`; both test suites read it.
- Respect `prefers-reduced-motion`; keep keyboard access and visible focus; give every image a real
  `alt` (or `alt=""` when decorative).

## Design

Dark, editorial and quiet: Cormorant Garamond for display, Manrope for text, gold only as an accent.
Prefer rules and lists over card grids, plain words over marketing phrases, and one clear action per
section. No gradients, glass effects, glowing accents or oversized icons. Check new screens at 320,
768 and 1440 pixels wide (`e2e/responsive.spec.ts`).

## Layout of the code

- `backend/internal/server/server.go`: every route and the background jobs.
- `backend/internal/<domain>`: handlers and storage per domain; hand-written SQL with pgx.
- `backend/migrations`: numbered up and down SQL files, embedded and applied with an advisory lock.
- `frontend/app`: routes; `frontend/features/<area>`: page components; `frontend/components`: shared UI;
  `frontend/lib`: API client, formatting, types.
