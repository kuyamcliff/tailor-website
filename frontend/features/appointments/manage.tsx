"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { CalendarCheck2, MapPin } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { useAccessToken, useUrlFlag } from "@/lib/client-hooks";
import type { Appointment } from "@/lib/types";
import { StatusBadge } from "@/components/ui/status-badge";
import { useConfig } from "@/components/providers/config";
import { useToast } from "@/components/providers/toast";
import { formatDateTime } from "@/lib/format";

type Resp = { appointment: Appointment; timezone: string; cancelNoticeHours: number };

export function ManageAppointment({ id }: { id: string }) {
  const cfg = useConfig();
  const toast = useToast();
  const token = useAccessToken("appointment", id);
  const booked = useUrlFlag("booked");
  const [mode, setMode] = useState<"view" | "reschedule">("view");
  const [slot, setSlot] = useState("");
  const [busy, setBusy] = useState(false);
  const q = useQuery({ queryKey: ["appt", id, token], enabled: token !== null, queryFn: () => api<Resp>(`/appointments/${id}`, { accessToken: token || undefined }) });
  const a = q.data?.appointment;
  const tz = q.data?.timezone ?? cfg.business.timezone;
  const slots = useQuery({
    queryKey: ["slots", a?.type, "reschedule"],
    enabled: mode === "reschedule" && Boolean(a),
    queryFn: () => api<{ slots: string[] }>(`/appointments/slots?type=${a!.type}`),
  });

  async function change(action: "cancel" | "reschedule") {
    setBusy(true);
    try {
      await api(`/appointments/${id}/change`, { body: { action, startsAt: action === "reschedule" ? slot : undefined }, accessToken: token || undefined });
      toast(action === "cancel" ? "Your appointment has been cancelled." : "Your appointment has been moved.");
      setMode("view");
      await q.refetch();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
      if (e instanceof ApiError && e.code === "slot_taken") await slots.refetch();
    } finally {
      setBusy(false);
    }
  }

  if (q.isLoading || token === null) return <div className="container-narrow section-tight"><div className="skeleton" style={{ height: 320 }} /></div>;
  if (!a)
    return (
      <div className="container-narrow section-tight stack-lg">
        <h1 className="display-2">We could not open this appointment.</h1>
        <Link className="btn btn-primary" href="/appointments">
          Book a new appointment
        </Link>
      </div>
    );
  const when = formatDateTime(a.startsAt, tz, true);
  return (
    <div className="container-narrow section-tight stack-lg">
      {booked && a.status === "booked" ? (
        <p className="notice notice-success" role="status">
          <CalendarCheck2 size={18} aria-hidden style={{ color: "var(--success)", flex: "none" }} />
          <span>You are booked. We will send a reminder the day before.</span>
        </p>
      ) : null}
      <header className="stack-sm">
        <span className="eyebrow">Appointment {a.number}</span>
        <h1 className="display-2">{a.typeLabel}</h1>
        <p className="lede">{when}</p>
        <div className="row-wrap">
          <StatusBadge status={a.status} />
          {a.orderNumber ? <span className="badge">Order {a.orderNumber}</span> : null}
        </div>
      </header>
      {a.location !== "video" && cfg.business.address.line1 ? (
        <p className="row">
          <MapPin size={18} aria-hidden style={{ color: "var(--gold)" }} />
          {[cfg.business.address.line1, cfg.business.address.city].filter(Boolean).join(", ")}
          {cfg.business.address.mapUrl ? (
            <a className="link small" href={cfg.business.address.mapUrl} target="_blank" rel="noopener noreferrer">
              Directions
            </a>
          ) : null}
        </p>
      ) : a.location === "video" ? (
        <p className="muted">We will send you the video call link before the appointment.</p>
      ) : null}
      {a.customerNotes ? <p className="muted">Your note: {a.customerNotes}</p> : null}
      {a.status === "booked" ? (
        a.canChange ? (
          mode === "view" ? (
            <div className="row-wrap">
              <button className="btn" onClick={() => setMode("reschedule")}>
                Change time
              </button>
              <button className="btn btn-danger" disabled={busy} onClick={() => confirm("Cancel this appointment?") && change("cancel")}>
                Cancel appointment
              </button>
            </div>
          ) : (
            <section className="panel panel-pad stack">
              <h2 className="title">Choose a new time</h2>
              {slots.isLoading ? (
                <div className="skeleton" style={{ height: 120 }} />
              ) : (
                <select className="select" value={slot} onChange={(e) => setSlot(e.target.value)} aria-label="New time">
                  <option value="">Select a time</option>
                  {(slots.data?.slots ?? []).map((s) => (
                    <option key={s} value={s}>
                      {formatDateTime(s, tz)}
                    </option>
                  ))}
                </select>
              )}
              <div className="row-wrap">
                <button className="btn btn-primary" disabled={!slot || busy} onClick={() => change("reschedule")}>
                  Move appointment
                </button>
                <button className="btn btn-ghost" onClick={() => setMode("view")}>
                  Keep current time
                </button>
              </div>
            </section>
          )
        ) : (
          <p className="notice">Changes need at least {q.data!.cancelNoticeHours} hours notice. Please call or message us to change this appointment.</p>
        )
      ) : (
        <Link href="/appointments" className="btn">
          Book another appointment
        </Link>
      )}
    </div>
  );
}
