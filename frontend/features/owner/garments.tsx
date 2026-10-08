"use client";

import Link from "next/link";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { humanize } from "@/lib/format";
import { exponentOf, toMinor } from "@/lib/money";
import type { FitRule } from "@/lib/fit";
import type { GarmentType, OptionGroup, OptionValue } from "@/lib/types";
import { useConfig } from "@/components/providers/config";
import { useToast } from "@/components/providers/toast";
import { Price } from "@/components/ui/price";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

export function OwnerGarments() {
  const q = useQuery({ queryKey: ["owner", "garments"], queryFn: () => api<GarmentType[]>("/owner/garments") });
  return (
    <>
      <PageHead
        title="Garments and fit"
        sub="What customers can order, the options they can choose and how fit is estimated."
      />
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 320 }} />
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Garment</th>
                <th>Starting price</th>
                <th>In the studio</th>
                <th>Shown</th>
              </tr>
            </thead>
            <tbody>
              {(q.data ?? []).map((g) => (
                <tr key={g.key}>
                  <td>
                    <Link className="link" href={`/owner/garments/${g.key}`}>
                      {g.name}
                    </Link>
                  </td>
                  <td className="tabular">{g.basePriceMinor ? <Price minor={g.basePriceMinor} /> : "Quoted"}</td>
                  <td>{g.studioEnabled ? "Yes" : "Request only"}</td>
                  <td>{g.active ? "Yes" : "Hidden"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

type Detail = {
  garment: GarmentType;
  groups: OptionGroup[];
  fitRules: FitRule[];
  sizes: { id: string; label: string; dims: Record<string, number>; sortOrder: number }[];
};

export function OwnerGarment({ garmentKey }: { garmentKey: string }) {
  const q = useQuery({
    queryKey: ["owner", "garment", garmentKey],
    queryFn: () => api<Detail>(`/owner/garments/${garmentKey}`),
  });
  if (q.isLoading) return <div className="skeleton" style={{ height: 480 }} />;
  if (!q.data) return <p className="notice notice-danger">This garment could not be loaded.</p>;
  const d = q.data;
  return (
    <>
      <PageHead
        title={d.garment.name}
        actions={
          d.garment.studioEnabled ? (
            <Link className="btn btn-sm" href={`/studio?garment=${d.garment.key}`} target="_blank">
              Open in studio
            </Link>
          ) : null
        }
      />
      <TypeForm key={JSON.stringify(d.garment)} g={d.garment} />
      <section className="stack" aria-labelledby="opt-h">
        <h2 id="opt-h" className={styles.h2}>
          Options
        </h2>
        {d.groups.map((g) => (
          <GroupEditor key={g.id} garmentKey={d.garment.key} group={g} />
        ))}
      </section>
      <FitRules key={JSON.stringify(d.fitRules)} garmentKey={d.garment.key} rules={d.fitRules} />
      <Sizes key={JSON.stringify(d.sizes)} garmentKey={d.garment.key} sizes={d.sizes} />
    </>
  );
}

function useSaver(garmentKey: string) {
  const qc = useQueryClient();
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  async function run(fn: () => Promise<unknown>, done: string) {
    setBusy(true);
    try {
      await fn();
      toast(done);
      await qc.invalidateQueries({ queryKey: ["owner", "garment", garmentKey] });
      await qc.invalidateQueries({ queryKey: ["owner", "garments"] });
    } catch (e) {
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }
  return { busy, run };
}

function TypeForm({ g }: { g: GarmentType }) {
  const cfg = useConfig();
  const exp = exponentOf(cfg.business.currency);
  const { busy, run } = useSaver(g.key);
  const [x, setX] = useState({ ...g, price: String(g.basePriceMinor / 10 ** exp) });
  return (
    <section className="panel panel-pad stack-sm" aria-label="Garment settings">
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Name</span>
          <input className="input" value={x.name} onChange={(e) => setX({ ...x, name: e.target.value })} />
        </label>
        <label className="field">
          <span className="label">Starting price ({cfg.business.currency})</span>
          <input
            className="input tabular"
            inputMode="decimal"
            value={x.price}
            onChange={(e) => setX({ ...x, price: e.target.value })}
          />
        </label>
      </div>
      <label className="field">
        <span className="label">Description</span>
        <input className="input" value={x.description} onChange={(e) => setX({ ...x, description: e.target.value })} />
      </label>
      <div className="row-wrap">
        <label className="check">
          <input
            type="checkbox"
            checked={x.studioEnabled}
            onChange={(e) => setX({ ...x, studioEnabled: e.target.checked })}
          />
          <span>Available in the 3D studio</span>
        </label>
        <label className="check">
          <input type="checkbox" checked={x.active} onChange={(e) => setX({ ...x, active: e.target.checked })} />
          <span>Shown to customers</span>
        </label>
      </div>
      <button
        className="btn btn-primary btn-sm"
        style={{ justifySelf: "start" }}
        disabled={busy}
        onClick={() =>
          run(
            () =>
              api("/owner/garments", {
                body: {
                  key: g.key,
                  name: x.name,
                  category: x.category,
                  description: x.description,
                  basePriceMinor: toMinor(x.price, cfg.business.currency) ?? -1,
                  studioEnabled: x.studioEnabled,
                  bodyModelHint: x.bodyModelHint,
                  quoteOnly: x.quoteOnly,
                  sortOrder: x.sortOrder,
                  active: x.active,
                },
              }),
            "Garment saved.",
          )
        }
      >
        Save garment
      </button>
    </section>
  );
}

function GroupEditor({ garmentKey, group }: { garmentKey: string; group: OptionGroup }) {
  const cfg = useConfig();
  const exp = exponentOf(cfg.business.currency);
  const { busy, run } = useSaver(garmentKey);
  const [rows, setRows] = useState(group.values.map((v) => ({ ...v, price: String(v.priceMinor / 10 ** exp) })));
  if (group.selection === "number")
    return (
      <p className="small" style={{ margin: 0 }}>
        <strong>{group.name}</strong>: {group.minValue} to {group.maxValue} {group.unit}, default {group.defaultNumber}
      </p>
    );
  const save = (v: OptionValue & { price: string }) =>
    run(
      () =>
        api(`/owner/option-groups/${group.id}/values`, {
          body: {
            key: v.key,
            name: v.name,
            description: v.description,
            priceMinor: toMinor(v.price || "0", cfg.business.currency) ?? -1,
            assetParts: v.assetParts,
            adjustments: v.adjustments,
            isDefault: v.isDefault,
            sortOrder: v.sortOrder,
            active: v.active,
          },
        }),
      `${v.name} saved.`,
    );
  return (
    <details className="panel" style={{ padding: "4px 16px" }}>
      <summary style={{ cursor: "pointer", padding: "10px 0" }}>
        <strong>{group.name}</strong>{" "}
        <span className="small muted">
          · {humanize(group.section)} · {group.values.length} choices
        </span>
      </summary>
      <div className="table-wrap" style={{ marginBottom: 12 }}>
        <table className="table">
          <thead>
            <tr>
              <th>Choice</th>
              <th>Description</th>
              <th>Adds ({cfg.business.currency})</th>
              <th>Default</th>
              <th>Offered</th>
              <th>
                <span className="visually-hidden">Save</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((v, i) => {
              const set = (patch: Partial<typeof v>) =>
                setRows((r) =>
                  r.map((y, j) => (j === i ? { ...y, ...patch } : patch.isDefault ? { ...y, isDefault: false } : y)),
                );
              return (
                <tr key={v.id}>
                  <td>
                    <input
                      className="input"
                      aria-label={`${group.name} choice ${i + 1} name`}
                      value={v.name}
                      onChange={(e) => set({ name: e.target.value })}
                      style={{ minWidth: 150 }}
                    />
                  </td>
                  <td>
                    <input
                      className="input"
                      aria-label={`${v.name} description`}
                      value={v.description}
                      onChange={(e) => set({ description: e.target.value })}
                      style={{ minWidth: 220 }}
                    />
                  </td>
                  <td>
                    <input
                      className="input tabular"
                      aria-label={`${v.name} price`}
                      inputMode="decimal"
                      value={v.price}
                      onChange={(e) => set({ price: e.target.value })}
                      style={{ width: 110 }}
                    />
                  </td>
                  <td>
                    <input
                      type="radio"
                      name={`def-${group.id}`}
                      aria-label={`${v.name} is the default`}
                      checked={v.isDefault}
                      onChange={() => set({ isDefault: true })}
                    />
                  </td>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={`${v.name} offered`}
                      checked={v.active}
                      onChange={(e) => set({ active: e.target.checked })}
                    />
                  </td>
                  <td>
                    <button className="btn btn-sm" disabled={busy} onClick={() => save(v)}>
                      Save
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </details>
  );
}

function FitRules({ garmentKey, rules: initial }: { garmentKey: string; rules: FitRule[] }) {
  const { busy, run } = useSaver(garmentKey);
  const [rules, setRules] = useState(initial);
  const num = (i: number, k: keyof FitRule, v: string) =>
    setRules((r) => r.map((x, j) => (j === i ? { ...x, [k]: Number(v) || 0 } : x)));
  return (
    <section className="stack-sm" aria-labelledby="fit-h">
      <h2 id="fit-h" className={styles.h2}>
        Fit rules
      </h2>
      <p className="small muted" style={{ margin: 0 }}>
        Ease is the room added to the body measurement for each fit, in millimetres. Tolerance is how far from the
        target still counts as a good fit.
      </p>
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Area</th>
              <th>Measurement</th>
              <th>Slim ease</th>
              <th>Regular ease</th>
              <th>Relaxed ease</th>
              <th>Tolerance</th>
              <th>Stretch %</th>
            </tr>
          </thead>
          <tbody>
            {rules.map((r, i) => (
              <tr key={r.zone}>
                <td>{r.label}</td>
                <td className="small muted">{humanize(r.measurementKey)}</td>
                {(["easeSlimMm", "easeRegularMm", "easeRelaxedMm", "toleranceMm", "stretchPct"] as const).map((k) => (
                  <td key={k}>
                    <input
                      className="input tabular"
                      aria-label={`${r.label} ${k}`}
                      inputMode="numeric"
                      value={String(r[k])}
                      onChange={(e) => num(i, k, e.target.value)}
                      style={{ width: 80 }}
                    />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <button
        className="btn btn-sm"
        style={{ justifySelf: "start" }}
        disabled={busy}
        onClick={() =>
          run(() => api(`/owner/garments/${garmentKey}/fit-rules`, { method: "PUT", body: rules }), "Fit rules saved.")
        }
      >
        Save fit rules
      </button>
    </section>
  );
}

function Sizes({ garmentKey, sizes: initial }: { garmentKey: string; sizes: Detail["sizes"] }) {
  const { busy, run } = useSaver(garmentKey);
  const [rows, setRows] = useState(
    initial.map((s) => ({
      label: s.label,
      dims: Object.entries(s.dims)
        .map(([k, v]) => `${k}=${v}`)
        .join(", "),
    })),
  );
  const parse = (s: string) =>
    Object.fromEntries(
      s
        .split(",")
        .map((p) => p.split("=").map((x) => x.trim()))
        .filter(([k, v]) => k && v && Number.isFinite(Number(v)))
        .map(([k, v]) => [k, Number(v)]),
    );
  return (
    <section className="stack-sm" aria-labelledby="sizes-h">
      <h2 id="sizes-h" className={styles.h2}>
        Standard sizes
      </h2>
      <p className="small muted" style={{ margin: 0 }}>
        Finished garment measurements in millimetres, for customers who start from a size. Example: chest=1020,
        waist=920.
      </p>
      {rows.map((r, i) => (
        <div key={i} className="row-wrap">
          <input
            className="input"
            aria-label={`Size ${i + 1} label`}
            value={r.label}
            onChange={(e) => setRows((x) => x.map((y, j) => (j === i ? { ...y, label: e.target.value } : y)))}
            style={{ width: 100 }}
          />
          <input
            className="input tabular"
            aria-label={`Size ${i + 1} measurements`}
            value={r.dims}
            onChange={(e) => setRows((x) => x.map((y, j) => (j === i ? { ...y, dims: e.target.value } : y)))}
            style={{ flex: 1, minWidth: 260 }}
          />
          <button className="btn btn-ghost btn-sm" onClick={() => setRows((x) => x.filter((_, j) => j !== i))}>
            Remove
          </button>
        </div>
      ))}
      <div className="row-wrap">
        <button className="btn btn-sm" onClick={() => setRows((x) => [...x, { label: "", dims: "" }])}>
          Add size
        </button>
        <button
          className="btn btn-sm"
          disabled={busy}
          onClick={() =>
            run(
              () =>
                api(`/owner/garments/${garmentKey}/sizes`, {
                  method: "PUT",
                  body: rows.map((r, i) => ({ label: r.label, dims: parse(r.dims), sortOrder: i })),
                }),
              "Sizes saved.",
            )
          }
        >
          Save sizes
        </button>
      </div>
    </section>
  );
}
