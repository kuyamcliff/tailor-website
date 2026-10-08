"use client";

import { useState } from "react";
import { api, ApiError, uploadImage } from "@/lib/api";

export type PhotoEstimateResult = { provider: string; heightMm: number; valuesMm: Record<string, number> };

type Shot = { id: string; name: string } | null;

// PhotoEstimate measures from a front and a side photo through the configured provider. It explains
// the conditions first, asks for consent each time, and never presents the result as exact: the
// numbers go into the form as an estimate for the customer to check and the tailor to verify.
// The server deletes both photos as soon as they have been measured.
export function PhotoEstimate({
  provider,
  bodyModel,
  onResult,
  onCancel,
}: {
  provider: string;
  bodyModel: string;
  onResult: (r: PhotoEstimateResult) => void;
  onCancel: () => void;
}) {
  const [front, setFront] = useState<Shot>(null);
  const [side, setSide] = useState<Shot>(null);
  const [progress, setProgress] = useState<Record<string, number>>({});
  const [height, setHeight] = useState("");
  const [weight, setWeight] = useState("");
  const [age, setAge] = useState("");
  const [figure, setFigure] = useState(bodyModel === "feminine" ? "feminine" : "masculine");
  const [consent, setConsent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState("");

  async function upload(which: "front" | "side", file: File | undefined) {
    if (!file) return;
    setError("");
    setProgress((p) => ({ ...p, [which]: 0.01 }));
    try {
      const u = await uploadImage(file, "body_photo", (f) => setProgress((p) => ({ ...p, [which]: f })));
      (which === "front" ? setFront : setSide)({ id: u.id, name: file.name });
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "The upload failed. Please try again.");
    } finally {
      setProgress((p) => ({ ...p, [which]: 0 }));
    }
  }

  async function measure() {
    if (!front || !side) return;
    setBusy(true);
    setErrors({});
    setError("");
    try {
      const r = await api<PhotoEstimateResult>("/me/measurement-estimates", {
        body: {
          frontUploadId: front.id,
          sideUploadId: side.id,
          heightMm: Math.round(Number(height.replace(",", ".")) * 10),
          weightKg: Number(weight.replace(",", ".")),
          age: Number(age),
          bodyModel: figure,
          consent,
        },
      });
      onResult(r);
    } catch (e) {
      // The photos are deleted whatever happened, so new ones are needed for another try.
      setFront(null);
      setSide(null);
      if (e instanceof ApiError) {
        setErrors(e.fields);
        setError(Object.keys(e.fields).length ? "" : e.message);
      } else setError("Please try again.");
    } finally {
      setBusy(false);
    }
  }

  const shot = (which: "front" | "side", label: string, value: Shot) => (
    <div className="field">
      <span className="label">{label}</span>
      {value ? (
        <span className="small">
          {value.name}{" "}
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => (which === "front" ? setFront : setSide)(null)}
          >
            Replace
          </button>
        </span>
      ) : (
        <input
          type="file"
          accept="image/jpeg,image/png,image/webp"
          capture="environment"
          aria-label={label}
          onChange={(e) => upload(which, e.target.files?.[0])}
        />
      )}
      {progress[which] ? <progress value={progress[which]} max={1} aria-label={`${label} upload`} /> : null}
      {errors[`${which}UploadId`] ? <span className="error small">{errors[`${which}UploadId`]}</span> : null}
    </div>
  );

  return (
    <section className="panel panel-pad stack" aria-labelledby="photo-est-h">
      <h3 id="photo-est-h" className="title" style={{ margin: 0 }}>
        Measure from photos
      </h3>
      <p className="small" style={{ margin: 0 }}>
        Two photos give an estimate of your measurements. It is a starting point, not a fitting: check each value, and
        your tailor verifies them before cutting.
      </p>
      <ul className="small muted" style={{ margin: 0, paddingLeft: 18 }}>
        <li>Wear fitted clothes or underwear, with your hair off your neck and shoulders.</li>
        <li>Stand on a plain background in good light, feet hip-width apart, arms slightly away from your body.</li>
        <li>
          Ask someone to hold the phone upright at waist height, 2 to 3 metres away, with your whole body in view.
        </li>
        <li>One photo facing the camera, one with your right side to the camera.</li>
      </ul>
      <div className="form-grid cols-2">
        {shot("front", "Front photo", front)}
        {shot("side", "Side photo (right side)", side)}
      </div>
      <div className="form-grid cols-2">
        <label className="field">
          <span className="label">Height (cm)</span>
          <input
            className="input tabular"
            inputMode="decimal"
            value={height}
            onChange={(e) => setHeight(e.target.value)}
          />
          {errors.heightMm ? <span className="error small">{errors.heightMm}</span> : null}
        </label>
        <label className="field">
          <span className="label">Weight (kg)</span>
          <input
            className="input tabular"
            inputMode="decimal"
            value={weight}
            onChange={(e) => setWeight(e.target.value)}
          />
          {errors.weightKg ? <span className="error small">{errors.weightKg}</span> : null}
        </label>
        <label className="field">
          <span className="label">Age</span>
          <input className="input tabular" inputMode="numeric" value={age} onChange={(e) => setAge(e.target.value)} />
          {errors.age ? <span className="error small">{errors.age}</span> : null}
        </label>
        <label className="field">
          <span className="label">Figure</span>
          <select className="select" value={figure} onChange={(e) => setFigure(e.target.value)}>
            <option value="masculine">Masculine</option>
            <option value="feminine">Feminine</option>
          </select>
        </label>
      </div>
      <p className="tiny muted" style={{ margin: 0 }}>
        Height, weight, age and figure are what the measuring service needs to read the photos. They are not kept apart
        from your height.
      </p>
      <label className="check">
        <input type="checkbox" checked={consent} onChange={(e) => setConsent(e.target.checked)} />
        <span>
          I agree to send these two photos to {provider} to estimate my measurements. The atelier deletes them as soon
          as they are measured and does not use them for anything else.
        </span>
      </label>
      {errors.consent ? <span className="error small">{errors.consent}</span> : null}
      {error ? (
        <p className="notice notice-danger" role="alert" style={{ margin: 0 }}>
          {error}
        </p>
      ) : null}
      <div className="row-wrap">
        <button
          type="button"
          className="btn btn-primary"
          disabled={busy || !front || !side || !consent || !height || !weight || !age}
          onClick={measure}
        >
          {busy ? <span className="spinner" aria-hidden /> : null} Get my estimate
        </button>
        <button
          type="button"
          className="btn btn-ghost"
          onClick={() => {
            // Photos not yet measured are deleted straight away rather than waiting for the daily clean-up.
            for (const s of [front, side]) if (s) void api(`/uploads/${s.id}`, { method: "DELETE" }).catch(() => {});
            onCancel();
          }}
        >
          Cancel
        </button>
      </div>
    </section>
  );
}
