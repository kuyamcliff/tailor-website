"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { formatDate, humanize } from "@/lib/format";
import type { PortfolioProject } from "@/lib/types";
import { useToast } from "@/components/providers/toast";
import { ImageUploader, uploadsPending, type UploadItem } from "@/components/ui/image-uploader";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

const categories = [
  "suits",
  "shirts",
  "trousers",
  "dresses",
  "gowns",
  "traditional",
  "wedding",
  "alterations",
  "other",
];
const permission: [string, string][] = [
  ["not_required", "No customer shown"],
  ["granted", "Customer agreed in writing"],
  ["pending", "Waiting for the customer's answer"],
  ["denied", "Customer said no"],
];

export function OwnerPortfolio() {
  const q = useQuery({ queryKey: ["owner", "portfolio"], queryFn: () => api<PortfolioProject[]>("/owner/portfolio") });
  const [open, setOpen] = useState<string | null>(null);
  return (
    <>
      <PageHead
        title="Portfolio"
        sub="Garments the atelier has made. Only publish photos of your own work, and only with the customer's permission when they can be recognised."
        actions={
          <button className="btn btn-primary btn-sm" onClick={() => setOpen("new")}>
            New project
          </button>
        }
      />
      {open === "new" ? <ProjectForm project={null} onDone={() => setOpen(null)} /> : null}
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 280 }} />
      ) : (
        <ul className="list-rows">
          {(q.data ?? []).map((p) => (
            <li key={p.id}>
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
                aria-expanded={open === p.id}
                onClick={() => setOpen(open === p.id ? null : p.id)}
              >
                <span className="row">
                  {p.media[0] ? (
                    // eslint-disable-next-line @next/next/no-img-element -- owner thumbnail
                    <img
                      src={p.media[0].url}
                      alt=""
                      width={48}
                      height={60}
                      style={{ objectFit: "cover", border: "1px solid var(--line)" }}
                    />
                  ) : null}
                  <span className="stack-xs">
                    <strong>{p.title}</strong>
                    <span className="small muted">
                      {humanize(p.category)} · {p.media.length} photos
                      {p.media.some((m) => m.sample) ? " · sample photos" : ""}
                    </span>
                  </span>
                </span>
                <span className="small muted">{humanize(p.status ?? "draft")}</span>
              </button>
              {open === p.id ? <ProjectForm project={p} onDone={() => setOpen(null)} /> : null}
            </li>
          ))}
        </ul>
      )}
      <Testimonials />
    </>
  );
}

