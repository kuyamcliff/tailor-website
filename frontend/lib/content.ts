// Typed access to owner-editable content blocks. Every field has a fallback so pages still render
// meaningful copy if a block is missing or the API is temporarily unavailable.

import type { ContentBlocks } from "./types";

export type Cta = { label: string; href: string };
export type Hero = {
  eyebrow: string;
  title: string;
  subtitle: string;
  primaryCta: Cta;
  secondaryCta: Cta;
  image: string;
  imageAlt?: string;
};
export type Step = { title: string; body: string };
export type Service = { title: string; body: string; href: string };
export type Faq = { q: string; a: string };
export type PolicySection = { heading: string; body: string };

const fallbacks = {
  "home.hero": {
    eyebrow: "Bespoke tailoring",
    title: "Made for your measurements.",
    subtitle: "Suits, shirts, dresses and gowns cut by hand for one person: you.",
    primaryCta: { label: "Create your outfit", href: "/custom-tailor" },
    secondaryCta: { label: "Book a consultation", href: "/appointments" },
    image: "",
  } as Hero,
  "home.signature": {
    title: "Your garment begins with a conversation.",
    body: "Tell us about the occasion and choose the details that matter to you.",
    cta: { label: "Start a custom request", href: "/custom-tailor/request" },
  },
  "home.studio": {
    title: "See the fit from every angle.",
    body: "Build your garment in the fitting studio and check the front, the back and the side.",
    note: "The studio gives an estimate. Your tailor confirms every measurement before cutting.",
    cta: { label: "Open the fitting studio", href: "/studio" },
  },
  "home.process": { title: "How we work", steps: [] as Step[] },
  services: { title: "Services", items: [] as Service[] },
  faqs: { title: "Questions", items: [] as Faq[] },
  about: { title: "About the atelier", body: "", image: "", imageAlt: "" },
  contact: { title: "Visit or contact us", body: "" },
  footer: { note: "" },
  "custom.landing": { title: "Create your outfit", subtitle: "", steps: [] as Step[] },
} as const;

type Fallbacks = typeof fallbacks;
type Widen<T> = T extends string
  ? string
  : T extends readonly (infer U)[]
    ? Widen<U>[]
    : T extends object
      ? { [K in keyof T]: Widen<T[K]> }
      : T;

export function block<K extends keyof Fallbacks>(content: ContentBlocks, key: K): Widen<Fallbacks[K]> {
  const v = content[key];
  const base = fallbacks[key] as Widen<Fallbacks[K]>;
  if (!v || typeof v !== "object") return base;
  return { ...base, ...(v as object) } as Widen<Fallbacks[K]>;
}

export function policy(content: ContentBlocks, key: string): { title: string; sections: PolicySection[] } | null {
  const v = content[`policy.${key}`] as { title?: string; sections?: PolicySection[] } | undefined;
  if (!v?.title) return null;
  return { title: v.title, sections: v.sections ?? [] };
}

export const policyKeys = ["privacy", "terms", "delivery", "alterations"] as const;
