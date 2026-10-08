// Shared display helpers for dates and statuses. No em dashes in any user-facing string.

// Dates are assembled from Intl parts rather than toLocaleString, because Node and browsers ship
// different locale data (for example "Thu, 8 Oct" versus "Thu 8 Oct"), which breaks hydration.
type Parts = Partial<Record<Intl.DateTimeFormatPartTypes, string>>;

function parts(d: Date, opts: Intl.DateTimeFormatOptions): Parts {
  const out: Parts = {};
  for (const p of new Intl.DateTimeFormat("en-GB", { ...opts, hourCycle: "h23" }).formatToParts(d))
    out[p.type] = p.value;
  return out;
}

function valid(iso: string | Date | null | undefined): Date | null {
  if (!iso) return null;
  const d = iso instanceof Date ? iso : new Date(iso);
  return Number.isNaN(d.getTime()) ? null : d;
}

// formatDate: "8 October 2026" (style "long"), "8 Oct" (style "short").
export function formatDate(iso: string | Date | null | undefined, style: "long" | "short" = "long", timeZone?: string) {
  const d = valid(iso);
  if (!d) return "";
  const p = parts(d, {
    day: "numeric",
    month: style === "long" ? "long" : "short",
    year: style === "long" ? "numeric" : undefined,
    timeZone,
  });
  return [p.day, p.month, p.year].filter(Boolean).join(" ");
}

// formatDay: "Thu 8 Oct" or, with long, "Thursday 8 October 2026".
export function formatDay(iso: string | Date | null | undefined, timeZone?: string, long = false) {
  const d = valid(iso);
  if (!d) return "";
  const p = parts(d, {
    weekday: long ? "long" : "short",
    day: "numeric",
    month: long ? "long" : "short",
    year: long ? "numeric" : undefined,
    timeZone,
  });
  return [p.weekday, p.day, p.month, p.year].filter(Boolean).join(" ");
}

// formatTime: "14:30".
export function formatTime(iso: string | Date, timeZone?: string) {
  const d = valid(iso);
  if (!d) return "";
  const p = parts(d, { hour: "2-digit", minute: "2-digit", timeZone });
  return `${p.hour}:${p.minute}`;
}

// formatDateTime: "Thu 8 Oct, 14:30".
export function formatDateTime(iso: string | Date | null | undefined, timeZone?: string, long = false) {
  const d = valid(iso);
  if (!d) return "";
  return `${formatDay(d, timeZone, long)}, ${formatTime(d, timeZone)}`;
}

// zonedTimeToIso turns a wall-clock date and time in a time zone (the atelier's, not the browser's)
// into an ISO instant: zonedTimeToIso("2026-10-08", "14:30", "Africa/Douala") is 13:30 UTC.
export function zonedTimeToIso(date: string, time: string, timeZone: string): string | null {
  const [y, mo, d] = date.split("-").map(Number);
  const [h, mi] = time.split(":").map(Number);
  if (!y || !mo || !d || h === undefined || mi === undefined || Number.isNaN(h) || Number.isNaN(mi)) return null;
  const wall = Date.UTC(y, mo - 1, d, h, mi);
  const offsetAt = (t: number) => {
    const p = parts(new Date(t), {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      timeZone,
    });
    return (
      Date.UTC(Number(p.year), Number(p.month) - 1, Number(p.day), Number(p.hour), Number(p.minute), Number(p.second)) -
      t
    );
  };
  // Two passes settle the offset across daylight saving changes.
  let t = wall - offsetAt(wall);
  t = wall - offsetAt(t);
  return new Date(t).toISOString();
}

export function humanize(s: string) {
  const t = s.replaceAll("_", " ");
  return t.charAt(0).toUpperCase() + t.slice(1);
}

export type Tone = "gold" | "success" | "warning" | "danger" | "info" | "";

const tones: Record<string, Tone> = {
  // orders
  submitted: "info",
  under_review: "info",
  awaiting_customer: "warning",
  deposit_paid: "gold",
  measurements_pending: "warning",
  measurements_verified: "gold",
  material_pending: "warning",
  patterning: "gold",
  cutting: "gold",
  sewing: "gold",
  quality_check: "gold",
  fitting_scheduled: "gold",
  fitting: "gold",
  alteration: "gold",
  ready: "success",
  dispatched: "success",
  delivered: "success",
  completed: "success",
  cancelled: "danger",
  refunded: "danger",
  // payments
  paid: "success",
  unpaid: "warning",
  partially_refunded: "warning",
  succeeded: "success",
  failed: "danger",
  expired: "danger",
  pending: "warning",
  customer_action_required: "warning",
  processing: "info",
  created: "info",
  // requests and quotes
  new: "info",
  reviewing: "gold",
  need_information: "warning",
  quote_sent: "gold",
  accepted: "success",
  converted: "success",
  closed: "",
  draft: "",
  sent: "gold",
  declined: "danger",
  changes_requested: "warning",
  withdrawn: "",
  // appointments and support
  booked: "gold",
  no_show: "danger",
  open: "warning",
  resolved: "success",
};

export function toneOf(status: string): Tone {
  return tones[status] ?? "";
}

export const requestLabels: Record<string, string> = {
  new: "Received",
  reviewing: "Being reviewed",
  need_information: "We need more information",
  quote_sent: "Quote sent",
  accepted: "Accepted",
  converted: "Order created",
  closed: "Closed",
};

export const paymentLabels: Record<string, string> = {
  unpaid: "Not paid yet",
  deposit_paid: "Deposit paid",
  paid: "Paid in full",
  partially_refunded: "Partly refunded",
  refunded: "Refunded",
};
