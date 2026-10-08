# Tailor atelier platform

A website and back office for a bespoke tailoring atelier: a ready-to-wear shop, a custom request
and quote workflow, a 3D fitting studio, appointments, customer accounts, Mobile Money payments
(MTN MoMo and Orange Money) and an owner dashboard for running the workshop.

- `frontend/`: Next.js 16, React 19, TypeScript, React Three Fiber
- `backend/`: Go API (net/http with chi), PostgreSQL via pgx, embedded SQL migrations
- `docs/`: deployment, operations, payments, 3D assets, performance budgets
- `IMPLEMENTATION_STATUS.md`: what is finished, what is a stand-in, and what blocks launch

## Run it locally

Requirements: Go 1.26+, Node 22+, PostgreSQL 16.

```sh
# 1. Database
createuser -s atelier || true
psql -c "ALTER USER atelier WITH PASSWORD 'atelier'"
createdb -O atelier atelier_dev

# 2. API (from backend/)
cp ../.env.example .env            # edit as needed; development defaults work as is
set -a; . ./.env; set +a
go run ./cmd/api migrate
go run ./cmd/api seed              # reference data: roles, garments, options, fit rules, content, 3D assets
go run ./cmd/api seed-dev          # development only: sample products, fabrics, portfolio and logins
go run ./cmd/api serve             # http://localhost:8080

# 3. Site (from frontend/)
npm install
npm run dev                        # http://localhost:3000
```

Development logins created by `seed-dev` (refused in production):

| Role  | Email                | Password            |
|-------|----------------------|---------------------|
| Owner | owner@atelier.test   | atelier-owner-dev   |
| Tailor| tailor@atelier.test  | atelier-tailor-dev  |

`seed-dev` downloads licensed sample photos (Unsplash licence, recorded per image). The site marks
them as "Sample photo" and explains them in the footer until the owner replaces them.

With `PAYMENTS_DEV_SIMULATOR=true`, checkout uses a payment simulator. The last four digits of the
phone number choose the outcome: `0001` success, `0002` stays pending, `0003` fails, `0004`
provider unavailable, `0005` duplicate callback, `0006` no callback (found by reconciliation),
`0007` cancelled, `0008` expired on the phone. Simulated payments are labelled as tests everywhere and never count as revenue.

## Commands

| Where      | Command                              | What it does |
|------------|--------------------------------------|--------------|
| backend    | `go test ./...`                      | unit tests |
| backend    | `TEST_DATABASE_URL=... go test ./tests/ -count=1` | integration tests against a disposable database |
| backend    | `go run ./cmd/api bootstrap-owner`   | create the first owner from `BOOTSTRAP_OWNER_*` |
| backend    | `go run ./cmd/api rollback`          | roll back the latest migration |
| frontend   | `npm run lint` / `typecheck` / `test`| ESLint, TypeScript, Vitest |
| frontend   | `npm run check:copy`                 | fail on em dashes, placeholder text and template phrases |
| frontend   | `npm run test:e2e`                   | Playwright against a running stack (`BASE_URL`, `CHROMIUM_PATH`) |
| frontend   | `npm run assets:build`               | regenerate the procedural stand-in 3D models and manifest |
| frontend   | `node scripts/render-previews.mjs`   | render the home page studio preview frames from the running studio |

## Where things are

- Public site: `frontend/app` (routes) and `frontend/features/*` (page logic)
- Owner dashboard: `frontend/app/owner`, `frontend/features/owner`
- 3D studio: `frontend/features/studio`, stand-in models in `frontend/public/3d`
- API routes: `backend/internal/server/server.go`; each domain lives in `backend/internal/<domain>`
- Database schema: `backend/migrations`

## Before launch

Read `IMPLEMENTATION_STATUS.md`. In short: production 3D garment models, real photography,
payment provider credentials, a legal review of the policies, and the deployment steps in
`docs/deployment.md`.
