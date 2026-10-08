// Fit estimation. This is a line-for-line port of backend/internal/fit/fit.go; both are tested
// against backend/internal/fit/testdata/fixtures.json so the studio's live estimate always matches
// the estimate saved on the server.

export type FitRule = {
  zone: string;
  label: string;
  measurementKey: string;
  kind: "circumference" | "length";
  easeSlimMm: number;
  easeRegularMm: number;
  easeRelaxedMm: number;
  toleranceMm: number;
  stretchPct: number;
};

export type FitState = "good" | "close" | "tight" | "loose" | "insufficient_data" | "verify";

export type FitInput = {
  rules: FitRule[];
  body: Record<string, number>;
  fitPreference: "slim" | "regular" | "relaxed" | string;
  baseline: Record<string, number> | null;
  adjustments: Record<string, number>;
  estimated: boolean;
  flaggedKeys: string[];
};

export type ZoneResult = { zone: string; label: string; state: FitState; deltaMm: number | null; message: string };
export type FitResult = { zones: ZoneResult[]; overall: FitState; summary: string };

function ease(r: FitRule, pref: string) {
  if (pref === "slim") return r.easeSlimMm;
  if (pref === "relaxed") return r.easeRelaxedMm;
  return r.easeRegularMm;
}

function describe(kind: string, delta: number, slight: boolean): string {
  const cm = (Math.abs(delta) / 10).toFixed(1);
  if (kind === "length") return `${cm} cm ${delta < 0 ? "short" : "long"}`;
  const word = delta < 0 ? "tight" : "loose";
  if (slight) return `Slightly ${word}`;
  return `${word[0]!.toUpperCase()}${word.slice(1)} by ${cm} cm`;
}

export function estimateFit(input: FitInput): FitResult {
  const flagged = new Set(input.flaggedKeys);
  const zones: ZoneResult[] = [];
  const counts: Partial<Record<FitState, number>> = {};
  const bump = (s: FitState) => (counts[s] = (counts[s] ?? 0) + 1);
  for (const rule of input.rules) {
    const body = input.body[rule.measurementKey];
    if (!body || body <= 0) {
      const z: ZoneResult = {
        zone: rule.zone,
        label: rule.label,
        state: "insufficient_data",
        deltaMm: null,
        message: `Add your ${rule.measurementKey.replaceAll("_", " ")} measurement`,
      };
      zones.push(z);
      bump(z.state);
      continue;
    }
    const target = body + ease(rule, input.fitPreference);
    let dim = target;
    if (input.baseline) {
      const b = input.baseline[rule.zone];
      if (b === undefined) {
        zones.push({
          zone: rule.zone,
          label: rule.label,
          state: "insufficient_data",
          deltaMm: null,
          message: "This size has no reference dimension for this area",
        });
        bump("insufficient_data");
        continue;
      }
      dim = b;
    }
    dim += input.adjustments[rule.zone] ?? 0;
    let delta = dim - target;
    if (rule.kind === "circumference" && delta < 0 && rule.stretchPct > 0) {
      const give = Math.round((body * rule.stretchPct) / 100);
      delta = Math.min(0, delta + give);
    }
    const tol = Math.max(rule.toleranceMm, 1);
    const abs = Math.abs(delta);
    let state: FitState;
    let message: string;
    if (abs <= tol) {
      state = "good";
      message = "Good";
    } else if (abs <= 2 * tol) {
      state = "close";
      message = describe(rule.kind, delta, true);
    } else if (delta < 0) {
      state = "tight";
      message = describe(rule.kind, delta, false);
    } else {
      state = "loose";
      message = describe(rule.kind, delta, false);
    }
    if (input.estimated || flagged.has(rule.measurementKey)) {
      state = "verify";
      message += ". Tailor verification required";
    }
    bump(state);
    zones.push({ zone: rule.zone, label: rule.label, state, deltaMm: delta === 0 ? 0 : delta, message });
  }
  const n = input.rules.length;
  let overall: FitState;
  let summary: string;
  if (n === 0) {
    overall = "insufficient_data";
    summary = "No fit rules are defined for this garment yet";
  } else if (counts.verify) {
    overall = "verify";
    summary = "Your tailor will verify these measurements before cutting";
  } else if ((counts.insufficient_data ?? 0) === n) {
    overall = "insufficient_data";
    summary = "Add measurements to see a fit estimate";
  } else if (counts.tight || counts.loose) {
    overall = counts.tight ? "tight" : "loose";
    summary = "Some areas need attention";
  } else if (counts.close) {
    overall = "close";
    summary = "Close fit with small differences";
  } else if (counts.insufficient_data) {
    overall = "insufficient_data";
    summary = "Some measurements are missing";
  } else {
    overall = "good";
    summary = "Estimated good fit across measured areas";
  }
  return { zones, overall, summary };
}

export const fitLabels: Record<FitState, string> = {
  good: "Good",
  close: "Close",
  tight: "Tight",
  loose: "Loose",
  insufficient_data: "Needs measurements",
  verify: "Tailor verification required",
};
