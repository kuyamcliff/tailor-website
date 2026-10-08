"use client";

import Link from "next/link";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { useNow } from "@/lib/client-hooks";
import { formatDay, formatDateTime, formatTime, humanize } from "@/lib/format";
import type { Appointment } from "@/lib/types";
import { useSession } from "@/components/providers/session";
import { useToast } from "@/components/providers/toast";
import { StatusBadge } from "@/components/ui/status-badge";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

type Block = { id: string; startsAt: string; endsAt: string; kind: string; reason: string };
type Rule = { id?: string; weekday: number; startMinute: number; endMinute: number };
const weekdays = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

function startOfWeek(ts: number, offsetWeeks: number) {
  const d = new Date(ts);
  d.setHours(0, 0, 0, 0);
  const day = (d.getDay() + 6) % 7; // Monday first
  d.setDate(d.getDate() - day + offsetWeeks * 7);
  return d;
}

const hhmm = (m: number) => `${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`;
const toMin = (s: string) => {
  const [h, m] = s.split(":").map(Number);
  return (h ?? 0) * 60 + (m ?? 0);
};

export function OwnerAppointments() {
  const now = useNow();
  const qc = useQueryClient();
  const toast = useToast();
  const { user } = useSession();
  const canWrite = user?.permissions.includes("appointments.write");
  const [week, setWeek] = useState(0);
  const from = startOfWeek(now, week);
  const to = new Date(from);
  to.setDate(to.getDate() + 7);
  const q = useQuery({
    queryKey: ["owner", "appointments", from.toISOString()],
    queryFn: () => api<{ appointments: Appointment[]; blocks: Block[]; timezone: string }>(`/owner/appointments?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`),
  });
  const tz = q.data?.timezone;
  const days = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(from);
    d.setDate(d.getDate() + i);
    return d;
  });
  const reload = () => qc.invalidateQueries({ queryKey: ["owner", "appointments"] });

  async function setStatus(a: Appointment, status: "completed" | "no_show") {
    try {
      await api(`/owner/appointments/${a.id}`, { method: "PATCH", body: { status } });
      toast(status === "completed" ? "Marked as completed." : "Marked as no-show.");
      await reload();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    }
  }
  async function cancel(a: Appointment) {
    if (!confirm(`Cancel ${a.customerName}'s appointment? They will be notified.`)) return;
    try {
      await api(`/owner/appointments/${a.id}/change`, { body: { action: "cancel" } });
      toast("Appointment cancelled.");
      await reload();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    }
  }

  return (
    <>
      <PageHead
        title="Appointments"
        sub={`Week of ${formatDay(from, tz, true)}${tz ? ` · times in ${tz}` : ""}`}
        actions={
          <div className="row">
            <button className="btn btn-sm" onClick={() => setWeek((w) => w - 1)}>
              Previous week
            </button>
            <button className="btn btn-sm" disabled={week === 0} onClick={() => setWeek(0)}>
              This week
            </button>
            <button className="btn btn-sm" onClick={() => setWeek((w) => w + 1)}>
              Next week
            </button>
          </div>
        }
      />
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 360 }} />
      ) : (
        <div className="stack">
          {days.map((d) => {
            const key = d.toDateString();
            const list = (q.data?.appointments ?? []).filter((a) => new Date(a.startsAt).toDateString() === key);
            const blocks = (q.data?.blocks ?? []).filter((b) => new Date(b.startsAt) <= new Date(d.getTime() + 86400000) && new Date(b.endsAt) >= d);
            return (
              <section key={key} aria-label={formatDay(d, tz, true)} className="stack-sm">
                <h2 className={styles.h2}>{formatDay(d, tz)}</h2>
                {blocks.map((b) => (
                  <p key={b.id} className="small" style={{ margin: 0, color: "var(--warning)" }}>
                    {humanize(b.kind)}: {formatDateTime(b.startsAt, tz)} to {formatDateTime(b.endsAt, tz)}
                    {b.reason ? ` · ${b.reason}` : ""}
                  </p>
                ))}
                {list.length ? (
                  <ul className="list-rows">
                    {list.map((a) => (
                      <li key={a.id} className="list-row" style={{ flexWrap: "wrap" }}>
                        <span className="stack-xs">
                          <span>
                            <strong className="tabular">
                              {formatTime(a.startsAt, tz)} to {formatTime(a.endsAt, tz)}
                            </strong>{" "}
                            <Link className="link" href={`/owner/customers/${a.customerId}`}>
                              {a.customerName}
                            </Link>
                          </span>
                          <span className="small muted">
                            {a.typeLabel} · {humanize(a.location)}
                            {a.orderNumber ? ` · order ${a.orderNumber}` : ""}
                            {a.customerNotes ? ` · "${a.customerNotes}"` : ""}
                          </span>
                        </span>
                        <span className="row-wrap">
                          <StatusBadge status={a.status} />
                          {canWrite && a.status === "booked" ? (
                            <>
                              {new Date(a.startsAt).getTime() < now ? (
                                <>
                                  <button className="btn btn-sm" onClick={() => setStatus(a, "completed")}>
                                    Completed
                                  </button>
                                  <button className="btn btn-ghost btn-sm" onClick={() => setStatus(a, "no_show")}>
                                    No-show
                                  </button>
                                </>
                              ) : null}
                              <button className="btn btn-ghost btn-sm" onClick={() => cancel(a)}>
                                Cancel
                              </button>
                            </>
                          ) : null}
                        </span>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="small muted" style={{ margin: 0 }}>
                    No appointments.
                  </p>
                )}
              </section>
            );
          })}
        </div>
      )}
      {canWrite ? <Availability /> : null}
    </>
  );
}

