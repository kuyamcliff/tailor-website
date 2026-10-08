import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { estimateFit, type FitInput, type FitRule } from "@/lib/fit";

// The same fixtures drive backend/internal/fit/fit_test.go, so the studio's live estimate and the
// estimate saved on the server can never drift apart.
type Case = {
  name: string;
  input: Omit<FitInput, "rules">;
  expect: { overall: string; zones: Record<string, [string, number | null]> };
  rules?: FitRule[];
};
const fixtures = JSON.parse(
  readFileSync(path.join(__dirname, "../../backend/internal/fit/testdata/fixtures.json"), "utf8"),
) as { rules: FitRule[]; cases: Case[]; stretchCase: Case };

function check(c: Case, rules: FitRule[]) {
  const res = estimateFit({ ...c.input, rules });
  expect(res.overall, c.name).toBe(c.expect.overall);
  for (const [zone, [state, delta]] of Object.entries(c.expect.zones)) {
    const z = res.zones.find((x) => x.zone === zone);
    expect(z, `${c.name}/${zone}`).toBeDefined();
    expect(z!.state, `${c.name}/${zone} state`).toBe(state);
    expect(z!.deltaMm, `${c.name}/${zone} delta`).toBe(delta);
  }
}

describe("fit engine parity with the Go implementation", () => {
  for (const c of fixtures.cases) it(c.name, () => check(c, fixtures.rules));
  it(fixtures.stretchCase.name ?? "stretch", () => check(fixtures.stretchCase, fixtures.stretchCase.rules!));
});
