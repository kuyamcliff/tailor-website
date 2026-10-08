import { describe, expect, it } from "vitest";
import { convertAll, formatLength, fromMM, toMM, validPrecision } from "@/lib/units";
import { formatMoney, toMinor } from "@/lib/money";
import { formatDate, formatDateTime, formatDay, formatTime } from "@/lib/format";
import { safeNext } from "@/lib/safe-next";
import { fieldError } from "@/features/measurements/measurement-form";

describe("units", () => {
  it("converts centimetres and inches to millimetres", () => {
    expect(toMM(98.5, "cm")).toBe(985);
    expect(toMM(38.75, "in")).toBe(984);
    expect(fromMM(985, "cm")).toBe(98.5);
    expect(fromMM(984, "in")).toBe(38.75);
  });
  it("enforces precision like the server", () => {
    expect(validPrecision(98.5, "cm")).toBe(true);
    expect(validPrecision(98.55, "cm")).toBe(false);
    expect(validPrecision(15.25, "in")).toBe(true);
    expect(validPrecision(15.3, "in")).toBe(false);
  });
  it("reports values rounded during unit conversion", () => {
    const r = convertAll({ chest: 98.3 }, "cm", "in");
    expect(r.values.chest).toBe(38.75);
    expect(r.rounded).toEqual(["chest"]);
  });
  it("formats lengths", () => {
    expect(formatLength(1000, "cm")).toBe("100 cm");
    expect(formatLength(985, "cm")).toBe("98.5 cm");
  });
});

describe("measurement validation", () => {
  it("rejects values outside the field range", () => {
    expect(fieldError("300", "cm", 600, 1600, true)).toMatch(/Expected between 60 and 160 cm/);
    expect(fieldError("98", "cm", 600, 1600, true)).toBe("");
    expect(fieldError("", "cm", 600, 1600, true)).toMatch(/Required/);
    expect(fieldError("", "cm", 600, 1600, false)).toBe("");
    expect(fieldError("98.55", "cm", 600, 1600, false)).toMatch(/one decimal/);
  });
});

describe("money", () => {
  it("formats XAF without decimals and with grouped thousands", () => {
    expect(formatMoney(220000, "XAF")).toBe("220 000 FCFA");
    expect(formatMoney(-5000, "XAF")).toBe("-5 000 FCFA");
  });
  it("formats two-decimal currencies", () => {
    expect(formatMoney(123456, "EUR")).toBe("1 234,56 €");
  });
  it("parses entered amounts without float drift", () => {
    expect(toMinor("1 234", "XAF")).toBe(1234);
    expect(toMinor("12.34", "EUR")).toBe(1234);
    expect(toMinor("12.345", "EUR")).toBeNull();
    expect(toMinor("abc", "XAF")).toBeNull();
  });
});

describe("dates", () => {
  const d = "2026-10-08T14:05:00Z";
  it("formats identically on server and browser", () => {
    expect(formatDate(d, "long", "UTC")).toBe("8 October 2026");
    expect(formatDate(d, "short", "UTC")).toBe("8 Oct");
    expect(formatDay(d, "UTC")).toBe("Thu 8 Oct");
    expect(formatTime(d, "Africa/Douala")).toBe("15:05");
    expect(formatDateTime(d, "UTC")).toBe("Thu 8 Oct, 14:05");
  });
  it("returns empty text for missing dates", () => {
    expect(formatDate(null)).toBe("");
    expect(formatDate("not a date")).toBe("");
  });
});

describe("safeNext", () => {
  it("only allows same-site paths", () => {
    expect(safeNext("/orders/1", "/account")).toBe("/orders/1");
    expect(safeNext("//evil.example", "/account")).toBe("/account");
    expect(safeNext("https://evil.example", "/account")).toBe("/account");
    expect(safeNext("/\\evil.example", "/account")).toBe("/account");
    expect(safeNext(null, "/account")).toBe("/account");
  });
});
