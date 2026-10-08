"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { formatDateTime, humanize } from "@/lib/format";
import { exponentOf, toMinor } from "@/lib/money";
import { useSession } from "@/components/providers/session";
import { useToast } from "@/components/providers/toast";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

type Business = {
  name: string;
  tagline: string;
  legalName: string;
  registrationNumber: string;
  taxId: string;
  logoUrl: string;
  phone: string;
  whatsapp: string;
  email: string;
  address: { line1: string; line2: string; city: string; region: string; country: string; postalCode: string; mapUrl: string; lat: number | null; lng: number | null };
  openingHours: { days: string; hours: string }[];
  currency: string;
  locale: string;
  timezone: string;
  countryCode: string;
  taxRateBp: number;
  taxLabel: string;
  pricesIncludeTax: boolean;
  quoteValidityDays: number;
  depositPercentBp: number;
  appointment: { slotMinutes: number; bufferMinutes: number; minNoticeHours: number; maxAdvanceDays: number; durations: Record<string, number>; cancelNoticeHours: number };
  delivery: { key: string; method: string; label: string; feeMinor: number; description: string; active: boolean }[];
  social: { network: string; url: string }[];
  orderWorkflow: string[];
  referenceRetentionDays: number;
};
type Flag = { key: string; enabled: boolean; description: string; updatedAt: string; blocker?: string };

const allStages = ["submitted", "under_review", "quote_sent", "awaiting_customer", "deposit_paid", "measurements_pending", "measurements_verified", "material_pending", "patterning", "cutting", "sewing", "quality_check", "fitting_scheduled", "fitting", "alteration", "ready", "dispatched", "delivered"];
const networks = ["instagram", "facebook", "tiktok", "whatsapp", "x", "youtube", "pinterest", "linkedin"];

export function OwnerSettings() {
  const q = useQuery({ queryKey: ["owner", "business"], queryFn: () => api<Business>("/owner/settings/business") });
  return (
    <>
      <PageHead title="Settings" />
      {q.data ? <BusinessForm key={JSON.stringify(q.data)} initial={q.data} /> : <div className="skeleton" style={{ height: 480 }} />}
      <Flags />
    </>
  );
}

