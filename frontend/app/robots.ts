import type { MetadataRoute } from "next";
import { siteUrl } from "@/lib/server-data";

export default function robots(): MetadataRoute.Robots {
  const base = siteUrl();
  const production = process.env.APP_ENV === "production";
  return {
    rules: production
      ? {
          userAgent: "*",
          allow: "/",
          disallow: ["/owner", "/account", "/checkout", "/orders/", "/quotes/", "/requests/", "/support/", "/documents/", "/appointments/", "/api/"],
        }
      : { userAgent: "*", disallow: "/" },
    sitemap: `${base}/sitemap.xml`,
  };
}
