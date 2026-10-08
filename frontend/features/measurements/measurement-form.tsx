"use client";

import { useState } from "react";
import { convertAll, fromMM, stepFor, toMM, validPrecision, type Unit } from "@/lib/units";
import type { MeasurementField } from "@/lib/types";
import styles from "./measurement-form.module.css";

export type MeasureState = { unit: Unit; height: string; values: Record<string, string> };

export const emptyMeasureState = (unit: Unit = "cm"): MeasureState => ({ unit, height: "", values: {} });

export function stateFromMM(valuesMm: Record<string, number>, heightMm: number | null, unit: Unit): MeasureState {
  return {
    unit,
    height: heightMm ? String(fromMM(heightMm, unit)) : "",
    values: Object.fromEntries(Object.entries(valuesMm).map(([k, mm]) => [k, String(fromMM(mm, unit))])),
  };
}

// fieldError mirrors the server rules so customers see problems as they type.
export function fieldError(raw: string, unit: Unit, minMm: number, maxMm: number, required: boolean): string {
  if (!raw.trim()) return required ? "Required for this garment." : "";
  const v = Number(raw.replace(",", "."));
  if (!Number.isFinite(v) || v <= 0) return "Enter a positive number.";
  if (!validPrecision(v, unit)) return unit === "cm" ? "Use at most one decimal place." : "Use quarter-inch steps, for example 15.25.";
  const mm = toMM(v, unit);
  if (mm < minMm || mm > maxMm) return `Expected between ${fromMM(minMm, unit)} and ${fromMM(maxMm, unit)} ${unit}. Please measure again.`;
  return "";
}

export function measurePayload(s: MeasureState) {
  const values: Record<string, number> = {};
  for (const [k, v] of Object.entries(s.values)) if (v.trim()) values[k] = Number(v.replace(",", "."));
  return { unit: s.unit, height: s.height.trim() ? Number(s.height.replace(",", ".")) : null, values };
}

export function measureErrors(s: MeasureState, fields: MeasurementField[], requireAll: boolean) {
  const out: Record<string, string> = {};
  for (const f of fields) {
    const e = fieldError(s.values[f.key] ?? "", s.unit, f.minMm, f.maxMm, requireAll && f.required);
    if (e) out[f.key] = e;
  }
  const h = fieldError(s.height, s.unit, 1200, 2300, false);
  if (h) out.height = h;
  return out;
}

type Props = {
  fields: MeasurementField[];
  state: MeasureState;
  onChange: (s: MeasureState) => void;
  serverErrors?: Record<string, string>;
  requireAll?: boolean;
  showHeight?: boolean;
};

export function MeasurementForm({ fields, state, onChange, serverErrors = {}, requireAll = false, showHeight = true }: Props) {
  const [touched, setTouched] = useState<Record<string, boolean>>({});
  const [rounded, setRounded] = useState<string[]>([]);

  function switchUnit(unit: Unit) {
    if (unit === state.unit) return;
    const nums: Record<string, number> = {};
    for (const [k, v] of Object.entries(state.values)) if (v.trim() && Number.isFinite(Number(v))) nums[k] = Number(v);
    if (state.height.trim() && Number.isFinite(Number(state.height))) nums.__height = Number(state.height);
    const c = convertAll(nums, state.unit, unit);
    const values = { ...state.values };
    for (const [k, v] of Object.entries(c.values)) if (k !== "__height") values[k] = String(v);
    onChange({ unit, height: c.values.__height !== undefined ? String(c.values.__height) : state.height, values });
    setRounded(c.rounded.map((k) => (k === "__height" ? "Height" : (fields.find((f) => f.key === k)?.label ?? k))));
  }

  const err = (key: string, raw: string, min: number, max: number, req: boolean) =>
    serverErrors[`values.${key}`] ?? serverErrors[key] ?? (touched[key] ? fieldError(raw, state.unit, min, max, req) : "");

  return (
    <div className="stack">
      <fieldset className={styles.units}>
        <legend className="label">Units</legend>
        <div className="choices" role="radiogroup">
          {(["cm", "in"] as const).map((u) => (
            <label key={u} className="choice">
              <input type="radio" name="measure-unit" value={u} checked={state.unit === u} onChange={() => switchUnit(u)} />
              <span>{u === "cm" ? "Centimetres" : "Inches"}</span>
            </label>
          ))}
        </div>
      </fieldset>
      {rounded.length ? (
        <p className="notice notice-warning small" role="status">
          Converted and rounded to the nearest {state.unit === "cm" ? "millimetre" : "quarter inch"}: {rounded.join(", ")}. Please check these values.
        </p>
      ) : null}
      <div className={styles.grid}>
        {showHeight ? (
          <MeasureInput
            id="height"
            label="Height"
            instruction="Stand straight against a wall without shoes."
            unit={state.unit}
            value={state.height}
            error={serverErrors.height ?? (touched.height ? fieldError(state.height, state.unit, 1200, 2300, false) : "")}
            onChange={(v) => onChange({ ...state, height: v })}
            onBlur={() => setTouched((t) => ({ ...t, height: true }))}
          />
        ) : null}
        {fields.map((f) => (
          <MeasureInput
            key={f.key}
            id={f.key}
            label={f.label}
            required={requireAll && f.required}
            instruction={f.instruction}
            helper={f.helperNote}
            unit={state.unit}
            value={state.values[f.key] ?? ""}
            error={err(f.key, state.values[f.key] ?? "", f.minMm, f.maxMm, requireAll && f.required)}
            onChange={(v) => onChange({ ...state, values: { ...state.values, [f.key]: v } })}
            onBlur={() => setTouched((t) => ({ ...t, [f.key]: true }))}
          />
        ))}
      </div>
    </div>
  );
}

function MeasureInput(props: {
  id: string;
  label: string;
  instruction: string;
  helper?: string;
  unit: Unit;
  value: string;
  error: string;
  required?: boolean;
  onChange: (v: string) => void;
  onBlur: () => void;
}) {
  const id = `m-${props.id}`;
  return (
    <div className={styles.item}>
      <label htmlFor={id} className={styles.label}>
        {props.label}
        {props.required ? <span className="faint small"> (required)</span> : null}
      </label>
      <div className="input-group">
        <input
          id={id}
          className="input tabular"
          inputMode="decimal"
          step={stepFor(props.unit)}
          value={props.value}
          onChange={(e) => props.onChange(e.target.value)}
          onBlur={props.onBlur}
          aria-invalid={Boolean(props.error)}
          aria-describedby={`${id}-how${props.error ? ` ${id}-err` : ""}`}
        />
        <span className="input-addon">{props.unit}</span>
      </div>
      <details className={styles.how}>
        <summary>How to measure</summary>
        <p id={`${id}-how`}>
          {props.instruction}
          {props.helper ? ` ${props.helper}` : ""}
        </p>
      </details>
      {props.error ? (
        <span id={`${id}-err`} className="error small" role="alert">
          {props.error}
        </span>
      ) : null}
    </div>
  );
}
