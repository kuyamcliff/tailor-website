"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { formatDateTime, humanize } from "@/lib/format";
import { useToast } from "@/components/providers/toast";
import { ImageUploader, type UploadItem } from "@/components/ui/image-uploader";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

type Block = { key: string; value: unknown; published: boolean; version: number; updatedAt: string };

const names: Record<string, string> = {
  "home.hero": "Home: opening",
  "home.signature": "Home: custom tailoring",
  "home.studio": "Home: fitting studio",
  "home.process": "Home: how we work",
  services: "Services",
  faqs: "Questions",
  about: "About page",
  contact: "Contact page",
  footer: "Footer",
  "custom.landing": "Custom tailoring page",
  "policy.privacy": "Policy: privacy",
  "policy.terms": "Policy: terms",
  "policy.refunds": "Policy: returns and refunds",
  "policy.delivery": "Policy: delivery and pickup",
  "policy.alterations": "Policy: alterations and remakes",
  "policy.cookies": "Policy: cookies",
};

export function OwnerContent() {
  const q = useQuery({ queryKey: ["owner", "content"], queryFn: () => api<{ items: Block[] }>("/owner/content") });
  const [open, setOpen] = useState<string | null>(null);
  return (
    <>
      <PageHead title="Pages and policies" sub="Words and images on the public site. Changes appear within a minute of saving." />
      <p className="notice small">
        The policies are a starting point. Have them checked against the law where you trade before relying on them. Plain text only.
      </p>
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 320 }} />
      ) : (
        <ul className="list-rows">
          {(q.data?.items ?? []).map((b) => (
            <li key={b.key}>
              <button
                className="list-row"
                style={{ width: "100%", background: "none", border: 0, color: "inherit", font: "inherit", cursor: "pointer", textAlign: "left" }}
                aria-expanded={open === b.key}
                onClick={() => setOpen(open === b.key ? null : b.key)}
              >
                <span>{names[b.key] ?? humanize(b.key)}</span>
                <span className="small muted">updated {formatDateTime(b.updatedAt)}</span>
              </button>
              {open === b.key ? <BlockEditor key={b.version} block={b} onDone={() => setOpen(null)} /> : null}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

function BlockEditor({ block, onDone }: { block: Block; onDone: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [value, setValue] = useState<unknown>(block.value);
  const [busy, setBusy] = useState(false);
  async function save() {
    setBusy(true);
    try {
      await api(`/owner/content/${block.key}`, { method: "PUT", body: { value, published: true, version: block.version } });
      toast("Saved.");
      await qc.invalidateQueries({ queryKey: ["owner", "content"] });
      onDone();
    } catch (e) {
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="panel panel-pad stack" style={{ margin: "8px 0 16px" }}>
      <ValueEditor value={value} onChange={setValue} path={block.key} />
      <div className="row-wrap">
        <button className="btn btn-primary btn-sm" disabled={busy} onClick={save}>
          Save
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onDone}>
          Cancel
        </button>
      </div>
    </div>
  );
}

const longKeys = new Set(["body", "subtitle", "a", "note", "description", "lede"]);

// ValueEditor renders a form for any content block shape: text, lists of items and nested groups.
function ValueEditor({ value, onChange, path, label }: { value: unknown; onChange: (v: unknown) => void; path: string; label?: string }) {
  if (typeof value === "string") {
    const key = path.split(".").pop() ?? "";
    if (key === "image") return <ImageField label={label ?? "Image"} value={value} onChange={onChange} />;
    const long = longKeys.has(key) || value.length > 90;
    return (
      <label className="field">
        <span className="label">{label ?? humanize(key)}</span>
        {long ? (
          <textarea className="textarea" rows={Math.min(8, Math.max(2, Math.ceil(value.length / 90)))} value={value} onChange={(e) => onChange(e.target.value)} />
        ) : (
          <input className="input" value={value} onChange={(e) => onChange(e.target.value)} />
        )}
      </label>
    );
  }
  if (typeof value === "boolean")
    return (
      <label className="check">
        <input type="checkbox" checked={value} onChange={(e) => onChange(e.target.checked)} />
        <span>{label ?? humanize(path.split(".").pop() ?? "")}</span>
      </label>
    );
  if (typeof value === "number")
    return (
      <label className="field">
        <span className="label">{label ?? humanize(path.split(".").pop() ?? "")}</span>
        <input className="input tabular" inputMode="decimal" value={String(value)} onChange={(e) => onChange(Number(e.target.value) || 0)} />
      </label>
    );
  if (Array.isArray(value)) {
    const template = value[0];
    return (
      <fieldset className="stack-sm" style={{ border: 0, padding: 0, margin: 0 }}>
        <legend className={styles.h2}>{label ?? humanize(path.split(".").pop() ?? "")}</legend>
        {value.map((item, i) => (
          <div key={i} className="stack-sm" style={{ borderLeft: "2px solid var(--line)", paddingLeft: 12 }}>
            <ValueEditor value={item} path={`${path}.${i}`} label={`${i + 1}`} onChange={(v) => onChange(value.map((x, j) => (j === i ? v : x)))} />
            <div className="row-wrap">
              <button className="link small" disabled={i === 0} onClick={() => onChange(swap(value, i, i - 1))}>
                Move up
              </button>
              <button className="link small" disabled={i === value.length - 1} onClick={() => onChange(swap(value, i, i + 1))}>
                Move down
              </button>
              <button className="link small" onClick={() => onChange(value.filter((_, j) => j !== i))}>
                Remove
              </button>
            </div>
          </div>
        ))}
        {template !== undefined ? (
          <button className="btn btn-sm" style={{ justifySelf: "start" }} onClick={() => onChange([...value, blank(template)])}>
            Add another
          </button>
        ) : null}
      </fieldset>
    );
  }
  if (value && typeof value === "object") {
    const obj = value as Record<string, unknown>;
    return (
      <div className="stack-sm">
        {label && !/^\d+$/.test(label) ? <p className={styles.h2} style={{ margin: 0 }}>{label}</p> : null}
        {Object.entries(obj).map(([k, v]) => (
          <ValueEditor key={k} value={v} path={`${path}.${k}`} onChange={(nv) => onChange({ ...obj, [k]: nv })} />
        ))}
      </div>
    );
  }
  return null;
}

function ImageField({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  const [items, setItems] = useState<UploadItem[]>([]);
  return (
    <div className="stack-sm">
      <span className="label">{label}</span>
      {value ? (
        // eslint-disable-next-line @next/next/no-img-element -- owner preview
        <img src={value} alt="" style={{ width: 200, height: 120, objectFit: "cover", border: "1px solid var(--line)" }} />
      ) : null}
      <ImageUploader
        purpose="content"
        items={items}
        max={1}
        label={value ? "Replace image" : "Upload image"}
        onChange={(update) =>
          setItems((prev) => {
            const nextItems = update(prev);
            const done = nextItems.find((x) => x.status === "done" && x.result);
            if (done?.result) onChange(done.result.url);
            return nextItems;
          })
        }
      />
    </div>
  );
}

function blank(t: unknown): unknown {
  if (typeof t === "string") return "";
  if (typeof t === "number") return 0;
  if (typeof t === "boolean") return false;
  if (Array.isArray(t)) return [];
  if (t && typeof t === "object") return Object.fromEntries(Object.entries(t as Record<string, unknown>).map(([k, v]) => [k, blank(v)]));
  return null;
}

function swap<T>(arr: T[], a: number, b: number): T[] {
  const n = [...arr];
  [n[a], n[b]] = [n[b]!, n[a]!];
  return n;
}
