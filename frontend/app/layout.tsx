import type { Metadata, Viewport } from "next";
import "@fontsource/cormorant-garamond/500.css";
import "@fontsource/cormorant-garamond/600.css";
import "@fontsource/cormorant-garamond/500-italic.css";
import "@fontsource-variable/manrope";
import "@/styles/globals.css";
import { AppProviders } from "@/components/providers/app-providers";
import { Header } from "@/components/layout/header";
import { Footer } from "@/components/layout/footer";
import { SiteOnly } from "@/components/layout/site-only";
import { CartDrawer } from "@/components/layout/cart-drawer";
import { WebVitals } from "@/components/layout/web-vitals";
import { businessName, getConfig, getContent, siteUrl } from "@/lib/server-data";
import { block } from "@/lib/content";

export async function generateMetadata(): Promise<Metadata> {
  const [cfg, content] = await Promise.all([getConfig(), getContent()]);
  const name = businessName(cfg);
  const hero = block(content, "home.hero");
  const description = `${hero.subtitle} Bespoke suits, shirts, dresses, gowns, traditional wear and alterations${cfg.business.address.city ? ` in ${cfg.business.address.city}` : ""}.`;
  return {
    metadataBase: new URL(siteUrl()),
    title: {
      default: `${name} | Bespoke tailoring${cfg.business.address.city ? ` in ${cfg.business.address.city}` : ""}`,
      template: `%s | ${name}`,
    },
    description,
    applicationName: name,
    alternates: { canonical: "/" },
    openGraph: { type: "website", siteName: name, title: name, description, url: "/" },
    twitter: { card: "summary_large_image", title: name, description },
    formatDetection: { telephone: false },
  };
}

export const viewport: Viewport = {
  themeColor: "#0b0b0c",
  colorScheme: "dark",
  width: "device-width",
  initialScale: 1,
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const [config, content] = await Promise.all([getConfig(), getContent()]);
  const b = config.business;
  const name = businessName(config);
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": ["ClothingStore", "LocalBusiness"],
    name,
    url: siteUrl(),
    ...(b.logoUrl ? { logo: new URL(b.logoUrl, siteUrl()).toString() } : {}),
    ...(b.phone ? { telephone: b.phone } : {}),
    ...(b.email ? { email: b.email } : {}),
    ...(b.address.line1
      ? {
          address: {
            "@type": "PostalAddress",
            streetAddress: [b.address.line1, b.address.line2].filter(Boolean).join(", "),
            addressLocality: b.address.city,
            addressRegion: b.address.region || undefined,
            postalCode: b.address.postalCode || undefined,
            addressCountry: b.address.country,
          },
        }
      : {}),
    ...(b.social?.length ? { sameAs: b.social.map((s) => s.url) } : {}),
    currenciesAccepted: b.currency,
  };
  return (
    <html lang="en">
      <body>
        <a href="#main" className="skip-link">
          Skip to content
        </a>
        <AppProviders config={config} content={content}>
          <div style={{ display: "flex", flexDirection: "column", minHeight: "100vh" }}>
            <SiteOnly>
              <Header />
            </SiteOnly>
            <main id="main" style={{ flex: 1 }}>
              {children}
            </main>
            <SiteOnly>
              <Footer config={config} content={content} />
            </SiteOnly>
          </div>
          <SiteOnly>
            <CartDrawer />
          </SiteOnly>
          <WebVitals />
        </AppProviders>
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }}
        />
      </body>
    </html>
  );
}
