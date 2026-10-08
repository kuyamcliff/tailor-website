"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { formatDate, humanize } from "@/lib/format";
import { formatMoney } from "@/lib/money";
import { useConfig } from "@/components/providers/config";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";
import chart from "./analytics.module.css";

type Data = {
  periodDays: number;
  values: Record<string, number>;
  rates: Record<string, number | null>;
  topGarments: { label: string; count: number }[];
  topFabrics: { label: string; count: number }[];
  requestsByWeek: { week: string; count: number }[];
  revenueByWeek: { week: string; amount: number }[];
  ordersByStatus: { label: string; count: number }[];
};

const pct = (v: number | null | undefined) => (v === null || v === undefined ? "Not enough data" : `${Math.round(v * 100)}%`);

export function OwnerAnalytics() {
  const cfg = useConfig();
  const cur = cfg.business.currency;
  const [period, setPeriod] = useState("90");
  const q = useQuery({ queryKey: ["owner", "analytics", period], queryFn: () => api<Data>(`/owner/analytics?period=${period}`) });
  const d = q.data;
  const money = (m: number) => formatMoney(Math.round(m), cur);
  return (
    <>
      <PageHead
        title="Analytics"
        sub="Money figures count only payments confirmed by the provider or recorded by staff. Test payments are excluded."
        actions={
          <label className="row small">
            <span>Period</span>
            <select className="select" value={period} onChange={(e) => setPeriod(e.target.value)} style={{ width: "auto" }}>
              <option value="30">Last 30 days</option>
              <option value="90">Last 90 days</option>
              <option value="365">Last 12 months</option>
            </select>
          </label>
        }
      />
      {!d ? (
        <div className="skeleton" style={{ height: 420 }} />
      ) : (
        <>
          <div className={styles.split}>
            <section aria-labelledby="money-h" className="stack-sm">
              <h2 id="money-h" className={styles.h2}>
                Money
              </h2>
              <dl className={styles.dl}>
                <dt>Collected</dt>
                <dd className="tabular">{money(d.values.revenueCollected ?? 0)}</dd>
                <dt>Of which deposits</dt>
                <dd className="tabular">{money(d.values.depositsCollected ?? 0)}</dd>
                <dt>Still owed on open orders</dt>
                <dd className="tabular">{money(d.values.outstandingBalances ?? 0)}</dd>
                <dt>Average order</dt>
                <dd className="tabular">{money(d.values.averageOrderValue ?? 0)}</dd>
              </dl>
            </section>
            <section aria-labelledby="flow-h" className="stack-sm">
              <h2 id="flow-h" className={styles.h2}>
                Requests to orders
              </h2>
              <dl className={styles.dl}>
                <dt>Requests</dt>
                <dd className="tabular">{d.values.requests ?? 0}</dd>
                <dt>Became orders</dt>
                <dd className="tabular">{pct(d.rates.requestConversion)}</dd>
                <dt>Quotes accepted</dt>
                <dd className="tabular">
                  {pct(d.rates.quoteAcceptance)}
                  {d.values.quotesExpired ? <span className="muted"> · {d.values.quotesExpired} expired</span> : null}
                </dd>
                <dt>Orders</dt>
                <dd className="tabular">{d.values.orders ?? 0}</dd>
                <dt>Consultations that led to an order</dt>
                <dd className="tabular">{pct(d.rates.appointmentConversion)}</dd>
              </dl>
            </section>
            <section aria-labelledby="svc-h" className="stack-sm">
              <h2 id="svc-h" className={styles.h2}>
                Service
              </h2>
              <dl className={styles.dl}>
                <dt>First reply to messages</dt>
                <dd className="tabular">{d.values.supportFirstResponseHours ? `${d.values.supportFirstResponseHours.toFixed(1)} hours on average` : "No replies yet"}</dd>
                <dt>Order to completion</dt>
                <dd className="tabular">{d.values.orderCompletionDays ? `${Math.round(d.values.orderCompletionDays)} days on average` : "No completed orders yet"}</dd>
                <dt>Appointments booked</dt>
                <dd className="tabular">{d.values.appointmentsBooked ?? 0}</dd>
              </dl>
            </section>
          </div>

          <div className={styles.split}>
            <WeeklyBars title="Money collected per week" rows={d.revenueByWeek.map((r) => ({ week: r.week, value: r.amount }))} format={money} />
            <WeeklyBars title="Requests per week" rows={d.requestsByWeek.map((r) => ({ week: r.week, value: r.count }))} format={(n) => String(n)} />
          </div>
          <div className={styles.split}>
            <RankBars title="Most requested garments" rows={d.topGarments} />
            <RankBars title="Most requested fabrics" rows={d.topFabrics} />
            <RankBars title="Open orders by stage" rows={d.ordersByStatus.map((r) => ({ ...r, label: humanize(r.label) }))} />
          </div>
        </>
      )}
    </>
  );
}

