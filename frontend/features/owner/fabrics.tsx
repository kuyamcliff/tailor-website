"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { humanize } from "@/lib/format";
import { exponentOf, toMinor } from "@/lib/money";
import type { Fabric, GarmentType } from "@/lib/types";
import { useConfig } from "@/components/providers/config";
import { useToast } from "@/components/providers/toast";
import { Field } from "@/components/ui/field";
import { Price } from "@/components/ui/price";
import { FabricSwatch } from "@/components/ui/fabric-swatch";
import { ImageUploader, type UploadItem } from "@/components/ui/image-uploader";
import { PageHead } from "./owner-shell";

const stock: [string, string][] = [
  ["available", "Available"],
  ["low_stock", "Low stock"],
  ["out_of_stock", "Out of stock"],
  ["custom_order", "Ordered in on request"],
  ["discontinued", "Discontinued"],
];

export function OwnerFabrics() {
  const q = useQuery({ queryKey: ["owner", "fabrics"], queryFn: () => api<Fabric[]>("/owner/fabrics") });
  const [open, setOpen] = useState<string | null>(null);
  return (
    <>
      <PageHead
        title="Fabrics"
        sub="Cloth shown in the studio, on products and in requests."
        actions={
          <button className="btn btn-primary btn-sm" onClick={() => setOpen("new")}>
            New fabric
          </button>
        }
      />
      {open === "new" ? <FabricForm fabric={null} onDone={() => setOpen(null)} /> : null}
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 320 }} />
      ) : (
        <ul className="list-rows">
          {(q.data ?? []).map((f) => (
            <li key={f.key}>
              <button
                className="list-row"
                style={{
                  width: "100%",
                  background: "none",
                  border: 0,
                  color: "inherit",
                  font: "inherit",
                  cursor: "pointer",
                  textAlign: "left",
                }}
                onClick={() => setOpen(open === f.key ? null : f.key)}
                aria-expanded={open === f.key}
              >
                <span className="row">
                  <span style={{ position: "relative", width: 40, height: 40, flex: "none" }}>
                    <FabricSwatch fabric={f} sizes="40px" round={false} />
                  </span>
                  <span className="stack-xs">
                    <strong>{f.name}</strong>
                    <span className="small muted">
                      {f.composition} · {f.colors.length} {f.colors.length === 1 ? "colour" : "colours"}
                      {f.priceImpactMinor ? (
                        <>
                          {" "}
                          · + <Price minor={f.priceImpactMinor} />
                        </>
                      ) : null}
                    </span>
                  </span>
                </span>
                <span
                  className="small"
                  style={{ color: f.stockStatus === "available" ? "var(--text-muted)" : "var(--warning)" }}
                >
                  {f.active ? humanize(f.stockStatus) : "Not shown"}
                </span>
              </button>
              {open === f.key ? <FabricForm fabric={f} onDone={() => setOpen(null)} /> : null}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

function FabricForm({ fabric: f, onDone }: { fabric: Fabric | null; onDone: () => void }) {
  const cfg = useConfig();
  const qc = useQueryClient();
  const toast = useToast();
  const exp = exponentOf(cfg.business.currency);
  const garments = useQuery({ queryKey: ["garments"], queryFn: () => api<GarmentType[]>("/garments") });
  const [x, setX] = useState({
    key: f?.key ?? "",
    name: f?.name ?? "",
    materialType: f?.materialType ?? "",
    composition: f?.composition ?? "",
    weightGsm: f?.weightGsm ? String(f.weightGsm) : "",
    textureDescription: f?.textureDescription ?? "",
    season: f?.season ?? "",
    careInstructions: f?.careInstructions ?? "",
    price: f ? String(f.priceImpactMinor / 10 ** exp) : "0",
    stockStatus: f?.stockStatus ?? "available",
    stockMeters: f?.stockMeters != null ? String(f.stockMeters) : "",
    lowStockMeters: f?.lowStockMeters != null ? String(f.lowStockMeters) : "",
    suitable: f?.suitableGarments ?? [],
    active: f?.active ?? true,
    pbr: JSON.stringify(f?.pbr ?? {}, null, 2),
  });
  const [colors, setColors] = useState(
    f?.colors.map((c) => ({ key: c.key, name: c.name, hex: c.hex })) ?? [{ key: "", name: "", hex: "#333333" }],
  );
  const [swatch, setSwatch] = useState<UploadItem[]>([]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const set = (k: keyof typeof x, v: unknown) => setX((s) => ({ ...s, [k]: v }));

  async function save() {
    let pbr: unknown;
    try {
      pbr = JSON.parse(x.pbr || "{}");
    } catch {
      setErrors({ pbr: "The material settings are not valid JSON." });
      return;
    }
    setBusy(true);
    setErrors({});
    const up = swatch.find((s) => s.status === "done")?.result;
    try {
      await api("/owner/fabrics", {
        body: {
          key: x.key,
          name: x.name,
          materialType: x.materialType,
          composition: x.composition,
          weightGsm: x.weightGsm ? Number(x.weightGsm) : null,
          textureDescription: x.textureDescription,
          season: x.season,
          careInstructions: x.careInstructions,
          priceImpactMinor: toMinor(x.price || "0", cfg.business.currency) ?? -1,
          stockStatus: x.stockStatus,
          stockMeters: x.stockMeters ? Number(x.stockMeters) : null,
          lowStockMeters: x.lowStockMeters ? Number(x.lowStockMeters) : null,
          swatchUrl: up?.url ?? f?.swatchUrl ?? null,
          pbr,
          suitableGarments: x.suitable,
          sortOrder: f?.sortOrder ?? 0,
          active: x.active,
          colors,
        },
      });
      toast("Fabric saved.");
      await qc.invalidateQueries({ queryKey: ["owner", "fabrics"] });
      await qc.invalidateQueries({ queryKey: ["fabrics"] });
      onDone();
    } catch (e) {
      if (e instanceof ApiError) setErrors(e.fields);
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="panel panel-pad stack" style={{ margin: "8px 0 16px" }}>
      <div className="form-grid cols-2">
        <Field label="Name" error={errors.name}>
          {(a) => <input {...a} className="input" value={x.name} onChange={(e) => set("name", e.target.value)} />}
        </Field>
        <Field
          label="Code"
          error={errors.key}
          hint={f ? "The code cannot be changed." : "Lowercase letters, numbers and hyphens."}
        >
          {(a) => (
            <input
              {...a}
              className="input"
              value={x.key}
              disabled={Boolean(f)}
              onChange={(e) => set("key", e.target.value)}
            />
          )}
        </Field>
        <Field label="Material">
          {(a) => (
            <input
              {...a}
              className="input"
              value={x.materialType}
              onChange={(e) => set("materialType", e.target.value)}
            />
          )}
        </Field>
        <Field label="Composition">
          {(a) => (
            <input
              {...a}
              className="input"
              value={x.composition}
              onChange={(e) => set("composition", e.target.value)}
            />
          )}
        </Field>
        <Field label="Weight (g/m²)">
          {(a) => (
            <input
              {...a}
              className="input tabular"
              inputMode="numeric"
              value={x.weightGsm}
              onChange={(e) => set("weightGsm", e.target.value)}
            />
          )}
        </Field>
        <Field label="Season">
          {(a) => <input {...a} className="input" value={x.season} onChange={(e) => set("season", e.target.value)} />}
        </Field>
        <Field label={`Price added (${cfg.business.currency})`} error={errors.priceImpactMinor}>
          {(a) => (
            <input
              {...a}
              className="input tabular"
              inputMode="decimal"
              value={x.price}
              onChange={(e) => set("price", e.target.value)}
            />
          )}
        </Field>
        <Field label="Stock">
          {(a) => (
            <select
              {...a}
              className="select"
              value={x.stockStatus}
              onChange={(e) => set("stockStatus", e.target.value)}
            >
              {stock.map(([k, l]) => (
                <option key={k} value={k}>
                  {l}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Metres in stock (optional)">
          {(a) => (
            <input
              {...a}
              className="input tabular"
              inputMode="decimal"
              value={x.stockMeters}
              onChange={(e) => set("stockMeters", e.target.value)}
            />
          )}
        </Field>
        <Field label="Warn below (metres)">
          {(a) => (
            <input
              {...a}
              className="input tabular"
              inputMode="decimal"
              value={x.lowStockMeters}
              onChange={(e) => set("lowStockMeters", e.target.value)}
            />
          )}
        </Field>
      </div>
      <Field label="Feel and look">
        {(a) => (
          <textarea
            {...a}
            className="textarea"
            rows={2}
            value={x.textureDescription}
            onChange={(e) => set("textureDescription", e.target.value)}
          />
        )}
      </Field>
      <Field label="Care">
        {(a) => (
          <textarea
            {...a}
            className="textarea"
            rows={2}
            value={x.careInstructions}
            onChange={(e) => set("careInstructions", e.target.value)}
          />
        )}
      </Field>
      <fieldset style={{ border: 0, padding: 0, margin: 0 }}>
        <legend className="label">Suitable for (none ticked means all)</legend>
        <div className="row-wrap">
          {(garments.data ?? []).map((g) => (
            <label key={g.key} className="check">
              <input
                type="checkbox"
                checked={x.suitable.includes(g.key)}
                onChange={(e) =>
                  set("suitable", e.target.checked ? [...x.suitable, g.key] : x.suitable.filter((k) => k !== g.key))
                }
              />
              <span>{g.name}</span>
            </label>
          ))}
        </div>
      </fieldset>
      <fieldset style={{ border: 0, padding: 0, margin: 0 }} className="stack-sm">
        <legend className="label">Colours</legend>
        {colors.map((c, i) => (
          <div key={i} className="row-wrap">
            <input
              type="color"
              aria-label={`Colour ${i + 1}`}
              value={c.hex}
              onChange={(e) => setColors((cs) => cs.map((y, j) => (j === i ? { ...y, hex: e.target.value } : y)))}
            />
            <input
              className="input"
              aria-label={`Colour ${i + 1} name`}
              placeholder="Name"
              value={c.name}
              onChange={(e) => setColors((cs) => cs.map((y, j) => (j === i ? { ...y, name: e.target.value } : y)))}
              style={{ width: 200 }}
            />
            <input
              className="input"
              aria-label={`Colour ${i + 1} code`}
              placeholder="code"
              value={c.key}
              onChange={(e) => setColors((cs) => cs.map((y, j) => (j === i ? { ...y, key: e.target.value } : y)))}
              style={{ width: 160 }}
            />
            {colors.length > 1 ? (
              <button
                className="icon-btn"
                aria-label={`Remove colour ${i + 1}`}
                onClick={() => setColors((cs) => cs.filter((_, j) => j !== i))}
              >
                <X size={16} aria-hidden />
              </button>
            ) : null}
          </div>
        ))}
        {errors.colors ? <p className="error small">{errors.colors}</p> : null}
        <button
          className="btn btn-sm"
          style={{ justifySelf: "start" }}
          onClick={() => setColors((cs) => [...cs, { key: "", name: "", hex: "#333333" }])}
        >
          Add colour
        </button>
      </fieldset>
      <div className="stack-sm">
        <span className="label">Swatch photo</span>
        <ImageUploader
          purpose="fabric"
          items={swatch}
          onChange={setSwatch}
          max={1}
          label={f?.swatchUrl ? "Replace the swatch photo" : "Upload a swatch photo"}
        />
      </div>
      <details>
        <summary className="small">Material settings for the 3D studio</summary>
        <p className="tiny muted">
          Texture paths, repeat, roughness and sheen. Change only if you know the texture set.
        </p>
        <textarea
          className="textarea tabular"
          rows={8}
          aria-label="Material settings"
          value={x.pbr}
          onChange={(e) => set("pbr", e.target.value)}
        />
        {errors.pbr ? <p className="error small">{errors.pbr}</p> : null}
      </details>
      <label className="check">
        <input type="checkbox" checked={x.active} onChange={(e) => set("active", e.target.checked)} />
        <span>Show this fabric to customers</span>
      </label>
      <div className="row-wrap">
        <button className="btn btn-primary btn-sm" disabled={busy} onClick={save}>
          Save fabric
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onDone}>
          Cancel
        </button>
      </div>
    </div>
  );
}
