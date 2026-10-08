import type { Metadata } from "next";
import Link from "next/link";
import { serverApiOr } from "@/lib/server";
import type { Facets, ListResponse, Product } from "@/lib/types";
import { ProductCard } from "@/features/shop/product-card";
import { ShopFilters } from "@/features/shop/filters";
import styles from "./shop.module.css";

export const metadata: Metadata = {
  title: "Shop",
  description: "Ready-to-wear and made-to-order suits, shirts, trousers and dresses, finished by our tailors.",
  alternates: { canonical: "/shop" },
};

const PAGE = 24;
const keys = ["q", "category", "size", "color", "fabric", "availability", "minPrice", "maxPrice", "sort"] as const;

type Search = Partial<Record<(typeof keys)[number] | "page", string | string[]>>;

function one(v: string | string[] | undefined) {
  return Array.isArray(v) ? v[0] : v;
}

export default async function ShopPage({ searchParams }: { searchParams: Promise<Search> }) {
  const sp = await searchParams;
  const params = new URLSearchParams();
  for (const k of keys) {
    const v = one(sp[k]);
    if (v) params.set(k, v);
  }
  const page = Math.max(1, Number(one(sp.page)) || 1);
  params.set("limit", String(PAGE));
  params.set("offset", String((page - 1) * PAGE));
  const [list, facets] = await Promise.all([
    serverApiOr<ListResponse<Product>>(`/products?${params}`, { items: [], total: 0, limit: PAGE, offset: 0 }, 30),
    serverApiOr<Facets>("/products/facets", { categories: [], sizes: [], colors: [], fabrics: [], price: { min: null, max: null } }, 60),
  ]);
  const pages = Math.ceil(list.total / PAGE);
  const current = Object.fromEntries(keys.map((k) => [k, one(sp[k]) ?? ""])) as Record<(typeof keys)[number], string>;
  const pageHref = (p: number) => {
    const q = new URLSearchParams();
    for (const k of keys) if (current[k]) q.set(k, current[k]);
    if (p > 1) q.set("page", String(p));
    const s = q.toString();
    return `/shop${s ? `?${s}` : ""}`;
  };
  const category = facets.categories.find((c) => c.slug === current.category);

  return (
    <div className="container section-tight">
      <nav aria-label="Breadcrumb" className="tiny muted">
        <Link href="/">Home</Link> / <span aria-current="page">Shop</span>
      </nav>
      <header className={styles.head}>
        <div>
          <h1 className="display-2">{category ? category.name : current.q ? `Results for “${current.q}”` : "Shop"}</h1>
          <p className="muted" style={{ marginTop: 8 }}>
            {list.total} {list.total === 1 ? "piece" : "pieces"}. Every garment can be adjusted at a fitting.
          </p>
        </div>
        <Link href="/custom-tailor" className="text-link">
          Prefer something made for you?
        </Link>
      </header>
      <div className={styles.layout}>
        <ShopFilters facets={facets} current={current} />
        <section aria-label="Products" aria-live="polite">
          {list.items.length === 0 ? (
            <div className="empty">
              <p className="display-3">No pieces match these filters.</p>
              <p className="muted">Try removing a filter, or tell us what you are looking for and we can make it.</p>
              <div className="row-wrap">
                <Link href="/shop" className="btn btn-sm">
                  Clear filters
                </Link>
                <Link href="/custom-tailor/request" className="btn btn-sm btn-primary">
                  Request a custom piece
                </Link>
              </div>
            </div>
          ) : (
            <div className={styles.grid}>
              {list.items.map((p, i) => (
                <ProductCard key={p.id} product={p} priority={i < 3} />
              ))}
            </div>
          )}
          {pages > 1 ? (
            <nav className={styles.pager} aria-label="Pagination">
              {page > 1 ? (
                <Link href={pageHref(page - 1)} className="btn btn-sm" rel="prev">
                  Previous
                </Link>
              ) : null}
              <span className="small muted">
                Page {page} of {pages}
              </span>
              {page < pages ? (
                <Link href={pageHref(page + 1)} className="btn btn-sm" rel="next">
                  Next
                </Link>
              ) : null}
            </nav>
          ) : null}
        </section>
      </div>
    </div>
  );
}