function Availability() {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["owner", "availability"], queryFn: () => api<{ rules: Rule[]; blocks: Block[] }>("/owner/availability") });
  if (!q.data) return null;
  return <AvailabilityForm key={JSON.stringify(q.data.rules)} rules={q.data.rules} blocks={q.data.blocks} onSaved={() => qc.invalidateQueries({ queryKey: ["owner"] })} toast={toast} />;
}

function AvailabilityForm({ rules: initial, blocks, onSaved, toast }: { rules: Rule[]; blocks: Block[]; onSaved: () => void; toast: ReturnType<typeof useToast> }) {
  const [rules, setRules] = useState(initial.map((r) => ({ weekday: r.weekday, start: hhmm(r.startMinute), end: hhmm(r.endMinute) })));
  const [block, setBlock] = useState({ start: "", end: "", kind: "blocked", reason: "" });
  const [busy, setBusy] = useState(false);
  async function saveRules() {
    setBusy(true);
    try {
      await api("/owner/availability/rules", { method: "PUT", body: rules.map((r) => ({ weekday: r.weekday, startMinute: toMin(r.start), endMinute: toMin(r.end) })) });
      toast("Opening hours for bookings saved.");
      onSaved();
    } catch (e) {
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }
  async function addBlock() {
    setBusy(true);
    try {
      await api("/owner/availability/blocks", { body: { startsAt: new Date(block.start).toISOString(), endsAt: new Date(block.end).toISOString(), kind: block.kind, reason: block.reason } });
      toast("Time blocked.");
      setBlock({ start: "", end: "", kind: "blocked", reason: "" });
      onSaved();
    } catch (e) {
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }
  async function removeBlock(id: string) {
    await api(`/owner/availability/blocks/${id}`, { method: "DELETE" });
    onSaved();
  }
  return (
    <div className={styles.split}>
      <section className="stack-sm" aria-labelledby="rules-h">
        <h2 id="rules-h" className={styles.h2}>
          Bookable hours
        </h2>
        <p className="small muted" style={{ margin: 0 }}>
          Customers can book inside these windows. Add two windows on the same day for a lunch break.
        </p>
        {rules.map((r, i) => (
          <div key={i} className="row-wrap">
            <select className="select" aria-label="Day" value={r.weekday} style={{ width: "auto" }} onChange={(e) => setRules((x) => x.map((y, j) => (j === i ? { ...y, weekday: Number(e.target.value) } : y)))}>
              {weekdays.map((d, k) => (
                <option key={d} value={k}>
                  {d}
                </option>
              ))}
            </select>
            <input className="input" type="time" aria-label="From" value={r.start} style={{ width: "auto" }} onChange={(e) => setRules((x) => x.map((y, j) => (j === i ? { ...y, start: e.target.value } : y)))} />
            <input className="input" type="time" aria-label="To" value={r.end} style={{ width: "auto" }} onChange={(e) => setRules((x) => x.map((y, j) => (j === i ? { ...y, end: e.target.value } : y)))} />
            <button className="btn btn-ghost btn-sm" onClick={() => setRules((x) => x.filter((_, j) => j !== i))}>
              Remove
            </button>
          </div>
        ))}
        <div className="row-wrap">
          <button className="btn btn-sm" onClick={() => setRules((x) => [...x, { weekday: 1, start: "09:00", end: "17:00" }])}>
            Add window
          </button>
          <button className="btn btn-primary btn-sm" disabled={busy} onClick={saveRules}>
            Save hours
          </button>
        </div>
      </section>
      <section className="stack-sm" aria-labelledby="blocks-h">
        <h2 id="blocks-h" className={styles.h2}>
          Time off and closures
        </h2>
        {blocks.length ? (
          <ul className="list-rows">
            {blocks.map((b) => (
              <li key={b.id} className="list-row small">
                <span>
                  {formatDateTime(b.startsAt)} to {formatDateTime(b.endsAt)}
                  {b.reason ? ` · ${b.reason}` : ""}
                </span>
                <button className="link small" onClick={() => removeBlock(b.id)}>
                  Remove
                </button>
              </li>
            ))}
          </ul>
        ) : null}
        <div className="form-grid cols-2">
          <label className="field">
            <span className="label">From</span>
            <input className="input" type="datetime-local" value={block.start} onChange={(e) => setBlock({ ...block, start: e.target.value })} />
          </label>
          <label className="field">
            <span className="label">To</span>
            <input className="input" type="datetime-local" value={block.end} onChange={(e) => setBlock({ ...block, end: e.target.value })} />
          </label>
          <label className="field">
            <span className="label">Type</span>
            <select className="select" value={block.kind} onChange={(e) => setBlock({ ...block, kind: e.target.value })}>
              <option value="blocked">Unavailable</option>
              <option value="holiday">Holiday</option>
            </select>
          </label>
          <label className="field">
            <span className="label">Reason (staff only)</span>
            <input className="input" value={block.reason} onChange={(e) => setBlock({ ...block, reason: e.target.value })} />
          </label>
        </div>
        <button className="btn btn-sm" style={{ justifySelf: "start" }} disabled={busy || !block.start || !block.end} onClick={addBlock}>
          Block this time
        </button>
      </section>
    </div>
  );
}
