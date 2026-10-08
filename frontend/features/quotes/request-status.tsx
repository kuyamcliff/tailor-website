"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { CheckCircle2 } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { accessTokenFromUrl, rememberLink } from "@/lib/links";
import { formatDate, humanize, requestLabels } from "@/lib/format";
import type { RequestView } from "@/lib/types";
import { Price } from "@/components/ui/price";
import { StatusBadge } from "@/components/ui/status-badge";
import { useToast } from "@/components/providers/toast";
import { formatLength } from "@/lib/units";

export function RequestStatus({ id }: { id: string }) {
  const toast = useToast();
  const [token, setToken] = useState<string | null>(null);
  const [submitted, setSubmitted] = useState(false);
  const [reply, setReply] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    setSubmitted(new URL(window.location.href).searchParams.get("submitted") === "1");
    setToken(accessTokenFromUrl("request", id));
  }, [id]);
  const q = useQuery({
    queryKey: ["request", id, token],
    enabled: token !== null,
    queryFn: () => api<RequestView>(`/requests/${id}`, { accessToken: token || undefined }),
  });
  useEffect(() => {
    if (q.data && token) rememberLink({ kind: "request", id, number: q.data.number, token });
  }, [q.data, token, id]);

  async function sendReply(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await api(`/requests/${id}/reply`, { body: { message: reply }, accessToken: token || undefined });
      setReply("");
      toast("Message sent to your tailor.");
      await q.refetch();
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "Could not send. Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }

  if (q.isLoading || token === null) return <div className="container section-tight"><div className="skeleton" style={{ height: 420 }} /></div>;
  if (!q.data)
    return (
      <div className="container-narrow section-tight stack-lg">
        <h1 className="display-2">We could not open this request.</h1>
        <p className="lede">Use the link from your confirmation, or sign in.</p>
        <Link className="btn btn-primary" href={`/account/sign-in?next=/requests/${id}`}>
          Sign in
        </Link>
      </div>
    );
  const r = q.data;
  const access = token ? `?access=${encodeURIComponent(token)}` : "";
  const tq = token ? `?token=${token}` : "";
  const m = r.verifiedMeasurements ?? r.measurements;

  return (
    <div className="container section-tight" style={{ display: "grid", gap: 32, gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 420px), 1fr))", alignItems: "start" }}>
      <div className="stack-lg">
        {submitted ? (
          <p className="notice notice-success" role="status">
            <CheckCircle2 size={18} aria-hidden style={{ color: "var(--success)", flex: "none", marginTop: 2 }} />
            <span>
              Thank you. Request <strong>{r.number}</strong> has been sent. Your tailor will review it and reply with a quote or questions. Keep this page bookmarked.
            </span>
          </p>
        ) : null}
        <header className="stack-sm">
          <span className="eyebrow">Request {r.number}</span>
          <h1 className="display-2">{r.garmentName} for {humanize(r.occasion).toLowerCase()}</h1>
          <div className="row-wrap">
            <StatusBadge status={r.status} label={requestLabels[r.status]} />
            <span className="tiny muted">Sent {formatDate(r.createdAt)}</span>
          </div>
        </header>
        {r.status === "need_information" && r.infoRequested ? (
          <section className="panel panel-pad stack" style={{ borderColor: "var(--warning)" }}>
            <h2 className="title">Your tailor has a question</h2>
            <p>{r.infoRequested}</p>
          </section>
        ) : null}
        {r.quotes.length ? (
          <section className="panel panel-pad stack">
            <h2 className="display-3">Quotes</h2>
            {r.quotes.map((qt) => (
              <Link key={qt.id} href={`/quotes/${qt.id}${tq}`} className="spread" style={{ padding: "10px 0", borderBottom: "1px solid var(--line)" }}>
                <span>
                  Quote {qt.number}
                  <span className="tiny muted" style={{ display: "block" }}>
                    {qt.expiresAt ? `Valid until ${formatDate(qt.expiresAt)}` : ""}
                  </span>
                </span>
                <span className="row">
                  {qt.totalMinor !== null ? <Price minor={qt.totalMinor} currency={qt.currency ?? undefined} /> : null}
                  <StatusBadge status={qt.status} />
                </span>
              </Link>
            ))}
          </section>
        ) : null}
        {r.orderId ? (
          <Link className="btn btn-primary" href={`/orders/${r.orderId}${tq}`}>
            View your order
          </Link>
        ) : null}
        <form onSubmit={sendReply} className="panel panel-pad stack">
          <label htmlFor="reply" className="title">
            Message your tailor
          </label>
          <textarea id="reply" className="textarea" value={reply} onChange={(e) => setReply(e.target.value)} maxLength={5000} placeholder="Answer a question or add details" />
          <button className="btn btn-sm" disabled={busy || !reply.trim()}>
            Send
          </button>
        </form>
      </div>
      <aside className="stack-lg">
        <section className="panel panel-pad stack">
          <h2 className="display-3">Details</h2>
          <dl style={{ margin: 0, display: "grid", gap: 10 }}>
            {[
              ["Fabric", r.fabricMode === "catalog" ? r.fabricName ?? r.fabricKey : r.fabricMode === "recommend" ? "Recommendation requested" : "From your reference"],
              ["Measurements", r.measurementMode === "in_store" ? "To be taken at the studio" : r.verifiedMeasurements ? "Verified by your tailor" : "Provided by you"],
              ["Desired date", r.desiredDate ? `${formatDate(r.desiredDate)} (${humanize(r.dateFlexibility).toLowerCase()})` : "Not set"],
              ["Urgency", humanize(r.urgency)],
            ].map(([k, v]) => (
              <div key={k as string} className="spread" style={{ alignItems: "flex-start" }}>
                <dt className="muted small">{k}</dt>
                <dd style={{ margin: 0, textAlign: "right" }}>{v}</dd>
              </div>
            ))}
          </dl>
          {r.design ? (
            <div className="stack-sm">
              <h3 className="label">Design</h3>
              <p className="small">{r.design.selections.map((s) => s.valueName ?? `${s.groupName} ${s.number}${s.unit ?? ""}`).join(", ")}</p>
              {r.designVersionId ? (
                <Link className="link small" href={`/studio?designVersion=${r.designVersionId}`}>
                  Open in the studio
                </Link>
              ) : null}
            </div>
          ) : null}
          {r.notes ? (
            <div>
              <h3 className="label">Your notes</h3>
              <p className="small" style={{ whiteSpace: "pre-line" }}>
                {r.notes}
              </p>
            </div>
          ) : null}
          {m ? (
            <details>
              <summary className="link small">Measurements ({m.unit})</summary>
              <ul className="small muted" style={{ columns: 2, marginTop: 8 }}>
                {Object.entries(m.valuesMm).map(([k, v]) => (
                  <li key={k}>
                    {humanize(k)}: {formatLength(v, m.unit)}
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </section>
        {r.references.length ? (
          <section className="panel panel-pad stack">
            <h2 className="display-3">References</h2>
            <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: 10, gridTemplateColumns: "repeat(auto-fill, minmax(110px, 1fr))" }}>
              {r.references.map((ref) => (
                <li key={ref.uploadId} className="stack-sm">
                  <span style={{ position: "relative", display: "block", aspectRatio: "1", background: "var(--surface-2)" }}>
                    {ref.removed ? (
                      <span className="tiny muted" style={{ position: "absolute", inset: 0, display: "grid", placeItems: "center" }}>
                        Removed
                      </span>
                    ) : (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img src={`${ref.thumb}${access}`} alt={`Reference: ${ref.tag}`} style={{ width: "100%", height: "100%", objectFit: "cover" }} />
                    )}
                  </span>
                  <span className="tiny muted">{humanize(ref.tag)}</span>
                </li>
              ))}
            </ul>
          </section>
        ) : null}
      </aside>
    </div>
  );
}
