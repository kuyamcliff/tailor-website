"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { formatDate, formatTime, humanize } from "@/lib/format";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

type Dash = {
  counts: Record<string, number>;
  timezone: string;
  todayAppointments: {
    id: string;
    number: string;
    type: string;
    status: string;
    starts_at: string;
    customer: string;
  }[];
  newRequestList: {
    id: string;
    number: string;
    customer: string;
    garment: string;
    occasion: string;
    urgency: string;
    created_at: string;
  }[];
  overdueList: { id: string; number: string; status: string; due_date: string; customer: string }[];
  lowStockList: { key: string; name: string; stock_status: string; stock_meters: number | null }[];
  recentActivity?: { action: string; object_type: string; actor: string | null; created_at: string }[];
};

const attention: [key: string, label: (n: number) => string, href: string][] = [
  ["newRequests", (n) => `${n} new ${n === 1 ? "request" : "requests"} to review`, "/owner/requests?status=new"],
  ["draftQuotes", (n) => `${n} draft ${n === 1 ? "quote" : "quotes"} not sent yet`, "/owner/quotes?status=draft"],
  ["ordersAttention", (n) => `${n} ${n === 1 ? "order needs" : "orders need"} action`, "/owner/orders"],
  [
    "overdueOrders",
    (n) => `${n} ${n === 1 ? "order is" : "orders are"} past the due date`,
    "/owner/orders?overdue=true",
  ],
  [
    "awaitingDeposit",
    (n) => `${n} ${n === 1 ? "order is" : "orders are"} waiting for a deposit`,
    "/owner/orders?status=awaiting_customer",
  ],
  [
    "paymentsPending",
    (n) => `${n} ${n === 1 ? "payment is" : "payments are"} still in progress`,
    "/owner/payments?status=pending",
  ],
  [
    "unreadSupport",
    (n) => `${n} ${n === 1 ? "conversation has" : "conversations have"} unread messages`,
    "/owner/support?unread=true",
  ],
  ["lowStockFabrics", (n) => `${n} ${n === 1 ? "fabric is" : "fabrics are"} low or out of stock`, "/owner/fabrics"],
];

export function OwnerOverview() {
  const q = useQuery({
    queryKey: ["owner", "dashboard"],
    queryFn: () => api<Dash>("/owner/dashboard"),
    refetchInterval: 60_000,
  });
  const d = q.data;
  const items = d ? attention.filter(([k]) => (d.counts[k] ?? 0) > 0) : [];
  return (
    <>
      <PageHead title="Overview" sub={formatDate(new Date(), "long", d?.timezone)} />
      {q.isLoading ? <div className="skeleton" style={{ height: 320 }} /> : null}
      {d ? (
        <div className={styles.split}>
          <section aria-labelledby="att-h" className="stack-sm">
            <h2 id="att-h" className={styles.h2}>
              Needs attention
            </h2>
            {items.length ? (
              <ul className="list-rows">
                {items.map(([k, label, href]) => (
                  <li key={k}>
                    <Link className="list-row" href={href}>
                      {label(d.counts[k] ?? 0)}
                      <span aria-hidden>→</span>
                    </Link>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="muted">Nothing is waiting on you.</p>
            )}
          </section>
          <section aria-labelledby="today-h" className="stack-sm">
            <h2 id="today-h" className={styles.h2}>
              Today
            </h2>
            {d.todayAppointments.length ? (
              <ul className="list-rows">
                {d.todayAppointments.map((a) => (
                  <li key={a.id}>
                    <Link className="list-row" href={`/owner/appointments?date=${a.starts_at.slice(0, 10)}`}>
                      <span>
                        <strong className="tabular">{formatTime(a.starts_at, d.timezone)}</strong> {a.customer}
                      </span>
                      <span className="small muted">{humanize(a.type)}</span>
                    </Link>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="muted">No appointments today.</p>
            )}
          </section>
        </div>
      ) : null}
      {d?.newRequestList.length ? (
        <section aria-labelledby="req-h" className="stack-sm">
          <h2 id="req-h" className={styles.h2}>
            Latest requests
          </h2>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Request</th>
                  <th>Customer</th>
                  <th>Garment</th>
                  <th>Occasion</th>
                  <th>Urgency</th>
                  <th>Received</th>
                </tr>
              </thead>
              <tbody>
                {d.newRequestList.map((r) => (
                  <tr key={r.id}>
                    <td>
                      <Link className="link" href={`/owner/requests/${r.id}`}>
                        {r.number}
                      </Link>
                    </td>
                    <td>{r.customer}</td>
                    <td>{r.garment}</td>
                    <td>{humanize(r.occasion)}</td>
                    <td>{r.urgency === "standard" ? "" : humanize(r.urgency)}</td>
                    <td>{formatDate(r.created_at, "short", d.timezone)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ) : null}
      {d?.overdueList.length || d?.lowStockList.length ? (
        <div className={styles.split}>
          {d.overdueList.length ? (
            <section aria-labelledby="od-h" className="stack-sm">
              <h2 id="od-h" className={styles.h2}>
                Past due date
              </h2>
              <ul className="list-rows">
                {d.overdueList.map((o) => (
                  <li key={o.id}>
                    <Link className="list-row" href={`/owner/orders/${o.id}`}>
                      <span>
                        {o.number} · {o.customer}
                      </span>
                      <span className="small" style={{ color: "var(--danger)" }}>
                        due {formatDate(o.due_date, "short")}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
          {d.lowStockList.length ? (
            <section aria-labelledby="ls-h" className="stack-sm">
              <h2 id="ls-h" className={styles.h2}>
                Fabric stock
              </h2>
              <ul className="list-rows">
                {d.lowStockList.map((f) => (
                  <li key={f.key} className="list-row">
                    <span>{f.name}</span>
                    <span className="small muted">
                      {humanize(f.stock_status)}
                      {f.stock_meters !== null ? `, ${f.stock_meters} m` : ""}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </div>
      ) : null}
      {d?.recentActivity?.length ? (
        <section aria-labelledby="act-h" className="stack-sm">
          <h2 id="act-h" className={styles.h2}>
            Recent activity
          </h2>
          <ul className={styles.activity}>
            {d.recentActivity.map((a, i) => (
              <li key={i}>
                <span className="small muted tabular">{formatDate(a.created_at, "short", d.timezone)}</span>
                <span className="small">
                  {a.actor ?? "System"}: {humanize(a.action.replace(".", " "))}
                </span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </>
  );
}
