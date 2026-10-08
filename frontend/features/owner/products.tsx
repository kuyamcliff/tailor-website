"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Suspense, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, X } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { humanize } from "@/lib/format";
import { exponentOf, toMinor } from "@/lib/money";
import type { Fabric, GarmentType, Product } from "@/lib/types";
import { useConfig } from "@/components/providers/config";
import { useToast } from "@/components/providers/toast";
import { Field } from "@/components/ui/field";
import { Price } from "@/components/ui/price";
import { StatusBadge } from "@/components/ui/status-badge";
import { ImageUploader, uploadsPending, type UploadItem } from "@/components/ui/image-uploader";
import { OwnerList } from "./owner-list";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

export function OwnerProducts() {
  return (
    <>
      <PageHead
        title="Products"
        sub="Ready-to-wear pieces in the shop."
        actions={
          <Link className="btn btn-primary btn-sm" href="/owner/products/new">
            New product
          </Link>
        }
      />
      <Suspense>
        <OwnerList<Product>
          endpoint="/owner/products"
          filters={[
            { key: "q", label: "Search", type: "search" },
            { key: "visibility", label: "Visibility", type: "select", options: [["published", "Published"], ["draft", "Draft"], ["hidden", "Hidden"], ["archived", "Archived"]] },
          ]}
          rowKey={(p) => p.id}
          href={(p) => `/owner/products/${p.id}`}
          empty="No products yet."
          columns={[
            { label: "Name", cell: (p) => p.name },
            { label: "Category", cell: (p) => p.category?.name ?? "" },
            { label: "Price", cell: (p) => <Price minor={p.priceMinor} maxMinor={p.priceMaxMinor} />, className: "tabular" },
            { label: "Stock", cell: (p) => humanize(p.availability) },
            { label: "Visibility", cell: (p) => <StatusBadge status={p.visibility === "published" ? "succeeded" : "draft"} label={humanize(p.visibility)} /> },
            { label: "Featured", cell: (p) => (p.featured ? "Yes" : "") },
          ]}
        />
      </Suspense>
    </>
  );
}

type V = { sku: string; sizeLabel: string; colorName: string; colorHex: string; price: string; stockQty: string; madeToOrder: boolean; active: boolean };
type M = { url: string; uploadId: string | null; alt: string; width: number | null; height: number | null };

export function OwnerProductEditor({ id }: { id: string | null }) {
  const q = useQuery({ queryKey: ["owner", "product", id], enabled: Boolean(id), queryFn: () => api<Product>(`/owner/products/${id}`) });
  if (id && q.isLoading) return <div className="skeleton" style={{ height: 480 }} />;
  if (id && !q.data) return <p className="notice notice-danger">This product could not be loaded.</p>;
  return <ProductForm key={q.data?.version ?? "new"} product={q.data ?? null} />;
}

