// Money is always integer minor units. Formatting uses Intl and the business currency/locale.

const exponents: Record<string, number> = { XAF: 0, XOF: 0, NGN: 2, GHS: 2, EUR: 2, USD: 2, GBP: 2 };

export function exponentOf(currency: string): number {
  return exponents[currency] ?? 2;
}

export function formatMoney(minor: number, currency = "XAF", locale = "fr-CM"): string {
  const exp = exponentOf(currency);
  const value = minor / 10 ** exp;
  try {
    return new Intl.NumberFormat(locale, {
      style: "currency",
      currency,
      minimumFractionDigits: exp,
      maximumFractionDigits: exp,
    }).format(value);
  } catch {
    return `${value.toFixed(exp)} ${currency}`;
  }
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
