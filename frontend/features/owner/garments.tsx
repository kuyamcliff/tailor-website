"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { humanize } from "@/lib/format";
import { keyFrom } from "@/lib/keys";
import { exponentOf, toMinor } from "@/lib/money";
import type { FitRule } from "@/lib/fit";
import type { GarmentType, MeasurementField, OptionGroup, OptionValue } from "@/lib/types";
import { useConfig } from "@/components/providers/config";
import { useToast } from "@/components/providers/toast";
import { Price } from "@/components/ui/price";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

export function OwnerGarments() {
  const q = useQuery({ queryKey: ["owner", "garments"], queryFn: () => api<GarmentType[]>("/owner/garments") });
  const [adding, setAdding] = useState(false);
  return (
    <>
      <PageHead
        title="Garments and fit"
        sub="What customers can order, the options they can choose and how fit is estimated."
        actions={
          adding ? null : (
            <button className="btn btn-sm" onClick={() => setAdding(true)}>
              Add a garment
            </button>
          )
        }
      />
      {adding ? <NewGarment existing={q.data ?? []} onCancel={() => setAdding(false)} /> : null}
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
  measurements: MeasurementField[];
  allMeasurementFields: MeasurementField[];
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
      <TypeForm key={`type:${JSON.stringify(d.garment)}`} g={d.garment} />
      <section className="stack" aria-labelledby="opt-h">
        <h2 id="opt-h" className={styles.h2}>
          Options
        </h2>
        {d.groups.map((g) => (
          <GroupEditor key={JSON.stringify(g)} garmentKey={d.garment.key} group={g} />
        ))}
        <NewGroup garmentKey={d.garment.key} groups={d.groups} />
      </section>
      <MeasurementFields
        key={`fields:${JSON.stringify(d.measurements)}:${d.allMeasurementFields.length}`}
        garmentKey={d.garment.key}
        selected={d.measurements}
        all={d.allMeasurementFields}
      />
      <FitRules
        key={`fit:${JSON.stringify(d.fitRules)}`}
        garmentKey={d.garment.key}
        rules={d.fitRules}
        fields={d.measurements}
      />
      <Sizes key={`sizes:${JSON.stringify(d.sizes)}`} garmentKey={d.garment.key} sizes={d.sizes} />
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
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Category</span>
          <input className="input" value={x.category} onChange={(e) => setX({ ...x, category: e.target.value })} />
        </label>
        <label className="field">
          <span className="label">3D figure</span>
          <select
            className="select"
            value={x.bodyModelHint}
            onChange={(e) => setX({ ...x, bodyModelHint: e.target.value as GarmentType["bodyModelHint"] })}
          >
            <option value="any">Either</option>
            <option value="masculine">Masculine</option>
            <option value="feminine">Feminine</option>
          </select>
        </label>
      </div>
      <div className="row-wrap">
        <label className="check">
          <input type="checkbox" checked={x.quoteOnly} onChange={(e) => setX({ ...x, quoteOnly: e.target.checked })} />
          <span>Always quoted (no fixed price)</span>
        </label>
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
  const [rows, setRows] = useState(
    group.values.map((v) => ({
      ...v,
      price: String(v.priceMinor / 10 ** exp),
      show: (v.assetParts.show ?? []).join(", "),
      hide: (v.assetParts.hide ?? []).join(", "),
    })),
  );
  const [newName, setNewName] = useState("");
  if (group.selection === "number") return <NumberGroup garmentKey={garmentKey} group={group} />;
  const list = (t: string) =>
    t
      .split(",")
      .map((p) => p.trim())
      .filter(Boolean);
  const save = (v: OptionValue & { price: string; show: string; hide: string }) =>
    run(
      () =>
        api(`/owner/option-groups/${group.id}/values`, {
          body: {
            key: v.key,
            name: v.name,
            description: v.description,
            priceMinor: toMinor(v.price || "0", cfg.business.currency) ?? -1,
            assetParts: { ...v.assetParts, show: list(v.show), hide: list(v.hide) },
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
              <th>3D parts shown</th>
              <th>3D parts hidden</th>
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
                      className="input"
                      aria-label={`${v.name} 3D parts shown`}
                      value={v.show}
                      placeholder="lapel_peak"
                      onChange={(e) => set({ show: e.target.value })}
                      style={{ minWidth: 150 }}
                    />
                  </td>
                  <td>
                    <input
                      className="input"
                      aria-label={`${v.name} 3D parts hidden`}
                      value={v.hide}
                      onChange={(e) => set({ hide: e.target.value })}
                      style={{ minWidth: 150 }}
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
      <form
        className="row-wrap"
        style={{ marginBottom: 12 }}
        onSubmit={(e) => {
          e.preventDefault();
          const name = newName.trim();
          if (!name) return;
          void run(
            () =>
              api(`/owner/option-groups/${group.id}/values`, {
                body: {
                  key: keyFrom(name, "_"),
                  name,
                  description: "",
                  priceMinor: 0,
                  assetParts: {},
                  adjustments: {},
                  isDefault: rows.length === 0,
                  sortOrder: (rows.at(-1)?.sortOrder ?? 0) + 10,
                  active: true,
                },
              }),
            `${name} added.`,
          ).then(() => setNewName(""));
        }}
      >
        <input
          className="input"
          aria-label={`New ${group.name} choice`}
          placeholder="New choice"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          style={{ maxWidth: 260 }}
        />
        <button className="btn btn-sm" disabled={busy || !newName.trim()}>
          Add choice
        </button>
      </form>
      <p className="tiny muted" style={{ marginTop: 0 }}>
        3D parts are node names in the garment model, separated by commas. Choices that no longer apply can be unticked;
        saved designs keep them.
      </p>
    </details>
  );
}

function NumberGroup({ garmentKey, group }: { garmentKey: string; group: OptionGroup }) {
  const { busy, run } = useSaver(garmentKey);
  const [x, setX] = useState({
    min: String(group.minValue ?? ""),
    max: String(group.maxValue ?? ""),
    step: String(group.stepValue ?? ""),
    def: String(group.defaultNumber ?? ""),
    unit: group.unit ?? "",
  });
  const n = (v: string) => (v.trim() === "" ? null : Number(v));
  return (
    <details className="panel" style={{ padding: "4px 16px" }}>
      <summary style={{ cursor: "pointer", padding: "10px 0" }}>
        <strong>{group.name}</strong>{" "}
        <span className="small muted">
          · {humanize(group.section)} · {group.minValue} to {group.maxValue} {group.unit}
        </span>
      </summary>
      <div className="row-wrap" style={{ marginBottom: 12, alignItems: "end" }}>
        {(
          [
            ["min", "Minimum"],
            ["max", "Maximum"],
            ["step", "Step"],
            ["def", "Default"],
            ["unit", "Unit"],
          ] as const
        ).map(([k, label]) => (
          <label key={k} className="field" style={{ width: 110 }}>
            <span className="small">{label}</span>
            <input
              className="input tabular"
              inputMode={k === "unit" ? "text" : "decimal"}
              value={x[k]}
              onChange={(e) => setX({ ...x, [k]: e.target.value })}
            />
          </label>
        ))}
        <button
          className="btn btn-sm"
          disabled={busy}
          onClick={() =>
            run(
              () =>
                api(`/owner/garments/${garmentKey}/groups`, {
                  body: {
                    key: group.key,
                    name: group.name,
                    section: group.section,
                    selection: "number",
                    required: group.required,
                    minValue: n(x.min),
                    maxValue: n(x.max),
                    stepValue: n(x.step),
                    defaultNumber: n(x.def),
                    unit: x.unit || null,
                    sortOrder: group.sortOrder,
                    active: group.active,
                  },
                }),
              `${group.name} saved.`,
            )
          }
        >
          Save
        </button>
      </div>
    </details>
  );
}

function NewGroup({ garmentKey, groups }: { garmentKey: string; groups: OptionGroup[] }) {
  const { busy, run } = useSaver(garmentKey);
  const sections = [...new Set(groups.map((g) => g.section))];
  const [open, setOpen] = useState(false);
  const [x, setX] = useState({
    name: "",
    section: sections[0] ?? "general",
    selection: "single" as "single" | "number",
    required: true,
    min: "",
    max: "",
    step: "1",
    def: "",
    unit: "cm",
  });
  if (!open)
    return (
      <button className="btn btn-sm" style={{ justifySelf: "start" }} onClick={() => setOpen(true)}>
        Add an option
      </button>
    );
  const n = (v: string) => (v.trim() === "" ? null : Number(v));
  return (
    <form
      className="panel panel-pad stack-sm"
      aria-label="New option"
      onSubmit={(e) => {
        e.preventDefault();
        void run(
          () =>
            api(`/owner/garments/${garmentKey}/groups`, {
              body: {
                key: keyFrom(x.name, "_"),
                name: x.name.trim(),
                section: keyFrom(x.section || "general", "_"),
                selection: x.selection,
                required: x.required,
                minValue: x.selection === "number" ? n(x.min) : null,
                maxValue: x.selection === "number" ? n(x.max) : null,
                stepValue: x.selection === "number" ? n(x.step) : null,
                defaultNumber: x.selection === "number" ? n(x.def) : null,
                unit: x.selection === "number" ? x.unit || null : null,
                sortOrder: (groups.at(-1)?.sortOrder ?? 0) + 10,
                active: true,
              },
            }),
          `${x.name.trim()} added. Add its choices below.`,
        ).then(() => setOpen(false));
      }}
    >
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Name</span>
          <input
            className="input"
            required
            value={x.name}
            placeholder="Collar, Embroidery, Sleeve length"
            onChange={(e) => setX({ ...x, name: e.target.value })}
          />
        </label>
        <label className="field">
          <span className="label">Section in the studio</span>
          <input
            className="input"
            list="option-sections"
            value={x.section}
            onChange={(e) => setX({ ...x, section: e.target.value })}
          />
          <datalist id="option-sections">
            {sections.map((sec) => (
              <option key={sec} value={sec} />
            ))}
          </datalist>
        </label>
        <label className="field">
          <span className="label">Customers choose</span>
          <select
            className="select"
            value={x.selection}
            onChange={(e) => setX({ ...x, selection: e.target.value as "single" | "number" })}
          >
            <option value="single">One of several choices</option>
            <option value="number">A number in a range</option>
          </select>
        </label>
        <label className="check" style={{ alignSelf: "end" }}>
          <input type="checkbox" checked={x.required} onChange={(e) => setX({ ...x, required: e.target.checked })} />
          <span>Required</span>
        </label>
      </div>
      {x.selection === "number" ? (
        <div className="row-wrap">
          {(
            [
              ["min", "Minimum"],
              ["max", "Maximum"],
              ["step", "Step"],
              ["def", "Default"],
              ["unit", "Unit"],
            ] as const
          ).map(([k, label]) => (
            <label key={k} className="field" style={{ width: 110 }}>
              <span className="small">{label}</span>
              <input
                className="input tabular"
                inputMode={k === "unit" ? "text" : "decimal"}
                value={x[k]}
                onChange={(e) => setX({ ...x, [k]: e.target.value })}
              />
            </label>
          ))}
        </div>
      ) : null}
      <div className="row-wrap">
        <button className="btn btn-primary btn-sm" disabled={busy || !x.name.trim()}>
          Add option
        </button>
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => setOpen(false)}>
          Cancel
        </button>
      </div>
    </form>
  );
}

function NewGarment({ existing, onCancel }: { existing: GarmentType[]; onCancel: () => void }) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const categories = [...new Set(existing.map((g) => g.category))];
  const [x, setX] = useState({
    name: "",
    category: "",
    description: "",
    bodyModelHint: "any" as GarmentType["bodyModelHint"],
  });
  const key = keyFrom(x.name, "-");
  return (
    <form
      className="panel panel-pad stack-sm"
      aria-label="New garment"
      onSubmit={async (e) => {
        e.preventDefault();
        setBusy(true);
        setErrors({});
        try {
          await api("/owner/garments", {
            body: {
              key,
              name: x.name.trim(),
              category: x.category.trim() || "other",
              description: x.description.trim(),
              basePriceMinor: 0,
              studioEnabled: false,
              bodyModelHint: x.bodyModelHint,
              quoteOnly: true,
              sortOrder: (existing.at(-1)?.sortOrder ?? 0) + 10,
              active: false,
            },
          });
          await qc.invalidateQueries({ queryKey: ["owner", "garments"] });
          toast("Garment added. Set its options and measurements, then show it to customers.");
          router.push(`/owner/garments/${key}`);
        } catch (err) {
          if (err instanceof ApiError) setErrors(err.fields);
          toast(err instanceof ApiError ? (Object.values(err.fields)[0] ?? err.message) : "Please try again.", "error");
        } finally {
          setBusy(false);
        }
      }}
    >
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Name</span>
          <input
            className="input"
            required
            value={x.name}
            placeholder="Kaftan, Kabba, School uniform"
            onChange={(e) => setX({ ...x, name: e.target.value })}
          />
          {errors.key || errors.name ? <span className="error small">{errors.key ?? errors.name}</span> : null}
          {key && existing.some((g) => g.key === key) ? (
            <span className="error small">A garment with this name already exists.</span>
          ) : null}
        </label>
        <label className="field">
          <span className="label">Category</span>
          <input
            className="input"
            list="garment-categories"
            value={x.category}
            onChange={(e) => setX({ ...x, category: e.target.value })}
          />
          <datalist id="garment-categories">
            {categories.map((c) => (
              <option key={c} value={c} />
            ))}
          </datalist>
        </label>
      </div>
      <label className="field">
        <span className="label">Description</span>
        <input className="input" value={x.description} onChange={(e) => setX({ ...x, description: e.target.value })} />
      </label>
      <label className="field" style={{ maxWidth: 260 }}>
        <span className="label">3D figure</span>
        <select
          className="select"
          value={x.bodyModelHint}
          onChange={(e) => setX({ ...x, bodyModelHint: e.target.value as GarmentType["bodyModelHint"] })}
        >
          <option value="any">Either</option>
          <option value="masculine">Masculine</option>
          <option value="feminine">Feminine</option>
        </select>
      </label>
      <p className="tiny muted" style={{ margin: 0 }}>
        New garments start hidden and quoted. Customers can request them once you show them; the 3D studio needs a model
        uploaded in 3D assets first.
      </p>
      <div className="row-wrap">
        <button className="btn btn-primary btn-sm" disabled={busy || !key || existing.some((g) => g.key === key)}>
          Add garment
        </button>
        <button type="button" className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}

function MeasurementFields({
  garmentKey,
  selected,
  all,
}: {
  garmentKey: string;
  selected: MeasurementField[];
  all: MeasurementField[];
}) {
  const { busy, run } = useSaver(garmentKey);
  const [rows, setRows] = useState(
    all.map((f) => {
      const s = selected.find((x) => x.key === f.key);
      return {
        key: f.key,
        label: f.label,
        used: Boolean(s),
        required: s?.required ?? false,
        order: s?.sortOrder ?? 999,
      };
    }),
  );
  const [adding, setAdding] = useState(false);
  const ordered = [...rows].sort((a, b) => Number(b.used) - Number(a.used) || a.order - b.order);
  return (
    <section className="stack-sm" aria-labelledby="mf-h">
      <h2 id="mf-h" className={styles.h2}>
        Measurements
      </h2>
      <p className="small muted" style={{ margin: 0 }}>
        The measurements customers are asked for with this garment. Required ones must be filled before the fit estimate
        shows; the rest are optional.
      </p>
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Measurement</th>
              <th>Asked for</th>
              <th>Required</th>
            </tr>
          </thead>
          <tbody>
            {ordered.map((r) => {
              const set = (patch: Partial<typeof r>) =>
                setRows((x) => x.map((y) => (y.key === r.key ? { ...y, ...patch } : y)));
              return (
                <tr key={r.key}>
                  <td>{r.label}</td>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={`Ask for ${r.label}`}
                      checked={r.used}
                      onChange={(e) => set({ used: e.target.checked, required: e.target.checked && r.required })}
                    />
                  </td>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={`${r.label} required`}
                      checked={r.required}
                      disabled={!r.used}
                      onChange={(e) => set({ required: e.target.checked })}
                    />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <div className="row-wrap">
        <button
          className="btn btn-sm"
          disabled={busy}
          onClick={() =>
            run(
              () =>
                api(`/owner/garments/${garmentKey}/measurement-fields`, {
                  method: "PUT",
                  body: ordered
                    .filter((r) => r.used)
                    .map((r, i) => ({ key: r.key, required: r.required, sortOrder: (i + 1) * 10 })),
                }),
              "Measurements saved.",
            )
          }
        >
          Save measurements
        </button>
        {adding ? null : (
          <button className="btn btn-ghost btn-sm" onClick={() => setAdding(true)}>
            New measurement
          </button>
        )}
      </div>
      {adding ? <NewMeasurementField garmentKey={garmentKey} onDone={() => setAdding(false)} /> : null}
    </section>
  );
}

function NewMeasurementField({ garmentKey, onDone }: { garmentKey: string; onDone: () => void }) {
  const { busy, run } = useSaver(garmentKey);
  const [x, setX] = useState({
    label: "",
    bodyLocation: "",
    instruction: "",
    kind: "length",
    min: "",
    max: "",
  });
  const mm = (cm: string) => Math.round(Number(cm.replace(",", ".")) * 10);
  return (
    <form
      className="panel panel-pad stack-sm"
      aria-label="New measurement"
      onSubmit={(e) => {
        e.preventDefault();
        void run(
          () =>
            api("/owner/measurement-fields", {
              body: {
                key: keyFrom(x.label, "_"),
                label: x.label.trim(),
                bodyLocation: x.bodyLocation.trim(),
                instruction: x.instruction.trim(),
                helperNote: "",
                diagramKey: null,
                kind: x.kind,
                minMm: mm(x.min),
                maxMm: mm(x.max),
                sortOrder: 500,
                active: true,
              },
            }),
          `${x.label.trim()} added. Tick it above to ask for it.`,
        ).then(onDone);
      }}
    >
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Name</span>
          <input
            className="input"
            required
            value={x.label}
            placeholder="Agbada length"
            onChange={(e) => setX({ ...x, label: e.target.value })}
          />
        </label>
        <label className="field">
          <span className="label">Kind</span>
          <select className="select" value={x.kind} onChange={(e) => setX({ ...x, kind: e.target.value })}>
            <option value="length">Length</option>
            <option value="circumference">Around the body</option>
            <option value="width">Width</option>
            <option value="height">Height</option>
          </select>
        </label>
        <label className="field">
          <span className="label">Where on the body</span>
          <input
            className="input"
            value={x.bodyLocation}
            placeholder="From the shoulder to the hem"
            onChange={(e) => setX({ ...x, bodyLocation: e.target.value })}
          />
        </label>
        <div className="row-wrap">
          <label className="field" style={{ width: 120 }}>
            <span className="label">Smallest (cm)</span>
            <input
              className="input tabular"
              inputMode="decimal"
              required
              value={x.min}
              onChange={(e) => setX({ ...x, min: e.target.value })}
            />
          </label>
          <label className="field" style={{ width: 120 }}>
            <span className="label">Largest (cm)</span>
            <input
              className="input tabular"
              inputMode="decimal"
              required
              value={x.max}
              onChange={(e) => setX({ ...x, max: e.target.value })}
            />
          </label>
        </div>
      </div>
      <label className="field">
        <span className="label">How to measure</span>
        <textarea
          className="textarea"
          rows={2}
          value={x.instruction}
          onChange={(e) => setX({ ...x, instruction: e.target.value })}
        />
      </label>
      <div className="row-wrap">
        <button className="btn btn-primary btn-sm" disabled={busy || !x.label.trim()}>
          Add measurement
        </button>
        <button type="button" className="btn btn-ghost btn-sm" onClick={onDone}>
          Cancel
        </button>
      </div>
    </form>
  );
}

function FitRules({
  garmentKey,
  rules: initial,
  fields,
}: {
  garmentKey: string;
  rules: FitRule[];
  fields: MeasurementField[];
}) {
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
              <th>
                <span className="visually-hidden">Remove</span>
              </th>
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
                <td>
                  <button
                    className="btn btn-ghost btn-sm"
                    aria-label={`Remove ${r.label}`}
                    onClick={() => setRules((x) => x.filter((_, j) => j !== i))}
                  >
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <NewFitRule
        fields={fields.filter((f) => !rules.some((r) => r.measurementKey === f.key))}
        onAdd={(rule) => setRules((x) => [...x, rule])}
      />
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

function NewFitRule({ fields, onAdd }: { fields: MeasurementField[]; onAdd: (r: FitRule) => void }) {
  const [key, setKey] = useState("");
  if (!fields.length) return null;
  return (
    <div className="row-wrap">
      <label className="visually-hidden" htmlFor="new-fit-rule">
        Measurement for a new fit rule
      </label>
      <select
        id="new-fit-rule"
        className="select"
        value={key}
        onChange={(e) => setKey(e.target.value)}
        style={{ maxWidth: 260 }}
      >
        <option value="">Add a fit area for...</option>
        {fields.map((f) => (
          <option key={f.key} value={f.key}>
            {f.label}
          </option>
        ))}
      </select>
      <button
        className="btn btn-ghost btn-sm"
        disabled={!key}
        onClick={() => {
          const f = fields.find((x) => x.key === key);
          if (!f) return;
          const around = f.kind === "circumference";
          onAdd({
            zone: f.key,
            label: f.label,
            measurementKey: f.key,
            kind: around ? "circumference" : "length",
            easeSlimMm: around ? 40 : 0,
            easeRegularMm: around ? 70 : 0,
            easeRelaxedMm: around ? 100 : 0,
            toleranceMm: around ? 15 : 10,
            stretchPct: 0,
          });
          setKey("");
        }}
      >
        Add
      </button>
    </div>
  );
}
