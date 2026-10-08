"use client";

import Link from "next/link";
import { Suspense, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { formatDate, formatDateTime, humanize } from "@/lib/format";
import type { SupportThread } from "@/lib/types";
import { useSession } from "@/components/providers/session";
import { useToast } from "@/components/providers/toast";
import { StatusBadge } from "@/components/ui/status-badge";
import { ImageUploader, uploadedIds, uploadsPending, type UploadItem } from "@/components/ui/image-uploader";
import { OwnerList } from "./owner-list";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";
import thread from "@/features/support/thread.module.css";

export function OwnerSupport() {
  return (
    <>
      <PageHead title="Messages" sub="Questions from customers, open first." />
      <Suspense>
        <OwnerList<SupportThread>
          endpoint="/owner/support"
          filters={[
            {
              key: "status",
              label: "Status",
              type: "select",
              options: [
                ["open", "Waiting for us"],
                ["pending", "Waiting for customer"],
                ["resolved", "Resolved"],
              ],
            },
            {
              key: "priority",
              label: "Priority",
              type: "select",
              options: [
                ["urgent", "Urgent"],
                ["high", "High"],
                ["normal", "Normal"],
                ["low", "Low"],
              ],
            },
            { key: "unread", label: "Unread", type: "select", options: [["true", "Unread only"]] },
          ]}
          rowKey={(t) => t.id}
          href={(t) => `/owner/support/${t.id}`}
          empty="No conversations match."
          columns={[
            { label: "Subject", cell: (t) => <>{t.unread ? <strong>{t.subject}</strong> : t.subject}</> },
            { label: "Customer", cell: (t) => t.customerName },
            {
              label: "About",
              cell: (t) => [humanize(t.category), t.orderNumber, t.requestNumber].filter(Boolean).join(" · "),
            },
            { label: "Priority", cell: (t) => (t.priority === "normal" ? "" : humanize(t.priority)) },
            { label: "Assigned", cell: (t) => t.assigneeName ?? "" },
            {
              label: "Status",
              cell: (t) => (
                <StatusBadge
                  status={t.status}
                  label={
                    t.status === "open"
                      ? "Waiting for us"
                      : t.status === "pending"
                        ? "Waiting for customer"
                        : "Resolved"
                  }
                />
              ),
            },
            { label: "Last message", cell: (t) => formatDate(t.lastMessageAt, "short") },
          ]}
        />
      </Suspense>
    </>
  );
}

export function OwnerSupportThread({ id }: { id: string }) {
  const qc = useQueryClient();
  const toast = useToast();
  const { user } = useSession();
  const q = useQuery({ queryKey: ["owner", "support", id], queryFn: () => api<SupportThread>(`/owner/support/${id}`) });
  const team = useQuery({
    queryKey: ["owner", "team"],
    queryFn: () => api<{ id: string; name: string }[]>("/owner/team"),
  });
  const [body, setBody] = useState("");
  const [internal, setInternal] = useState(false);
  const [files, setFiles] = useState<UploadItem[]>([]);
  const [busy, setBusy] = useState(false);
  const reload = () => qc.invalidateQueries({ queryKey: ["owner", "support", id] });
  const canWrite = user?.permissions.includes("support.write");

  async function patch(b: Record<string, unknown>, done: string) {
    setBusy(true);
    try {
      await api(`/owner/support/${id}`, { method: "PATCH", body: b });
      toast(done);
      await reload();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }
  async function reply(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await api(`/owner/support/${id}/reply`, { body: { body, internal, attachments: uploadedIds(files) } });
      setBody("");
      setFiles([]);
      setInternal(false);
      toast(internal ? "Internal note added." : "Reply sent.");
      await reload();
    } catch (err) {
      toast(err instanceof ApiError ? (err.fields.body ?? err.message) : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }

  if (q.isLoading) return <div className="skeleton" style={{ height: 420 }} />;
  const t = q.data;
  if (!t) return <p className="notice notice-danger">This conversation could not be loaded.</p>;
  return (
    <>
      <PageHead
        title={t.subject}
        sub={`${t.number} · ${t.customerName} · ${humanize(t.category)}`}
        actions={<StatusBadge status={t.status} />}
      />
      <div className={styles.detail}>
        <div className="stack-lg">
          <ol className={thread.messages} aria-label="Messages">
            {(t.messages ?? []).map((m) => (
              <li
                key={m.id}
                className={thread.message}
                data-author={m.authorType === "staff" ? "customer" : "staff"}
                style={m.internal ? { borderStyle: "dashed" } : undefined}
              >
                <div className={thread.meta}>
                  <strong>
                    {m.authorType === "customer" ? t.customerName : (m.author ?? "Staff")}
                    {m.internal ? " (internal note)" : ""}
                  </strong>
                  <time className="small muted" dateTime={m.createdAt}>
                    {formatDateTime(m.createdAt)}
                  </time>
                </div>
                <p className={thread.body}>{m.body}</p>
                {m.attachments.length ? (
                  <ul className={thread.attachments}>
                    {m.attachments.map((a) => (
                      <li key={a.id}>
                        <a href={a.url} target="_blank" rel="noopener noreferrer">
                          {/* eslint-disable-next-line @next/next/no-img-element -- private upload served with the staff session */}
                          <img src={a.thumb} alt="Attached photo" width={96} height={96} loading="lazy" />
                        </a>
                      </li>
                    ))}
                  </ul>
                ) : null}
              </li>
            ))}
          </ol>
          {canWrite ? (
            <form onSubmit={reply} className="panel panel-pad stack">
              <label className="visually-hidden" htmlFor="owner-reply">
                Reply
              </label>
              <textarea
                id="owner-reply"
                className="textarea"
                rows={5}
                value={body}
                onChange={(e) => setBody(e.target.value)}
                placeholder={internal ? "Note for the team" : `Reply to ${t.customerName}`}
              />
              <ImageUploader purpose="support" items={files} onChange={setFiles} max={4} label="Attach photos" />
              <div className="spread">
                <label className="check">
                  <input type="checkbox" checked={internal} onChange={(e) => setInternal(e.target.checked)} />
                  <span>Internal note (the customer will not see it)</span>
                </label>
                <button className="btn btn-primary" disabled={busy || !body.trim() || uploadsPending(files)}>
                  {internal ? "Add note" : "Send reply"}
                </button>
              </div>
            </form>
          ) : null}
        </div>
        <aside className={styles.side}>
          <section className="panel panel-pad stack-sm" aria-labelledby="th-h">
            <h2 id="th-h" className={styles.h2}>
              Conversation
            </h2>
            <Link className="link" href={`/owner/customers/${t.customerId}`}>
              {t.customerName}
            </Link>
            {t.orderId ? (
              <Link className="link small" href={`/owner/orders/${t.orderId}`}>
                Order {t.orderNumber}
              </Link>
            ) : null}
            {t.requestId ? (
              <Link className="link small" href={`/owner/requests/${t.requestId}`}>
                Request {t.requestNumber}
              </Link>
            ) : null}
            {canWrite ? (
              <>
                <label className="field">
                  <span className="label">Status</span>
                  <select
                    className="select"
                    value={t.status}
                    disabled={busy}
                    onChange={(e) => patch({ status: e.target.value }, "Status updated.")}
                  >
                    <option value="open">Waiting for us</option>
                    <option value="pending">Waiting for customer</option>
                    <option value="resolved">Resolved</option>
                  </select>
                </label>
                <label className="field">
                  <span className="label">Priority</span>
                  <select
                    className="select"
                    value={t.priority}
                    disabled={busy}
                    onChange={(e) => patch({ priority: e.target.value }, "Priority updated.")}
                  >
                    {["low", "normal", "high", "urgent"].map((p) => (
                      <option key={p} value={p}>
                        {humanize(p)}
                      </option>
                    ))}
                  </select>
                </label>
                {team.data && team.data.length > 1 ? (
                  <label className="field">
                    <span className="label">Assigned to</span>
                    <select
                      className="select"
                      value={t.assignedTo ?? ""}
                      disabled={busy}
                      onChange={(e) =>
                        e.target.value
                          ? patch(
                              { assignedTo: e.target.value },
                              `Assigned to ${team.data?.find((m) => m.id === e.target.value)?.name ?? "them"}.`,
                            )
                          : patch({ unassign: true }, "Unassigned.")
                      }
                    >
                      <option value="">Nobody</option>
                      {team.data.map((m) => (
                        <option key={m.id} value={m.id}>
                          {m.id === user?.id ? `${m.name} (you)` : m.name}
                        </option>
                      ))}
                    </select>
                  </label>
                ) : (
                  <p className="small" style={{ margin: 0 }}>
                    {t.assigneeName ? `Assigned to ${t.assigneeName}` : "Not assigned"}
                  </p>
                )}
                {team.data && team.data.length > 1 ? null : t.assignedTo === user?.id ? (
                  <button
                    className="btn btn-sm"
                    disabled={busy}
                    onClick={() => patch({ unassign: true }, "Unassigned.")}
                  >
                    Unassign me
                  </button>
                ) : (
                  <button
                    className="btn btn-sm"
                    disabled={busy}
                    onClick={() => patch({ assignedTo: user?.id }, "Assigned to you.")}
                  >
                    Assign to me
                  </button>
                )}
              </>
            ) : null}
          </section>
        </aside>
      </div>
    </>
  );
}
