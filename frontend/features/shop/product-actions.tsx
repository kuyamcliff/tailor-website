"use client";

import { GitCompareArrows, Heart, Share2 } from "lucide-react";
import { useSaved } from "@/stores/saved";
import { useToast } from "@/components/providers/toast";
import type { Product } from "@/lib/types";

export function ProductActions({ product }: { product: Product }) {
  const wished = useSaved((s) => s.wishlist.some((w) => w.id === product.id));
  const compared = useSaved((s) => s.compare.some((c) => c.id === product.id));
  const { toggleWish, toggleCompare } = useSaved();
  const toast = useToast();
  const item = {
    type: "product" as const,
    id: product.id,
    slug: product.slug,
    name: product.name,
    image: product.media[0]?.url ?? null,
  };
  return (
    <div className="row-wrap">
      <button className="btn btn-sm btn-ghost" aria-pressed={wished} onClick={() => toggleWish(item)}>
        <Heart size={16} aria-hidden fill={wished ? "currentColor" : "none"} /> {wished ? "Saved" : "Save"}
      </button>
      <button
        className="btn btn-sm btn-ghost"
        aria-pressed={compared}
        onClick={() => {
          if (!toggleCompare(item)) toast("You can compare up to four pieces.", "error");
        }}
      >
        <GitCompareArrows size={16} aria-hidden /> {compared ? "In comparison" : "Compare"}
      </button>
      <button
        className="btn btn-sm btn-ghost"
        onClick={async () => {
          const url = window.location.href;
          try {
            if (navigator.share) await navigator.share({ title: product.name, url });
            else {
              await navigator.clipboard.writeText(url);
              toast("Link copied", "info");
            }
          } catch {
            // user cancelled the share sheet
          }
        }}
      >
        <Share2 size={16} aria-hidden /> Share
      </button>
    </div>
  );
}
