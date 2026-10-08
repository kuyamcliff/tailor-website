"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, uploadAssetFile, type AssetUpload } from "@/lib/api";
import { humanize } from "@/lib/format";
import type { Asset, GarmentType } from "@/lib/types";
import { useToast } from "@/components/providers/toast";
import { StatusBadge } from "@/components/ui/status-badge";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

const next: Record<string, string[]> = {
  uploaded: ["validating", "rejected"],
  validating: ["processed", "rejected"],
  processed: ["preview", "rejected"],
  preview: ["approved", "rejected"],
  approved: ["published", "rejected"],
  published: ["archived"],
  archived: ["published"],
  rejected: [],
};

const actionLabel: Record<string, string> = {
  validating: "Start validation",
  processed: "Mark processed",
  preview: "Ready for preview",
  approved: "Approve",
  published: "Publish",
  archived: "Archive",
  rejected: "Reject",
};

export function OwnerAssets() {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["owner", "assets"], queryFn: () => api<Asset[]>("/owner/assets") });
  const [adding, setAdding] = useState(false);
  const groups = useMemo(() => {
    const m = new Map<string, Asset[]>();
    for (const a of q.data ?? []) m.set(a.assetKey, [...(m.get(a.assetKey) ?? []), a]);
    return [...m.entries()];
  }, [q.data]);
  const standins = (q.data ?? []).filter((a) => a.status === "published" && !a.productionQuality);

  async function move(a: Asset, status: string) {
    if (status === "published" && !a.productionQuality && !confirm("This version is marked as not production quality. Publish it anyway?")) return;
    const note = status === "rejected" ? (prompt("Why is it rejected?") ?? "") : "";
    try {
      await api(`/owner/assets/${a.id}/status`, { body: { status, note } });
      toast(`${a.assetKey} v${a.version}: ${humanize(status)}.`);
      await qc.invalidateQueries({ queryKey: ["owner", "assets"] });
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    }
  }

  return (
    <>
      <PageHead
        title="3D assets"
        sub="Models used by the fitting studio. Saved designs keep the exact version they were made with."
        actions={
          <button className="btn btn-primary btn-sm" onClick={() => setAdding((v) => !v)}>
            Add a version
          </button>
        }
      />
      {standins.length ? (
        <p className="notice notice-warning">
          {standins.length} published {standins.length === 1 ? "model is a development stand-in" : "models are development stand-ins"}. Replace them with professionally made, licensed models before
          relying on the studio for sales.
        </p>
      ) : null}
      {adding ? <NewVersion assets={q.data ?? []} onDone={() => setAdding(false)} /> : null}
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 320 }} />
      ) : (
        groups.map(([key, versions]) => (
          <section key={key} className="stack-sm" aria-label={key}>
            <h2 className={styles.h2}>
              {key} {versions[0]?.garmentTypeKey ? <span className="muted">· {humanize(versions[0].garmentTypeKey)}</span> : null}
            </h2>
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Version</th>
                    <th>Status</th>
                    <th>Quality</th>
                    <th>Files</th>
                    <th>Source and licence</th>
                    <th>
                      <span className="visually-hidden">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {versions.map((a) => (
                    <tr key={a.id}>
                      <td className="tabular">v{a.version}</td>
                      <td>
                        <StatusBadge status={a.status === "published" ? "succeeded" : a.status === "rejected" ? "failed" : "pending"} label={humanize(a.status)} />
                      </td>
                      <td>{a.productionQuality ? "Production" : <span style={{ color: "var(--warning)" }}>Stand-in</span>}</td>
                      <td className="small tabular">{a.files.map((f) => `${f.lod} ${(f.bytes / 1024 / 1024).toFixed(1)} MB`).join(" · ")}</td>
                      <td className="small">
                        {a.license.source ?? ""}
                        {a.license.license ? <span className="muted"> · {a.license.license}</span> : null}
                      </td>
                      <td>
                        <div className="row-wrap">
                          {(next[a.status] ?? []).map((s) => (
                            <button key={s} className={`btn btn-sm ${s === "rejected" || s === "archived" ? "btn-ghost" : ""}`} onClick={() => move(a, s)}>
                              {actionLabel[s] ?? humanize(s)}
                            </button>
                          ))}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        ))
      )}
    </>
  );
}

