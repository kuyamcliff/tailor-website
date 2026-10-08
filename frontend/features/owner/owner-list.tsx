"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { ListResponse } from "@/lib/types";
import styles from "./tables.module.css";

export type Filter =
  | { key: string; label: string; type: "search" }
  | { key: string; label: string; type: "select"; options: [string, string][] };
export type Column<T> = { label: string; cell: (row: T) => React.ReactNode; className?: string };

type Props<T> = {
  endpoint: string;
  filters?: Filter[];
  columns: Column<T>[];
  href?: (row: T) => string;
  rowKey: (row: T) => string;
  empty?: string;
  pageSize?: number;
  // Some endpoints return a plain array instead of a paged list.
  unpaged?: boolean;
};

// OwnerList renders a filterable, paged table whose filters live in the URL, so a filtered view can
// be bookmarked or linked from the overview.
export function OwnerList<T>({
  endpoint,
  filters = [],
  columns,
  href,
  rowKey,
  empty = "Nothing here yet.",
  pageSize = 50,
  unpaged,
}: Props<T>) {
  const router = useRouter();
  const path = usePathname();
  const sp = useSearchParams();
  const offset = Number(sp.get("offset") ?? 0) || 0;
  const params = new URLSearchParams();
  for (const f of filters) {
    const v = sp.get(f.key);
    if (v) params.set(f.key, v);
  }
  for (const [k, v] of sp.entries()) if (!params.has(k) && k !== "offset") params.set(k, v);
  if (!unpaged) {
    params.set("limit", String(pageSize));
    params.set("offset", String(offset));
  }
  const qs = params.toString();
  const q = useQuery({
    queryKey: ["owner-list", endpoint, qs],
    queryFn: async () => {
      const r = await api<ListResponse<T> | T[]>(`${endpoint}${qs ? `?${qs}` : ""}`);
      return Array.isArray(r) ? { items: r, total: r.length, limit: r.length, offset: 0 } : r;
    },
    placeholderData: (prev) => prev,
  });

  const setParam = (k: string, v: string) => {
    const next = new URLSearchParams(sp.toString());
    if (v) next.set(k, v);
    else next.delete(k);
    next.delete("offset");
    router.replace(`${path}${next.size ? `?${next}` : ""}`, { scroll: false });
  };

  const items = q.data?.items ?? [];
  const total = q.data?.total ?? 0;
  return (
    <div className="stack">
      {filters.length ? (
        <div className={styles.filters} role="search">
          {filters.map((f) =>
            f.type === "search" ? (
              <SearchBox
                key={f.key}
                label={f.label}
                initial={sp.get(f.key) ?? ""}
                onSearch={(v) => setParam(f.key, v)}
              />
            ) : (
              <label key={f.key} className="field" style={{ gap: 4 }}>
                <span className="label">{f.label}</span>
                <select
                  className="select"
                  value={sp.get(f.key) ?? ""}
                  onChange={(e) => setParam(f.key, e.target.value)}
                >
                  <option value="">All</option>
                  {f.options.map(([v, l]) => (
                    <option key={v} value={v}>
                      {l}
                    </option>
                  ))}
                </select>
              </label>
            ),
          )}
        </div>
      ) : null}
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 280 }} />
      ) : items.length ? (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                {columns.map((c) => (
                  <th key={c.label} className={c.className}>
                    {c.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {items.map((row) => (
                <tr key={rowKey(row)}>
                  {columns.map((c, i) => (
                    <td key={c.label} className={c.className}>
                      {i === 0 && href ? (
                        <Link className="link" href={href(row)}>
                          {c.cell(row)}
                        </Link>
                      ) : (
                        c.cell(row)
                      )}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="muted">{empty}</p>
      )}
      {!unpaged && total > pageSize ? (
        <div className={styles.pager}>
          <span className="small muted">
            {offset + 1} to {Math.min(offset + pageSize, total)} of {total}
          </span>
          <div className="row">
            <button
              className="btn btn-sm"
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - pageSize))}
            >
              Previous
            </button>
            <button
              className="btn btn-sm"
              disabled={offset + pageSize >= total}
              onClick={() => setOffset(offset + pageSize)}
            >
              Next
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );

  function setOffset(n: number) {
    const next = new URLSearchParams(sp.toString());
    next.set("offset", String(n));
    router.replace(`${path}?${next}`, { scroll: false });
  }
}

function SearchBox({ label, initial, onSearch }: { label: string; initial: string; onSearch: (v: string) => void }) {
  const [v, setV] = useState(initial);
  useEffect(() => {
    const t = setTimeout(() => {
      if (v !== initial) onSearch(v.trim());
    }, 350);
    return () => clearTimeout(t);
  }, [v, initial, onSearch]);
  return (
    <label className="field" style={{ gap: 4 }}>
      <span className="label">{label}</span>
      <input className="input" type="search" value={v} onChange={(e) => setV(e.target.value)} />
    </label>
  );
}
