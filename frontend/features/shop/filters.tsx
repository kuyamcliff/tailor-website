"use client";

import { usePathname, useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import { SlidersHorizontal } from "lucide-react";
import type { Facets } from "@/lib/types";
import { Sheet } from "@/components/ui/sheet";
import { useConfig } from "@/components/providers/config";
import { exponentOf, toMinor } from "@/lib/money";
import styles from "./filters.module.css";

type Current = Record<"q" | "category" | "size" | "color" | "fabric" | "availability" | "minPrice" | "maxPrice" | "sort", string>;

export function ShopFilters({ facets, current }: { facets: Facets; current: Current }) {
  const router = useRouter();
  const pathname = usePathname();
  const [pending, start] = useTransition();
  const [open, setOpen] = useState(false);
  const cfg = useConfig();
  const exp = exponentOf(cfg.business.currency);
  const [q, setQ] = useState(current.q);
  const [minP, setMinP] = useState(current.minPrice ? String(Number(current.minPrice) / 10 ** exp) : "");
  const [maxP, setMaxP] = useState(current.maxPrice ? String(Number(current.maxPrice) / 10 ** exp) : "");

  function apply(patch: Partial<Current>) {
    const next = { ...current, ...patch };
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(next)) if (v) params.set(k, v);
    start(() => router.push(`${pathname}${params.size ? `?${params}` : ""}`, { scroll: false }));
  }

  const active = (Object.keys(current) as (keyof Current)[]).filter((k) => current[k] && k !== "sort");

  const panel = (
    <div className={styles.panel} aria-busy={pending}>
      <form
        role="search"
        onSubmit={(e) => {
          e.preventDefault();
          apply({ q: q.trim() });
          setOpen(false);
        }}
      >
        <label htmlFor="shop-q" className="label">
          Search
        </label>
        <div className="input-group" style={{ marginTop: 6 }}>
          <input id="shop-q" className="input" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Linen, navy..." />
          <button className="btn btn-sm" type="submit" style={{ minHeight: 48 }}>
            Go
          </button>
        </div>
      </form>

      <div>
        <label htmlFor="shop-sort" className="label">
          Sort
        </label>
        <select id="shop-sort" className="select" value={current.sort} onChange={(e) => apply({ sort: e.target.value })} style={{ marginTop: 6 }}>
          <option value="">Featured</option>
          <option value="newest">Newest</option>
          <option value="price_asc">Price, low to high</option>
          <option value="price_desc">Price, high to low</option>
          <option value="name">Name</option>
        </select>
      </div>

      {facets.categories.length ? (
        <fieldset className={styles.group}>
          <legend className="label">Category</legend>
          <div className={styles.chips}>
            <button type="button" className={styles.chip} aria-pressed={!current.category} onClick={() => apply({ category: "" })}>
              All
            </button>
            {facets.categories.map((c) => (
              <button key={c.slug} type="button" className={styles.chip} aria-pressed={current.category === c.slug} onClick={() => apply({ category: c.slug === current.category ? "" : c.slug })}>
                {c.name}
              </button>
            ))}
          </div>
        </fieldset>
      ) : null}

      {facets.sizes.length ? (
        <fieldset className={styles.group}>
          <legend className="label">Size</legend>
          <div className={styles.chips}>
            {facets.sizes.map((s) => (
              <button key={s} type="button" className={styles.chip} aria-pressed={current.size === s} onClick={() => apply({ size: current.size === s ? "" : s })}>
                {s}
              </button>
            ))}
          </div>
        </fieldset>
      ) : null}

      {facets.colors.length ? (
        <fieldset className={styles.group}>
          <legend className="label">Colour</legend>
          <div className={styles.chips}>
            {facets.colors.map((c) => (
              <button key={c} type="button" className={styles.chip} aria-pressed={current.color === c} onClick={() => apply({ color: current.color === c ? "" : c })}>
                {c}
              </button>
            ))}
          </div>
        </fieldset>
      ) : null}

      {facets.fabrics.length ? (
        <fieldset className={styles.group}>
          <legend className="label">Fabric</legend>
          <div className={styles.chips}>
            {facets.fabrics.map((f) => (
              <button key={f.key} type="button" className={styles.chip} aria-pressed={current.fabric === f.key} onClick={() => apply({ fabric: current.fabric === f.key ? "" : f.key })}>
                {f.name}
              </button>
            ))}
          </div>
        </fieldset>
      ) : null}

      <fieldset className={styles.group}>
        <legend className="label">Availability</legend>
        <div className={styles.chips}>
          {[
            ["", "Any"],
            ["in_stock", "In stock"],
            ["made_to_order", "Made to order"],
          ].map(([v, l]) => (
            <button key={v} type="button" className={styles.chip} aria-pressed={current.availability === v} onClick={() => apply({ availability: v })}>
              {l}
            </button>
          ))}
        </div>
      </fieldset>

      <form
        className={styles.group}
        onSubmit={(e) => {
          e.preventDefault();
          const min = minP ? toMinor(minP, cfg.business.currency) : null;
          const max = maxP ? toMinor(maxP, cfg.business.currency) : null;
          apply({ minPrice: min !== null ? String(min) : "", maxPrice: max !== null ? String(max) : "" });
          setOpen(false);
        }}
      >
        <span className="label">Price ({cfg.business.currency})</span>
        <div className={styles.price}>
          <input className="input" inputMode="decimal" aria-label="Minimum price" placeholder="Min" value={minP} onChange={(e) => setMinP(e.target.value)} />
          <span className="muted">to</span>
          <input className="input" inputMode="decimal" aria-label="Maximum price" placeholder="Max" value={maxP} onChange={(e) => setMaxP(e.target.value)} />
        </div>
        <button className="btn btn-sm" type="submit">
          Apply price
        </button>
      </form>

      {active.length ? (
        <button
          className="btn btn-sm btn-ghost"
          onClick={() => {
            setQ("");
            setMinP("");
            setMaxP("");
            start(() => router.push(pathname, { scroll: false }));
          }}
        >
          Clear all filters
        </button>
      ) : null}
    </div>
  );

  return (
    <>
      <aside className={styles.desktop} aria-label="Filters">
        {panel}
      </aside>
      <div className={styles.mobileBar}>
        <button className="btn btn-sm" onClick={() => setOpen(true)}>
          <SlidersHorizontal size={16} aria-hidden /> Filter and sort{active.length ? ` (${active.length})` : ""}
        </button>
        {pending ? <span className="spinner" aria-label="Updating" /> : null}
      </div>
      <Sheet open={open} onClose={() => setOpen(false)} title="Filter and sort" side="bottom" footer={<button className="btn btn-primary btn-block" onClick={() => setOpen(false)}>Show results</button>}>
        {panel}
      </Sheet>
    </>
  );
}
