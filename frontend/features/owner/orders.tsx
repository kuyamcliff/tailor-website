"use client";

import Link from "next/link";
import { Suspense, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { formatDate, formatDateTime, humanize, paymentLabels } from "@/lib/format";
import { exponentOf, toMinor } from "@/lib/money";
import type { Order } from "@/lib/types";
import { useToast } from "@/components/providers/toast";
import { StatusBadge } from "@/components/ui/status-badge";
import { Price } from "@/components/ui/price";
import { OwnerList } from "./owner-list";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

type Row = {
  id: string;
  number: string;
  kind: string;
  status: string;
  paymentStatus: string;
  totalMinor: number;
  paidMinor: number;
  currency: string;
  urgency: string;
  dueDate: string | null;
  customerName: string | null;
  createdAt: string;
};

const stageOptions: [string, string][] = [
  ["submitted", "Received"],
  ["under_review", "Under review"],
  ["awaiting_customer", "Awaiting customer"],
  ["deposit_paid", "Deposit paid"],
  ["measurements_pending", "Measurements pending"],
  ["cutting", "Cutting"],
  ["sewing", "Sewing"],
  ["fitting", "Fitting"],
  ["alteration", "Alteration"],
  ["ready", "Ready"],
  ["dispatched", "Dispatched"],
  ["delivered", "Delivered"],
  ["completed", "Completed"],
  ["cancelled", "Cancelled"],
];

export function OwnerOrders() {
  return (
    <>
      <PageHead title="Orders" />
      <Suspense>
        <OwnerList<Row>
          endpoint="/owner/orders"
          filters={[
            { key: "q", label: "Search", type: "search" },
            { key: "status", label: "Stage", type: "select", options: stageOptions },
            { key: "paymentStatus", label: "Payment", type: "select", options: Object.entries(paymentLabels) },
            { key: "kind", label: "Type", type: "select", options: [["bespoke", "Bespoke"], ["ready_made", "Ready to wear"]] },
          ]}
          rowKey={(r) => r.id}
          href={(r) => `/owner/orders/${r.id}`}
          empty="No orders match."
          columns={[
            { label: "Order", cell: (r) => r.number },
            { label: "Customer", cell: (r) => r.customerName ?? "" },
            { label: "Type", cell: (r) => (r.kind === "bespoke" ? "Bespoke" : "Ready to wear") },
            { label: "Stage", cell: (r) => <StatusBadge status={r.status} /> },
            { label: "Payment", cell: (r) => <StatusBadge status={r.paymentStatus} label={paymentLabels[r.paymentStatus]} /> },
            { label: "Total", cell: (r) => <Price minor={r.totalMinor} currency={r.currency} />, className: "tabular" },
            { label: "Due", cell: (r) => (r.dueDate ? <span style={new Date(r.dueDate) < new Date() && !["completed", "delivered", "cancelled"].includes(r.status) ? { color: "var(--danger)" } : undefined}>{formatDate(r.dueDate, "short")}</span> : "") },
            { label: "Placed", cell: (r) => formatDate(r.createdAt, "short") },
          ]}
        />
      </Suspense>
    </>
  );
}

type Task = { id: string; stage: string; title: string; status: string; assignee: string | null; dueAt: string | null; completedAt: string | null };
type Resp = { order: Order; tasks: Task[]; supportThreads: { id: string; number: string; subject: string; status: string }[]; workflow: string[]; statusLabels: Record<string, string> };

export function OwnerOrderDetail({ id }: { id: string }) {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["owner", "order", id], queryFn: () => api<Resp>(`/owner/orders/${id}`) });
  const [stage, setStage] = useState("");
  const [stageNote, setStageNote] = useState("");
  const [tell, setTell] = useState(true);
  const [busy, setBusy] = useState(false);
  const reload = () => qc.invalidateQueries({ queryKey: ["owner", "order", id] });

  async function run(fn: () => Promise<unknown>, done: string) {
    setBusy(true);
    try {
      await fn();
      toast(done);
      await reload();
      return true;
    } catch (e) {
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
      if (e instanceof ApiError && e.code === "stale_version") await reload();
      return false;
    } finally {
      setBusy(false);
    }
  }

  if (q.isLoading) return <div className="skeleton" style={{ height: 480 }} />;
  if (!q.data) return <p className="notice notice-danger">This order could not be loaded.</p>;
  const { order: o, tasks, workflow, statusLabels } = q.data;
  const cur = o.currency;
  const stages = [...workflow.filter((s) => s !== o.status && s !== "draft"), ...(o.status !== "completed" ? ["completed"] : []), "cancelled"].filter((v, i, a) => a.indexOf(v) === i);
  const backwards = stage && workflow.indexOf(stage) > -1 && workflow.indexOf(stage) < workflow.indexOf(o.status);
  const needsNote = Boolean(stage && (stage === "cancelled" || stage === "refunded" || backwards));

  return (
    <>
      <PageHead
        title={`Order ${o.number}`}
        sub={
          <>
            {o.contact.name} · {o.kind === "bespoke" ? "Bespoke" : "Ready to wear"} · placed {formatDateTime(o.createdAt)}
          </>
        }
        actions={
          <>
            <StatusBadge status={o.status} label={statusLabels[o.status]} />
            <StatusBadge status={o.paymentStatus} label={paymentLabels[o.paymentStatus]} />
          </>
        }
      />
      <div className={styles.detail}>
        <div className="stack-lg">
          <section className="stack-sm" aria-labelledby="items-h">
            <h2 id="items-h" className={styles.h2}>
              Items
            </h2>
            <div className="table-wrap">
              <table className="table">
                <tbody>
                  {o.items.map((it) => (
                    <tr key={it.id}>
                      <td>
                        {it.name}
                        {it.description ? <div className="tiny muted">{it.description}</div> : null}
                      </td>
                      <td className="tabular">× {it.quantity}</td>
                      <td className="tabular">
                        <Price minor={it.totalMinor} currency={cur} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <dl className={styles.dl} style={{ maxWidth: 420, marginLeft: "auto" }}>
              {o.deliveryMinor ? (
                <>
                  <dt>Delivery</dt>
                  <dd className="tabular">
                    <Price minor={o.deliveryMinor} currency={cur} />
                  </dd>
                </>
              ) : null}
              {o.taxMinor ? (
                <>
                  <dt>Tax</dt>
                  <dd className="tabular">
                    <Price minor={o.taxMinor} currency={cur} />
                  </dd>
                </>
              ) : null}
              <dt>
                <strong>Total</strong>
              </dt>
              <dd className="tabular">
                <strong>
                  <Price minor={o.totalMinor} currency={cur} />
                </strong>
              </dd>
              <dt>Paid</dt>
              <dd className="tabular">
                <Price minor={o.amountPaidMinor} currency={cur} />
              </dd>
              {o.amountRefundedMinor ? (
                <>
                  <dt>Refunded</dt>
                  <dd className="tabular">
                    <Price minor={o.amountRefundedMinor} currency={cur} />
                  </dd>
                </>
              ) : null}
              <dt>Balance</dt>
              <dd className="tabular">
                <Price minor={o.balanceMinor} currency={cur} />
              </dd>
            </dl>
          </section>

          <Payments order={o} busy={busy} run={run} />
          <Tasks orderId={o.id} tasks={tasks} busy={busy} run={run} />
          <Fittings order={o} busy={busy} run={run} />
          <Notes order={o} busy={busy} run={run} />

          <section className="stack-sm" aria-labelledby="hist-h">
            <h2 id="hist-h" className={styles.h2}>
              History
            </h2>
            <ul className="list-rows">
              {o.history.map((h, i) => (
                <li key={i} className="list-row small">
                  <span>
                    {h.oldStatus ? `${humanize(h.oldStatus)} to ` : ""}
                    {humanize(h.newStatus)}
                    {h.note ? <span className="muted"> · {h.note}</span> : null}
                  </span>
                  <span className="muted">
                    {humanize(h.actor)} · {formatDateTime(h.createdAt)}
                  </span>
                </li>
              ))}
            </ul>
          </section>
        </div>

        <aside className={styles.side}>
          <section className="panel panel-pad stack-sm" aria-labelledby="stage-h">
            <h2 id="stage-h" className={styles.h2}>
              Stage
            </h2>
            <p style={{ margin: 0 }}>{humanize(o.status)}</p>
            {!["cancelled", "refunded"].includes(o.status) ? (
              <>
                <select className="select" aria-label="Move to stage" value={stage} onChange={(e) => setStage(e.target.value)}>
                  <option value="">Move to</option>
                  {stages.map((s) => (
                    <option key={s} value={s}>
                      {humanize(s)}
                    </option>
                  ))}
                </select>
                {stage ? (
                  <>
                    <label className="field">
                      <span className="label">{needsNote ? "Reason (required)" : "Note (optional)"}</span>
                      <textarea className="textarea" rows={2} value={stageNote} onChange={(e) => setStageNote(e.target.value)} />
                    </label>
                    <label className="check">
                      <input type="checkbox" checked={tell} onChange={(e) => setTell(e.target.checked)} />
                      <span>Show the note to the customer</span>
                    </label>
                    <button
                      className={`btn btn-sm ${stage === "cancelled" ? "btn-danger" : "btn-primary"}`}
                      disabled={busy || (needsNote && !stageNote.trim())}
                      onClick={async () => {
                        if (stage === "cancelled" && !confirm("Cancel this order? Paid amounts must be refunded separately.")) return;
                        if (await run(() => api(`/owner/orders/${o.id}/status`, { body: { status: stage, note: stageNote, customerVisible: tell, version: o.version } }), "Stage updated.")) {
                          setStage("");
                          setStageNote("");
                        }
                      }}
                    >
                      Move to {humanize(stage).toLowerCase()}
                    </button>
                  </>
                ) : null}
              </>
            ) : null}
          </section>

          <Details order={o} busy={busy} run={run} />

          <section className="panel panel-pad stack-sm" aria-labelledby="cust-h">
            <h2 id="cust-h" className={styles.h2}>
              Customer
            </h2>
            <Link className="link" href={`/owner/customers/${o.customerId}`}>
              {o.contact.name}
            </Link>
            <span className="small">
              <a href={`tel:+${o.contact.phone.replace(/^\+/, "")}`}>+{o.contact.phone.replace(/^\+/, "")}</a>
              {o.contact.email ? ` · ${o.contact.email}` : ""}
            </span>
            <span className="small muted">
              {humanize(o.fulfillmentMethod)}
              {o.deliveryAddress ? `: ${[o.deliveryAddress.line1, o.deliveryAddress.city].filter(Boolean).join(", ")}` : ""}
            </span>
            {o.requestId ? (
              <Link className="link small" href={`/owner/requests/${o.requestId}`}>
                Original request
              </Link>
            ) : null}
            {o.quoteId ? (
              <Link className="link small" href={`/owner/quotes/${o.quoteId}`}>
                Accepted quote
              </Link>
            ) : null}
            {q.data.supportThreads.map((t) => (
              <Link key={t.id} className="link small" href={`/owner/support/${t.id}`}>
                {t.number}: {t.subject}
              </Link>
            ))}
          </section>
        </aside>
      </div>
    </>
  );
}

type RunFn = (fn: () => Promise<unknown>, done: string) => Promise<boolean>;

function Payments({ order: o, busy, run }: { order: Order; busy: boolean; run: RunFn }) {
  const [open, setOpen] = useState(false);
  const [amount, setAmount] = useState("");
  const [purpose, setPurpose] = useState(o.amountPaidMinor === 0 && o.depositRequiredMinor < o.totalMinor ? "deposit" : "balance");
  const [note, setNote] = useState("");
  const [refundFor, setRefundFor] = useState<string | null>(null);
  const [refundAmount, setRefundAmount] = useState("");
  const [reason, setReason] = useState("");
  const exp = exponentOf(o.currency);
  return (
    <section className="stack-sm" aria-labelledby="pay-h">
      <div className="spread">
        <h2 id="pay-h" className={styles.h2}>
          Payments
        </h2>
        {o.balanceMinor > 0 && !["cancelled", "refunded"].includes(o.status) ? (
          <button className="btn btn-sm" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
            Record a payment
          </button>
        ) : null}
      </div>
      {open ? (
        <div className="panel panel-pad stack-sm">
          <p className="small muted" style={{ margin: 0 }}>
            For cash, bank transfer or Mobile Money received outside the website. Record only money you have actually received.
          </p>
          <div className="form-grid cols-2">
            <label className="field">
              <span className="label">Amount ({o.currency})</span>
              <input className="input tabular" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder={String(o.balanceMinor / 10 ** exp)} />
            </label>
            <label className="field">
              <span className="label">For</span>
              <select className="select" value={purpose} onChange={(e) => setPurpose(e.target.value)}>
                <option value="deposit">Deposit</option>
                <option value="balance">Balance</option>
                <option value="full">Full payment</option>
              </select>
            </label>
          </div>
          <label className="field">
            <span className="label">How it was paid (required)</span>
            <input className="input" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Cash at the studio, receipt 0142" />
          </label>
          <button
            className="btn btn-primary btn-sm"
            style={{ justifySelf: "start" }}
            disabled={busy || !note.trim() || toMinor(amount, o.currency) === null}
            onClick={async () => {
              if (
                await run(
                  () => api(`/owner/orders/${o.id}/payments/manual`, { body: { amountMinor: toMinor(amount, o.currency), purpose, note }, idempotencyKey: `manual-${o.id}-${o.version}-${amount}` }),
                  "Payment recorded.",
                )
              ) {
                setOpen(false);
                setAmount("");
                setNote("");
              }
            }}
          >
            Record payment
          </button>
        </div>
      ) : null}
      {o.payments.length ? (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>Date</th>
                <th>Method</th>
                <th>For</th>
                <th className="tabular">Amount</th>
                <th>Status</th>
                <th>
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {o.payments.map((p) => (
                <tr key={p.id}>
                  <td>{formatDate(p.succeededAt ?? p.createdAt, "short")}</td>
                  <td>
                    {p.provider === "mtn" ? "MTN MoMo" : p.provider === "orange" ? "Orange Money" : humanize(p.provider)}
                    {p.simulated ? <span className="badge badge-warning" style={{ marginLeft: 6 }}>Test</span> : null}
                  </td>
                  <td>{humanize(p.purpose)}</td>
                  <td className="tabular">
                    <Price minor={p.amountMinor} currency={o.currency} />
                  </td>
                  <td>
                    <StatusBadge status={p.status} />
                  </td>
                  <td>
                    {["pending", "processing", "customer_action_required", "created"].includes(p.status) ? (
                      <button className="link small" disabled={busy} onClick={() => run(() => api(`/owner/payments/${p.id}/recheck`, { method: "POST", body: {} }), "Checked with the provider.")}>
                        Check again
                      </button>
                    ) : p.status === "succeeded" || p.status === "partially_refunded" ? (
                      <button className="link small" onClick={() => setRefundFor(refundFor === p.id ? null : p.id)}>
                        Refund
                      </button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="small muted">No payments yet.</p>
      )}
      {refundFor ? (
        <div className="panel panel-pad stack-sm">
          <p className="small" style={{ margin: 0 }}>
            Refunds are recorded here after you have sent the money back. Mobile Money refunds must be sent from the merchant account.
          </p>
          <div className="form-grid cols-2">
            <label className="field">
              <span className="label">Amount ({o.currency})</span>
              <input className="input tabular" inputMode="decimal" value={refundAmount} onChange={(e) => setRefundAmount(e.target.value)} />
            </label>
            <label className="field">
              <span className="label">Reason</span>
              <input className="input" value={reason} onChange={(e) => setReason(e.target.value)} />
            </label>
          </div>
          <button
            className="btn btn-danger btn-sm"
            style={{ justifySelf: "start" }}
            disabled={busy || !reason.trim() || toMinor(refundAmount, o.currency) === null}
            onClick={async () => {
              if (!confirm("Record this refund?")) return;
              if (await run(() => api(`/owner/payments/${refundFor}/refunds`, { body: { amountMinor: toMinor(refundAmount, o.currency), reason }, idempotencyKey: `refund-${refundFor}-${refundAmount}-${reason.length}` }), "Refund recorded.")) {
                setRefundFor(null);
                setRefundAmount("");
                setReason("");
              }
            }}
          >
            Record refund
          </button>
        </div>
      ) : null}
    </section>
  );
}

function Tasks({ orderId, tasks, busy, run }: { orderId: string; tasks: Task[]; busy: boolean; run: RunFn }) {
  const [title, setTitle] = useState("");
  const [stage, setStage] = useState("cutting");
  return (
    <section className="stack-sm" aria-labelledby="tasks-h">
      <h2 id="tasks-h" className={styles.h2}>
        Production tasks
      </h2>
      {tasks.length ? (
        <ul className="list-rows">
          {tasks.map((t) => (
            <li key={t.id} className="list-row">
              <label className="check">
                <input
                  type="checkbox"
                  checked={t.status === "done"}
                  disabled={busy}
                  onChange={(e) => run(() => api(`/owner/orders/${orderId}/tasks`, { body: { taskId: t.id, stage: t.stage, title: t.title, status: e.target.checked ? "done" : "open" } }), "Task updated.")}
                />
                <span style={t.status === "done" ? { textDecoration: "line-through", color: "var(--text-muted)" } : undefined}>{t.title}</span>
              </label>
              <span className="small muted">
                {humanize(t.stage)}
                {t.assignee ? ` · ${t.assignee}` : ""}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      <div className="row-wrap">
        <select className="select" aria-label="Task stage" value={stage} onChange={(e) => setStage(e.target.value)} style={{ width: "auto" }}>
          {["patterning", "cutting", "sewing", "quality_check", "fitting", "alteration", "finishing"].map((s) => (
            <option key={s} value={s}>
              {humanize(s)}
            </option>
          ))}
        </select>
        <input className="input" aria-label="New task" placeholder="Add a task" value={title} onChange={(e) => setTitle(e.target.value)} style={{ flex: 1, minWidth: 200 }} />
        <button
          className="btn btn-sm"
          disabled={busy || !title.trim()}
          onClick={async () => {
            if (await run(() => api(`/owner/orders/${orderId}/tasks`, { body: { stage, title, status: "open" } }), "Task added.")) setTitle("");
          }}
        >
          Add
        </button>
      </div>
    </section>
  );
}

function Fittings({ order: o, busy, run }: { order: Order; busy: boolean; run: RunFn }) {
  const [open, setOpen] = useState(false);
  const [adj, setAdj] = useState([{ area: "", change: "" }]);
  const [notes, setNotes] = useState("");
  const [customerNotes, setCustomerNotes] = useState("");
  if (o.kind !== "bespoke") return null;
  return (
    <section className="stack-sm" aria-labelledby="fit-h">
      <div className="spread">
        <h2 id="fit-h" className={styles.h2}>
          Fittings
        </h2>
        <button className="btn btn-sm" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
          Record a fitting
        </button>
      </div>
      {o.fittings.map((f) => (
        <div key={f.id} className="small">
          <strong>{formatDate(f.createdAt)}</strong>
          {f.adjustments.length ? (
            <ul style={{ margin: "4px 0", paddingLeft: 18 }}>
              {f.adjustments.map((a, i) => (
                <li key={i}>
                  {a.area}: {a.change}
                </li>
              ))}
            </ul>
          ) : null}
          {f.notes ? <p className="muted" style={{ margin: 0 }}>{f.notes}</p> : null}
        </div>
      ))}
      {!o.fittings.length && !open ? <p className="small muted">No fittings recorded.</p> : null}
      {open ? (
        <div className="panel panel-pad stack-sm">
          {adj.map((a, i) => (
            <div key={i} className="form-grid cols-2">
              <input className="input" aria-label={`Adjustment ${i + 1} area`} placeholder="Area, for example sleeve length" value={a.area} onChange={(e) => setAdj((x) => x.map((y, j) => (j === i ? { ...y, area: e.target.value } : y)))} />
              <input className="input" aria-label={`Adjustment ${i + 1} change`} placeholder="Change, for example shorten 1.5 cm" value={a.change} onChange={(e) => setAdj((x) => x.map((y, j) => (j === i ? { ...y, change: e.target.value } : y)))} />
            </div>
          ))}
          <button className="link small" style={{ justifySelf: "start" }} onClick={() => setAdj((x) => [...x, { area: "", change: "" }])}>
            Add another adjustment
          </button>
          <label className="field">
            <span className="label">Notes for the workroom</span>
            <textarea className="textarea" rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
          </label>
          <label className="field">
            <span className="label">Notes the customer will see</span>
            <textarea className="textarea" rows={2} value={customerNotes} onChange={(e) => setCustomerNotes(e.target.value)} />
          </label>
          <button
            className="btn btn-primary btn-sm"
            style={{ justifySelf: "start" }}
            disabled={busy}
            onClick={async () => {
              if (
                await run(
                  () => api(`/owner/orders/${o.id}/fittings`, { body: { notes, customerNotes, adjustments: adj.filter((a) => a.area.trim() && a.change.trim()) } }),
                  "Fitting recorded.",
                )
              ) {
                setOpen(false);
                setAdj([{ area: "", change: "" }]);
                setNotes("");
                setCustomerNotes("");
              }
            }}
          >
            Save fitting
          </button>
        </div>
      ) : null}
    </section>
  );
}

function Notes({ order: o, busy, run }: { order: Order; busy: boolean; run: RunFn }) {
  const [body, setBody] = useState("");
  const [visibility, setVisibility] = useState("internal");
  return (
    <section className="stack-sm" aria-labelledby="notes-h">
      <h2 id="notes-h" className={styles.h2}>
        Notes
      </h2>
      {o.notes.length ? (
        <ul className="list-rows">
          {o.notes.map((n) => (
            <li key={n.id} style={{ padding: "10px 0" }}>
              <p className="tiny muted" style={{ margin: 0 }}>
                {n.author ?? "Staff"} · {formatDateTime(n.createdAt)} · {n.visibility === "customer" ? "visible to customer" : "internal"}
              </p>
              <p style={{ margin: "4px 0 0", whiteSpace: "pre-wrap" }}>{n.body}</p>
            </li>
          ))}
        </ul>
      ) : null}
      <textarea className="textarea" rows={3} aria-label="New note" value={body} onChange={(e) => setBody(e.target.value)} />
      <div className="row-wrap">
        <select className="select" aria-label="Note visibility" value={visibility} onChange={(e) => setVisibility(e.target.value)} style={{ width: "auto" }}>
          <option value="internal">Internal</option>
          <option value="customer">Visible to the customer</option>
        </select>
        <button
          className="btn btn-sm"
          disabled={busy || !body.trim()}
          onClick={async () => {
            if (await run(() => api(`/owner/orders/${o.id}/notes`, { body: { body, visibility } }), "Note added.")) setBody("");
          }}
        >
          Add note
        </button>
      </div>
    </section>
  );
}

function Details({ order: o, busy, run }: { order: Order; busy: boolean; run: RunFn }) {
  const [due, setDue] = useState(o.dueDate?.slice(0, 10) ?? "");
  const [urgency, setUrgency] = useState(o.urgency);
  const [delivery, setDelivery] = useState(o.deliveryStatus);
  const dirty = due !== (o.dueDate?.slice(0, 10) ?? "") || urgency !== o.urgency || delivery !== o.deliveryStatus;
  return (
    <section className="panel panel-pad stack-sm" aria-labelledby="det-h">
      <h2 id="det-h" className={styles.h2}>
        Schedule
      </h2>
      <label className="field">
        <span className="label">Due date</span>
        <input className="input" type="date" value={due} onChange={(e) => setDue(e.target.value)} />
      </label>
      <label className="field">
        <span className="label">Urgency</span>
        <select className="select" value={urgency} onChange={(e) => setUrgency(e.target.value)}>
          <option value="standard">Standard</option>
          <option value="soon">Soon</option>
          <option value="urgent">Urgent</option>
        </select>
      </label>
      <label className="field">
        <span className="label">Delivery</span>
        <select className="select" value={delivery} onChange={(e) => setDelivery(e.target.value)}>
          {["not_started", "preparing", "ready_for_pickup", "dispatched", "delivered", "picked_up"].map((s) => (
            <option key={s} value={s}>
              {humanize(s)}
            </option>
          ))}
        </select>
      </label>
      {dirty ? (
        <button className="btn btn-sm" disabled={busy} onClick={() => run(() => api(`/owner/orders/${o.id}`, { method: "PATCH", body: { dueDate: due || null, urgency, deliveryStatus: delivery, version: o.version } }), "Saved.")}>
          Save
        </button>
      ) : null}
    </section>
  );
}