function BusinessForm({ initial }: { initial: Business }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [b, setB] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const exp = exponentOf(b.currency);
  const set = <K extends keyof Business>(k: K, v: Business[K]) => setB((x) => ({ ...x, [k]: v }));
  const addr = (k: keyof Business["address"], v: string) => setB((x) => ({ ...x, address: { ...x.address, [k]: v } }));
  const appt = (k: keyof Business["appointment"], v: number) => setB((x) => ({ ...x, appointment: { ...x.appointment, [k]: v } }));

  async function save() {
    setBusy(true);
    setErrors({});
    try {
      await api("/owner/settings/business", { method: "PUT", body: b });
      toast("Settings saved.");
      await qc.invalidateQueries({ queryKey: ["owner", "business"] });
    } catch (e) {
      if (e instanceof ApiError) setErrors(e.fields);
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }
  const text = (label: string, value: string, on: (v: string) => void, err?: string, hint?: string) => (
    <label className="field">
      <span className="label">{label}</span>
      <input className="input" value={value} onChange={(e) => on(e.target.value)} aria-invalid={Boolean(err)} />
      {hint ? <span className="hint">{hint}</span> : null}
      {err ? <span className="error">{err}</span> : null}
    </label>
  );
  const num = (label: string, value: number, on: (v: number) => void, hint?: string) => (
    <label className="field">
      <span className="label">{label}</span>
      <input className="input tabular" inputMode="numeric" value={String(value)} onChange={(e) => on(Number(e.target.value) || 0)} />
      {hint ? <span className="hint">{hint}</span> : null}
    </label>
  );

  return (
    <div className="stack-lg">
      <section className="stack-sm" aria-labelledby="biz-h">
        <h2 id="biz-h" className={styles.h2}>
          Business
        </h2>
        <div className="form-grid cols-2">
          {text("Name shown on the site", b.name, (v) => set("name", v), errors.name)}
          {text("Tagline", b.tagline, (v) => set("tagline", v))}
          {text("Registered business name", b.legalName ?? "", (v) => set("legalName", v), undefined, "Shown in the footer and on invoices.")}
          {text("Trade register number (RCCM)", b.registrationNumber ?? "", (v) => set("registrationNumber", v))}
          {text("Taxpayer number (NIU)", b.taxId ?? "", (v) => set("taxId", v))}
          {text("Logo image address", b.logoUrl, (v) => set("logoUrl", v), undefined, "Leave empty to show the name in type.")}
          {text("Phone", b.phone, (v) => set("phone", v), errors.phone)}
          {text("WhatsApp number", b.whatsapp, (v) => set("whatsapp", v), errors.whatsapp)}
          {text("Email", b.email, (v) => set("email", v), errors.email)}
        </div>
      </section>

      <section className="stack-sm" aria-labelledby="addr-h">
        <h2 id="addr-h" className={styles.h2}>
          Address and hours
        </h2>
        <div className="form-grid cols-2">
          {text("Street", b.address.line1, (v) => addr("line1", v))}
          {text("Building or landmark", b.address.line2, (v) => addr("line2", v))}
          {text("City", b.address.city, (v) => addr("city", v))}
          {text("Region", b.address.region, (v) => addr("region", v))}
          {text("Country", b.address.country, (v) => addr("country", v))}
          {text("Map link", b.address.mapUrl, (v) => addr("mapUrl", v), errors["address.mapUrl"])}
        </div>
        {b.openingHours.map((h, i) => (
          <div key={i} className="row-wrap">
            <input className="input" aria-label={`Opening days ${i + 1}`} value={h.days} onChange={(e) => set("openingHours", b.openingHours.map((x, j) => (j === i ? { ...x, days: e.target.value } : x)))} style={{ width: 220 }} />
            <input className="input" aria-label={`Opening hours ${i + 1}`} value={h.hours} onChange={(e) => set("openingHours", b.openingHours.map((x, j) => (j === i ? { ...x, hours: e.target.value } : x)))} style={{ width: 220 }} />
            <button className="icon-btn" aria-label={`Remove opening hours ${i + 1}`} onClick={() => set("openingHours", b.openingHours.filter((_, j) => j !== i))}>
              <X size={16} aria-hidden />
            </button>
          </div>
        ))}
        <button className="btn btn-sm" style={{ justifySelf: "start" }} onClick={() => set("openingHours", [...b.openingHours, { days: "", hours: "" }])}>
          Add opening hours
        </button>
      </section>

      <section className="stack-sm" aria-labelledby="soc-h">
        <h2 id="soc-h" className={styles.h2}>
          Social links
        </h2>
        {b.social.map((s, i) => (
          <div key={i} className="row-wrap">
            <select className="select" aria-label={`Network ${i + 1}`} value={s.network} style={{ width: "auto" }} onChange={(e) => set("social", b.social.map((x, j) => (j === i ? { ...x, network: e.target.value } : x)))}>
              {networks.map((n) => (
                <option key={n} value={n}>
                  {humanize(n)}
                </option>
              ))}
            </select>
            <input className="input" aria-label={`Link ${i + 1}`} value={s.url} onChange={(e) => set("social", b.social.map((x, j) => (j === i ? { ...x, url: e.target.value } : x)))} style={{ flex: 1, minWidth: 240 }} />
            <button className="icon-btn" aria-label={`Remove link ${i + 1}`} onClick={() => set("social", b.social.filter((_, j) => j !== i))}>
              <X size={16} aria-hidden />
            </button>
          </div>
        ))}
        {errors.social ? <p className="error small">{errors.social}</p> : null}
        <button className="btn btn-sm" style={{ justifySelf: "start" }} onClick={() => set("social", [...b.social, { network: "instagram", url: "https://" }])}>
          Add a link
        </button>
      </section>

      <section className="stack-sm" aria-labelledby="money-h">
        <h2 id="money-h" className={styles.h2}>
          Prices, tax and quotes
        </h2>
        <div className="form-grid cols-2">
          {text("Tax name", b.taxLabel, (v) => set("taxLabel", v))}
          {num("Tax rate (basis points, 1925 = 19.25%)", b.taxRateBp, (v) => set("taxRateBp", v))}
          {num("Default deposit (basis points, 5000 = 50%)", b.depositPercentBp, (v) => set("depositPercentBp", v))}
          {num("Quotes are valid for (days)", b.quoteValidityDays, (v) => set("quoteValidityDays", v))}
          {num("Delete unused reference photos after (days)", b.referenceRetentionDays, (v) => set("referenceRetentionDays", v))}
        </div>
        <label className="check">
          <input type="checkbox" checked={b.pricesIncludeTax} onChange={(e) => set("pricesIncludeTax", e.target.checked)} />
          <span>Prices already include tax</span>
        </label>
        <p className="small muted" style={{ margin: 0 }}>
          Currency {b.currency}, time zone {b.timezone}. Changing these needs a developer because it affects stored orders.
        </p>
      </section>

      <section className="stack-sm" aria-labelledby="del-h">
        <h2 id="del-h" className={styles.h2}>
          Delivery and pickup
        </h2>
        {b.delivery.map((d, i) => {
          const sd = (patch: Partial<Business["delivery"][number]>) => set("delivery", b.delivery.map((x, j) => (j === i ? { ...x, ...patch } : x)));
          return (
            <div key={i} className="row-wrap" style={{ alignItems: "end" }}>
              <label className="field">
                <span className="label">Name</span>
                <input className="input" value={d.label} onChange={(e) => sd({ label: e.target.value })} />
              </label>
              <label className="field">
                <span className="label">Type</span>
                <select className="select" value={d.method} onChange={(e) => sd({ method: e.target.value })}>
                  <option value="pickup">Pickup</option>
                  <option value="local_delivery">Local delivery</option>
                  <option value="courier">Courier</option>
                </select>
              </label>
              <label className="field">
                <span className="label">Fee ({b.currency})</span>
                <input className="input tabular" inputMode="decimal" value={String(d.feeMinor / 10 ** exp)} onChange={(e) => sd({ feeMinor: toMinor(e.target.value || "0", b.currency) ?? 0 })} style={{ width: 120 }} />
              </label>
              <label className="check">
                <input type="checkbox" checked={d.active} onChange={(e) => sd({ active: e.target.checked })} />
                <span>Offered</span>
              </label>
            </div>
          );
        })}
        <button className="btn btn-sm" style={{ justifySelf: "start" }} onClick={() => set("delivery", [...b.delivery, { key: `zone-${b.delivery.length + 1}`, method: "local_delivery", label: "", feeMinor: 0, description: "", active: true }])}>
          Add a delivery option
        </button>
      </section>

      <section className="stack-sm" aria-labelledby="ap-h">
        <h2 id="ap-h" className={styles.h2}>
          Appointments
        </h2>
        <div className="form-grid cols-2">
          {num("Gap between bookings (minutes)", b.appointment.bufferMinutes, (v) => appt("bufferMinutes", v))}
          {num("Earliest booking (hours ahead)", b.appointment.minNoticeHours, (v) => appt("minNoticeHours", v))}
          {num("Latest booking (days ahead)", b.appointment.maxAdvanceDays, (v) => appt("maxAdvanceDays", v))}
          {num("Customers can change until (hours before)", b.appointment.cancelNoticeHours, (v) => appt("cancelNoticeHours", v))}
        </div>
        <div className="form-grid cols-2">
          {Object.entries(b.appointment.durations).map(([k, v]) => num(`${humanize(k)} length (minutes)`, v, (n) => setB((x) => ({ ...x, appointment: { ...x.appointment, durations: { ...x.appointment.durations, [k]: n } } }))))}
        </div>
      </section>

      <section className="stack-sm" aria-labelledby="wf-h">
        <h2 id="wf-h" className={styles.h2}>
          Order stages you use
        </h2>
        <div className="row-wrap">
          {allStages.map((s) => (
            <label key={s} className="check">
              <input
                type="checkbox"
                checked={b.orderWorkflow.includes(s)}
                onChange={(e) => set("orderWorkflow", e.target.checked ? allStages.filter((x) => x === s || b.orderWorkflow.includes(x)) : b.orderWorkflow.filter((x) => x !== s))}
              />
              <span>{humanize(s)}</span>
            </label>
          ))}
        </div>
      </section>

      <div>
        <button className="btn btn-primary" disabled={busy} onClick={save}>
          Save settings
        </button>
      </div>
    </div>
  );
}

function Flags() {
  const qc = useQueryClient();
  const toast = useToast();
  const { user } = useSession();
  const q = useQuery({ queryKey: ["owner", "flags"], queryFn: () => api<Flag[]>("/owner/flags") });
  async function toggle(f: Flag, enabled: boolean) {
    try {
      await api(`/owner/flags/${f.key}`, { method: "PUT", body: { enabled } });
      toast(`${humanize(f.key)} ${enabled ? "turned on" : "turned off"}.`);
      await qc.invalidateQueries({ queryKey: ["owner", "flags"] });
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    }
  }
  return (
    <section className="stack-sm" aria-labelledby="flags-h">
      <h2 id="flags-h" className={styles.h2}>
        Features
      </h2>
      <ul className="list-rows">
        {(q.data ?? []).map((f) => {
          const paymentFlag = f.key.startsWith("payments") || f.key === "online_payments";
          const locked = paymentFlag && !user?.permissions.includes("payments.settings");
          return (
            <li key={f.key} className="list-row" style={{ alignItems: "flex-start" }}>
              <span className="stack-xs">
                <strong>{humanize(f.key)}</strong>
                <span className="small muted">{f.description}</span>
                {f.blocker && !f.enabled ? <span className="small" style={{ color: "var(--warning)" }}>{f.blocker}</span> : null}
                <span className="tiny faint">Changed {formatDateTime(f.updatedAt)}</span>
              </span>
              <label className="check">
                <input type="checkbox" checked={f.enabled} disabled={locked || (!f.enabled && Boolean(f.blocker))} onChange={(e) => toggle(f, e.target.checked)} />
                <span>{f.enabled ? "On" : "Off"}</span>
              </label>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
