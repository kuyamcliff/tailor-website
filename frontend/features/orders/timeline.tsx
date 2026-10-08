import { Check } from "lucide-react";
import { formatDateTime } from "@/lib/format";
import type { Order } from "@/lib/types";
import styles from "./timeline.module.css";

const bespokeStages = [
  ["awaiting_customer", "Deposit"],
  ["measurements_verified", "Measurements"],
  ["cutting", "Cutting"],
  ["sewing", "Sewing"],
  ["fitting", "Fitting"],
  ["ready", "Ready"],
  ["completed", "Collected"],
] as const;

const readyStages = [
  ["submitted", "Received"],
  ["under_review", "Preparing"],
  ["ready", "Ready"],
  ["delivered", "Delivered"],
] as const;

const order = [
  "draft",
  "submitted",
  "under_review",
  "quote_sent",
  "awaiting_customer",
  "deposit_paid",
  "measurements_pending",
  "measurements_verified",
  "material_pending",
  "patterning",
  "cutting",
  "sewing",
  "quality_check",
  "fitting_scheduled",
  "fitting",
  "alteration",
  "ready",
  "dispatched",
  "delivered",
  "completed",
];

// Timeline shows the production milestones with the customer-visible status history beneath.
export function Timeline({ order: o }: { order: Order }) {
  const stages = o.kind === "bespoke" ? bespokeStages : readyStages;
  const pos = order.indexOf(o.status);
  const ended = o.status === "cancelled" || o.status === "refunded";
  return (
    <div className="stack-lg">
      {!ended ? (
        <ol className={styles.track} aria-label="Milestones">
          {stages.map(([key, label]) => {
            const done = pos >= order.indexOf(key) && pos !== -1;
            const current = done && stages.findLast(([k]) => pos >= order.indexOf(k))?.[0] === key;
            return (
              <li
                key={key}
                className={`${styles.stage} ${done ? styles.done : ""} ${current ? styles.current : ""}`}
                aria-current={current ? "step" : undefined}
              >
                <span className={styles.marker}>{done ? <Check size={12} aria-hidden /> : null}</span>
                <span className={styles.label}>{label}</span>
              </li>
            );
          })}
        </ol>
      ) : null}
      <ol className={styles.history} aria-label="History">
        {[...o.history].reverse().map((h, i) => (
          <li key={i}>
            <span className={styles.dot} aria-hidden />
            <div>
              <p>
                <strong>{h.newStatus === h.oldStatus ? "Updated" : labelFor(h.newStatus)}</strong>
                {h.note ? <span className="muted"> · {h.note}</span> : null}
              </p>
              <span className="tiny muted">{formatDateTime(h.createdAt)}</span>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}

function labelFor(s: string) {
  const map: Record<string, string> = {
    submitted: "Order received",
    awaiting_customer: "Waiting for your deposit",
    deposit_paid: "Deposit received",
    measurements_pending: "Measurements needed",
    measurements_verified: "Measurements confirmed",
    material_pending: "Sourcing fabric",
    patterning: "Pattern making",
    cutting: "Cutting",
    sewing: "Sewing",
    quality_check: "Quality check",
    fitting_scheduled: "Fitting booked",
    fitting: "Fitting",
    alteration: "Adjustments",
    ready: "Ready",
    dispatched: "Dispatched",
    delivered: "Delivered",
    completed: "Completed",
    cancelled: "Cancelled",
    refunded: "Refunded",
    under_review: "Being prepared",
  };
  return map[s] ?? s.replaceAll("_", " ");
}