function ProjectForm({ project: p, onDone }: { project: PortfolioProject | null; onDone: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [x, setX] = useState({
    title: p?.title ?? "",
    slug: p?.slug ?? "",
    category: p?.category ?? "suits",
    description: p?.description ?? "",
    materials: p?.materials ?? "",
    tags: p?.tags.join(", ") ?? "",
    customerPermission: p?.customerPermission ?? "not_required",
    featured: p?.featured ?? false,
    status: p?.status ?? "draft",
  });
  const [media, setMedia] = useState(
    p?.media.map((m) => ({ uploadId: m.uploadId, url: m.url, alt: m.alt, width: m.width, height: m.height })) ?? [],
  );
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const [busy, setBusy] = useState(false);
  async function save() {
    setBusy(true);
    const added = uploads
      .filter((u) => u.status === "done" && u.result)
      .map((u) => ({
        uploadId: u.result!.id,
        url: u.result!.url,
        alt: x.title,
        width: u.result!.width,
        height: u.result!.height,
      }));
    const body = {
      ...x,
      tags: x.tags
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean),
      videoUrl: null,
      sortOrder: p?.sortOrder ?? 0,
      media: [...media, ...added],
    };
    try {
      await api(p ? `/owner/portfolio/${p.id}` : "/owner/portfolio", { method: p ? "PUT" : "POST", body });
      toast("Project saved.");
      await qc.invalidateQueries({ queryKey: ["owner", "portfolio"] });
      onDone();
    } catch (e) {
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (!p || !confirm(`Delete "${p.title}"?`)) return;
    await api(`/owner/portfolio/${p.id}`, { method: "DELETE" });
    await qc.invalidateQueries({ queryKey: ["owner", "portfolio"] });
    onDone();
  }
  return (
    <div className="panel panel-pad stack" style={{ margin: "8px 0 16px" }}>
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Title</span>
          <input className="input" value={x.title} onChange={(e) => setX({ ...x, title: e.target.value })} />
        </label>
        <label className="field">
          <span className="label">Category</span>
          <select className="select" value={x.category} onChange={(e) => setX({ ...x, category: e.target.value })}>
            {categories.map((c) => (
              <option key={c} value={c}>
                {humanize(c)}
              </option>
            ))}
          </select>
        </label>
      </div>
      <label className="field">
        <span className="label">About this piece</span>
        <textarea
          className="textarea"
          rows={4}
          value={x.description}
          onChange={(e) => setX({ ...x, description: e.target.value })}
        />
      </label>
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Materials</span>
          <input className="input" value={x.materials} onChange={(e) => setX({ ...x, materials: e.target.value })} />
        </label>
        <label className="field">
          <span className="label">Tags (comma separated)</span>
          <input className="input" value={x.tags} onChange={(e) => setX({ ...x, tags: e.target.value })} />
        </label>
        <label className="field">
          <span className="label">Customer permission</span>
          <select
            className="select"
            value={x.customerPermission}
            onChange={(e) => setX({ ...x, customerPermission: e.target.value })}
          >
            {permission.map(([k, l]) => (
              <option key={k} value={k}>
                {l}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span className="label">Status</span>
          <select className="select" value={x.status} onChange={(e) => setX({ ...x, status: e.target.value })}>
            <option value="draft">Draft</option>
            <option value="published" disabled={!["not_required", "granted"].includes(x.customerPermission)}>
              Published
            </option>
          </select>
        </label>
      </div>
      {media.length ? (
        <ul className="row-wrap" style={{ listStyle: "none", padding: 0, margin: 0 }}>
          {media.map((m, i) => (
            <li key={m.url} style={{ position: "relative" }}>
              {/* eslint-disable-next-line @next/next/no-img-element -- owner thumbnail */}
              <img
                src={m.url}
                alt={m.alt}
                width={96}
                height={120}
                style={{ objectFit: "cover", border: "1px solid var(--line)", display: "block" }}
              />
              <input
                className="input"
                aria-label={`Photo ${i + 1} description`}
                value={m.alt}
                onChange={(e) => setMedia((ms) => ms.map((y, j) => (j === i ? { ...y, alt: e.target.value } : y)))}
                style={{ width: 96, fontSize: 12, padding: 4 }}
              />
              <button
                className="icon-btn"
                aria-label={`Remove photo ${i + 1}`}
                style={{ position: "absolute", top: 2, right: 2 }}
                onClick={() => setMedia((ms) => ms.filter((_, j) => j !== i))}
              >
                <X size={14} aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      <ImageUploader
        purpose="portfolio"
        items={uploads}
        onChange={setUploads}
        max={12}
        label="Add photos of the finished piece"
      />
      <label className="check">
        <input type="checkbox" checked={x.featured} onChange={(e) => setX({ ...x, featured: e.target.checked })} />
        <span>Show on the home page</span>
      </label>
      <div className="row-wrap">
        <button className="btn btn-primary btn-sm" disabled={busy || uploadsPending(uploads)} onClick={save}>
          Save project
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onDone}>
          Cancel
        </button>
        {p ? (
          <button className="btn btn-danger btn-sm" onClick={remove}>
            Delete
          </button>
        ) : null}
      </div>
    </div>
  );
}

type T = {
  id: string;
  customerName: string;
  quote: string;
  context: string;
  source: string;
  consent: boolean;
  status: string;
  publishedAt: string | null;
  createdAt: string;
};

function Testimonials() {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["owner", "testimonials"], queryFn: () => api<T[]>("/owner/testimonials") });
  const [form, setForm] = useState<Partial<T> | null>(null);
  async function save() {
    if (!form) return;
    try {
      await api(form.id ? `/owner/testimonials/${form.id}` : "/owner/testimonials", {
        method: form.id ? "PUT" : "POST",
        body: {
          customerName: form.customerName ?? "",
          quote: form.quote ?? "",
          context: form.context ?? "",
          source: form.source ?? "",
          consent: Boolean(form.consent),
          status: form.status ?? "draft",
        },
      });
      toast("Testimonial saved.");
      setForm(null);
      await qc.invalidateQueries({ queryKey: ["owner", "testimonials"] });
    } catch (e) {
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    }
  }
  return (
    <section className="stack-sm" aria-labelledby="t-h">
      <div className="spread">
        <h2 id="t-h" className={styles.h2}>
          Testimonials
        </h2>
        <button className="btn btn-sm" onClick={() => setForm({ status: "draft", consent: false })}>
          Add a testimonial
        </button>
      </div>
      <p className="small muted" style={{ margin: 0 }}>
        Only real words from real customers, with their permission. Testimonials appear on the home page only when at
        least one is published.
      </p>
      {form ? (
        <div className="panel panel-pad stack-sm">
          <div className="form-grid cols-2">
            <label className="field">
              <span className="label">Name as the customer agreed to be shown</span>
              <input
                className="input"
                value={form.customerName ?? ""}
                onChange={(e) => setForm({ ...form, customerName: e.target.value })}
              />
            </label>
            <label className="field">
              <span className="label">What it was for</span>
              <input
                className="input"
                value={form.context ?? ""}
                onChange={(e) => setForm({ ...form, context: e.target.value })}
                placeholder="Wedding suit, 2026"
              />
            </label>
          </div>
          <label className="field">
            <span className="label">Their words</span>
            <textarea
              className="textarea"
              rows={3}
              value={form.quote ?? ""}
              onChange={(e) => setForm({ ...form, quote: e.target.value })}
            />
          </label>
          <label className="field">
            <span className="label">Where they said it</span>
            <input
              className="input"
              value={form.source ?? ""}
              onChange={(e) => setForm({ ...form, source: e.target.value })}
              placeholder="WhatsApp message, 12 March"
            />
          </label>
          <label className="check">
            <input
              type="checkbox"
              checked={Boolean(form.consent)}
              onChange={(e) => setForm({ ...form, consent: e.target.checked })}
            />
            <span>The customer agreed to have this published</span>
          </label>
          <label className="field">
            <span className="label">Status</span>
            <select
              className="select"
              value={form.status ?? "draft"}
              onChange={(e) => setForm({ ...form, status: e.target.value })}
            >
              <option value="draft">Draft</option>
              <option value="published" disabled={!form.consent}>
                Published
              </option>
            </select>
          </label>
          <div className="row-wrap">
            <button className="btn btn-primary btn-sm" onClick={save}>
              Save
            </button>
            <button className="btn btn-ghost btn-sm" onClick={() => setForm(null)}>
              Cancel
            </button>
          </div>
        </div>
      ) : null}
      {(q.data ?? []).length ? (
        <ul className="list-rows">
          {(q.data ?? []).map((t) => (
            <li key={t.id} className="list-row">
              <span className="stack-xs">
                <span>&ldquo;{t.quote}&rdquo;</span>
                <span className="small muted">
                  {t.customerName}
                  {t.context ? `, ${t.context}` : ""} ·{" "}
                  {t.publishedAt ? `published ${formatDate(t.publishedAt, "short")}` : humanize(t.status)}
                </span>
              </span>
              <button className="btn btn-ghost btn-sm" onClick={() => setForm(t)}>
                Edit
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="small muted">None yet.</p>
      )}
    </section>
  );
}
