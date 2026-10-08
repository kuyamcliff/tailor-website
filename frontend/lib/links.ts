"use client";

// Guests reach their requests, quotes, orders and bookings through private links with an access
// token. We remember those links on this device so a guest can come back without searching email.

export type SavedLink = { kind: "order" | "request" | "quote" | "appointment" | "support"; id: string; number: string; token: string; createdAt: number };

const KEY = "atelier.links";

export function rememberLink(link: Omit<SavedLink, "createdAt">) {
  try {
    const all = savedLinks().filter((l) => !(l.kind === link.kind && l.id === link.id));
    all.unshift({ ...link, createdAt: Date.now() });
    localStorage.setItem(KEY, JSON.stringify(all.slice(0, 50)));
  } catch {
    // storage unavailable (private mode); the link is still in the URL and the confirmation message
  }
}

export function savedLinks(): SavedLink[] {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? "[]") as SavedLink[];
  } catch {
    return [];
  }
}

export function tokenFor(kind: SavedLink["kind"], id: string): string {
  return savedLinks().find((l) => l.kind === kind && l.id === id)?.token ?? "";
}

// accessTokenFromUrl reads ?token= and remembers it, then removes it from the address bar so it is
// not shared accidentally in screenshots or copied links.
export function accessTokenFromUrl(kind: SavedLink["kind"], id: string): string {
  if (typeof window === "undefined") return "";
  const url = new URL(window.location.href);
  const t = url.searchParams.get("token");
  if (t) {
    rememberLink({ kind, id, number: "", token: t });
    url.searchParams.delete("token");
    window.history.replaceState(null, "", url.toString());
    return t;
  }
  return tokenFor(kind, id);
}
