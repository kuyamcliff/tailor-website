"use client";

import Image from "next/image";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Sheet } from "@/components/ui/sheet";
import { api } from "@/lib/api";
import type { Product } from "@/lib/types";
import { PurchasePanel } from "./purchase-panel";

export function QuickView({ slug, onClose }: { slug: string; onClose: () => void }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["product", slug],
    queryFn: () => api<{ product: Product }>(`/products/${slug}`),
  });
  const p = data?.product;
  return (
    <Sheet open onClose={onClose} title={p?.name ?? "Quick view"} side="center" size="lg">
      {isLoading ? (
        <div className="skeleton" style={{ height: 360 }} aria-label="Loading" />
      ) : error || !p ? (
        <p className="notice notice-danger">This piece could not be loaded. Please try again.</p>
      ) : (
        <div style={{ display: "grid", gap: 24, gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))" }}>
          <div style={{ position: "relative", aspectRatio: "4 / 5", background: "var(--surface)" }}>
            {p.media[0] ? <Image src={p.media[0].url} alt={p.media[0].alt} fill sizes="400px" style={{ objectFit: "cover" }} /> : null}
          </div>
          <div className="stack">
            <p className="muted small">{p.summary}</p>
            <PurchasePanel product={p} compact />
            <Link href={`/shop/${p.slug}`} className="text-link" onClick={onClose}>
              Full details
            </Link>
          </div>
        </div>
      )}
    </Sheet>
  );
}
