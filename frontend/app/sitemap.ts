import type { MetadataRoute } from "next";
import { serverApiOr } from "@/lib/server";
import { siteUrl } from "@/lib/server-data";
import { policyKeys } from "@/lib/content";
import type { ListResponse, PortfolioProject, Product } from "@/lib/types";

// Rendered per request (cached by the CDN) so PUBLIC_SITE_URL comes from the running server.
export const dynamic = "force-dynamic";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const base = siteUrl();
  const [products, work] = await Promise.all([
    serverApiOr<ListResponse<Product>>("/products?limit=500", { items: [], total: 0, limit: 0, offset: 0 }, 3600),
    serverApiOr<PortfolioProject[]>("/portfolio", [], 3600),
  ]);
  const fixed = [
    "",
    "/shop",
    "/custom-tailor",
    "/studio",
    "/our-work",
    "/about",
    "/contact",
    "/appointments",
    "/support",
  ].map((p) => ({
    url: `${base}${p}`,
    changeFrequency: "weekly" as const,
    priority: p === "" ? 1 : 0.7,
  }));
  return [
    ...fixed,
    ...products.items.map((p) => ({ url: `${base}/shop/${p.slug}`, priority: 0.6 })),
    ...(Array.isArray(work) ? work : []).map((w) => ({ url: `${base}/our-work/${w.slug}`, priority: 0.5 })),
    ...policyKeys.map((k) => ({ url: `${base}/policies/${k}`, priority: 0.2 })),
  ];
}
