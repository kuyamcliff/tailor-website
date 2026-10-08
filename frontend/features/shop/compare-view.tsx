"use client";

import Image from "next/image";
import { useHydrated } from "@/lib/client-hooks";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import type { ListResponse, Product, SavedDesign } from "@/lib/types";
import { useSaved } from "@/stores/saved";
import { Price } from "@/components/ui/price";
import { availabilityLabel } from "./product-card";
import styles from "./compare.module.css";

// Comparison of products (and of saved designs). On narrow screens the table scrolls sideways with
// the attribute column pinned, so every column stays readable.
export function CompareView() {
  const ready = useHydrated();
  const { compare, toggleCompare, clearCompare } = useSaved();
  const productIds = compare.filter((c) => c.type === "product").map((c) => c.id);
  const designIds = compare.filter((c) => c.type === "design").map((c) => c.id);
  const products = useQuery({
    queryKey: ["compare", productIds],
    enabled: ready && productIds.length > 0,
    queryFn: () => api<ListResponse<Product>>(`/products?ids=${productIds.join(",")}&limit=4`),
  });
  const designs = useQuery({
    queryKey: ["compare-designs", designIds],
    enabled: ready && designIds.length > 0,
    queryFn: () => Promise.all(designIds.map((id) => api<SavedDesign>(`/designs/${id}`).catch(() => null))),
  });
  if (!ready) return <div className="skeleton" style={{ height: 240 }} />;
  if (!compare.length)
    return (
      <div className="empty">
        <p className="display-3">Nothing to compare yet.</p>
        <p className="muted">Use the compare button on up to four pieces or saved designs.</p>
        <Link href="/shop" className="btn btn-sm">
          Browse the shop
        </Link>
      </div>
    );
  const items = products.data?.items ?? [];
  const rows: [string, (p: Product) => React.ReactNode][] = [
    ["Price", (p) => <Price minor={p.priceMinor} maxMinor={p.priceMaxMinor} />],
    ["Availability", (p) => availabilityLabel[p.availability]],
    ["Fabric", (p) => (p.fabric ? `${p.fabric.name}${p.fabric.composition ? `, ${p.fabric.composition}` : ""}` : "Not specified")],
    ["Sizes", (p) => p.sizes.join(", ")],
    ["Fitting included", (p) => (p.requiresFitting ? "Yes" : "No")],
    ["Can be customised", (p) => (p.customizable ? "Yes" : "No")],
  ];
  const ds = (designs.data ?? []).filter((d): d is SavedDesign => Boolean(d));
  return (
    <div className="stack-lg">
      {items.length ? (
        <div className={styles.wrap}>
          <table className={styles.table}>
            <caption className="visually-hidden">Product comparison</caption>
            <thead>
              <tr>
                <th scope="col">
                  <span className="visually-hidden">Attribute</span>
                </th>
                {items.map((p) => (
                  <th key={p.id} scope="col">
                    <Link href={`/shop/${p.slug}`} className={styles.head}>
                      <span className={styles.thumb}>{p.media[0] ? <Image src={p.media[0].url} alt="" fill sizes="160px" style={{ objectFit: "cover" }} /> : null}</span>
                      <span className="serif">{p.name}</span>
                    </Link>
                    <button className="btn btn-sm btn-ghost" onClick={() => toggleCompare({ type: "product", id: p.id, name: p.name })}>
                      Remove
                    </button>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map(([label, render]) => (
                <tr key={label}>
                  <th scope="row">{label}</th>
                  {items.map((p) => (
                    <td key={p.id}>{render(p)}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : products.isLoading ? (
        <div className="skeleton" style={{ height: 300 }} />
      ) : null}
      {ds.length ? (
        <div className={styles.wrap}>
          <table className={styles.table}>
            <caption className="visually-hidden">Saved design comparison</caption>
            <thead>
              <tr>
                <th scope="col">
                  <span className="visually-hidden">Attribute</span>
                </th>
                {ds.map((d) => (
                  <th key={d.id} scope="col">
                    <span className="serif">{d.name}</span>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              <tr>
                <th scope="row">Garment</th>
                {ds.map((d) => (
                  <td key={d.id}>{d.snapshot.garment.name}</td>
                ))}
              </tr>
              <tr>
                <th scope="row">Fabric</th>
                {ds.map((d) => (
                  <td key={d.id}>{d.snapshot.fabric ? `${d.snapshot.fabric.name}, ${d.snapshot.fabric.colorName}` : "To be chosen"}</td>
                ))}
              </tr>
              <tr>
                <th scope="row">Details</th>
                {ds.map((d) => (
                  <td key={d.id}>{d.snapshot.selections.map((s) => s.valueName ?? `${s.groupName} ${s.number}${s.unit ?? ""}`).join(", ")}</td>
                ))}
              </tr>
              <tr>
                <th scope="row">Estimate</th>
                {ds.map((d) => (
                  <td key={d.id}>
                    <Price minor={d.snapshot.price.totalMinor} currency={d.snapshot.price.currency} />
                  </td>
                ))}
              </tr>
            </tbody>
          </table>
        </div>
      ) : null}
      <button className="btn btn-sm btn-ghost" onClick={clearCompare}>
        Clear comparison
      </button>
    </div>
  );
}
