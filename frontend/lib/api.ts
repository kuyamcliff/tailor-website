"use client";

// Browser API client. Same-origin requests go through the Next.js rewrite to the Go API, so the
// session cookie is first-party. Unsafe methods carry the CSRF token; anonymous visitors carry a
// random guest token so their uploads and saved designs belong to this device.

export class ApiError extends Error {
  status: number;
  code: string;
  fields: Record<string, string>;
  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

let csrfToken = "";
export function setCsrfToken(t: string) {
  csrfToken = t;
}

const GUEST_KEY = "atelier.guest";

export function guestToken(): string {
  if (typeof window === "undefined") return "";
  try {
    let t = window.localStorage.getItem(GUEST_KEY);
    if (!t || t.length < 32) {
      const bytes = new Uint8Array(24);
      crypto.getRandomValues(bytes);
      t = Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
      window.localStorage.setItem(GUEST_KEY, t);
    }
    return t;
  } catch {
    return "";
  }
}

export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}

type Options = {
  method?: string;
  body?: unknown;
  idempotencyKey?: string;
  accessToken?: string;
  signal?: AbortSignal;
  timeoutMs?: number;
};

export async function api<T>(path: string, opts: Options = {}): Promise<T> {
  const method = opts.method ?? (opts.body === undefined ? "GET" : "POST");
  const headers: Record<string, string> = { Accept: "application/json" };
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  if (method !== "GET" && csrfToken) headers["X-CSRF-Token"] = csrfToken;
  const g = guestToken();
  if (g) headers["X-Guest-Token"] = g;
  if (opts.idempotencyKey) headers["Idempotency-Key"] = opts.idempotencyKey;
  if (opts.accessToken) headers["X-Access-Token"] = opts.accessToken;

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), opts.timeoutMs ?? 30000);
  opts.signal?.addEventListener("abort", () => controller.abort());
  let res: Response;
  try {
    res = await fetch(`/api/v1${path}`, {
      method,
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      credentials: "same-origin",
      signal: controller.signal,
      cache: "no-store",
    });
  } catch (e) {
    if (opts.signal?.aborted) throw e;
    const offline = typeof navigator !== "undefined" && !navigator.onLine;
    throw new ApiError(
      0,
      offline ? "offline" : "network",
      offline
        ? "You appear to be offline. Check your connection and try again."
        : "We could not reach the atelier. Please try again.",
    );
  } finally {
    clearTimeout(timer);
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: unknown = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = null;
  }
  if (!res.ok) {
    const err = (data as { error?: { code?: string; message?: string; fields?: Record<string, string> } } | null)?.error;
    throw new ApiError(
      res.status,
      err?.code ?? "error",
      err?.message ?? "Something went wrong. Please try again.",
      err?.fields ?? {},
    );
  }
  return data as T;
}

export type UploadResult = {
  id: string;
  url: string;
  variants: Record<string, string>;
  width: number;
  height: number;
  duplicate?: boolean;
  originalName: string;
};

// uploadImage uses XMLHttpRequest for upload progress events.
export function uploadImage(
  file: File,
  purpose: string,
  onProgress?: (fraction: number) => void,
  signal?: AbortSignal,
): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `/api/v1/uploads?purpose=${encodeURIComponent(purpose)}`);
    xhr.withCredentials = true;
    const g = guestToken();
    if (g) xhr.setRequestHeader("X-Guest-Token", g);
    if (csrfToken) xhr.setRequestHeader("X-CSRF-Token", csrfToken);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.(e.loaded / e.total);
    };
    xhr.onload = () => {
      let data: unknown = null;
      try {
        data = JSON.parse(xhr.responseText);
      } catch {
        data = null;
      }
      if (xhr.status >= 200 && xhr.status < 300) resolve(data as UploadResult);
      else {
        const err = (data as { error?: { code?: string; message?: string } } | null)?.error;
        reject(new ApiError(xhr.status, err?.code ?? "upload_failed", err?.message ?? "The upload failed. Please try again."));
      }
    };
    xhr.onerror = () => reject(new ApiError(0, "network", "The upload was interrupted. Check your connection and retry."));
    xhr.onabort = () => reject(new ApiError(0, "aborted", "Upload cancelled."));
    signal?.addEventListener("abort", () => xhr.abort());
    const form = new FormData();
    form.append("file", file);
    xhr.send(form);
  });
}

export type AssetUpload = { file: { lod: string; url: string; bytes: number; sha256: string }; info: { nodes: string[]; morphTargets: string[]; meshes: number; materials: number } | null };

// uploadAssetFile sends a GLB or KTX2 file to the staff asset store (content-addressed, validated server-side).
export function uploadAssetFile(file: File, onProgress?: (fraction: number) => void): Promise<AssetUpload> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/v1/owner/assets/files");
    xhr.withCredentials = true;
    if (csrfToken) xhr.setRequestHeader("X-CSRF-Token", csrfToken);
    xhr.upload.onprogress = (e) => e.lengthComputable && onProgress?.(e.loaded / e.total);
    xhr.onload = () => {
      let data: unknown = null;
      try {
        data = JSON.parse(xhr.responseText);
      } catch {
        data = null;
      }
      if (xhr.status >= 200 && xhr.status < 300) resolve(data as AssetUpload);
      else {
        const err = (data as { error?: { code?: string; message?: string; fields?: Record<string, string> } } | null)?.error;
        reject(new ApiError(xhr.status, err?.code ?? "upload_failed", err?.fields?.file ?? err?.message ?? "The upload failed.", err?.fields ?? {}));
      }
    };
    xhr.onerror = () => reject(new ApiError(0, "network", "The upload was interrupted."));
    const form = new FormData();
    form.append("file", file);
    xhr.send(form);
  });
}
