import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { serverApi, ServerApiError } from "@/lib/server";
import { getConfig, siteUrl } from "@/lib/server-data";
import type { Product } from "@/lib/types";
import { Gallery } from "@/features/shop/gallery";
import { PurchasePanel } from "@/features/shop/purchase-panel";
import { ProductCard, availabilityLabel } from "@/features/shop/product-card";
import { ProductActions } from "@/features/shop/product-actions";
import { exponentOf } from "@/lib/money";
import styles from "./product.module.css";

type Data = { product: Product; related: Product[] };

async function load(slug: string): Promise<Data | null> {
  try {
    return await serverApi<Data>(`/products/${encodeURIComponent(slug)}`, 30);
  } catch (e) {
    if (e instanceof ServerApiError && e.status === 404) return null;
    throw e;
  }
}

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const { slug } = await params;
  const data = await load(slug);
  if (!data) return { title: "Not found" };
  const p = data.product;
  return {
    title: p.name,
    description: p.summary || p.description?.slice(0, 160),
    alternates: { canonical: `/shop/${p.slug}` },
    openGraph: { title: p.name, description: p.summary, images: p.media[0] ? [{ url: p.media[0].url, alt: p.media[0].alt }] : undefined },
  };
}

export default async function ProductPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const [data, cfg] = await Promise.all([load(slug), getConfig()]);
  if (!data) notFound();
  const { product: p, related } = data;
  const base = siteUrl();
  const jsonLd = [
    {
      "@context": "https://schema.org",
      "@type": "Product",
      name: p.name,
      description: p.description || p.summary,
      image: p.media.map((m) => new URL(m.url, base).toString()),
      sku: p.variants[0]?.sku,
      ...(cfg.business.name ? { brand: { "@type": "Brand", name: cfg.business.name } } : {}),
      offers: p.variants.map((v) => ({
        "@type": "Offer",
        sku: v.sku,
        price: (v.priceMinor / 10 ** exponentOf(cfg.business.currency)).toFixed(exponentOf(cfg.business.currency)),
        priceCurrency: cfg.business.currency,
        availability: v.available ? (v.madeToOrder && !v.lowStock ? "https://schema.org/PreOrder" : "https://schema.org/InStock") : "https://schema.org/OutOfStock",
        url: `${base}/shop/${p.slug}`,
      })),
    },
    {
      "@context": "https://schema.org",
      "@type": "BreadcrumbList",
      itemListElement: [
        { "@type": "ListItem", position: 1, name: "Shop", item: `${base}/shop` },
        ...(p.category ? [{ "@type": "ListItem", position: 2, name: p.category.name, item: `${base}/shop?category=${p.category.slug}` }] : []),
        { "@type": "ListItem", position: p.category ? 3 : 2, name: p.name, item: `${base}/shop/${p.slug}` },
      ],
    },
  ];
  const details: [string, string | undefined][] = [
    ["Fabric", p.fabric ? [p.fabric.name, p.fabric.composition, p.fabric.weightGsm ? `${p.fabric.weightGsm} g/m²` : ""].filter(Boolean).join(", ") : undefined],
    ["Fit", p.fitNotes],
    ["Care", p.care || p.fabric?.careInstructions],
    ["Sizing and measurements", p.measurementInfo],
  ];

  return (
    <div className="container section-tight">
      <nav aria-label="Breadcrumb" className="tiny muted">
        <Link href="/shop">Shop</Link>
        {p.category ? (
          <>
            {" / "}
            <Link href={`/shop?category=${p.category.slug}`}>{p.category.name}</Link>
          </>
        ) : null}
        {" / "}
        <span aria-current="page">{p.name}</span>
      </nav>
      <div className={styles.layout}>
        <Gallery media={p.media} name={p.name} />
        <div className={styles.info}>
          <div className="stack-sm">
            <span className="badge">{availabilityLabel[p.availability]}</span>
            <h1 className="display-2">{p.name}</h1>
            {p.summary ? <p className="lede">{p.summary}</p> : null}
          </div>
          <PurchasePanel product={p} />
          <ProductActions product={p} />
          {p.description ? <p className="muted">{p.description}</p> : null}
          <dl className={styles.details}>
            {details
              .filter(([, v]) => v)
              .map(([k, v]) => (
                <div key={k}>
                  <dt>{k}</dt>
                  <dd>{v}</dd>
                </div>
              ))}
          </dl>
          {p.requiresFitting || p.customizable ? (
            <div className="notice">
              <span>
                {p.requiresFitting ? "This piece is finished at a fitting. " : ""}
                Book a time with a tailor to try it on or discuss changes.{" "}
                <Link className="link" href="/appointments?type=fitting">
                  Book a fitting
                </Link>
              </span>
            </div>
          ) : null}
        </div>
      </div>
      {related.length ? (
        <section className="section-tight" aria-labelledby="related">
          <h2 id="related" className="display-3" style={{ marginBottom: 24 }}>
            You may also like
          </h2>
          <div className={styles.related}>
            {related.map((r) => (
              <ProductCard key={r.id} product={r} />
            ))}
          </div>
        </section>
      ) : null}
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }} />
    </div>
  );
}
