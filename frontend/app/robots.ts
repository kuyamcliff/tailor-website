import type { MetadataRoute } from "next";
import { siteUrl } from "@/lib/server-data";

// Rendered per request so APP_ENV and PUBLIC_SITE_URL come from the running server, not the build machine.
export const dynamic = "force-dynamic";

export default function robots(): MetadataRoute.Robots {
  const base = siteUrl();
  const production = process.env.APP_ENV === "production";
  return {
    rules: production
      ? {
          userAgent: "*",
          allow: "/",
          disallow: [
            "/owner",
            "/account",
            "/checkout",
            "/orders/",
            "/quotes/",
            "/requests/",
            "/support/",
            "/documents/",
            "/appointments/",
            "/api/",
          ],
        }
      : { userAgent: "*", disallow: "/" },
    sitemap: `${base}/sitemap.xml`,
  };
}
