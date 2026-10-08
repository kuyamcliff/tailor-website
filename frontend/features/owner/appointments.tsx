"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { useNow } from "@/lib/client-hooks";
import { formatDay, formatDateTime, formatTime, humanize, zonedTimeToIso } from "@/lib/format";
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
  const sp = useSearchParams();
  const [booking, setBooking] = useState(sp.get("book") === "1");
  const [week, setWeek] = useState(0);
  const team = useQuery({ queryKey: ["owner", "team"], queryFn: () => api<TeamMember[]>("/owner/team") });
  const from = startOfWeek(now, week);
  const to = new Date(from);
  to.setDate(to.getDate() + 7);
  const q = useQuery({
    queryKey: ["owner", "appointments", from.toISOString()],
    queryFn: () =>
      api<{ appointments: Appointment[]; blocks: Block[]; timezone: string }>(
        `/owner/appointments?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`,
      ),
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
  async function assign(a: Appointment, staffId: string) {
    try {
      await api(`/owner/appointments/${a.id}`, { method: "PATCH", body: { staffId } });
      toast("Assigned.");
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
          <div className="row-wrap">
            {canWrite && !booking ? (
              <button className="btn btn-primary btn-sm" onClick={() => setBooking(true)}>
                Book for a customer
              </button>
            ) : null}
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
      {booking && canWrite ? (
        <BookForCustomer
          team={team.data ?? []}
          initial={{
            customerId: sp.get("customer") ?? "",
            orderId: sp.get("order") ?? "",
            requestId: sp.get("request") ?? "",
            type: sp.get("type") ?? "",
          }}
          onDone={() => {
            setBooking(false);
            void reload();
          }}
        />
      ) : null}
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 360 }} />
      ) : (
        <div className="stack">
          {days.map((d) => {
            const key = d.toDateString();
            const list = (q.data?.appointments ?? []).filter((a) => new Date(a.startsAt).toDateString() === key);
            const blocks = (q.data?.blocks ?? []).filter(
              (b) => new Date(b.startsAt) <= new Date(d.getTime() + 86400000) && new Date(b.endsAt) >= d,
            );
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
                            {a.staffName ? ` · with ${a.staffName}` : ""}
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
                              {team.data?.length ? (
                                <select
                                  className="select"
                                  aria-label={`Who sees ${a.customerName}`}
                                  value={a.staffId ?? ""}
                                  onChange={(e) => e.target.value && assign(a, e.target.value)}
                                  style={{ width: "auto" }}
                                >
                                  <option value="">Assign</option>
                                  {team.data.map((m) => (
                                    <option key={m.id} value={m.id}>
                                      {m.name}
                                    </option>
                                  ))}
                                </select>
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

type TeamMember = { id: string; name: string };
type CustomerHit = { id: string; name: string; phone: string | null; email: string | null };

const allTypes: [string, string][] = [
  ["consultation", "Consultation"],
  ["measuring", "Measurement session"],
  ["fitting", "Fitting"],
  ["final_fitting", "Final fitting"],
  ["pickup", "Pickup"],
  ["alteration", "Alteration"],
  ["video_consultation", "Video consultation"],
  ["other", "Other"],
];

// BookForCustomer books on a customer's behalf (for example after a phone call or at a fitting).
// It offers the open slots first; staff can also pick any time and override availability.
function BookForCustomer({
  team,
  initial,
  onDone,
}: {
  team: TeamMember[];
  initial: { customerId: string; orderId: string; requestId: string; type: string };
  onDone: () => void;
}) {
  const toast = useToast();
  const now = useNow();
  const [search, setSearch] = useState("");
  const [customer, setCustomer] = useState<CustomerHit | null>(null);
  const [type, setType] = useState(allTypes.some(([k]) => k === initial.type) ? initial.type : "consultation");
  const [date, setDate] = useState(() => new Date(now).toISOString().slice(0, 10));
  const [slot, setSlot] = useState("");
  const [manual, setManual] = useState(false);
  const [time, setTime] = useState("10:00");
  const [override, setOverride] = useState(false);
  const [staffId, setStaffId] = useState("");
  const [location, setLocation] = useState("");
  const [customerNotes, setCustomerNotes] = useState("");
  const [internalNotes, setInternalNotes] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const preset = useQuery({
    queryKey: ["owner", "customer", initial.customerId],
    enabled: Boolean(initial.customerId) && !customer,
    queryFn: () => api<{ customer: CustomerHit }>(`/owner/customers/${initial.customerId}`),
  });
  const chosen = customer ?? preset.data?.customer ?? null;
  const hits = useQuery({
    queryKey: ["owner", "customers", "pick", search],
    enabled: search.trim().length >= 2 && !chosen,
    queryFn: () => api<{ items: CustomerHit[] }>(`/owner/customers?q=${encodeURIComponent(search.trim())}&limit=8`),
  });
  const slots = useQuery({
    queryKey: ["owner", "slots", type, date],
    enabled: !manual,
    retry: false,
    queryFn: () =>
      api<{ slots: string[]; timezone: string; durationMinutes: number }>(
        `/appointments/slots?type=${type}&from=${date}`,
      ),
  });
  const tz = slots.data?.timezone;
  const daySlots = (slots.data?.slots ?? []).filter((s) => formatDay(s, tz) === formatDay(`${date}T12:00:00Z`, "UTC"));

  async function book() {
    if (!chosen) return;
    const startsAt = manual ? zonedTimeToIso(date, time, tz ?? "UTC") : slot;
    if (!startsAt) {
      setError("Choose a time.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api("/owner/appointments", {
        body: {
          customerId: chosen.id,
          type,
          startsAt,
          orderId: initial.orderId || null,
          requestId: initial.requestId || null,
          staffId: staffId || null,
          location: location.trim(),
          customerNotes: customerNotes.trim(),
          internalNotes: internalNotes.trim(),
          override: manual && override,
        },
      });
      toast(`Booked for ${chosen.name}. They will be notified.`);
      onDone();
    } catch (e) {
      setError(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel panel-pad stack" aria-labelledby="book-h" style={{ marginBottom: 24 }}>
      <h2 id="book-h" className={styles.h2} style={{ margin: 0 }}>
        Book for a customer
      </h2>
      {chosen ? (
        <p className="small" style={{ margin: 0 }}>
          <strong>{chosen.name}</strong> {chosen.phone ? `· ${chosen.phone}` : ""}{" "}
          {!initial.customerId ? (
            <button className="btn btn-ghost btn-sm" onClick={() => setCustomer(null)}>
              Change
            </button>
          ) : null}
        </p>
      ) : (
        <div className="stack-sm">
          <label className="field">
            <span className="label">Customer</span>
            <input
              className="input"
              type="search"
              placeholder="Name, phone or email"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </label>
          {hits.data?.items.length ? (
            <ul className="list-rows" aria-label="Matching customers">
              {hits.data.items.map((c) => (
                <li key={c.id} className="list-row">
                  <span>
                    {c.name} <span className="small muted">{c.phone ?? c.email ?? ""}</span>
                  </span>
                  <button className="btn btn-sm" onClick={() => setCustomer(c)}>
                    Choose
                  </button>
                </li>
              ))}
            </ul>
          ) : search.trim().length >= 2 && hits.isFetched ? (
            <p className="small muted" style={{ margin: 0 }}>
              No customer matches. Customers are created when they send a request, order or message.
            </p>
          ) : null}
        </div>
      )}
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Type</span>
          <select
            className="select"
            value={type}
            onChange={(e) => {
              setType(e.target.value);
              setSlot("");
            }}
          >
            {allTypes.map(([k, label]) => (
              <option key={k} value={k}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span className="label">Day</span>
          <input
            className="input"
            type="date"
            value={date}
            onChange={(e) => {
              setDate(e.target.value);
              setSlot("");
            }}
          />
        </label>
      </div>
      {!manual ? (
        <fieldset className="stack-sm" style={{ border: 0, padding: 0, margin: 0 }}>
          <legend className="label">Open times{tz ? ` (${tz})` : ""}</legend>
          {slots.isError ? (
            <p className="small muted" style={{ margin: 0 }}>
              Online booking is switched off, so open times are not listed. Choose a time yourself.
            </p>
          ) : daySlots.length ? (
            <div className="row-wrap">
              {daySlots.map((s) => (
                <button
                  key={s}
                  type="button"
                  className="btn btn-sm"
                  aria-pressed={slot === s}
                  onClick={() => setSlot(s)}
                >
                  {formatTime(s, tz)}
                </button>
              ))}
            </div>
          ) : slots.isLoading ? null : (
            <p className="small muted" style={{ margin: 0 }}>
              No open times that day.
            </p>
          )}
        </fieldset>
      ) : (
        <div className="row-wrap" style={{ alignItems: "end" }}>
          <label className="field" style={{ width: 140 }}>
            <span className="label">Time{tz ? ` (${tz})` : ""}</span>
            <input className="input" type="time" value={time} onChange={(e) => setTime(e.target.value)} />
          </label>
          <label className="check">
            <input type="checkbox" checked={override} onChange={(e) => setOverride(e.target.checked)} />
            <span>Book even outside opening hours or over another appointment</span>
          </label>
        </div>
      )}
      <label className="check">
        <input type="checkbox" checked={manual} onChange={(e) => setManual(e.target.checked)} />
        <span>Choose another time</span>
      </label>
      <div className="form-grid cols-2">
        {team.length ? (
          <label className="field">
            <span className="label">With</span>
            <select className="select" value={staffId} onChange={(e) => setStaffId(e.target.value)}>
              <option value="">Anyone</option>
              {team.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.name}
                </option>
              ))}
            </select>
          </label>
        ) : null}
        <label className="field">
          <span className="label">Where (optional)</span>
          <input
            className="input"
            value={location}
            placeholder={type === "video_consultation" ? "Video call" : "At the studio"}
            onChange={(e) => setLocation(e.target.value)}
          />
        </label>
      </div>
      <label className="field">
        <span className="label">Note for the customer (optional)</span>
        <input className="input" value={customerNotes} onChange={(e) => setCustomerNotes(e.target.value)} />
      </label>
      <label className="field">
        <span className="label">Internal note (staff only)</span>
        <input className="input" value={internalNotes} onChange={(e) => setInternalNotes(e.target.value)} />
      </label>
      {initial.orderId ? (
        <p className="tiny muted" style={{ margin: 0 }}>
          Linked to the order. A fitting moves the order to Fitting scheduled.
        </p>
      ) : null}
      {error ? (
        <p className="error small" role="alert" style={{ margin: 0 }}>
          {error}
        </p>
      ) : null}
      <div className="row-wrap">
        <button className="btn btn-primary btn-sm" disabled={busy || !chosen || (!manual && !slot)} onClick={book}>
          Book appointment
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onDone}>
          Cancel
        </button>
      </div>
    </section>
  );
}

function Availability() {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({
    queryKey: ["owner", "availability"],
    queryFn: () => api<{ rules: Rule[]; blocks: Block[] }>("/owner/availability"),
  });
  if (!q.data) return null;
  return (
    <AvailabilityForm
      key={JSON.stringify(q.data.rules)}
      rules={q.data.rules}
      blocks={q.data.blocks}
      onSaved={() => qc.invalidateQueries({ queryKey: ["owner"] })}
      toast={toast}
    />
  );
}

function AvailabilityForm({
  rules: initial,
  blocks,
  onSaved,
  toast,
}: {
  rules: Rule[];
  blocks: Block[];
  onSaved: () => void;
  toast: ReturnType<typeof useToast>;
}) {
  const [rules, setRules] = useState(
    initial.map((r) => ({ weekday: r.weekday, start: hhmm(r.startMinute), end: hhmm(r.endMinute) })),
  );
  const [block, setBlock] = useState({ start: "", end: "", kind: "blocked", reason: "" });
  const [busy, setBusy] = useState(false);
  async function saveRules() {
    setBusy(true);
    try {
      await api("/owner/availability/rules", {
        method: "PUT",
        body: rules.map((r) => ({ weekday: r.weekday, startMinute: toMin(r.start), endMinute: toMin(r.end) })),
      });
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
      await api("/owner/availability/blocks", {
        body: {
          startsAt: new Date(block.start).toISOString(),
          endsAt: new Date(block.end).toISOString(),
          kind: block.kind,
          reason: block.reason,
        },
      });
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
            <select
              className="select"
              aria-label="Day"
              value={r.weekday}
              style={{ width: "auto" }}
              onChange={(e) =>
                setRules((x) => x.map((y, j) => (j === i ? { ...y, weekday: Number(e.target.value) } : y)))
              }
            >
              {weekdays.map((d, k) => (
                <option key={d} value={k}>
                  {d}
                </option>
              ))}
            </select>
            <input
              className="input"
              type="time"
              aria-label="From"
              value={r.start}
              style={{ width: "auto" }}
              onChange={(e) => setRules((x) => x.map((y, j) => (j === i ? { ...y, start: e.target.value } : y)))}
            />
            <input
              className="input"
              type="time"
              aria-label="To"
              value={r.end}
              style={{ width: "auto" }}
              onChange={(e) => setRules((x) => x.map((y, j) => (j === i ? { ...y, end: e.target.value } : y)))}
            />
            <button className="btn btn-ghost btn-sm" onClick={() => setRules((x) => x.filter((_, j) => j !== i))}>
              Remove
            </button>
          </div>
        ))}
        <div className="row-wrap">
          <button
            className="btn btn-sm"
            onClick={() => setRules((x) => [...x, { weekday: 1, start: "09:00", end: "17:00" }])}
          >
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
            <input
              className="input"
              type="datetime-local"
              value={block.start}
              onChange={(e) => setBlock({ ...block, start: e.target.value })}
            />
          </label>
          <label className="field">
            <span className="label">To</span>
            <input
              className="input"
              type="datetime-local"
              value={block.end}
              onChange={(e) => setBlock({ ...block, end: e.target.value })}
            />
          </label>
          <label className="field">
            <span className="label">Type</span>
            <select
              className="select"
              value={block.kind}
              onChange={(e) => setBlock({ ...block, kind: e.target.value })}
            >
              <option value="blocked">Unavailable</option>
              <option value="holiday">Holiday</option>
            </select>
          </label>
          <label className="field">
            <span className="label">Reason (staff only)</span>
            <input
              className="input"
              value={block.reason}
              onChange={(e) => setBlock({ ...block, reason: e.target.value })}
            />
          </label>
        </div>
        <button
          className="btn btn-sm"
          style={{ justifySelf: "start" }}
          disabled={busy || !block.start || !block.end}
          onClick={addBlock}
        >
          Block this time
        </button>
      </section>
    </div>
  );
}
