// Shared display helpers for dates and statuses. No em dashes in any user-facing string.

export function formatDate(iso: string | null | undefined, locale = "en-GB", opts: Intl.DateTimeFormatOptions = { day: "numeric", month: "long", year: "numeric" }) {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleDateString(locale, opts);
}

export function formatDateTime(iso: string | null | undefined, timeZone?: string) {
  if (!iso) return "";
  const d = new Date(iso);
  return d.toLocaleString("en-GB", { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", timeZone });
}

export function formatTime(iso: string, timeZone?: string) {
  return new Date(iso).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", timeZone });
}

export function humanize(s: string) {
  const t = s.replaceAll("_", " ");
  return t.charAt(0).toUpperCase() + t.slice(1);
}

export type Tone = "gold" | "success" | "warning" | "danger" | "info" | "";

const tones: Record<string, Tone> = {
  // orders
  submitted: "info", under_review: "info", awaiting_customer: "warning", deposit_paid: "gold", measurements_pending: "warning",
  measurements_verified: "gold", material_pending: "warning", patterning: "gold", cutting: "gold", sewing: "gold", quality_check: "gold",
  fitting_scheduled: "gold", fitting: "gold", alteration: "gold", ready: "success", dispatched: "success", delivered: "success",
  completed: "success", cancelled: "danger", refunded: "danger",
  // payments
  paid: "success", unpaid: "warning", partially_refunded: "warning", succeeded: "success", failed: "danger", expired: "danger",
  pending: "warning", customer_action_required: "warning", processing: "info", created: "info",
  // requests and quotes
  new: "info", reviewing: "gold", need_information: "warning", quote_sent: "gold", accepted: "success", converted: "success", closed: "",
  draft: "", sent: "gold", declined: "danger", changes_requested: "warning", withdrawn: "",
  // appointments and support
  booked: "gold", no_show: "danger", open: "warning", resolved: "success",
};

export function toneOf(status: string): Tone {
  return tones[status] ?? "";
}

export const requestLabels: Record<string, string> = {
  new: "Received", reviewing: "Being reviewed", need_information: "We need more information", quote_sent: "Quote sent",
  accepted: "Accepted", converted: "Order created", closed: "Closed",
};

export const paymentLabels: Record<string, string> = {
  unpaid: "Not paid yet", deposit_paid: "Deposit paid", paid: "Paid in full", partially_refunded: "Partly refunded", refunded: "Refunded",
};
