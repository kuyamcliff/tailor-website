"use client";

import { useEffect, useRef, useState } from "react";
import { ImagePlus, RotateCcw, X } from "lucide-react";
import { api, ApiError, uploadImage, type UploadResult } from "@/lib/api";
import styles from "./image-uploader.module.css";

export type UploadItem = {
  key: string;
  name: string;
  preview: string;
  progress: number;
  status: "uploading" | "done" | "error";
  error?: string;
  result?: UploadResult;
  file: File;
};

const accepted = ["image/jpeg", "image/png", "image/webp"];
const maxBytes = 15 * 1024 * 1024;

type Props = {
  purpose: "reference" | "support" | "body_photo";
  items: UploadItem[];
  onChange: (update: (items: UploadItem[]) => UploadItem[]) => void;
  max?: number;
  label?: string;
  hint?: string;
};

// ImageUploader uploads each selected image immediately with progress, retry and removal.
export function ImageUploader({ purpose, items, onChange, max = 6, label = "Add photos", hint }: Props) {
  const input = useRef<HTMLInputElement>(null);
  const controllers = useRef(new Map<string, AbortController>());
  const [notice, setNotice] = useState("");

  useEffect(() => {
    const ctrls = controllers.current;
    return () => ctrls.forEach((c) => c.abort());
  }, []);

  function start(item: UploadItem) {
    const ctrl = new AbortController();
    controllers.current.set(item.key, ctrl);
    const patch = (p: Partial<UploadItem>) => onChange((list) => list.map((i) => (i.key === item.key ? { ...i, ...p } : i)));
    uploadImage(item.file, purpose, (f) => patch({ progress: f }), ctrl.signal)
      .then((result) => patch({ status: "done", progress: 1, result }))
      .catch((e) => {
        if (e instanceof ApiError && e.code === "aborted") return;
        patch({ status: "error", error: e instanceof ApiError ? e.message : "The upload failed." });
      })
      .finally(() => controllers.current.delete(item.key));
  }

  function add(files: FileList | null) {
    if (!files) return;
    setNotice("");
    const room = max - items.length;
    const next: UploadItem[] = [];
    for (const file of Array.from(files)) {
      if (next.length >= room) {
        setNotice(`You can add up to ${max} images.`);
        break;
      }
      if (!accepted.includes(file.type)) {
        setNotice(`${file.name} is not a JPEG, PNG or WebP image.`);
        continue;
      }
      if (file.size > maxBytes) {
        setNotice(`${file.name} is larger than 15 MB.`);
        continue;
      }
      next.push({ key: crypto.randomUUID(), name: file.name, preview: URL.createObjectURL(file), progress: 0, status: "uploading", file });
    }
    onChange((list) => [...list, ...next]);
    next.forEach(start);
    if (input.current) input.current.value = "";
  }

  async function remove(item: UploadItem) {
    controllers.current.get(item.key)?.abort();
    onChange((list) => list.filter((i) => i.key !== item.key));
    URL.revokeObjectURL(item.preview);
    if (item.result && !item.result.duplicate) {
      try {
        await api(`/uploads/${item.result.id}`, { method: "DELETE" });
      } catch {
        // The upload stays unattached and is purged by the retention job.
      }
    }
  }

  function retry(item: UploadItem) {
    const fresh = { ...item, status: "uploading" as const, progress: 0, error: undefined };
    onChange((list) => list.map((i) => (i.key === item.key ? fresh : i)));
    start(fresh);
  }

  return (
    <div className="stack-sm">
      {items.length ? (
        <ul className={styles.grid} aria-label="Selected images">
          {items.map((i) => (
            <li key={i.key} className={styles.item} data-status={i.status}>
              {/* eslint-disable-next-line @next/next/no-img-element -- local object URL preview */}
              <img src={i.preview} alt="" className={styles.thumb} />
              {i.status === "uploading" ? (
                <progress className={styles.progress} value={i.progress} max={1} aria-label={`Uploading ${i.name}`} />
              ) : null}
              {i.status === "error" ? (
                <div className={styles.error} role="alert">
                  <span>{i.error}</span>
                  <button type="button" className="btn btn-sm" onClick={() => retry(i)}>
                    <RotateCcw size={14} aria-hidden /> Retry
                  </button>
                </div>
              ) : null}
              <button type="button" className={styles.remove} onClick={() => remove(i)} aria-label={`Remove ${i.name}`}>
                <X size={14} aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      {items.length < max ? (
        <label className={styles.drop}>
          <ImagePlus size={20} aria-hidden />
          <span>{label}</span>
          {hint ? <span className="small muted">{hint}</span> : null}
          <input ref={input} type="file" accept={accepted.join(",")} multiple className="visually-hidden" onChange={(e) => add(e.target.files)} />
        </label>
      ) : null}
      {notice ? (
        <p className="small" role="status" style={{ color: "var(--warning)" }}>
          {notice}
        </p>
      ) : null}
    </div>
  );
}

export function uploadsPending(items: UploadItem[]) {
  return items.some((i) => i.status === "uploading");
}

export function uploadedIds(items: UploadItem[]) {
  return items.filter((i) => i.status === "done" && i.result).map((i) => i.result!.id);
}
