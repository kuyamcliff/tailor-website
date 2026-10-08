import "server-only";
import { cache } from "react";
import { serverApiOr } from "./server";
import type { ContentBlocks, PublicConfig } from "./types";

const emptyConfig: PublicConfig = {
  business: {
    name: "",
    tagline: "",
    logoUrl: "",
    phone: "",
    whatsapp: "",
    email: "",
    address: { line1: "", line2: "", city: "", region: "", country: "", postalCode: "", mapUrl: "" },
    openingHours: [],
    currency: "XAF",
    locale: "fr-CM",
    timezone: "Africa/Douala",
    countryCode: "237",
    social: [],
    delivery: [],
    quoteValidityDays: 14,
    depositPercentBp: 5000,
    taxLabel: "VAT",
    taxRateBp: 0,
    pricesIncludeTax: true,
  },
  flags: {
    studio: true,
    appointments: true,
    customer_accounts: true,
    support_inbox: true,
    guest_checkout: true,
    online_payments: false,
    reference_analysis: false,
    photo_body_estimation: false,
  },
};

export const getConfig = cache(() => serverApiOr<PublicConfig>("/config", emptyConfig, 30));
export const getContent = cache(() => serverApiOr<ContentBlocks>("/content", {}, 30));

export function businessName(cfg: PublicConfig) {
  return cfg.business.name || "Atelier";
}

export function siteUrl() {
  return (process.env.PUBLIC_SITE_URL ?? "http://localhost:3000").replace(/\/$/, "");
}
