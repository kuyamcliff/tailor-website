"use client";

import { useRouter } from "next/navigation";
import { useNow } from "@/lib/client-hooks";
import { useQuery } from "@tanstack/react-query";
import { useMemo, useRef, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { api, ApiError, newIdempotencyKey } from "@/lib/api";
import { rememberLink, tokenFor } from "@/lib/links";
import { Field } from "@/components/ui/field";
import { useSession } from "@/components/providers/session";
import { useConfig } from "@/components/providers/config";
import styles from "./booking.module.css";

export const appointmentTypes = [
  { key: "consultation", label: "Consultation", body: "Talk through a new garment and see fabrics." },
  { key: "measuring", label: "Measuring session", body: "A tailor takes your full measurements." },
  { key: "fitting", label: "Fitting", body: "Try on a garment in progress." },
  { key: "final_fitting", label: "Final fitting", body: "The last check before your garment is finished." },
  { key: "alteration", label: "Alteration", body: "Bring a garment you would like adjusted." },
  { key: "pickup", label: "Pickup", body: "Collect a finished order." },
  { key: "video_consultation", label: "Video consultation", body: "A consultation by video call." },
];

type Slots = { slots: string[]; timezone: string; durationMinutes: number; from: string; to: string };

function ymd(d: Date, tz: string) {
  return new Intl.DateTimeFormat("en-CA", { timeZone: tz, year: "numeric", month: "2-digit", day: "2-digit" }).format(d);
}

export function BookingFlow({ initialType, orderId, requestId }: { initialType?: string; orderId?: string; requestId?: string }) {
  const router = useRouter();
  const cfg = useConfig();
  const tz = cfg.business.timezone || "Africa/Douala";
  const { user } = useSession();
  const [type, setType] = useState(appointmentTypes.some((t) => t.key === initialType) ? initialType! : "consultation");
  const [weekOffset, setWeekOffset] = useState(0);
  const now = useNow();
  const from = useMemo(() => ymd(new Date(now + weekOffset * 14 * 86400000), tz), [now, weekOffset, tz]);
  const slots = useQuery({ queryKey: ["slots", type, from], queryFn: () => api<Slots>(`/appointments/slots?type=${type}&from=${from}`), refetchInterval: 60000 });
  const [day, setDay] = useState<string | null>(null);
  const [slot, setSlot] = useState<string | null>(null);
  const [contact, setContact] = useState({ name: user?.name ?? "", phone: "", email: user?.email ?? "", preferredContact: "whatsapp" });
  const [notes, setNotes] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const key = useRef(newIdempotencyKey());

  const byDay = useMemo(() => {
    const m = new Map<string, string[]>();
    for (const s of slots.data?.slots ?? []) {
      const d = ymd(new Date(s), tz);
      m.set(d, [...(m.get(d) ?? []), s]);
    }
    return m;
  }, [slots.data, tz]);
  const days = useMemo(() => {
    const out: string[] = [];
    const start = new Date(`${from}T12:00:00Z`);
    for (let i = 0; i < 14; i++) out.push(ymd(new Date(start.getTime() + i * 86400000), tz));
    return out;
  }, [from, tz]);
  const activeDay = day && byDay.has(day) ? day : days.find((d) => byDay.has(d)) ?? null;

  async function book(e: React.FormEvent) {
    e.preventDefault();
    if (!slot) {
      setError("Choose a time first.");
      return;
    }
    setBusy(true);
    setError("");
    setErrors({});
    try {
      const r = await api<{ id: string; number: string; accessToken: string }>("/appointments", {
        idempotencyKey: key.current,
        accessToken: orderId ? tokenFor("order", orderId) || undefined : undefined,
        body: { type, startsAt: slot, contact: { ...contact, email: contact.email || null }, notes, orderId, requestId },
      });
      rememberLink({ kind: "appointment", id: r.id, number: r.number, token: r.accessToken });
      router.push(`/appointments/${r.id}?token=${r.accessToken}&booked=1`);
    } catch (err) {
      if (err instanceof ApiError) {
        setErrors(Object.fromEntries(Object.entries(err.fields).map(([k, v]) => [k.replace("contact.", ""), v])));
        setError(err.message);
        if (err.code === "slot_taken") {
          setSlot(null);
          key.current = newIdempotencyKey();
          await slots.refetch();
        }
      }
    } finally {
      setBusy(false);
    }
  }

  const fmtDay = (d: string) => new Date(`${d}T12:00:00Z`).toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short", timeZone: "UTC" });
  const fmtTime = (s: string) => new Date(s).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", timeZone: tz });

  return (
    <form onSubmit={book} className={styles.layout} noValidate>
      <div className="stack-lg">
        <fieldset className={styles.fieldset}>
          <legend className="display-3">1. What is it for?</legend>
          <div className="choices">
            {appointmentTypes.map((t) => (
              <label key={t.key} className="choice">
                <input type="radio" name="type" checked={type === t.key} onChange={() => { setType(t.key); setSlot(null); }} />
                <span className="choice-title">{t.label}</span>
                <span className="choice-meta">{t.body}</span>
              </label>
            ))}
          </div>
        </fieldset>

        <fieldset className={styles.fieldset}>
          <legend className="display-3">2. Choose a time</legend>
          <div className={styles.dayNav}>
            <button type="button" className="icon-btn" onClick={() => { setWeekOffset((w) => Math.max(0, w - 1)); setDay(null); }} disabled={weekOffset === 0} aria-label="Earlier dates">
              <ChevronLeft size={18} aria-hidden />
            </button>
            <div className={styles.days} role="group" aria-label="Day">
              {days.map((d) => (
                <button key={d} type="button" className={styles.day} aria-pressed={d === activeDay} disabled={!byDay.has(d)} onClick={() => { setDay(d); setSlot(null); }}>
                  {fmtDay(d)}
                  <span className="tiny">{byDay.get(d)?.length ?? 0 ? `${byDay.get(d)!.length} free` : "Full"}</span>
                </button>
              ))}
            </div>
            <button type="button" className="icon-btn" onClick={() => { setWeekOffset((w) => Math.min(4, w + 1)); setDay(null); }} disabled={weekOffset >= 4} aria-label="Later dates">
              <ChevronRight size={18} aria-hidden />
            </button>
          </div>
          {slots.isLoading ? (
            <div className="skeleton" style={{ height: 100 }} />
          ) : slots.error ? (
            <p className="notice notice-danger">Times could not be loaded. <button type="button" className="link" onClick={() => slots.refetch()}>Try again</button></p>
          ) : activeDay ? (
            <div className={styles.slots} role="radiogroup" aria-label="Time">
              {byDay.get(activeDay)!.map((s) => (
                <label key={s} className={`choice ${styles.slot}`}>
                  <input type="radio" name="slot" checked={slot === s} onChange={() => setSlot(s)} />
                  <span className="choice-title tabular">{fmtTime(s)}</span>
                </label>
              ))}
            </div>
          ) : (
            <p className="notice">No times are free in these two weeks. Try later dates, or contact us.</p>
          )}
          {slots.data ? <p className="tiny muted">Each {appointmentTypes.find((t) => t.key === type)?.label.toLowerCase()} lasts about {slots.data.durationMinutes} minutes.</p> : null}
        </fieldset>
      </div>

      <fieldset className={`panel panel-pad ${styles.fieldset} ${styles.side}`}>
        <legend className="display-3">3. Your details</legend>
        {error ? (
          <p className="notice notice-danger" role="alert">
            {error}
          </p>
        ) : null}
        <Field label="Name" error={errors.name}>
          {(p) => <input {...p} className="input" autoComplete="name" value={contact.name} onChange={(e) => setContact({ ...contact, name: e.target.value })} />}
        </Field>
        <Field label="Phone" error={errors.phone}>
          {(p) => <input {...p} className="input" inputMode="tel" autoComplete="tel" value={contact.phone} onChange={(e) => setContact({ ...contact, phone: e.target.value })} />}
        </Field>
        <Field label="Email (optional)" error={errors.email}>
          {(p) => <input {...p} className="input" type="email" autoComplete="email" value={contact.email} onChange={(e) => setContact({ ...contact, email: e.target.value })} />}
        </Field>
        <Field label="Reminders by" error={errors.preferredContact}>
          {(p) => (
            <select {...p} className="select" value={contact.preferredContact} onChange={(e) => setContact({ ...contact, preferredContact: e.target.value })}>
              <option value="whatsapp">WhatsApp</option>
              <option value="sms">SMS</option>
              <option value="phone">Phone call</option>
              <option value="email">Email</option>
            </select>
          )}
        </Field>
        <Field label="Anything we should prepare? (optional)" error={errors.notes}>
          {(p) => <textarea {...p} className="textarea" value={notes} onChange={(e) => setNotes(e.target.value)} maxLength={1000} />}
        </Field>
        {slot ? (
          <p className="small">
            <strong>{appointmentTypes.find((t) => t.key === type)?.label}</strong> on {new Date(slot).toLocaleString("en-GB", { weekday: "long", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit", timeZone: tz })}
          </p>
        ) : null}
        <button className="btn btn-primary btn-block" type="submit" disabled={busy || !slot}>
          {busy ? <span className="spinner" aria-hidden /> : null} Confirm booking
        </button>
      </fieldset>
    </form>
  );
}
