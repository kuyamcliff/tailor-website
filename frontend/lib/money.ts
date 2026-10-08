// Money is always integer minor units. Formatting uses Intl and the business currency/locale.

const exponents: Record<string, number> = { XAF: 0, XOF: 0, NGN: 2, GHS: 2, EUR: 2, USD: 2, GBP: 2 };

export function exponentOf(currency: string): number {
  return exponents[currency] ?? 2;
}

const symbols: Record<string, string> = { XAF: "FCFA", XOF: "FCFA", EUR: "€", USD: "$", GBP: "£", NGN: "₦", GHS: "GH₵" };

// formatMoney groups digits by hand instead of using Intl currency formatting, because Node and
// browsers disagree on separators and spacing, which breaks hydration. Example: "220 000 FCFA".
export function formatMoney(minor: number, currency = "XAF"): string {
  const exp = exponentOf(currency);
  const negative = minor < 0;
  const abs = Math.abs(Math.round(minor));
  const whole = Math.floor(abs / 10 ** exp).toString().replace(/\B(?=(\d{3})+(?!\d))/g, "\u202f");
  const frac = exp ? "," + (abs % 10 ** exp).toString().padStart(exp, "0") : "";
  return `${negative ? "-" : ""}${whole}${frac}\u00a0${symbols[currency] ?? currency}`;
}

// toMinor parses a user-entered major-unit amount into minor units without floating point drift.
export function toMinor(input: string, currency = "XAF"): number | null {
  const exp = exponentOf(currency);
  const clean = input.replace(/[\s  ]/g, "").replace(",", ".");
  if (!/^\d+(\.\d+)?$/.test(clean)) return null;
  const [whole, frac = ""] = clean.split(".");
  if (frac.length > exp) return null;
  return Number(whole) * 10 ** exp + Number((frac + "0".repeat(exp)).slice(0, exp) || "0");
}
