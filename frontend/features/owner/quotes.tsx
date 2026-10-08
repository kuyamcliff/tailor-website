"use client";

import Link from "next/link";
import { Suspense, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { formatDate, formatDateTime, humanize } from "@/lib/format";
import { exponentOf, toMinor } from "@/lib/money";
import type { Quote } from "@/lib/types";
import { useConfig } from "@/components/providers/config";
import { useToast } from "@/components/providers/toast";
import { StatusBadge } from "@/components/ui/status-badge";
import { Price } from "@/components/ui/price";
import { OwnerList } from "./owner-list";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

type Row = { id: string; number: string; status: string; customer: string; requestNumber: string | null; totalMinor: number | null; currency: string | null; expiresAt: string | null; updatedAt: string };

const statuses: [string, string][] = [
  ["draft", "Draft"],
  ["sent", "Sent"],
  ["changes_requested", "Changes requested"],
  ["accepted", "Accepted"],
  ["declined", "Declined"],
  ["expired", "Expired"],
  ["withdrawn", "Withdrawn"],
];

export function OwnerQuotes() {
  return (
    <>
      <PageHead title="Quotes" sub="Quotes are drafted from a request. Open a request to start one." />
      <Suspense>
        <OwnerList<Row>
          endpoint="/owner/quotes"
          filters={[{ key: "status", label: "Status", type: "select", options: statuses }]}
          rowKey={(r) => r.id}
          href={(r) => `/owner/quotes/${r.id}`}
          empty="No quotes match."
          columns={[
            { label: "Quote", cell: (r) => r.number },
            { label: "Customer", cell: (r) => r.customer },
            { label: "Request", cell: (r) => r.requestNumber ?? "" },
            { label: "Total", cell: (r) => (r.totalMinor !== null ? <Price minor={r.totalMinor} currency={r.currency ?? undefined} /> : ""), className: "tabular" },
            { label: "Expires", cell: (r) => (r.expiresAt ? formatDate(r.expiresAt, "short") : "") },
            { label: "Status", cell: (r) => <StatusBadge status={r.status} /> },
            { label: "Updated", cell: (r) => formatDate(r.updatedAt, "short") },
          ]}
        />
      </Suspense>
    </>
  );
}

const kinds: [string, string][] = [
  ["garment", "Garment"],
  ["fabric", "Fabric"],
  ["customization", "Customisation"],
  ["complexity", "Complexity"],
  ["rush", "Rush"],
  ["alteration_allowance", "Alterations"],
  ["delivery", "Delivery"],
  ["discount", "Discount"],
  ["other", "Other"],
];

type EditLine = { kind: string; description: string; quantity: string; unit: string };

export function OwnerQuoteEditor({ id }: { id: string }) {
  const q = useQuery({ queryKey: ["owner", "quote", id], queryFn: () => api<Quote>(`/owner/quotes/${id}`) });
  if (q.isLoading) return <div className="skeleton" style={{ height: 480 }} />;
  if (!q.data) return <p className="notice notice-danger">This quote could not be loaded.</p>;
  return <Editor key={`${q.data.id}-${q.data.version}`} quote={q.data} />;
}

function Editor({ quote }: { quote: Quote }) {
  const cfg = useConfig();
  const qc = useQueryClient();
  const toast = useToast();
  const cur = quote.current;
  const currency = cur?.currency ?? cfg.business.currency;
  const editable = ["draft", "sent", "changes_requested"].includes(quote.status);
  const [lines, setLines] = useState<EditLine[]>(
    () => cur?.lines.map((l) => ({ kind: l.kind, description: l.description, quantity: String(l.quantity), unit: String(l.unitMinor / 10 ** exponentOf(cur?.currency ?? "XAF")) })) ?? [{ kind: "garment", description: "", quantity: "1", unit: "0" }],
  );
  const [depositMode, setDepositMode] = useState<"percent" | "fixed">("percent");
  const [depositPercent, setDepositPercent] = useState(() => String(cur && cur.totalMinor ? Math.round((cur.depositMinor / cur.totalMinor) * 100) : cfg.business.depositPercentBp / 100));
  const [depositFixed, setDepositFixed] = useState(() => String((cur?.depositMinor ?? 0) / 10 ** exponentOf(currency)));
  const [validDays, setValidDays] = useState(String(cfg.business.quoteValidityDays || 14));
  const [ready, setReady] = useState(cur?.estimatedReadyDate?.slice(0, 10) ?? "");
  const [notes, setNotes] = useState(cur?.customerNotes ?? "");
  const [terms, setTerms] = useState(cur?.terms ?? "");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const reload = () => qc.invalidateQueries({ queryKey: ["owner", "quote", quote.id] });

  // Local preview using the same rules as the server; the saved figures always come from the server.
  const preview = useMemo(() => {
    let sub = 0;
    let disc = 0;
    let del = 0;
    for (const l of lines) {
      const t = (toMinor(l.unit, currency) ?? 0) * (Number(l.quantity) || 0);
      if (l.kind === "discount") disc += t;
      else if (l.kind === "delivery") del += t;
      else sub += t;
    }
    const taxable = sub - disc + del;
    const rate = cfg.business.taxRateBp;
    const tax = cfg.business.pricesIncludeTax ? taxable - Math.round((taxable * 10000) / (10000 + rate)) : Math.round((taxable * rate) / 10000);
    const total = cfg.business.pricesIncludeTax ? taxable : taxable + tax;
    const deposit = depositMode === "fixed" ? (toMinor(depositFixed, currency) ?? 0) : Math.round((total * (Number(depositPercent) || 0)) / 100);
    return { sub, disc, del, tax, total, deposit };
  }, [lines, currency, cfg.business, depositMode, depositFixed, depositPercent]);

  const body = () => ({
    lines: lines.map((l) => ({ kind: l.kind, description: l.description, quantity: Number(l.quantity) || 0, unitMinor: toMinor(l.unit, currency) ?? -1 })),
    depositPercentBp: depositMode === "percent" ? Math.round((Number(depositPercent) || 0) * 100) : undefined,
    depositMinor: depositMode === "fixed" ? (toMinor(depositFixed, currency) ?? 0) : undefined,
    validDays: Number(validDays) || 14,
    customerNotes: notes,
    terms,
    estimatedReadyDate: ready || null,
    version: quote.version,
  });

  async function run(fn: () => Promise<unknown>, done: string) {
    setBusy(true);
    setErrors({});
    try {
      await fn();
      toast(done);
      await reload();
    } catch (e) {
      if (e instanceof ApiError) {
        setErrors(e.fields);
        toast(Object.values(e.fields)[0] ?? e.message, "error");
        if (e.code === "stale_version") await reload();
      } else toast("Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }

  const save = () => run(() => api(`/owner/quotes/${quote.id}`, { method: "PUT", body: body() }), "Revision saved.");
  const send = () =>
    run(async () => {
      if (dirty) await api(`/owner/quotes/${quote.id}`, { method: "PUT", body: body() });
      const fresh = await api<Quote>(`/owner/quotes/${quote.id}`);
      await api(`/owner/quotes/${quote.id}/send`, { body: { version: fresh.version } });
    }, "Quote sent to the customer.");
  const withdraw = () => {
    if (!confirm("Withdraw this quote? The customer will no longer be able to accept it.")) return;
    void run(() => api(`/owner/quotes/${quote.id}/withdraw`, { body: { version: quote.version } }), "Quote withdrawn.");
  };

  const dirty =
    JSON.stringify(lines) !== JSON.stringify(cur?.lines.map((l) => ({ kind: l.kind, description: l.description, quantity: String(l.quantity), unit: String(l.unitMinor / 10 ** exponentOf(cur?.currency ?? "XAF")) })) ?? []) ||
    notes !== (cur?.customerNotes ?? "") ||
    terms !== (cur?.terms ?? "");

  return (
    <>
      <PageHead
        title={`Quote ${quote.number}`}
        sub={
          <>
            {quote.customerName}
            {quote.requestId ? (
              <>
                {" · "}
                <Link className="link" href={`/owner/requests/${quote.requestId}`}>
                  Request {quote.requestNumber}
                </Link>
              </>
            ) : null}
            {cur ? ` · revision ${cur.revisionNo}` : ""}
          </>
        }
        actions={
          <>
            <StatusBadge status={quote.status} />
            {quote.orderId ? (
              <Link className="btn btn-sm" href={`/owner/orders/${quote.orderId}`}>
                Open order
              </Link>
            ) : null}
          </>
        }
      />
      {quote.decisionNote ? (
        <p className="notice notice-warning">
          Customer note{quote.decidedAt ? ` (${formatDateTime(quote.decidedAt)})` : ""}: {quote.decisionNote}
        </p>
      ) : null}
      <div className={styles.detail}>
        <div className="stack">
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Type</th>
                  <th>Description</th>
                  <th>Qty</th>
                  <th>Unit price ({currency})</th>
                  <th className="tabular">Total</th>
                  <th>
                    <span className="visually-hidden">Remove</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {lines.map((l, i) => {
                  const set = (k: keyof EditLine, v: string) => setLines((ls) => ls.map((x, j) => (j === i ? { ...x, [k]: v } : x)));
                  const err = errors[`lines.${i}`];
                  return (
                    <tr key={i}>
                      <td style={{ minWidth: 170 }}>
                        <select className="select" aria-label={`Line ${i + 1} type`} value={l.kind} disabled={!editable} onChange={(e) => set("kind", e.target.value)}>
                          {kinds.map(([k, lab]) => (
                            <option key={k} value={k}>
                              {lab}
                            </option>
                          ))}
                        </select>
                      </td>
                      <td style={{ minWidth: 280, width: "100%" }}>
                        <input className="input" aria-label={`Line ${i + 1} description`} aria-invalid={Boolean(err)} value={l.description} disabled={!editable} onChange={(e) => set("description", e.target.value)} />
                        {err ? <span className="error tiny">{err}</span> : null}
                      </td>
                      <td style={{ minWidth: 72 }}>
                        <input className="input tabular" aria-label={`Line ${i + 1} quantity`} inputMode="numeric" value={l.quantity} disabled={!editable} onChange={(e) => set("quantity", e.target.value)} />
                      </td>
                      <td style={{ minWidth: 140 }}>
                        <input className="input tabular" aria-label={`Line ${i + 1} unit price`} inputMode="decimal" value={l.unit} disabled={!editable} onChange={(e) => set("unit", e.target.value)} />
                      </td>
                      <td className="tabular" style={{ whiteSpace: "nowrap" }}>
                        {l.kind === "discount" ? "- " : ""}
                        <Price minor={(toMinor(l.unit, currency) ?? 0) * (Number(l.quantity) || 0)} currency={currency} />
                      </td>
                      <td>
                        {editable && lines.length > 1 ? (
                          <button className="icon-btn" aria-label={`Remove line ${i + 1}`} onClick={() => setLines((ls) => ls.filter((_, j) => j !== i))}>
                            <Trash2 size={16} aria-hidden />
                          </button>
                        ) : null}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          {editable ? (
            <button className="btn btn-sm" style={{ justifySelf: "start" }} onClick={() => setLines((ls) => [...ls, { kind: "customization", description: "", quantity: "1", unit: "0" }])}>
              Add line
            </button>
          ) : null}
          {errors.lines ? <p className="error small">{errors.lines}</p> : null}

          <div className="form-grid cols-2">
            <label className="field">
              <span className="label">Message to the customer</span>
              <textarea className="textarea" rows={4} value={notes} disabled={!editable} onChange={(e) => setNotes(e.target.value)} />
            </label>
            <label className="field">
              <span className="label">Terms for this quote</span>
              <textarea className="textarea" rows={4} value={terms} disabled={!editable} onChange={(e) => setTerms(e.target.value)} />
            </label>
          </div>
        </div>

        <aside className={styles.side}>
          <section className="panel panel-pad stack-sm" aria-labelledby="totals-h">
            <h2 id="totals-h" className={styles.h2}>
              {dirty ? "Preview" : "Totals"}
            </h2>
            <dl className={styles.dl}>
              <dt>Subtotal</dt>
              <dd className="tabular">
                <Price minor={preview.sub} currency={currency} />
              </dd>
              {preview.disc ? (
                <>
                  <dt>Discount</dt>
                  <dd className="tabular">
                    - <Price minor={preview.disc} currency={currency} />
                  </dd>
                </>
              ) : null}
              {preview.del ? (
                <>
                  <dt>Delivery</dt>
                  <dd className="tabular">
                    <Price minor={preview.del} currency={currency} />
                  </dd>
                </>
              ) : null}
              {cfg.business.taxRateBp ? (
                <>
                  <dt>
                    {cfg.business.taxLabel} {cfg.business.pricesIncludeTax ? "(included)" : ""}
                  </dt>
                  <dd className="tabular">
                    <Price minor={preview.tax} currency={currency} />
                  </dd>
                </>
              ) : null}
              <dt>
                <strong>Total</strong>
              </dt>
              <dd className="tabular">
                <strong>
                  <Price minor={preview.total} currency={currency} />
                </strong>
              </dd>
              <dt>Deposit</dt>
              <dd className="tabular">
                <Price minor={preview.deposit} currency={currency} />
              </dd>
            </dl>
          </section>
          <section className="panel panel-pad stack-sm" aria-labelledby="terms-h">
            <h2 id="terms-h" className={styles.h2}>
              Deposit and validity
            </h2>
            <div className="row-wrap">
              <label className="check">
                <input type="radio" name="dep" checked={depositMode === "percent"} disabled={!editable} onChange={() => setDepositMode("percent")} />
                <span>Percent</span>
              </label>
              <label className="check">
                <input type="radio" name="dep" checked={depositMode === "fixed"} disabled={!editable} onChange={() => setDepositMode("fixed")} />
                <span>Fixed amount</span>
              </label>
            </div>
            {depositMode === "percent" ? (
              <label className="field">
                <span className="label">Deposit (%)</span>
                <input className="input tabular" inputMode="decimal" value={depositPercent} disabled={!editable} onChange={(e) => setDepositPercent(e.target.value)} />
              </label>
            ) : (
              <label className="field">
                <span className="label">Deposit ({currency})</span>
                <input className="input tabular" inputMode="decimal" value={depositFixed} disabled={!editable} onChange={(e) => setDepositFixed(e.target.value)} />
              </label>
            )}
            {errors.depositMinor || errors.depositPercentBp ? <p className="error small">{errors.depositMinor ?? errors.depositPercentBp}</p> : null}
            <label className="field">
              <span className="label">Valid for (days)</span>
              <input className="input tabular" inputMode="numeric" value={validDays} disabled={!editable} onChange={(e) => setValidDays(e.target.value)} />
            </label>
            <label className="field">
              <span className="label">Estimated ready date</span>
              <input className="input" type="date" value={ready} disabled={!editable} onChange={(e) => setReady(e.target.value)} />
            </label>
            {cur ? (
              <p className="tiny muted" style={{ margin: 0 }}>
                Current revision expires {formatDate(cur.expiresAt)}
                {cur.sentAt ? `, sent ${formatDateTime(cur.sentAt)}` : ", not sent yet"}.
              </p>
            ) : null}
          </section>
          {editable ? (
            <div className="stack-sm">
              <button className="btn btn-primary" disabled={busy} onClick={send}>
                {quote.status === "draft" ? "Send to customer" : "Send updated quote"}
              </button>
              <button className="btn" disabled={busy || !dirty} onClick={save}>
                Save draft revision
              </button>
              {quote.status !== "draft" ? (
                <button className="btn btn-ghost btn-sm" disabled={busy} onClick={withdraw}>
                  Withdraw quote
                </button>
              ) : null}
            </div>
          ) : null}
          {quote.history.length ? (
            <section className="stack-sm" aria-labelledby="qh-h">
              <h2 id="qh-h" className={styles.h2}>
                History
              </h2>
              <ul className="list-rows">
                {quote.history.map((h, i) => (
                  <li key={i} className="small" style={{ padding: "8px 0" }}>
                    {humanize(h.newStatus)}
                    {h.note ? ` · ${h.note}` : ""}
                    <br />
                    <span className="muted">
                      {h.actor} · {formatDateTime(h.createdAt)}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </aside>
      </div>
    </>
  );
}

