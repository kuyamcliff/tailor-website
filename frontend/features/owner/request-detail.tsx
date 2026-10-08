"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { formatDate, formatDateTime, humanize, requestLabels } from "@/lib/format";
import { formatLength } from "@/lib/units";
import type { MeasurementField, MeasurementVersion, RequestView } from "@/lib/types";
import { StatusBadge } from "@/components/ui/status-badge";
import { Price } from "@/components/ui/price";
import { useToast } from "@/components/providers/toast";
import { MeasurementForm, measurePayload, stateFromMM, type MeasureState } from "@/features/measurements/measurement-form";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

type Resp = {
  request: RequestView;
  messages: { id: string; author_type: string; author: string | null; body: string; internal: boolean; created_at: string }[];
  measurementFields: MeasurementField[];
};

const transitions: Record<string, string[]> = {
  new: ["reviewing", "need_information", "closed"],
  reviewing: ["need_information", "closed", "new"],
  need_information: ["reviewing", "closed"],
  quote_sent: ["reviewing", "closed"],
  closed: ["reviewing"],
};

export function OwnerRequestDetail({ id }: { id: string }) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["owner", "request", id], queryFn: () => api<Resp>(`/owner/requests/${id}`) });
  const [status, setStatus] = useState("");
  const [message, setMessage] = useState("");
  const [notes, setNotes] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [verifying, setVerifying] = useState(false);
  const reload = () => qc.invalidateQueries({ queryKey: ["owner", "request", id] });

  if (q.isLoading) return <div className="skeleton" style={{ height: 480 }} />;
  if (!q.data) return <p className="notice notice-danger">This request could not be loaded.</p>;
  const r = q.data.request;
  const fields = q.data.measurementFields;
  const label = (k: string) => fields.find((f) => f.key === k)?.label ?? humanize(k);

  async function update(body: Record<string, unknown>, done: string) {
    setBusy(true);
    try {
      await api(`/owner/requests/${id}`, { method: "PATCH", body: { ...body, version: r.version } });
      toast(done);
      setStatus("");
      setMessage("");
      setNotes(null);
      await reload();
    } catch (e) {
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
      if (e instanceof ApiError && e.code === "stale_version") await reload();
    } finally {
      setBusy(false);
    }
  }

  async function createQuote() {
    setBusy(true);
    try {
      // Start from the studio design when there is one; otherwise one garment line to price.
      const d = q.data!.request.design;
      const lines = d
        ? [
            { kind: "garment", description: d.garment.name, quantity: 1, unitMinor: d.price.baseMinor },
            ...d.selections.filter((x) => x.priceMinor > 0).map((x) => ({ kind: "customization", description: `${x.groupName}: ${x.valueName ?? x.number}`, quantity: 1, unitMinor: x.priceMinor })),
            ...(d.fabric && d.price.fabricMinor ? [{ kind: "fabric", description: `${d.fabric.name}, ${d.fabric.colorName}`, quantity: 1, unitMinor: d.price.fabricMinor }] : []),
          ]
        : [{ kind: "garment", description: q.data!.request.garmentName, quantity: 1, unitMinor: 0 }];
      const res = await api<{ id: string }>(`/owner/requests/${id}/quotes`, { body: { lines, validDays: 14 } });
      router.push(`/owner/quotes/${res.id}`);
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
      setBusy(false);
    }
  }

  const d = r.design;
  return (
    <>
      <PageHead
        title={`Request ${r.number}`}
        sub={
          <>
            {r.contact.name} · received {formatDateTime(r.createdAt)}
          </>
        }
        actions={
          <>
            <StatusBadge status={r.status} label={requestLabels[r.status]} />
            {r.orderId ? (
              <Link className="btn btn-sm" href={`/owner/orders/${r.orderId}`}>
                Open order
              </Link>
            ) : null}
          </>
        }
      />
      <div className={styles.detail}>
        <div className="stack-lg">
          <section className="stack-sm" aria-labelledby="what-h">
            <h2 id="what-h" className={styles.h2}>
              What they want
            </h2>
            <dl className={styles.dl}>
              <dt>Garment</dt>
              <dd>{r.garmentName}</dd>
              <dt>Occasion</dt>
              <dd>
                {humanize(r.occasion)}
                {r.occasionNote ? `: ${r.occasionNote}` : ""}
              </dd>
              <dt>Needed by</dt>
              <dd>{r.desiredDate ? `${formatDate(r.desiredDate)} (${humanize(r.dateFlexibility).toLowerCase()})` : "No date given"}</dd>
              <dt>Urgency</dt>
              <dd>{humanize(r.urgency)}</dd>
              <dt>Cut and fit</dt>
              <dd>
                {humanize(r.bodyModel)}, {r.fitPreference}
              </dd>
              <dt>Fabric</dt>
              <dd>{r.fabricMode === "catalog" ? `${r.fabricName ?? r.fabricKey}${r.colorKey ? `, ${humanize(r.colorKey)}` : ""}` : r.fabricMode === "reference" ? "From the customer's photo" : "Recommend one"}</dd>
              {r.notes ? (
                <>
                  <dt>Notes</dt>
                  <dd style={{ whiteSpace: "pre-wrap" }}>{r.notes}</dd>
                </>
              ) : null}
            </dl>
          </section>

          {d ? (
            <section className="stack-sm" aria-labelledby="design-h">
              <h2 id="design-h" className={styles.h2}>
                Studio design
              </h2>
              <dl className={styles.dl}>
                {d.selections.map((s) => (
                  <div key={s.group} style={{ display: "contents" }}>
                    <dt>{s.groupName}</dt>
                    <dd>
                      {s.valueName ?? `${s.number} ${s.unit ?? ""}`}
                      {s.priceMinor ? (
                        <span className="small muted">
                          {" "}
                          + <Price minor={s.priceMinor} />
                        </span>
                      ) : null}
                    </dd>
                  </div>
                ))}
                {d.fabric ? (
                  <>
                    <dt>Fabric</dt>
                    <dd>
                      {d.fabric.name}, {d.fabric.colorName}
                    </dd>
                  </>
                ) : null}
                <dt>Studio estimate</dt>
                <dd>
                  <Price minor={d.price.totalMinor} currency={d.price.currency} />
                </dd>
              </dl>
            </section>
          ) : null}

          {r.references.length ? (
            <section className="stack-sm" aria-labelledby="refs-h">
              <h2 id="refs-h" className={styles.h2}>
                Reference photos
              </h2>
              <div className={styles.thumbs}>
                {r.references.map((ref) => (
                  <figure key={ref.uploadId}>
                    {ref.removed ? (
                      <div className="empty small">Removed</div>
                    ) : (
                      <a href={ref.url} target="_blank" rel="noopener noreferrer">
                        {/* eslint-disable-next-line @next/next/no-img-element -- private upload served with the staff session */}
                        <img src={ref.thumb} alt={`Reference: ${humanize(ref.tag)}`} loading="lazy" />
                      </a>
                    )}
                    <figcaption className="tiny">
                      <strong>{humanize(ref.tag)}</strong>
                      {ref.note ? <span className="muted"> · {ref.note}</span> : null}
                    </figcaption>
                  </figure>
                ))}
              </div>
            </section>
          ) : null}

          <section className="stack-sm" aria-labelledby="meas-h">
            <div className="spread">
              <h2 id="meas-h" className={styles.h2}>
                Measurements
              </h2>
              {!verifying ? (
                <button className="btn btn-sm" onClick={() => setVerifying(true)}>
                  {r.verifiedMeasurements ? "Record new verified measurements" : "Verify measurements"}
                </button>
              ) : null}
            </div>
            <p className="small muted" style={{ margin: 0 }}>
              {r.measurementMode === "in_store" ? "The customer asked to be measured at the studio." : r.measurementMode === "saved_profile" ? "From the customer's saved profile." : "Entered by the customer."}
            </p>
            {verifying ? (
              <VerifyForm
                requestId={id}
                fields={fields}
                start={r.verifiedMeasurements ?? r.measurements}
                onDone={async () => {
                  setVerifying(false);
                  await reload();
                }}
              />
            ) : (
              <div className={styles.split}>
                {r.measurements ? <Values title="Customer" v={r.measurements} label={label} /> : null}
                {r.verifiedMeasurements ? <Values title={`Verified by ${r.verifiedMeasurements.verifiedBy ?? "staff"}`} v={r.verifiedMeasurements} label={label} /> : null}
              </div>
            )}
          </section>

          {q.data.messages.length ? (
            <section className="stack-sm" aria-labelledby="msg-h">
              <h2 id="msg-h" className={styles.h2}>
                Conversation
              </h2>
              <ul className="list-rows">
                {q.data.messages.map((m) => (
                  <li key={m.id} style={{ padding: "10px 0" }}>
                    <p className="small muted" style={{ margin: 0 }}>
                      {m.author_type === "customer" ? r.contact.name : (m.author ?? "Staff")}
                      {m.internal ? " (internal)" : ""} · {formatDateTime(m.created_at)}
                    </p>
                    <p style={{ margin: "4px 0 0", whiteSpace: "pre-wrap" }}>{m.body}</p>
                  </li>
                ))}
              </ul>
              {r.supportThreadId ? (
                <Link className="link small" href={`/owner/support/${r.supportThreadId}`}>
                  Reply in messages
                </Link>
              ) : null}
            </section>
          ) : null}

          <section className="stack-sm" aria-labelledby="hist-h">
            <h2 id="hist-h" className={styles.h2}>
              History
            </h2>
            <ul className="list-rows">
              {r.history.map((h, i) => (
                <li key={i} className="list-row small">
                  <span>
                    {h.oldStatus ? `${requestLabels[h.oldStatus] ?? humanize(h.oldStatus)} to ` : ""}
                    {requestLabels[h.newStatus] ?? humanize(h.newStatus)}
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
          <section className="panel panel-pad stack-sm" aria-labelledby="contact-h">
            <h2 id="contact-h" className={styles.h2}>
              Customer
            </h2>
            <p style={{ margin: 0 }}>
              <Link className="link" href={`/owner/customers/${r.customerId}`}>
                {r.contact.name}
              </Link>
            </p>
            <p className="small" style={{ margin: 0 }}>
              <a href={`tel:+${r.contact.phone.replace(/^\+/, "")}`}>+{r.contact.phone.replace(/^\+/, "")}</a>
              {r.contact.email ? (
                <>
                  {" · "}
                  <a href={`mailto:${r.contact.email}`}>{r.contact.email}</a>
                </>
              ) : null}
            </p>
            <p className="small muted" style={{ margin: 0 }}>
              Prefers {humanize(r.contact.preferredContact).toLowerCase()}
            </p>
          </section>

          <section className="panel panel-pad stack-sm" aria-labelledby="quotes-h">
            <h2 id="quotes-h" className={styles.h2}>
              Quotes
            </h2>
            {r.quotes.length ? (
              <ul className="list-rows">
                {r.quotes.map((x) => (
                  <li key={x.id}>
                    <Link className="list-row small" href={`/owner/quotes/${x.id}`}>
                      <span>{x.number}</span>
                      <span className="row">
                        {x.totalMinor !== null ? <Price minor={x.totalMinor} currency={x.currency ?? undefined} /> : null}
                        <StatusBadge status={x.status} />
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="small muted" style={{ margin: 0 }}>
                No quote yet.
              </p>
            )}
            {!["converted", "closed"].includes(r.status) ? (
              <button className="btn btn-primary btn-sm" disabled={busy} onClick={createQuote} style={{ justifySelf: "start" }}>
                Draft a quote
              </button>
            ) : null}
          </section>

          {transitions[r.status]?.length ? (
            <section className="panel panel-pad stack-sm" aria-labelledby="status-h">
              <h2 id="status-h" className={styles.h2}>
                Status
              </h2>
              <select className="select" aria-label="New status" value={status} onChange={(e) => setStatus(e.target.value)}>
                <option value="">Change status</option>
                {transitions[r.status]!.map((s) => (
                  <option key={s} value={s}>
                    {requestLabels[s] ?? humanize(s)}
                  </option>
                ))}
              </select>
              {status ? (
                <>
                  <label className="field">
                    <span className="label">{status === "need_information" ? "What do you need from the customer?" : "Message to the customer (optional)"}</span>
                    <textarea className="textarea" rows={3} value={message} onChange={(e) => setMessage(e.target.value)} />
                  </label>
                  <button className="btn btn-sm" disabled={busy} onClick={() => update({ status, message }, "Status updated.")}>
                    Update status
                  </button>
                </>
              ) : null}
            </section>
          ) : null}

          <section className="panel panel-pad stack-sm" aria-labelledby="notes-h">
            <h2 id="notes-h" className={styles.h2}>
              Internal notes
            </h2>
            <textarea className="textarea" rows={5} aria-label="Internal notes" value={notes ?? r.internalNotes ?? ""} onChange={(e) => setNotes(e.target.value)} />
            {notes !== null && notes !== (r.internalNotes ?? "") ? (
              <button className="btn btn-sm" disabled={busy} onClick={() => update({ internalNotes: notes }, "Notes saved.")}>
                Save notes
              </button>
            ) : (
              <p className="tiny muted" style={{ margin: 0 }}>
                Only staff can see these.
              </p>
            )}
          </section>
        </aside>
      </div>
    </>
  );
}

function Values({ title, v, label }: { title: string; v: MeasurementVersion; label: (k: string) => string }) {
  return (
    <div className="stack-sm">
      <p className="small" style={{ margin: 0 }}>
        <strong>{title}</strong> <span className="muted">· {formatDate(v.createdAt, "short")}</span>
      </p>
      <dl className={styles.dl}>
        {v.heightMm ? (
          <>
            <dt>Height</dt>
            <dd className="tabular">{formatLength(v.heightMm, v.unit)}</dd>
          </>
        ) : null}
        {Object.entries(v.valuesMm).map(([k, mm]) => (
          <div key={k} style={{ display: "contents" }}>
            <dt>{label(k)}</dt>
            <dd className="tabular">{formatLength(mm, v.unit)}</dd>
          </div>
        ))}
      </dl>
      {v.reviewFlags?.length ? (
        <ul className="small" style={{ margin: 0, paddingLeft: 18, color: "var(--warning)" }}>
          {v.reviewFlags.map((f) => (
            <li key={f.message}>{f.message}</li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function VerifyForm({ requestId, fields, start, onDone }: { requestId: string; fields: MeasurementField[]; start: MeasurementVersion | null; onDone: () => void }) {
  const toast = useToast();
  const [state, setState] = useState<MeasureState>(() => (start ? stateFromMM(start.valuesMm, start.heightMm, start.unit) : { unit: "cm", height: "", values: {} }));
  const [notes, setNotes] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  async function save() {
    setBusy(true);
    try {
      await api(`/owner/requests/${requestId}/verify-measurements`, { body: { ...measurePayload(state), notes } });
      toast("Verified measurements saved.");
      onDone();
    } catch (e) {
      if (e instanceof ApiError) setErrors(e.fields);
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="panel panel-pad stack">
      <MeasurementForm fields={fields} state={state} onChange={setState} serverErrors={errors} requireAll />
      <label className="field">
        <span className="label">Notes (optional)</span>
        <textarea className="textarea" rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
      </label>
      <div className="row-wrap">
        <button className="btn btn-primary btn-sm" disabled={busy} onClick={save}>
          Save as verified
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onDone}>
          Cancel
        </button>
      </div>
    </div>
  );
}