// WeeklyBars: one series over time, so one hue and no legend; the title names the series.
function WeeklyBars({ title, rows, format }: { title: string; rows: { week: string; value: number }[]; format: (n: number) => string }) {
  const [hover, setHover] = useState<number | null>(null);
  const max = Math.max(1, ...rows.map((r) => r.value));
  const W = 560;
  const H = 180;
  const pad = 24;
  const bw = rows.length ? Math.max(4, Math.min(28, (W - pad) / rows.length - 4)) : 0;
  const step = rows.length ? (W - pad) / rows.length : 0;
  const h = hover !== null ? rows[hover] : null;
  return (
    <figure className={chart.figure}>
      <figcaption className={styles.h2}>{title}</figcaption>
      {rows.length ? (
        <>
          <div className={chart.plot}>
            <svg viewBox={`0 0 ${W} ${H + 22}`} role="img" aria-label={`${title}, ${rows.length} weeks, highest ${format(max)}`}>
              <line x1={pad} x2={W} y1={H} y2={H} className={chart.axis} />
              <text x={0} y={12} className={chart.tick}>
                {format(max)}
              </text>
              {rows.map((r, i) => {
                const bh = Math.max(1, (r.value / max) * (H - 20));
                const x = pad + i * step + (step - bw) / 2;
                return (
                  <g key={r.week} onMouseEnter={() => setHover(i)} onMouseLeave={() => setHover(null)} onFocus={() => setHover(i)} onBlur={() => setHover(null)} tabIndex={0} aria-label={`Week of ${formatDate(r.week, "short")}: ${format(r.value)}`}>
                    <rect x={pad + i * step} y={0} width={step} height={H} fill="transparent" />
                    <path d={roundedTop(x, H - bh, bw, bh, Math.min(4, bw / 2))} className={chart.bar} data-active={hover === i} />
                  </g>
                );
              })}
              {rows.length > 1 ? (
                <>
                  <text x={pad} y={H + 16} className={chart.tick}>
                    {formatDate(rows[0]!.week, "short")}
                  </text>
                  <text x={W} y={H + 16} textAnchor="end" className={chart.tick}>
                    {formatDate(rows[rows.length - 1]!.week, "short")}
                  </text>
                </>
              ) : null}
            </svg>
            {h ? (
              <div className={chart.tip} style={{ left: `${((pad + (hover ?? 0) * step + step / 2) / W) * 100}%` }} role="status">
                <span className="tiny muted">Week of {formatDate(h.week, "short")}</span>
                <strong className="tabular">{format(h.value)}</strong>
              </div>
            ) : null}
          </div>
          <details>
            <summary className="tiny muted">Show as a table</summary>
            <table className="table">
              <tbody>
                {rows.map((r) => (
                  <tr key={r.week}>
                    <td>{formatDate(r.week, "short")}</td>
                    <td className="tabular">{format(r.value)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </details>
        </>
      ) : (
        <p className="small muted">Nothing in this period yet.</p>
      )}
    </figure>
  );
}

function RankBars({ title, rows }: { title: string; rows: { label: string; count: number }[] }) {
  const max = Math.max(1, ...rows.map((r) => r.count));
  return (
    <figure className={chart.figure}>
      <figcaption className={styles.h2}>{title}</figcaption>
      {rows.length ? (
        <ul className={chart.rank}>
          {rows.map((r) => (
            <li key={r.label} title={`${r.label}: ${r.count}`}>
              <span className="small">{r.label}</span>
              <span className={chart.track} aria-hidden>
                <span className={chart.fill} style={{ width: `${(r.count / max) * 100}%` }} />
              </span>
              <span className="small tabular">{r.count}</span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="small muted">Nothing in this period yet.</p>
      )}
    </figure>
  );
}

function roundedTop(x: number, y: number, w: number, h: number, r: number) {
  const rr = Math.min(r, h);
  return `M${x},${y + h} V${y + rr} Q${x},${y} ${x + rr},${y} H${x + w - rr} Q${x + w},${y} ${x + w},${y + rr} V${y + h} Z`;
}