function ProductForm({ product: p }: { product: Product | null }) {
  const router = useRouter();
  const cfg = useConfig();
  const qc = useQueryClient();
  const toast = useToast();
  const exp = exponentOf(cfg.business.currency);
  const cats = useQuery({ queryKey: ["owner", "categories"], queryFn: () => api<{ slug: string; name: string }[]>("/owner/categories") });
  const garments = useQuery({ queryKey: ["garments"], queryFn: () => api<GarmentType[]>("/garments") });
  const fabrics = useQuery({ queryKey: ["fabrics"], queryFn: () => api<Fabric[]>("/fabrics") });
  const [f, setF] = useState({
    name: p?.name ?? "",
    slug: p?.slug ?? "",
    summary: p?.summary ?? "",
    description: p?.description ?? "",
    categorySlug: p?.category?.slug ?? "",
    garmentTypeKey: p?.garmentTypeKey ?? "",
    fabricKey: p?.fabric?.key ?? "",
    fitNotes: p?.fitNotes ?? "",
    care: p?.care ?? "",
    measurementInfo: p?.measurementInfo ?? "",
    requiresFitting: p?.requiresFitting ?? false,
    customizable: p?.customizable ?? false,
    visibility: p?.visibility ?? "draft",
    price: p ? String(p.priceMinor / 10 ** exp) : "",
    featured: p?.featured ?? false,
  });
  const [variants, setVariants] = useState<V[]>(
    () =>
      p?.variants.map((v) => ({
        sku: v.sku,
        sizeLabel: v.sizeLabel,
        colorName: v.colorName,
        colorHex: v.colorHex ?? "",
        price: v.priceMinor !== p.priceMinor ? String(v.priceMinor / 10 ** exp) : "",
        stockQty: String(v.stockQty ?? 0),
        madeToOrder: v.madeToOrder,
        active: v.active,
      })) ?? [{ sku: "", sizeLabel: "", colorName: "", colorHex: "", price: "", stockQty: "0", madeToOrder: false, active: true }],
  );
  const [media, setMedia] = useState<M[]>(() => p?.media.map((m) => ({ url: m.url, uploadId: m.uploadId, alt: m.alt, width: m.width, height: m.height })) ?? []);
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const set = (k: keyof typeof f, v: string | boolean) => setF((x) => ({ ...x, [k]: v }));

  async function save() {
    setBusy(true);
    setErrors({});
    const added: M[] = uploads.filter((u) => u.status === "done" && u.result).map((u) => ({ url: u.result!.url, uploadId: u.result!.id, alt: f.name, width: u.result!.width, height: u.result!.height }));
    const body = {
      ...f,
      categorySlug: f.categorySlug || null,
      garmentTypeKey: f.garmentTypeKey || null,
      fabricKey: f.fabricKey || null,
      priceMinor: toMinor(f.price, cfg.business.currency) ?? -1,
      variants: variants.map((v) => ({
        sku: v.sku,
        sizeLabel: v.sizeLabel,
        colorName: v.colorName,
        colorHex: v.colorHex || null,
        priceMinor: v.price ? toMinor(v.price, cfg.business.currency) : null,
        stockQty: Number(v.stockQty) || 0,
        madeToOrder: v.madeToOrder,
        active: v.active,
      })),
      media: [...media, ...added],
      version: p?.version ?? 0,
    };
    try {
      const r = await api<Product>(p ? `/owner/products/${p.id}` : "/owner/products", { method: p ? "PUT" : "POST", body });
      toast("Product saved.");
      await qc.invalidateQueries({ queryKey: ["owner"] });
      setUploads([]);
      if (!p) router.replace(`/owner/products/${r.id}`);
    } catch (e) {
      if (e instanceof ApiError) {
        setErrors(e.fields);
        toast(Object.values(e.fields)[0] ?? e.message, "error");
      } else toast("Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHead
        title={p ? p.name : "New product"}
        actions={
          <>
            {p && p.visibility === "published" ? (
              <Link className="btn btn-sm" href={`/shop/${p.slug}`} target="_blank">
                View in shop
              </Link>
            ) : null}
            <button className="btn btn-primary btn-sm" disabled={busy || uploadsPending(uploads)} onClick={save}>
              Save
            </button>
          </>
        }
      />
      <div className={styles.detail}>
        <div className="stack-lg">
          <section className="stack" aria-label="Details">
            <div className="form-grid cols-2">
              <Field label="Name" error={errors.name}>
                {(a) => <input {...a} className="input" value={f.name} onChange={(e) => set("name", e.target.value)} />}
              </Field>
              <Field label="Web address" error={errors.slug} hint="Lowercase words joined by hyphens. Leave empty to use the name.">
                {(a) => <input {...a} className="input" value={f.slug} onChange={(e) => set("slug", e.target.value)} />}
              </Field>
            </div>
            <Field label="Short summary" error={errors.summary}>
              {(a) => <input {...a} className="input" value={f.summary} maxLength={200} onChange={(e) => set("summary", e.target.value)} />}
            </Field>
            <Field label="Description" error={errors.description}>
              {(a) => <textarea {...a} className="textarea" rows={6} value={f.description} onChange={(e) => set("description", e.target.value)} />}
            </Field>
            <div className="form-grid cols-2">
              <Field label="Fit notes">{(a) => <textarea {...a} className="textarea" rows={3} value={f.fitNotes} onChange={(e) => set("fitNotes", e.target.value)} />}</Field>
              <Field label="Care">{(a) => <textarea {...a} className="textarea" rows={3} value={f.care} onChange={(e) => set("care", e.target.value)} />}</Field>
            </div>
            <Field label="How it is measured" hint="Shown on the size guide tab.">
              {(a) => <textarea {...a} className="textarea" rows={2} value={f.measurementInfo} onChange={(e) => set("measurementInfo", e.target.value)} />}
            </Field>
          </section>

          <section className="stack-sm" aria-labelledby="var-h">
            <h2 id="var-h" className={styles.h2}>
              Sizes and stock
            </h2>
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>SKU</th>
                    <th>Size</th>
                    <th>Colour</th>
                    <th>Price if different</th>
                    <th>In stock</th>
                    <th>Made to order</th>
                    <th>On sale</th>
                    <th>
                      <span className="visually-hidden">Remove</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {variants.map((v, i) => {
                    const sv = (k: keyof V, val: string | boolean) => setVariants((x) => x.map((y, j) => (j === i ? { ...y, [k]: val } : y)));
                    return (
                      <tr key={i}>
                        <td>
                          <input className="input" aria-label={`Variant ${i + 1} SKU`} value={v.sku} onChange={(e) => sv("sku", e.target.value)} style={{ minWidth: 120 }} />
                          {errors[`variants.${i}`] ? <span className="error tiny">{errors[`variants.${i}`]}</span> : null}
                        </td>
                        <td>
                          <input className="input" aria-label={`Variant ${i + 1} size`} value={v.sizeLabel} onChange={(e) => sv("sizeLabel", e.target.value)} style={{ minWidth: 70 }} />
                        </td>
                        <td>
                          <div className="row">
                            <input className="input" aria-label={`Variant ${i + 1} colour`} value={v.colorName} onChange={(e) => sv("colorName", e.target.value)} style={{ minWidth: 100 }} />
                            <input type="color" aria-label={`Variant ${i + 1} colour swatch`} value={v.colorHex || "#000000"} onChange={(e) => sv("colorHex", e.target.value)} />
                          </div>
                        </td>
                        <td>
                          <input className="input tabular" aria-label={`Variant ${i + 1} price`} inputMode="decimal" value={v.price} onChange={(e) => sv("price", e.target.value)} style={{ minWidth: 100 }} />
                        </td>
                        <td>
                          <input className="input tabular" aria-label={`Variant ${i + 1} stock`} inputMode="numeric" value={v.stockQty} onChange={(e) => sv("stockQty", e.target.value)} style={{ width: 80 }} />
                        </td>
                        <td>
                          <input type="checkbox" aria-label={`Variant ${i + 1} made to order`} checked={v.madeToOrder} onChange={(e) => sv("madeToOrder", e.target.checked)} />
                        </td>
                        <td>
                          <input type="checkbox" aria-label={`Variant ${i + 1} on sale`} checked={v.active} onChange={(e) => sv("active", e.target.checked)} />
                        </td>
                        <td>
                          {variants.length > 1 ? (
                            <button className="icon-btn" aria-label={`Remove variant ${i + 1}`} onClick={() => setVariants((x) => x.filter((_, j) => j !== i))}>
                              <X size={16} aria-hidden />
                            </button>
                          ) : null}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <button className="btn btn-sm" style={{ justifySelf: "start" }} onClick={() => setVariants((x) => [...x, { sku: "", sizeLabel: "", colorName: x[0]?.colorName ?? "", colorHex: x[0]?.colorHex ?? "", price: "", stockQty: "0", madeToOrder: false, active: true }])}>
              Add size or colour
            </button>
          </section>

          <section className="stack-sm" aria-labelledby="media-h">
            <h2 id="media-h" className={styles.h2}>
              Photos
            </h2>
            <p className="small muted" style={{ margin: 0 }}>
              Use photographs of the actual piece. Describe each photo for people using screen readers.
            </p>
            {media.length ? (
              <ol className="stack-sm" style={{ listStyle: "none", padding: 0, margin: 0 }}>
                {media.map((m, i) => (
                  <li key={m.url} className="row" style={{ alignItems: "center", gap: 12 }}>
                    {/* eslint-disable-next-line @next/next/no-img-element -- owner preview of an uploaded image */}
                    <img src={m.url} alt="" width={64} height={80} style={{ objectFit: "cover", border: "1px solid var(--line)" }} />
                    <input className="input" aria-label={`Photo ${i + 1} description`} value={m.alt} onChange={(e) => setMedia((x) => x.map((y, j) => (j === i ? { ...y, alt: e.target.value } : y)))} style={{ flex: 1 }} />
                    <button className="icon-btn" aria-label={`Move photo ${i + 1} up`} disabled={i === 0} onClick={() => setMedia((x) => swap(x, i, i - 1))}>
                      <ArrowUp size={16} aria-hidden />
                    </button>
                    <button className="icon-btn" aria-label={`Move photo ${i + 1} down`} disabled={i === media.length - 1} onClick={() => setMedia((x) => swap(x, i, i + 1))}>
                      <ArrowDown size={16} aria-hidden />
                    </button>
                    <button className="icon-btn" aria-label={`Remove photo ${i + 1}`} onClick={() => setMedia((x) => x.filter((_, j) => j !== i))}>
                      <X size={16} aria-hidden />
                    </button>
                  </li>
                ))}
              </ol>
            ) : null}
            <ImageUploader purpose="product" items={uploads} onChange={setUploads} max={12} label="Upload photos" hint="New photos are added after the ones above when you save." />
          </section>
        </div>

        <aside className={styles.side}>
          <section className="panel panel-pad stack-sm" aria-label="Publishing">
            <Field label="Visibility" error={errors.visibility}>
              {(a) => (
                <select {...a} className="select" value={f.visibility} onChange={(e) => set("visibility", e.target.value)}>
                  <option value="draft">Draft</option>
                  <option value="published">Published</option>
                  <option value="hidden">Hidden (link only)</option>
                  <option value="archived">Archived</option>
                </select>
              )}
            </Field>
            <Field label={`Price (${cfg.business.currency})`} error={errors.priceMinor}>
              {(a) => <input {...a} className="input tabular" inputMode="decimal" value={f.price} onChange={(e) => set("price", e.target.value)} />}
            </Field>
            <label className="check">
              <input type="checkbox" checked={f.featured} onChange={(e) => set("featured", e.target.checked)} />
              <span>Feature on the home page</span>
            </label>
            <label className="check">
              <input type="checkbox" checked={f.customizable} onChange={(e) => set("customizable", e.target.checked)} />
              <span>Can be customised in the studio</span>
            </label>
            <label className="check">
              <input type="checkbox" checked={f.requiresFitting} onChange={(e) => set("requiresFitting", e.target.checked)} />
              <span>Needs a fitting</span>
            </label>
          </section>
          <section className="panel panel-pad stack-sm" aria-label="Organisation">
            <Field label="Category">
              {(a) => (
                <select {...a} className="select" value={f.categorySlug} onChange={(e) => set("categorySlug", e.target.value)}>
                  <option value="">None</option>
                  {(cats.data ?? []).map((c) => (
                    <option key={c.slug} value={c.slug}>
                      {c.name}
                    </option>
                  ))}
                </select>
              )}
            </Field>
            <Field label="Garment type">
              {(a) => (
                <select {...a} className="select" value={f.garmentTypeKey} onChange={(e) => set("garmentTypeKey", e.target.value)}>
                  <option value="">None</option>
                  {(garments.data ?? []).map((g) => (
                    <option key={g.key} value={g.key}>
                      {g.name}
                    </option>
                  ))}
                </select>
              )}
            </Field>
            <Field label="Fabric">
              {(a) => (
                <select {...a} className="select" value={f.fabricKey} onChange={(e) => set("fabricKey", e.target.value)}>
                  <option value="">None</option>
                  {(fabrics.data ?? []).map((x) => (
                    <option key={x.key} value={x.key}>
                      {x.name}
                    </option>
                  ))}
                </select>
              )}
            </Field>
          </section>
        </aside>
      </div>
    </>
  );
}

function swap<T>(arr: T[], a: number, b: number): T[] {
  const next = [...arr];
  [next[a], next[b]] = [next[b]!, next[a]!];
  return next;
}
