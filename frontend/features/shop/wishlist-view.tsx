"use client";

import Image from "next/image";
import Link from "next/link";
import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { useSaved } from "@/stores/saved";

export function WishlistView() {
  const [ready, setReady] = useState(false);
  const { wishlist, toggleWish } = useSaved();
  useEffect(() => setReady(true), []);
  if (!ready) return <div className="skeleton" style={{ height: 240 }} />;
  if (!wishlist.length)
    return (
      <div className="empty">
        <p className="display-3">Nothing saved yet.</p>
        <p className="muted">Tap the heart on any piece or fabric to keep it here.</p>
        <Link href="/shop" className="btn btn-sm">
          Browse the shop
        </Link>
      </div>
    );
  return (
    <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: 24, gridTemplateColumns: "repeat(auto-fill, minmax(min(100%, 220px), 1fr))" }}>
      {wishlist.map((w) => {
        const href = w.type === "product" ? `/shop/${w.slug}` : w.type === "fabric" ? `/studio?fabric=${w.slug ?? w.id}` : `/studio?design=${w.id}`;
        return (
          <li key={w.id} style={{ position: "relative" }}>
            <Link href={href} className="stack-sm" style={{ display: "grid" }}>
              <span style={{ position: "relative", aspectRatio: "4 / 5", background: "var(--surface)", display: "block" }}>
                {w.image ? <Image src={w.image} alt="" fill sizes="240px" style={{ objectFit: "cover" }} /> : null}
              </span>
              <span className="serif" style={{ fontSize: "1.25rem" }}>
                {w.name}
              </span>
              <span className="tiny muted">{w.type === "product" ? "Ready to wear" : w.type === "fabric" ? "Fabric" : "Saved design"}</span>
            </Link>
            <button className="icon-btn" style={{ position: "absolute", top: 6, right: 6, background: "rgba(11,11,12,.75)" }} aria-label={`Remove ${w.name}`} onClick={() => toggleWish(w)}>
              <X size={16} aria-hidden />
            </button>
          </li>
        );
      })}
    </ul>
  );
}
