"use client";

import Link from "next/link";
import { useAccessToken } from "@/lib/client-hooks";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { CalendarClock } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { rememberLink } from "@/lib/links";

import { formatDate, humanize } from "@/lib/format";
import type { Quote } from "@/lib/types";
import { Price } from "@/components/ui/price";
import { StatusBadge } from "@/components/ui/status-badge";
import { Sheet } from "@/components/ui/sheet";
import { useToast } from "@/components/providers/toast";

type Resp = { quote: Quote; business: { name: string; taxLabel: string; pricesIncludeTax: boolean } };

const statusText: Record<Quote["status"], string> = {
  draft: "Draft",
  sent: "Awaiting your decision",
  accepted: "Accepted",
  declined: "Declined",
  changes_requested: "Changes requested",
  expired: "Expired",
  withdrawn: "Withdrawn",
};

export function QuoteView({ id }: { id: string }) {
  const router = useRouter();
  const toast = useToast();
  const token = useAccessToken("quote", id);
  const [dialog, setDialog] = useState<null | "decline" | "changes">(null);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const q = useQuery({
    queryKey: ["quote", id, token],
    enabled: token !== null,
    queryFn: () => api<Resp>(`/quotes/${id}`, { accessToken: token || undefined }),
  });

  async function decide(action: "accept" | "decline" | "request-changes") {
    const rev = q.data?.quote.current;
    if (!rev) return;
    setBusy(true);
    setError("");
    try {
      const r = await api<{ orderId?: string; orderNumber?: string }>(`/quotes/${id}/${action}`, {
        body: { revisionId: rev.id, note },
        accessToken: token || undefined,
      });
      setDialog(null);
      if (action === "accept" && r?.orderId) {
        // The quote link token also opens the order.
        if (token) rememberLink({ kind: "order", id: r.orderId, number: r.orderNumber ?? "", token });
        router.push(`/orders/${r.orderId}${token ? `?token=${token}` : ""}`);
        return;
      }
      toast(
        action === "decline" ? "We have recorded your decision." : "Thank you. Your tailor will send an updated quote.",
      );
      await q.refetch();
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : "Please try again.";
      setError(msg);
      if (e instanceof ApiError && (e.code === "quote_expired" || e.code === "revision_changed")) await q.refetch();
    } finally {
      setBusy(false);
    }
  }

  if (q.isLoading || token === null)
    return (
      <div className="container section-tight">
        <div className="skeleton" style={{ height: 420 }} />
      </div>
    );
  if (!q.data)
    return (
      <div className="container-narrow section-tight stack-lg">
        <h1 className="display-2">We could not open this quote.</h1>
        <p className="lede">Use the link from your message, or sign in to your account.</p>
        <Link href={`/account/sign-in?next=/quotes/${id}`} className="btn btn-primary">
          Sign in
        </Link>
      </div>
    );
  const { quote: qt, business } = q.data;
  const rev = qt.current!;
  const open = qt.status === "sent" && !qt.expired;
  const product = rev.lines.filter((l) => l.kind !== "discount" && l.kind !== "delivery");

  return (
    <div className="container-narrow section-tight stack-lg">
      <header className="stack-sm">
        <span className="eyebrow">Quote {qt.number}</span>
        <h1 className="display-2">
          {qt.expired && qt.status === "sent" ? "This quote has expired" : statusText[qt.status]}
        </h1>
        <div className="row-wrap">
          <StatusBadge
            status={qt.expired ? "expired" : qt.status}
            label={qt.expired ? "Expired" : statusText[qt.status]}
          />
          <span className="badge">
            <CalendarClock size={13} aria-hidden /> Valid until {formatDate(rev.expiresAt)}
          </span>
          {rev.revisionNo > 1 ? <span className="badge">Revision {rev.revisionNo}</span> : null}
        </div>
      </header>

      {error ? (
        <p className="notice notice-danger" role="alert">
          {error}
        </p>
      ) : null}
      {qt.expired ? (
        <p className="notice notice-warning">
          The validity period has ended. Ask for an updated quote and your tailor will send a new one.
        </p>
      ) : null}

      <section className="panel panel-pad stack">
        <table className="table" style={{ border: 0 }}>
          <thead>
            <tr>
              <th>Item</th>
              <th className="num">Qty</th>
              <th className="num">Amount</th>
            </tr>
          </thead>
          <tbody>
            {product.map((l, i) => (
              <tr key={i}>
                <td>
                  {l.description}
                  <span className="tiny muted" style={{ display: "block" }}>
                    {humanize(l.kind)}
                  </span>
                </td>
                <td className="num">{l.quantity}</td>
                <td className="num">
                  <Price minor={l.totalMinor} currency={rev.currency} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <dl style={{ margin: 0, display: "grid", gap: 6, maxWidth: 360, marginLeft: "auto", width: "100%" }}>
          {[
            ["Subtotal", rev.subtotalMinor],
            ...(rev.discountMinor ? [["Discount", -rev.discountMinor]] : []),
            ...(rev.deliveryMinor ? [["Delivery", rev.deliveryMinor]] : []),
            ...(rev.taxMinor
              ? [[`${business.taxLabel}${business.pricesIncludeTax ? " (included)" : ""}`, rev.taxMinor]]
              : []),
          ].map(([k, v]) => (
            <div key={k as string} className="spread">
              <dt className="muted">{k}</dt>
              <dd style={{ margin: 0 }}>
                {(v as number) < 0 ? "-" : ""}
                <Price minor={Math.abs(v as number)} currency={rev.currency} />
              </dd>
            </div>
          ))}
          <div
            className="spread"
            style={{ borderTop: "1px solid var(--line)", paddingTop: 8, fontWeight: 600, fontSize: "1.1rem" }}
          >
            <dt>Total</dt>
            <dd style={{ margin: 0 }}>
              <Price minor={rev.totalMinor} currency={rev.currency} />
            </dd>
          </div>
        </dl>
      </section>

      <section className="panel panel-pad stack">
        <h2 className="display-3">Payment schedule</h2>
        <ul style={{ margin: 0, paddingLeft: 18 }} className="stack-sm">
          {rev.depositMinor > 0 ? (
            <li>
              Deposit of <Price minor={rev.depositMinor} currency={rev.currency} /> when you accept, before work starts.
            </li>
          ) : (
            <li>No deposit is required.</li>
          )}
          {rev.balanceMinor > 0 ? (
            <li>
              Balance of <Price minor={rev.balanceMinor} currency={rev.currency} /> when your garment is ready.
            </li>
          ) : null}
        </ul>
        {rev.estimatedReadyDate ? (
          <p className="muted">Estimated ready date: {formatDate(rev.estimatedReadyDate)}</p>
        ) : null}
        {rev.customerNotes ? (
          <div>
            <h3 className="label">Notes from your tailor</h3>
            <p style={{ whiteSpace: "pre-line" }}>{rev.customerNotes}</p>
          </div>
        ) : null}
        {rev.terms ? (
          <details>
            <summary className="link small">Terms</summary>
            <p className="small muted" style={{ whiteSpace: "pre-line", marginTop: 8 }}>
              {rev.terms}
            </p>
          </details>
        ) : null}
      </section>

      {open ? (
        <div className="row-wrap">
          <button className="btn btn-primary" disabled={busy} onClick={() => decide("accept")}>
            {busy ? <span className="spinner" aria-hidden /> : null} Accept quote
          </button>
          <button className="btn" disabled={busy} onClick={() => setDialog("changes")}>
            Request changes
          </button>
          <button className="btn btn-ghost" disabled={busy} onClick={() => setDialog("decline")}>
            Decline
          </button>
        </div>
      ) : qt.expired || qt.status === "changes_requested" ? (
        <button className="btn" onClick={() => setDialog("changes")} disabled={qt.status === "changes_requested"}>
          {qt.status === "changes_requested" ? "Changes requested" : "Ask for an updated quote"}
        </button>
      ) : qt.orderId ? (
        <Link className="btn btn-primary" href={`/orders/${qt.orderId}${token ? `?token=${token}` : ""}`}>
          View your order
        </Link>
      ) : null}
      {open ? (
        <p className="tiny muted">Accepting creates your order with exactly the items and prices above.</p>
      ) : null}

      <Sheet
        open={dialog !== null}
        onClose={() => setDialog(null)}
        title={dialog === "decline" ? "Decline quote" : "Request changes"}
        side="center"
        footer={
          <div className="row-wrap" style={{ justifyContent: "flex-end" }}>
            <button className="btn btn-ghost" onClick={() => setDialog(null)}>
              Cancel
            </button>
            <button
              className="btn btn-primary"
              disabled={busy || (dialog === "changes" && !note.trim())}
              onClick={() => decide(dialog === "decline" ? "decline" : "request-changes")}
            >
              {dialog === "decline" ? "Decline" : "Send request"}
            </button>
          </div>
        }
      >
        <div className="field">
          <label htmlFor="decision-note">
            {dialog === "decline" ? "Would you like to tell us why? (optional)" : "What would you like to change?"}
          </label>
          <textarea
            id="decision-note"
            className="textarea"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            maxLength={2000}
          />
        </div>
      </Sheet>
    </div>
  );
}
