// Measurement unit helpers. Values are stored in millimetres. Centimetres allow one decimal,
// inches allow quarter-inch steps, matching the server validation.

export type Unit = "cm" | "in";

export function toMM(value: number, unit: Unit): number {
  return unit === "in" ? Math.round(value * 25.4) : Math.round(value * 10);
}

export function fromMM(mm: number, unit: Unit): number {
  return unit === "in" ? Math.round((mm / 25.4) * 4) / 4 : Math.round(mm) / 10;
}

export function stepFor(unit: Unit): number {
  return unit === "in" ? 0.25 : 0.1;
}

export function formatLength(mm: number, unit: Unit): string {
  const v = fromMM(mm, unit);
  return unit === "in" ? `${v} in` : `${v.toFixed(v % 1 === 0 ? 0 : 1)} cm`;
}

export function validPrecision(value: number, unit: Unit): boolean {
  const step = stepFor(unit);
  const q = value / step;
  return Math.abs(q - Math.round(q)) < 1e-6;
}

// convertAll converts entered values to another unit and reports which were rounded, so the UI
// can tell the customer instead of silently changing their numbers.
export function convertAll(values: Record<string, number>, from: Unit, to: Unit) {
  const out: Record<string, number> = {};
  const rounded: string[] = [];
  for (const [k, v] of Object.entries(values)) {
    const mm = from === "in" ? v * 25.4 : v * 10;
    const exact = to === "in" ? mm / 25.4 : mm / 10;
    const next = fromMM(Math.round(mm), to);
    out[k] = next;
    if (Math.abs(exact - next) > 0.001) rounded.push(k);
  }
  return { values: out, rounded };
}
