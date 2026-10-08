"use client";

import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { formatDate } from "@/lib/format";
import type { SavedDesign } from "@/lib/types";
import { Price } from "@/components/ui/price";
import { useToast } from "@/components/providers/toast";

export function useMyDesigns() {
  return useQuery({ queryKey: ["designs"], queryFn: () => api<SavedDesign[]>("/designs") });
}

export function DesignList({ designs }: { designs: SavedDesign[] }) {
  const qc = useQueryClient();
  const toast = useToast();
  async function remove(d: SavedDesign) {
    if (!confirm(`Delete "${d.name}"?`)) return;
    try {
      await api(`/designs/${d.id}`, { method: "DELETE" });
      await qc.invalidateQueries({ queryKey: ["designs"] });
      toast("Design deleted.");
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    }
  }
  return (
    <ul className="list-rows">
      {designs.map((d) => (
        <li key={d.id} className="list-row" style={{ flexWrap: "wrap" }}>
          <span className="row" style={{ alignItems: "flex-start" }}>
            <span
              aria-hidden
              style={{
                width: 40,
                height: 40,
                flex: "none",
                border: "1px solid var(--line)",
                background: d.snapshot.fabric?.colorHex ?? "var(--surface-2)",
              }}
            />
            <span className="stack-xs">
              <strong>{d.name}</strong>
              <span className="small muted">
                {d.snapshot.garment.name}
                {d.snapshot.fabric ? ` · ${d.snapshot.fabric.name}, ${d.snapshot.fabric.colorName}` : ""} · saved{" "}
                {formatDate(d.updatedAt)}
              </span>
              <span className="small">
                Estimate <Price minor={d.snapshot.price.totalMinor} currency={d.snapshot.price.currency} />
              </span>
              {!d.assetAvailable ? (
                <span className="small" style={{ color: "var(--warning)" }}>
                  The 3D model for this design was updated. Opening it uses the current model.
                </span>
              ) : null}
            </span>
          </span>
          <span className="row-wrap">
            <Link className="btn btn-sm" href={`/studio?design=${d.id}`}>
              Open in studio
            </Link>
            <Link className="btn btn-primary btn-sm" href={`/custom-tailor/request?design=${d.id}`}>
              Request a quote
            </Link>
            <button className="btn btn-ghost btn-sm" onClick={() => remove(d)}>
              Delete
            </button>
          </span>
        </li>
      ))}
    </ul>
  );
}

export function AccountDesigns() {
  const q = useMyDesigns();
  return (
    <>
      <header className="stack-sm">
        <h1 className="display-2">Saved designs</h1>
        <p className="lede">
          Designs you save in the fitting studio. Open one to keep editing or send it to us for a quote.
        </p>
      </header>
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 200 }} />
      ) : q.data?.length ? (
        <DesignList designs={q.data} />
      ) : (
        <div className="empty">
          <p>No saved designs yet.</p>
          <Link className="btn btn-primary" href="/studio">
            Open the fitting studio
          </Link>
        </div>
      )}
    </>
  );
}
