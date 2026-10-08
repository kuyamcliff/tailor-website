# Performance budgets

Most customers will browse on mid-range Android phones over mobile data. Budgets are set for that.

## Targets (75th percentile, mobile, field data)

| Metric | Budget |
|---|---|
| Largest Contentful Paint | under 2.5 s |
| Interaction to Next Paint | under 200 ms |
| Cumulative Layout Shift | under 0.1 |
| Time to first byte (HTML) | under 800 ms |

The site reports these through `web-vitals` to `NEXT_PUBLIC_TELEMETRY_URL` when it is set
(anonymous: metric name, value and path only).

## Transfer budgets

| Resource | Budget | Measured (8 October 2026, production build) |
|---|---|---|
| Initial JavaScript per page (gzip) | 230 KB | 208 KB on home, shop and the dashboard; 214 KB on the studio |
| three.js and the studio viewer (gzip, loaded only on /studio after the panel is interactive) | 300 KB | 268 KB |
| Studio body + garment models, detailed (Meshopt) | 3 MB | 0.24 + 0.43 MB (suit stand-in; 0.8 + 1.4 MB before compression) |
| Studio body + garment models, light (Medium and Light levels) | 1.5 MB | 0.12 + 0.25 MB |
| Still renders for the no-WebGL fallback (WebP, per angle) | 60 KB | 6 to 21 KB |
| Fabric texture set (colour, normal, roughness and swatch, WebP, 1024 px) | 700 KB | about 650 KB |
| Hero image (WebP via next/image, mobile width) | 150 KB | served responsive by next/image |

Fonts are self-hosted (Cormorant Garamond and Manrope variable) with `unicode-range` subsets, so
browsers download only the Latin files a page uses; no third-party font or script requests are made.

## Rules that keep it fast

- three.js is imported only by `features/studio/viewer.tsx`, through `next/dynamic` with `ssr:
  false`. Never import it from shared components.
- The studio picks one of four quality levels from the device and adjusts it from measured frame
  times; it renders on demand rather than every frame (see `docs/3d-assets.md`). Customers can
  choose a level under Preview > Detail.
- Product and portfolio images go through `next/image` with explicit `sizes`.
- Public API responses used by server components are cached briefly (`revalidate` 30 to 60 s).
- Below-the-fold reveal animations only apply to elements that start off screen and are disabled
  with `prefers-reduced-motion`.

## Checking

```sh
cd frontend
npm run build
# per page initial JS: open the built page and sum the gzip size of the referenced chunks,
# or run Lighthouse against a production deployment (mobile, slow 4G):
npx lighthouse https://your-site/ --form-factor=mobile --throttling-method=simulate
```

Lighthouse and load tests against a production deployment are still to do (see
`IMPLEMENTATION_STATUS.md`).
