"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Printer } from "lucide-react";
import { api } from "@/lib/api";
import { accessTokenFromUrl } from "@/lib/links";
import { formatDate, humanize } from "@/lib/format";
import { formatMoney } from "@/lib/money";
import { fromMM } from "@/lib/units";
import type { MeasurementField, MeasurementVersion, Order } from "@/lib/types";
import { useConfig } from "@/components/providers/config";
import styles from "./document.module.css";

const titles = { summary: "Order summary", invoice: "Invoice", receipt: "Receipt", measurements: "Measurement sheet" } as const;

// Documents are rendered from the order's stored snapshot (prices, items, measurement version), so
// they never change when the catalog does. Use the browser's print dialog to save as PDF.
export function OrderDocument({ id, kind }: { id: string; kind: keyof typeof titles }) {
  const cfg = useConfig();
  const b = cfg.business;
  const [token, setToken] = useState<string | null>(null);
  useEffect(() => setToken(accessTokenFromUrl("order", id)), [id]);
  const order = useQuery({
    queryKey: ["order", id, token],
    enabled: token !== null,
    queryFn: () => api<{ order: Order }>(`/orders/${id}`, { accessToken: token || undefined }),
  });
  const fields = useQuery({ queryKey: ["fields-all"], queryFn: () => api<MeasurementField[]>("/measurements/fields"), enabled: kind === "measurements" });
  const mv = useQuery({
    queryKey: ["doc-mv", order.data?.order.measurementVersionId],
    enabled: kind === "measurements" && Boolean(order.data?.order.measurementVersionId),
    queryFn: () => api<{ version: MeasurementVersion }>(`/orders/${id}/measurements`, { accessToken: token || undefined }).catch(() => null),
  });
  if (order.isLoading || token === null) return <div className="container section-tight"><div className="skeleton" style={{ height: 400 }} /></div>;
  if (!order.data) return <div className="container section-tight"><p className="notice notice-danger">This document could not be opened.</p></div>;
  const o = order.data.order;
  const money = (m: number) => formatMoney(m, o.currency, b.locale);
  const paid = o.payments.filter((p) => p.status === "succeeded" || p.status === "partially_refunded" || p.status === "refunded");
  return (
    <div className={styles.page}>
      <div className={`no-print ${styles.toolbar}`}>
        <button className="btn btn-sm btn-cream" onClick={() => window.print()}>
          <Printer size={15} aria-hidden /> Print or save as PDF
        </button>
      </div>
      <article className={styles.sheet}>
        <header className={styles.head}>
          <div>
            <p className={styles.brand}>{b.name || "Atelier"}</p>
            <p>{[b.address.line1, b.address.line2, b.address.city, b.address.country].filter(Boolean).join(", ")}</p>
            <p>{[b.phone, b.email].filter(Boolean).join(" · ")}</p>
          </div>
          <div className={styles.meta}>
            <h1>{titles[kind]}</h1>
            <p>Order {o.number}</p>
            <p>Date {formatDate(kind === "receipt" && paid[0]?.succeededAt ? paid[0].succeededAt : o.createdAt)}</p>
          </div>
        </header>
        <section className={styles.parties}>
          <div>
            <h2>Customer</h2>
            <p>{o.contact.name}</p>
            <p>{o.contact.phone}</p>
            {o.contact.email ? <p>{o.contact.email}</p> : null}
          </div>
          <div>
            <h2>{o.fulfillmentMethod === "pickup" ? "Pickup" : "Delivery"}</h2>
            {o.deliveryAddress ? <p>{[o.deliveryAddress.line1, o.deliveryAddress.line2, o.deliveryAddress.city].filter(Boolean).join(", ")}</p> : <p>Collect from the studio</p>}
            {o.dueDate ? <p>Expected {formatDate(o.dueDate)}</p> : null}
          </div>
        </section>
        {kind === "measurements" ? (
          mv.data ? (
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>Measurement</th>
                  <th className={styles.num}>Value ({mv.data.version.unit})</th>
                </tr>
              </thead>
              <tbody>
                {mv.data.version.heightMm ? (
                  <tr>
                    <td>Height</td>
                    <td className={styles.num}>{fromMM(mv.data.version.heightMm, mv.data.version.unit)}</td>
                  </tr>
                ) : null}
                {Object.entries(mv.data.version.valuesMm).map(([k, mm]) => (
                  <tr key={k}>
                    <td>{fields.data?.find((f) => f.key === k)?.label ?? humanize(k)}</td>
                    <td className={styles.num}>{fromMM(mm, mv.data!.version.unit)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <p>Your measurement sheet is available from the atelier. Ask us at your next fitting.</p>
          )
        ) : (
          <>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>Item</th>
                  <th className={styles.num}>Qty</th>
                  <th className={styles.num}>Unit price</th>
                  <th className={styles.num}>Amount</th>
                </tr>
              </thead>
              <tbody>
                {o.items.map((it) => (
                  <tr key={it.id}>
                    <td>
                      {it.name}
                      {it.description ? <span className={styles.sub}>{it.description}</span> : null}
                    </td>
                    <td className={styles.num}>{it.quantity}</td>
                    <td className={styles.num}>{money(it.unitPriceMinor)}</td>
                    <td className={styles.num}>{money(it.totalMinor)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <dl className={styles.totals}>
              <div><dt>Subtotal</dt><dd>{money(o.subtotalMinor)}</dd></div>
              {o.discountMinor ? <div><dt>Discount</dt><dd>-{money(o.discountMinor)}</dd></div> : null}
              {o.deliveryMinor ? <div><dt>Delivery</dt><dd>{money(o.deliveryMinor)}</dd></div> : null}
              {o.taxMinor ? <div><dt>{b.taxLabel}{b.pricesIncludeTax ? " included" : ""}</dt><dd>{money(o.taxMinor)}</dd></div> : null}
              <div className={styles.grand}><dt>Total</dt><dd>{money(o.totalMinor)}</dd></div>
              {kind !== "summary" ? (
                <>
                  <div><dt>Paid</dt><dd>{money(o.amountPaidMinor - o.amountRefundedMinor)}</dd></div>
                  <div><dt>Balance due</dt><dd>{money(o.balanceMinor)}</dd></div>
                </>
              ) : null}
            </dl>
            {kind === "receipt" && paid.length ? (
              <table className={styles.table}>
                <thead>
                  <tr>
                    <th>Payment</th>
                    <th>Method</th>
                    <th className={styles.num}>Amount</th>
                  </tr>
                </thead>
                <tbody>
                  {paid.map((p) => (
                    <tr key={p.id}>
                      <td>{formatDate(p.succeededAt)} · {humanize(p.purpose)}{p.simulated ? " (test, no money moved)" : ""}</td>
                      <td>{p.provider === "manual" ? "Paid at the atelier" : p.provider === "mtn" ? "MTN Mobile Money" : "Orange Money"}</td>
                      <td className={styles.num}>{money(p.amountMinor - p.refundedMinor)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : null}
          </>
        )}
        <footer className={styles.foot}>
          <p>Thank you for choosing {b.name || "the atelier"}.</p>
        </footer>
      </article>
    </div>
  );
}
