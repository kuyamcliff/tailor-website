# Deployment

The platform is two services and a database:

- **API** (`backend/`, Go): stateless, listens on `HTTP_ADDR`, runs background jobs (payment
  reconciliation, notifications, quote expiry, upload retention) in the same process.
- **Site** (`frontend/`, Next.js standalone server): renders pages and proxies `/api/v1/*` to the API.
- **PostgreSQL 16** and **S3-compatible object storage** for uploads and 3D asset files.

Both services have Dockerfiles (`backend/Dockerfile`, built from the repository root, and
`frontend/Dockerfile`). `docker-compose.yml` runs a local production-like stack.

## 1. Prepare services

- PostgreSQL 16 with daily backups and point-in-time recovery if your host offers it.
- An S3-compatible bucket (private). Set `STORAGE_DRIVER=s3` and the `STORAGE_*` variables.
  Public images (products, portfolio, content) are served through the API with long cache headers;
  private uploads (references, support photos, body photos) are only served to their owner and staff.
- SMTP for email (`EMAIL_PROVIDER=smtp`). The development log sender is refused in production.
- HTTPS in front of both services (load balancer or reverse proxy). Set `TRUSTED_PROXY_HOPS` to the
  number of proxies so rate limits and logs see the real client address.

## 2. Configure

Copy `.env.example`, fill in production values and store them in your host's secret manager, never
in the repository. The API refuses to start in production when:

- `PAYMENTS_DEV_SIMULATOR` is on,
- `COOKIE_SECURE` is off,
- `PUBLIC_SITE_URL` or `PUBLIC_API_URL` is not https,
- `INTERNAL_API_KEY` is shorter than 32 characters,
- `EMAIL_PROVIDER=log`.

Generate secrets with `openssl rand -base64 48` (callback secrets and the internal key).

The site needs `BACKEND_URL` and `PUBLIC_SITE_URL` **at build time** (the `/api/v1` rewrite and the
canonical URLs in prerendered pages are fixed when the image is built) and `API_INTERNAL_URL`,
`PUBLIC_SITE_URL` and `APP_ENV=production` at run time.

```sh
docker build -f backend/Dockerfile -t atelier-api .
docker build --build-arg BACKEND_URL=http://api.internal:8080 \
             --build-arg PUBLIC_SITE_URL=https://www.example.com \
             -t atelier-web frontend
```

## 3. First release

```sh
api migrate              # applies embedded migrations under an advisory lock
api seed                 # roles, garments, options, fit rules, default content, 3D asset manifest
api bootstrap-owner      # creates the first owner from BOOTSTRAP_OWNER_* (then remove those variables)
api serve
```

Never run `seed-dev` in production; it refuses to run when `APP_ENV=production`.

With `AUTO_MIGRATE=true` the API migrates on start. For zero-downtime releases with several
instances, set it to false and run `api migrate` as a release step before rolling out.

## 4. After the first deploy

1. Sign in as the owner, add staff in Owner > Staff, and change the bootstrap password.
2. Owner > Settings: business, address, hours, tax, delivery, appointment
   rules and order stages.
3. Owner > Pages and policies: review every policy (see the legal review note in
   `IMPLEMENTATION_STATUS.md`).
4. Replace sample photography; publish real products, fabrics and portfolio work.
5. Payments: follow `docs/payment-integration.md`, then turn on the payment switches.
6. Check `https://your-site/robots.txt` allows indexing and `sitemap.xml` uses the public address.

## Rollback

- Application: redeploy the previous images.
- Database: `api rollback` reverts the most recent migration (every migration has a down file).
  Prefer restoring from backup if a migration has already changed data.

## Health

- `GET /healthz`: process is up.
- `GET /readyz`: database reachable.
- `GET /metrics`: Prometheus-style request metrics (protect it at the proxy).
