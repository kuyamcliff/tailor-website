"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { useAccessToken } from "@/lib/client-hooks";
import { formatDateTime, humanize } from "@/lib/format";
import type { SupportThread } from "@/lib/types";
import { StatusBadge } from "@/components/ui/status-badge";
import { ImageUploader, uploadedIds, uploadsPending, type UploadItem } from "@/components/ui/image-uploader";
import { useToast } from "@/components/providers/toast";
import { useConfig } from "@/components/providers/config";
import styles from "./thread.module.css";

const statusLabel = { open: "Waiting for us", pending: "We replied", resolved: "Resolved" } as const;

export function SupportThreadView({ id }: { id: string }) {
  const cfg = useConfig();
  const toast = useToast();
  const token = useAccessToken("support", id);
  const [body, setBody] = useState("");
  const [files, setFiles] = useState<UploadItem[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const q = useQuery({
    queryKey: ["support", id, token],
    enabled: token !== null,
    refetchInterval: 30_000,
    queryFn: () => api<SupportThread>(`/support/${id}`, { accessToken: token || undefined }),
  });
  const t = q.data;
  const access = token ? `?access=${encodeURIComponent(token)}` : "";

  async function send(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api(`/support/${id}/reply`, { body: { body, attachments: uploadedIds(files) }, accessToken: token || undefined });
      setBody("");
      setFiles([]);
      toast("Message sent.");
      await q.refetch();
    } catch (err) {
      setError(err instanceof ApiError ? (err.fields.body ?? err.message) : "Please try again.");
    } finally {
      setBusy(false);
    }
  }

  if (token === null || q.isLoading) return <div className="container-narrow section-tight"><div className="skeleton" style={{ height: 360 }} /></div>;
  if (!t)
    return (
      <div className="container-narrow section-tight stack-lg">
        <h1 className="display-2">We could not open this conversation.</h1>
        <p className="lede">Open the link from your confirmation message, or sign in to the account you used.</p>
        <div className="row-wrap">
          <Link className="btn btn-primary" href={`/account/sign-in?next=/support/${id}`}>
            Sign in
          </Link>
          <Link className="btn" href="/support">
            Start a new message
          </Link>
        </div>
      </div>
    );

  return (
    <div className="container-narrow section-tight stack-lg">
      <header className="stack-sm">
        <span className="eyebrow">Conversation {t.number}</span>
        <h1 className="display-2">{t.subject}</h1>
        <div className="row-wrap">
          <StatusBadge status={t.status} label={statusLabel[t.status]} />
          <span className="badge">{humanize(t.category)}</span>
          {t.orderNumber && t.orderId ? (
            <Link className="badge" href={`/orders/${t.orderId}`}>
              Order {t.orderNumber}
            </Link>
          ) : null}
          {t.requestNumber && t.requestId ? (
            <Link className="badge" href={`/requests/${t.requestId}`}>
              Request {t.requestNumber}
            </Link>
          ) : null}
        </div>
      </header>
      <ol className={styles.messages} aria-label="Messages">
        {(t.messages ?? []).map((m) => (
          <li key={m.id} className={styles.message} data-author={m.authorType}>
            <div className={styles.meta}>
              <strong>{m.authorType === "customer" ? "You" : m.authorType === "staff" ? cfg.business.name : "Update"}</strong>
              <time className="small muted" dateTime={m.createdAt}>
                {formatDateTime(m.createdAt, cfg.business.timezone)}
              </time>
            </div>
            <p className={styles.body}>{m.body}</p>
            {m.attachments.length ? (
              <ul className={styles.attachments} aria-label="Attachments">
                {m.attachments.map((a) => (
                  <li key={a.id}>
                    <a href={a.url + access} target="_blank" rel="noopener noreferrer">
                      {/* eslint-disable-next-line @next/next/no-img-element -- private, token-scoped image */}
                      <img src={a.thumb + access} alt="Attached photo" width={96} height={96} loading="lazy" />
                    </a>
                  </li>
                ))}
              </ul>
            ) : null}
          </li>
        ))}
      </ol>
      <form onSubmit={send} className="panel panel-pad stack" aria-labelledby="reply-title">
        <h2 id="reply-title" className="title">
          {t.status === "resolved" ? "Reopen with a new message" : "Reply"}
        </h2>
        {error ? (
          <p className="notice notice-danger" role="alert">
            {error}
          </p>
        ) : null}
        <label className="visually-hidden" htmlFor="reply-body">
          Your message
        </label>
        <textarea id="reply-body" className="textarea" value={body} onChange={(e) => setBody(e.target.value)} maxLength={5000} required />
        <ImageUploader purpose="support" items={files} onChange={setFiles} max={4} label="Attach photos" />
        <button className="btn btn-primary" type="submit" disabled={busy || !body.trim() || uploadsPending(files)}>
          {busy ? <span className="spinner" aria-hidden /> : null} Send
        </button>
      </form>
    </div>
  );
}