function NewVersion({ assets, onDone }: { assets: Asset[]; onDone: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const garments = useQuery({ queryKey: ["garments"], queryFn: () => api<GarmentType[]>("/garments") });
  const [key, setKey] = useState("");
  const [kind, setKind] = useState("garment");
  const [garment, setGarment] = useState("");
  const [files, setFiles] = useState<{ lod: string; up: AssetUpload | null; progress: number; error: string }[]>([
    { lod: "high", up: null, progress: 0, error: "" },
    { lod: "low", up: null, progress: 0, error: "" },
  ]);
  const [source, setSource] = useState("");
  const [license, setLicense] = useState("");
  const [production, setProduction] = useState(false);
  const [notes, setNotes] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const prev = assets.find((a) => a.assetKey === key);

  function upload(i: number, file: File | undefined) {
    if (!file) return;
    setFiles((f) => f.map((x, j) => (j === i ? { ...x, up: null, progress: 0, error: "" } : x)));
    uploadAssetFile(file, (p) => setFiles((f) => f.map((x, j) => (j === i ? { ...x, progress: p } : x))))
      .then((up) => setFiles((f) => f.map((x, j) => (j === i ? { ...x, up, progress: 1 } : x))))
      .catch((e) => setFiles((f) => f.map((x, j) => (j === i ? { ...x, error: e instanceof ApiError ? e.message : "Upload failed." } : x))));
  }

  async function create() {
    setBusy(true);
    setErrors({});
    try {
      const ready = files.filter((f) => f.up);
      await api("/owner/assets", {
        body: {
          assetKey: key,
          kind,
          garmentTypeKey: kind === "garment" ? garment || prev?.garmentTypeKey || null : null,
          files: ready.map((f) => ({ ...f.up!.file, lod: f.lod })),
          bodyCompat: prev?.bodyCompat ?? {},
          supportedOptions: { ...(prev?.supportedOptions ?? {}), parts: ready[0]?.up?.info?.nodes ?? [], morphs: ready[0]?.up?.info?.morphTargets ?? [] },
          textureSetVersion: prev?.textureSetVersion ?? "",
          license: { source, license },
          productionQuality: production,
          notes,
        },
      });
      toast("New version added. Review it before publishing.");
      await qc.invalidateQueries({ queryKey: ["owner", "assets"] });
      onDone();
    } catch (e) {
      if (e instanceof ApiError) setErrors(e.fields);
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }

  const keys = [...new Set(assets.map((a) => a.assetKey))];
  return (
    <section className="panel panel-pad stack" aria-label="New asset version">
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Asset</span>
          <input className="input" list="asset-keys" value={key} onChange={(e) => setKey(e.target.value)} placeholder="suit-standin, or a new name" />
          <datalist id="asset-keys">
            {keys.map((k) => (
              <option key={k} value={k} />
            ))}
          </datalist>
          {errors.assetKey ? <span className="error small">{errors.assetKey}</span> : null}
        </label>
        <label className="field">
          <span className="label">Kind</span>
          <select className="select" value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="garment">Garment</option>
            <option value="body">Body</option>
            <option value="texture_pack">Texture pack</option>
          </select>
        </label>
        {kind === "garment" ? (
          <label className="field">
            <span className="label">Garment</span>
            <select className="select" value={garment || prev?.garmentTypeKey || ""} onChange={(e) => setGarment(e.target.value)}>
              <option value="">Choose</option>
              {(garments.data ?? []).map((g) => (
                <option key={g.key} value={g.key}>
                  {g.name}
                </option>
              ))}
            </select>
          </label>
        ) : null}
      </div>
      {files.map((f, i) => (
        <div key={f.lod} className="stack-sm">
          <label className="field">
            <span className="label">{f.lod === "high" ? "Detailed model (GLB)" : "Light model for phones (GLB, optional)"}</span>
            <input type="file" accept=".glb,.ktx2,model/gltf-binary" onChange={(e) => upload(i, e.target.files?.[0])} />
          </label>
          {f.progress > 0 && f.progress < 1 ? <progress value={f.progress} max={1} /> : null}
          {f.error ? <span className="error small">{f.error}</span> : null}
          {f.up ? (
            <span className="small muted">
              {(f.up.file.bytes / 1024 / 1024).toFixed(1)} MB · {f.up.info?.nodes.length ?? 0} parts · {f.up.info?.morphTargets.length ?? 0} shape keys
            </span>
          ) : null}
        </div>
      ))}
      {errors.files ? <span className="error small">{errors.files}</span> : null}
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Where it came from</span>
          <input className="input" value={source} onChange={(e) => setSource(e.target.value)} placeholder="Studio or artist, purchase reference" />
        </label>
        <label className="field">
          <span className="label">Licence</span>
          <input className="input" value={license} onChange={(e) => setLicense(e.target.value)} placeholder="Commercial licence, work for hire" />
        </label>
      </div>
      {errors.license ? <span className="error small">{errors.license}</span> : null}
      <label className="check">
        <input type="checkbox" checked={production} onChange={(e) => setProduction(e.target.checked)} />
        <span>Professionally made and approved for customers (not a stand-in)</span>
      </label>
      <label className="field">
        <span className="label">Notes</span>
        <textarea className="textarea" rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
      </label>
      <div className="row-wrap">
        <button className="btn btn-primary btn-sm" disabled={busy || !files[0]?.up || !key} onClick={create}>
          Add version
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onDone}>
          Cancel
        </button>
      </div>
      {prev ? (
        <p className="tiny muted">
          Body fit and option settings are copied from {prev.assetKey} v{prev.version}. Part names in the new model must match the option settings.
        </p>
      ) : null}
    </section>
  );
}
