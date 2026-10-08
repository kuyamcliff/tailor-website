"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { CalendarDays, CheckCircle2, FileText, MessageCircle, Package, Ruler } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { useAccessToken, useNow, useUrlFlag } from "@/lib/client-hooks";
import { formatDate, formatDateTime, humanize, paymentLabels } from "@/lib/format";
import type { Order } from "@/lib/types";
import { Price } from "@/components/ui/price";
import { StatusBadge } from "@/components/ui/status-badge";
import { useConfig } from "@/components/providers/config";
import { PaymentPanel } from "./payment-panel";
import { Timeline } from "./timeline";
import styles from "./order.module.css";

const deliveryLabels: Record<string, string> = {
  not_started: "Not started", preparing: "Being prepared", ready_for_pickup: "Ready for pickup", dispatched: "On its way", delivered: "Delivered", picked_up: "Collected",
};

export function OrderView({ id }: { id: string }) {
  const cfg = useConfig();
  const token = useAccessToken("order", id);
  const placed = useUrlFlag("placed");
  const now = useNow();
  const q = useQuery({
    queryKey: ["order", id, token],
    enabled: token !== null,
    queryFn: () => api<{ order: Order; onlinePayments: boolean }>(`/orders/${id}`, { accessToken: token || undefined }),
    refetchInterval: (query) => {
      const o = query.state.data?.order;
      return o && o.payments.some((p) => ["created", "pending", "customer_action_required", "processing"].includes(p.status)) ? 4000 : 60000;
    },
  });

  if (q.isLoading || token === null) return <div className="container section-tight"><div className="skeleton" style={{ height: 480 }} /></div>;
  if (q.error) {
    const notFound = q.error instanceof ApiError && q.error.status === 404;
    return (
      <div className="container-narrow section-tight stack-lg">
        <h1 className="display-2">{notFound ? "We could not open this order." : "This order could not load."}</h1>
        <p className="lede">
          {notFound
            ? "Use the link from your confirmation message, or sign in to the account you ordered with."
            : "Please check your connection and try again."}
        </p>
        <div className="row-wrap">
          {notFound ? (
            <Link href={`/account/sign-in?next=/orders/${id}`} className="btn btn-primary">
              Sign in
            </Link>
          ) : (
            <button className="btn btn-primary" onClick={() => q.refetch()}>
              Try again
            </button>
          )}
          <Link href="/support" className="btn">
            Contact us
          </Link>
        </div>
      </div>
    );
  }
  const o = q.data!.order;
  const due = o.status === "cancelled" || o.status === "refunded" ? 0 : Math.max(0, o.depositRequiredMinor - o.amountPaidMinor) || o.balanceMinor;
  const tokenQuery = token ? `?token=${token}` : "";
  const recentPayment = o.payments.find((p) => p.status === "succeeded" && p.succeededAt && now - new Date(p.succeededAt).getTime() < 30 * 60 * 1000);

  return (
    <div className="container section-tight">
      {placed ? (
        <div className="notice notice-success" role="status" style={{ marginBottom: 24 }}>
          <CheckCircle2 size={18} aria-hidden style={{ color: "var(--success)", flex: "none", marginTop: 2 }} />
          <span>
            Thank you. Order <strong>{o.number}</strong> has been received.{" "}
            {token ? "Bookmark this page or keep the link we sent you to follow your order." : ""}
          </span>
        </div>
      ) : null}
      <header className={styles.head}>
        <div className="stack-sm">
          <span className="eyebrow">Order {o.number}</span>
          <h1 className="display-2">{o.statusLabel}</h1>
          <p className="muted">Placed {formatDate(o.createdAt)}{o.dueDate ? ` · Expected ${formatDate(o.dueDate)}` : ""}</p>
        </div>
        <div className="row-wrap">
          <StatusBadge status={o.paymentStatus} label={paymentLabels[o.paymentStatus]} />
          <span className="badge">
            <Package size={13} aria-hidden /> {o.fulfillmentMethod === "pickup" ? "Pickup" : "Delivery"}: {deliveryLabels[o.deliveryStatus] ?? humanize(o.deliveryStatus)}
          </span>
        </div>
      </header>

      <div className={styles.layout}>
        <div className="stack-lg">
          {recentPayment ? (
            <p className="notice notice-success" role="status">
              <CheckCircle2 size={18} aria-hidden style={{ color: "var(--success)", flex: "none", marginTop: 2 }} />
              <span>
                Payment received: <Price minor={recentPayment.amountMinor} currency={o.currency} />
                {recentPayment.simulated ? " (test payment, no money moved)" : ""}. Thank you.
              </span>
            </p>
          ) : null}
          {due > 0 && o.status !== "draft" ? (
            <PaymentPanel order={o} accessToken={token || undefined} online={q.data!.onlinePayments} onSettled={() => q.refetch()} />
          ) : null}

          <section className="panel panel-pad" aria-labelledby="progress">
            <h2 id="progress" className="display-3" style={{ marginBottom: 20 }}>
              Progress
            </h2>
            <Timeline order={o} />
          </section>

          {o.appointments.length ? (
            <section className="panel panel-pad" aria-labelledby="appts">
              <h2 id="appts" className="display-3" style={{ marginBottom: 16 }}>
                Appointments
              </h2>
              <ul className={styles.list}>
                {o.appointments.map((a) => (
                  <li key={a.id}>
                    <CalendarDays size={18} aria-hidden />
                    <span>
                      <strong>{humanize(a.type)}</strong> · {formatDateTime(a.startsAt, cfg.business.timezone)}
                    </span>
                    <StatusBadge status={a.status} />
                    <Link href={`/appointments/${a.id}${tokenQuery}`} className="link small">
                      Manage
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          {o.notes.length || o.fittings.length ? (
            <section className="panel panel-pad" aria-labelledby="notes">
              <h2 id="notes" className="display-3" style={{ marginBottom: 16 }}>
                From your tailor
              </h2>
              <ul className={styles.notes}>
                {o.notes.map((n) => (
                  <li key={n.id}>
                    <p>{n.body}</p>
                    <span className="tiny muted">{formatDateTime(n.createdAt)}</span>
                  </li>
                ))}
                {o.fittings.map((f) => (
                  <li key={f.id}>
                    <p className="row">
                      <Ruler size={15} aria-hidden /> Fitting notes
                    </p>
                    {f.customerNotes ? <p className="muted">{f.customerNotes}</p> : null}
                    {f.adjustments.length ? (
                      <ul className="small muted">
                        {f.adjustments.map((a, i) => (
                          <li key={i}>
                            {a.area}: {a.change}
                          </li>
                        ))}
                      </ul>
                    ) : null}
                    <span className="tiny muted">{formatDateTime(f.createdAt)}</span>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </div>

        <aside className="stack-lg">
          <section className="panel panel-pad" aria-labelledby="items">
            <h2 id="items" className="display-3" style={{ marginBottom: 16 }}>
              Summary
            </h2>
            <ul className={styles.items}>
              {o.items.map((it) => (
                <li key={it.id}>
                  <span>
                    <span className={styles.itemName}>{it.name}</span>
                    {it.description ? <span className="tiny muted">{it.description}</span> : null}
                    {it.designVersionId ? (
                      <Link className="link tiny" href={`/studio?designVersion=${it.designVersionId}`}>
                        View design
                      </Link>
                    ) : null}
                  </span>
                  <span className="tabular small">
                    {it.quantity > 1 ? `${it.quantity} × ` : ""}
                    <Price minor={it.unitPriceMinor} currency={o.currency} />
                  </span>
                </li>
              ))}
            </ul>
            <dl className={styles.totals}>
              <div><dt>Subtotal</dt><dd><Price minor={o.subtotalMinor} currency={o.currency} /></dd></div>
              {o.discountMinor ? <div><dt>Discount</dt><dd>-<Price minor={o.discountMinor} currency={o.currency} /></dd></div> : null}
              {o.deliveryMinor ? <div><dt>Delivery</dt><dd><Price minor={o.deliveryMinor} currency={o.currency} /></dd></div> : null}
              {o.taxMinor ? <div><dt>{cfg.business.taxLabel}{cfg.business.pricesIncludeTax ? " (included)" : ""}</dt><dd><Price minor={o.taxMinor} currency={o.currency} /></dd></div> : null}
              <div className={styles.total}><dt>Total</dt><dd><Price minor={o.totalMinor} currency={o.currency} /></dd></div>
              {o.depositRequiredMinor && o.depositRequiredMinor < o.totalMinor ? <div><dt>Deposit</dt><dd><Price minor={o.depositRequiredMinor} currency={o.currency} /></dd></div> : null}
              <div><dt>Paid</dt><dd><Price minor={o.amountPaidMinor - o.amountRefundedMinor} currency={o.currency} /></dd></div>
              <div><dt>Balance</dt><dd><Price minor={o.balanceMinor} currency={o.currency} /></dd></div>
            </dl>
          </section>

          {o.payments.length ? (
            <section className="panel panel-pad" aria-labelledby="payments">
              <h2 id="payments" className="display-3" style={{ marginBottom: 16 }}>
                Payments
              </h2>
              <ul className={styles.list}>
                {o.payments.map((p) => (
                  <li key={p.id}>
                    <span>
                      <Price minor={p.amountMinor} currency={o.currency} /> · {humanize(p.purpose)}
                      <span className="tiny muted" style={{ display: "block" }}>
                        {p.provider === "manual" ? "Recorded at the atelier" : p.provider === "mtn" ? "MTN Mobile Money" : "Orange Money"} · {formatDateTime(p.createdAt)}
                        {p.simulated ? " · Test payment, no money moved" : ""}
                      </span>
                      {p.failureMessage && p.status !== "succeeded" ? <span className="tiny" style={{ color: "var(--danger)", display: "block" }}>{p.failureMessage}</span> : null}
                    </span>
                    <StatusBadge status={p.status} />
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          <section className="panel panel-pad stack" aria-labelledby="docs">
            <h2 id="docs" className="display-3">
              Documents and help
            </h2>
            <div className={styles.docs}>
              {(["summary", "invoice", "receipt"] as const).map((k) =>
                k === "receipt" && o.amountPaidMinor === 0 ? null : (
                  <Link key={k} href={`/documents/orders/${o.id}/${k}${tokenQuery}`} className="btn btn-sm" target="_blank">
                    <FileText size={14} aria-hidden /> {humanize(k === "summary" ? "order summary" : k)}
                  </Link>
                ),
              )}
              {o.measurementVersionId ? (
                <Link href={`/documents/orders/${o.id}/measurements${tokenQuery}`} className="btn btn-sm" target="_blank">
                  <FileText size={14} aria-hidden /> Measurement sheet
                </Link>
              ) : null}
            </div>
            <Link href={`/support?order=${o.id}`} className="text-link">
              <MessageCircle size={15} aria-hidden /> Get help with this order
            </Link>
          </section>
        </aside>
      </div>
    </div>
  );
}
