"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, ApiError, newIdempotencyKey } from "@/lib/api";
import { rememberLink } from "@/lib/links";
import { humanize } from "@/lib/format";
import type { Fabric, GarmentType, MeasurementField, MeasurementProfile, SavedDesign } from "@/lib/types";
import { useConfig } from "@/components/providers/config";
import { useSession } from "@/components/providers/session";
import { Field } from "@/components/ui/field";
import { Price } from "@/components/ui/price";
import { FabricSwatch } from "@/components/ui/fabric-swatch";
import { ImageUploader, uploadsPending, type UploadItem } from "@/components/ui/image-uploader";
import {
  MeasurementForm,
  measureErrors,
  measurePayload,
  stateFromMM,
  type MeasureState,
} from "@/features/measurements/measurement-form";
import styles from "./wizard.module.css";

const occasions = [
  ["wedding", "Wedding"],
  ["ceremony", "Ceremony or celebration"],
  ["work", "Work"],
  ["graduation", "Graduation"],
  ["photoshoot", "Photoshoot"],
  ["everyday", "Everyday"],
  ["casual", "Casual"],
  ["other", "Something else"],
] as const;

const refTags = [
  ["overall", "Overall look"],
  ["silhouette", "Shape"],
  ["color", "Colour"],
  ["fabric", "Fabric"],
  ["collar", "Collar or neckline"],
  ["sleeve", "Sleeves"],
  ["pocket", "Pockets"],
  ["front", "Front detail"],
  ["back", "Back detail"],
  ["embroidery", "Embroidery"],
  ["embellishment", "Embellishment"],
  ["other", "Other"],
] as const;

const steps = [
  "Garment",
  "Occasion",
  "Fit",
  "Measurements",
  "Fabric",
  "References",
  "Details",
  "Contact",
  "Review",
] as const;
type Step = (typeof steps)[number];

type Draft = {
  garment: string;
  occasion: string;
  occasionNote: string;
  desiredDate: string;
  dateFlexibility: "fixed" | "flexible" | "very_flexible";
  urgency: "standard" | "soon" | "urgent";
  bodyModel: "masculine" | "feminine";
  fitPreference: "slim" | "regular" | "relaxed";
  measurementMode: "entered" | "saved_profile" | "in_store";
  measure: MeasureState;
  profileVersionId: string;
  fabricMode: "catalog" | "recommend" | "reference";
  fabricKey: string;
  colorKey: string;
  notes: string;
  contact: { name: string; phone: string; email: string; preferredContact: string };
  designId: string;
  designVersionId: string;
};

const DRAFT_KEY = "atelier.requestDraft";

const blankDraft = (): Draft => ({
  garment: "",
  occasion: "",
  occasionNote: "",
  desiredDate: "",
  dateFlexibility: "flexible",
  urgency: "standard",
  bodyModel: "masculine",
  fitPreference: "regular",
  measurementMode: "in_store",
  measure: { unit: "cm", height: "", values: {} },
  profileVersionId: "",
  fabricMode: "recommend",
  fabricKey: "",
  colorKey: "",
  notes: "",
  contact: { name: "", phone: "", email: "", preferredContact: "whatsapp" },
  designId: "",
  designVersionId: "",
});

function loadDraft(): { draft: Draft; key: string } | null {
  try {
    const raw = sessionStorage.getItem(DRAFT_KEY);
    return raw ? (JSON.parse(raw) as { draft: Draft; key: string }) : null;
  } catch {
    return null;
  }
}

// Which step owns a server validation field, so errors send the customer back to the right place.
function stepForField(field: string): Step {
  if (field === "garment") return "Garment";
  if (["occasion", "occasionNote", "desiredDate", "dateFlexibility", "urgency"].includes(field)) return "Occasion";
  if (["bodyModel", "fitPreference"].includes(field)) return "Fit";
  if (field.startsWith("measurement")) return "Measurements";
  if (field.startsWith("fabric") || field === "colorKey") return "Fabric";
  if (field.startsWith("references")) return "References";
  if (field === "notes") return "Details";
  if (field.startsWith("contact")) return "Contact";
  return "Review";
}

export function RequestWizard() {
  const router = useRouter();
  const sp = useSearchParams();
  const cfg = useConfig();
  const { user } = useSession();
  const [step, setStep] = useState(0);
  const [draft, setDraft] = useState<Draft>(blankDraft);
  const keyRef = useRef("");
  const [restored, setRestored] = useState(false);
  const [refs, setRefs] = useState<UploadItem[]>([]);
  const [refMeta, setRefMeta] = useState<Record<string, { tag: string; note: string; hint?: string }>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [busy, setBusy] = useState(false);
  const heading = useRef<HTMLHeadingElement>(null);

  const garments = useQuery({ queryKey: ["garments"], queryFn: () => api<GarmentType[]>("/garments") });
  const garment = garments.data?.find((g) => g.key === draft.garment);
  const fields = useQuery({
    queryKey: ["measure-fields", draft.garment],
    enabled: Boolean(draft.garment),
    queryFn: () => api<MeasurementField[]>(`/measurements/fields?garment=${draft.garment}`),
  });
  const fabrics = useQuery({ queryKey: ["fabrics"], queryFn: () => api<Fabric[]>("/fabrics") });
  const profiles = useQuery({
    queryKey: ["me", "profiles"],
    enabled: Boolean(user?.customerId),
    queryFn: () => api<MeasurementProfile[]>("/me/measurement-profiles"),
  });
  const suitable = useMemo(
    () =>
      (fabrics.data ?? []).filter(
        (f) =>
          f.stockStatus !== "discontinued" &&
          (!draft.garment || !f.suitableGarments.length || f.suitableGarments.includes(draft.garment)),
      ),
    [fabrics.data, draft.garment],
  );

  // Restore an unfinished draft (same tab), or start from a saved design or ?garment=.
  useEffect(() => {
    const saved = loadDraft();
    const designId = sp.get("design");
    const g = sp.get("garment");

    setRestored(true);
    if (saved && !designId && (!g || saved.draft.garment === g)) {
      keyRef.current = saved.key;
      setDraft(saved.draft);
      return;
    }
    keyRef.current = newIdempotencyKey();
    if (g) setDraft((d) => ({ ...d, garment: g }));
    if (designId) {
      api<SavedDesign>(`/designs/${designId}`)
        .then((d) => {
          const s = d.snapshot;
          setDraft((x) => ({
            ...x,
            garment: s.garment.key,
            designId: d.id,
            designVersionId: d.versionId ?? "",
            bodyModel: s.bodyModel === "feminine" ? "feminine" : "masculine",
            fitPreference: (["slim", "regular", "relaxed"].includes(s.fitPreference)
              ? s.fitPreference
              : "regular") as Draft["fitPreference"],
            fabricMode: s.fabric ? "catalog" : x.fabricMode,
            fabricKey: s.fabric?.key ?? "",
            colorKey: s.fabric?.colorKey ?? "",
            measurementMode: s.measurements?.versionId
              ? "saved_profile"
              : s.measurements
                ? "entered"
                : x.measurementMode,
            profileVersionId: s.measurements?.versionId ?? "",
            measure:
              s.measurements && !s.measurements.versionId
                ? stateFromMM(
                    s.measurements.valuesMm,
                    s.measurements.heightMm,
                    s.measurements.unit === "in" ? "in" : "cm",
                  )
                : x.measure,
            notes: s.notes || x.notes,
          }));
        })
        .catch(() => setFormError("We could not load that design. You can still describe what you want."));
    }
  }, [sp]);

  useEffect(() => {
    if (!restored) return;
    try {
      sessionStorage.setItem(DRAFT_KEY, JSON.stringify({ draft, key: keyRef.current }));
    } catch {
      // storage unavailable: the draft lives only in memory
    }
  }, [draft, restored]);

  useEffect(() => {
    if (user)
      setDraft((d) => ({
        ...d,
        contact: { ...d.contact, name: d.contact.name || user.name, email: d.contact.email || user.email },
      }));
  }, [user]);

  // Suggest the body model that matches the garment the first time a garment is chosen.
  function chooseGarment(g: GarmentType) {
    setDraft((d) => ({
      ...d,
      garment: g.key,
      bodyModel:
        g.bodyModelHint === "feminine" ? "feminine" : g.bodyModelHint === "masculine" ? "masculine" : d.bodyModel,
    }));
  }

  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setDraft((d) => ({ ...d, [k]: v }));

  function validate(s: Step): Record<string, string> {
    const e: Record<string, string> = {};
    if (s === "Garment" && !draft.garment) e.garment = "Choose a garment.";
    if (s === "Occasion") {
      if (!draft.occasion) e.occasion = "Choose an occasion.";
      if (draft.desiredDate && new Date(draft.desiredDate) <= new Date())
        e.desiredDate = "Choose a date in the future.";
    }
    if (s === "Measurements") {
      if (draft.measurementMode === "entered") {
        const m = measureErrors(draft.measure, fields.data ?? [], true);
        for (const [k, v] of Object.entries(m)) e[`measurements.${k}`] = v;
      }
      if (draft.measurementMode === "saved_profile" && !draft.profileVersionId)
        e.measurementVersionId = "Choose saved measurements.";
    }
    if (s === "Fabric" && draft.fabricMode === "catalog" && !draft.fabricKey) e.fabricKey = "Choose a fabric.";
    if (s === "References") {
      if (uploadsPending(refs)) e.references = "Wait for the uploads to finish.";
      if (draft.fabricMode === "reference" && !refs.some((r) => r.status === "done"))
        e.references = "Add a photo of the fabric you have in mind.";
    }
    if (s === "Contact") {
      if (!user?.customerId) {
        if (!draft.contact.name.trim()) e["contact.name"] = "Enter your name.";
        if (!draft.contact.phone.trim()) e["contact.phone"] = "Enter a phone number.";
      }
    }
    return e;
  }

  function go(to: number) {
    setStep(to);
    setFormError("");
    requestAnimationFrame(() => heading.current?.focus());
  }

  function next() {
    const e = validate(steps[step]!);
    setErrors(e);
    if (Object.keys(e).length) return;
    go(Math.min(step + 1, steps.length - 1));
  }

  async function submit() {
    for (let i = 0; i < steps.length - 1; i++) {
      const e = validate(steps[i]!);
      if (Object.keys(e).length) {
        setErrors(e);
        go(i);
        return;
      }
    }
    setBusy(true);
    setFormError("");
    const m = measurePayload(draft.measure);
    try {
      const r = await api<{ id: string; number: string; accessToken: string }>("/requests", {
        idempotencyKey: keyRef.current,
        body: {
          garment: draft.garment,
          occasion: draft.occasion,
          occasionNote: draft.occasionNote,
          measurementMode: draft.measurementMode,
          measurements: draft.measurementMode === "entered" ? m : undefined,
          measurementVersionId: draft.measurementMode === "saved_profile" ? draft.profileVersionId : undefined,
          bodyModel: draft.bodyModel,
          fitPreference: draft.fitPreference,
          fabricMode: draft.fabricMode,
          fabricKey: draft.fabricMode === "catalog" ? draft.fabricKey : "",
          colorKey: draft.fabricMode === "catalog" ? draft.colorKey : "",
          designVersionId: draft.designVersionId || undefined,
          references: refs
            .filter((x) => x.status === "done" && x.result)
            .map((x) => ({
              uploadId: x.result!.id,
              tag: refMeta[x.key]?.tag || "overall",
              note: refMeta[x.key]?.note ?? "",
            })),
          notes: draft.notes,
          desiredDate: draft.desiredDate,
          dateFlexibility: draft.dateFlexibility,
          urgency: draft.urgency,
          contact: { ...draft.contact, email: draft.contact.email || null },
        },
      });
      rememberLink({ kind: "request", id: r.id, number: r.number, token: r.accessToken });
      try {
        sessionStorage.removeItem(DRAFT_KEY);
      } catch {
        // ignore
      }
      router.push(`/requests/${r.id}?token=${r.accessToken}&submitted=1`);
    } catch (err) {
      setBusy(false);
      if (err instanceof ApiError) {
        setErrors(err.fields);
        const first = Object.keys(err.fields)[0];
        if (first) go(steps.indexOf(stepForField(first)));
        setFormError(err.message);
      } else setFormError("We could not send your request. Please try again.");
    }
  }

  async function analyse(item: UploadItem) {
    if (!item.result) return;
    try {
      const r = await api<{
        hints: {
          garmentCategory: string;
          colors: string[];
          silhouette: string;
          collar: string;
          sleeves: string;
          fabricCues: string;
          notableDetails: string[];
        };
      }>(`/uploads/${item.result.id}/analysis`, {
        body: { garment: draft.garment, tag: refMeta[item.key]?.tag ?? "overall" },
      });
      const h = r.hints;
      const text = [
        h.silhouette,
        h.collar,
        h.sleeves,
        h.colors.length ? `Colours: ${h.colors.join(", ")}` : "",
        h.fabricCues,
        ...h.notableDetails,
      ]
        .filter(Boolean)
        .join(". ");
      setRefMeta((m) => ({ ...m, [item.key]: { ...(m[item.key] ?? { tag: "overall", note: "" }), hint: text } }));
    } catch (e) {
      setRefMeta((m) => ({
        ...m,
        [item.key]: {
          ...(m[item.key] ?? { tag: "overall", note: "" }),
          hint: e instanceof ApiError ? e.message : "No suggestion available.",
        },
      }));
    }
  }

  const current = steps[step]!;
  const fabric = suitable.find((f) => f.key === draft.fabricKey);
  const err = (k: string) => errors[k];

  return (
    <div className={`container section-tight ${styles.wrap}`}>
      <header className={styles.head}>
        <span className="eyebrow">Custom request</span>
        <p className={styles.progressText} aria-live="polite">
          Step {step + 1} of {steps.length}
        </p>
        <div className={styles.progress} aria-hidden>
          <span style={{ width: `${((step + 1) / steps.length) * 100}%` }} />
        </div>
        <ol className={styles.stepList} aria-label="Steps">
          {steps.map((s, i) => (
            <li key={s}>
              <button
                type="button"
                onClick={() => i < step && go(i)}
                disabled={i >= step}
                aria-current={i === step ? "step" : undefined}
              >
                {s}
              </button>
            </li>
          ))}
        </ol>
      </header>

      <section className={styles.body} aria-labelledby="step-title">
        <h1 id="step-title" className="display-3" ref={heading} tabIndex={-1}>
          {titles[current]}
        </h1>
        {formError ? (
          <p className="notice notice-danger" role="alert">
            {formError}
          </p>
        ) : null}
        {draft.designId && step === 0 ? (
          <p className="notice">Your saved studio design is attached. You can still change anything here.</p>
        ) : null}

        {current === "Garment" ? (
          <fieldset className={styles.fieldset}>
            <legend className="visually-hidden">Garment</legend>
            <div className={styles.options}>
              {(garments.data ?? []).map((g) => (
                <label key={g.key} className="choice">
                  <input
                    type="radio"
                    name="garment"
                    checked={draft.garment === g.key}
                    onChange={() => chooseGarment(g)}
                  />
                  <span className="choice-title">{g.name}</span>
                  {g.description ? <span className="choice-meta">{g.description}</span> : null}
                </label>
              ))}
            </div>
            {err("garment") ? <p className="error small">{err("garment")}</p> : null}
          </fieldset>
        ) : null}

        {current === "Occasion" ? (
          <div className="stack">
            <fieldset className={styles.fieldset}>
              <legend className="label">What is it for?</legend>
              <div className={styles.options}>
                {occasions.map(([k, l]) => (
                  <label key={k} className="choice">
                    <input
                      type="radio"
                      name="occasion"
                      checked={draft.occasion === k}
                      onChange={() => set("occasion", k)}
                    />
                    <span className="choice-title">{l}</span>
                  </label>
                ))}
              </div>
              {err("occasion") ? <p className="error small">{err("occasion")}</p> : null}
            </fieldset>
            <Field label="Anything about the occasion we should know? (optional)" error={err("occasionNote")}>
              {(p) => (
                <input
                  {...p}
                  className="input"
                  value={draft.occasionNote}
                  maxLength={200}
                  onChange={(e) => set("occasionNote", e.target.value)}
                />
              )}
            </Field>
            <div className="form-grid cols-2">
              <Field label="When do you need it? (optional)" error={err("desiredDate")}>
                {(p) => (
                  <input
                    {...p}
                    type="date"
                    className="input"
                    value={draft.desiredDate}
                    onChange={(e) => set("desiredDate", e.target.value)}
                  />
                )}
              </Field>
              <Field label="How fixed is that date?" error={err("dateFlexibility")}>
                {(p) => (
                  <select
                    {...p}
                    className="select"
                    value={draft.dateFlexibility}
                    onChange={(e) => set("dateFlexibility", e.target.value as Draft["dateFlexibility"])}
                  >
                    <option value="fixed">Fixed, for an event</option>
                    <option value="flexible">A week or two either way</option>
                    <option value="very_flexible">No fixed date</option>
                  </select>
                )}
              </Field>
            </div>
            <p className="small muted">
              If the date is close, we will tell you in the quote whether we can make it in time and whether a rush fee
              applies.
            </p>
          </div>
        ) : null}

        {current === "Fit" ? (
          <div className="stack">
            <fieldset className={styles.fieldset}>
              <legend className="label">Cut</legend>
              <div className={styles.options}>
                {(
                  [
                    ["masculine", "Masculine cut"],
                    ["feminine", "Feminine cut"],
                  ] as const
                ).map(([k, l]) => (
                  <label key={k} className="choice">
                    <input
                      type="radio"
                      name="body"
                      checked={draft.bodyModel === k}
                      onChange={() => set("bodyModel", k)}
                    />
                    <span className="choice-title">{l}</span>
                  </label>
                ))}
              </div>
            </fieldset>
            <fieldset className={styles.fieldset}>
              <legend className="label">How should it fit?</legend>
              <div className={styles.options}>
                {(
                  [
                    ["slim", "Slim", "Close to the body, with less room to move."],
                    ["regular", "Regular", "Comfortable, with a clean line."],
                    ["relaxed", "Relaxed", "Easy and roomy."],
                  ] as const
                ).map(([k, l, d]) => (
                  <label key={k} className="choice">
                    <input
                      type="radio"
                      name="fit"
                      checked={draft.fitPreference === k}
                      onChange={() => set("fitPreference", k)}
                    />
                    <span className="choice-title">{l}</span>
                    <span className="choice-meta">{d}</span>
                  </label>
                ))}
              </div>
            </fieldset>
          </div>
        ) : null}

        {current === "Measurements" ? (
          <div className="stack">
            <fieldset className={styles.fieldset}>
              <legend className="visually-hidden">How we get your measurements</legend>
              <div className={styles.options}>
                <label className="choice">
                  <input
                    type="radio"
                    name="mm"
                    checked={draft.measurementMode === "in_store"}
                    onChange={() => set("measurementMode", "in_store")}
                  />
                  <span className="choice-title">Measure me at the studio</span>
                  <span className="choice-meta">
                    Most accurate. We will arrange a time after reviewing your request.
                  </span>
                </label>
                <label className="choice">
                  <input
                    type="radio"
                    name="mm"
                    checked={draft.measurementMode === "entered"}
                    onChange={() => set("measurementMode", "entered")}
                  />
                  <span className="choice-title">I will enter them now</span>
                  <span className="choice-meta">Use a soft tape. Your tailor checks every number before cutting.</span>
                </label>
                {user?.customerId && (profiles.data ?? []).some((p) => p.current) ? (
                  <label className="choice">
                    <input
                      type="radio"
                      name="mm"
                      checked={draft.measurementMode === "saved_profile"}
                      onChange={() => set("measurementMode", "saved_profile")}
                    />
                    <span className="choice-title">Use my saved measurements</span>
                  </label>
                ) : null}
              </div>
            </fieldset>
            {draft.measurementMode === "saved_profile" ? (
              <Field label="Saved measurements" error={err("measurementVersionId")}>
                {(p) => (
                  <select
                    {...p}
                    className="select"
                    value={draft.profileVersionId}
                    onChange={(e) => set("profileVersionId", e.target.value)}
                    style={{ maxWidth: 420 }}
                  >
                    <option value="">Choose a profile</option>
                    {(profiles.data ?? [])
                      .filter((x) => x.current)
                      .map((x) => (
                        <option key={x.id} value={x.current!.id}>
                          {x.name} (version {x.current!.versionNo})
                        </option>
                      ))}
                  </select>
                )}
              </Field>
            ) : null}
            {draft.measurementMode === "entered" ? (
              fields.isLoading ? (
                <div className="skeleton" style={{ height: 320 }} />
              ) : (
                <MeasurementForm
                  fields={fields.data ?? []}
                  state={draft.measure}
                  onChange={(m) => set("measure", m)}
                  requireAll
                  serverErrors={Object.fromEntries(
                    Object.entries(errors)
                      .filter(([k]) => k.startsWith("measurements."))
                      .map(([k, v]) => [k.slice(13), v]),
                  )}
                />
              )
            ) : null}
          </div>
        ) : null}

        {current === "Fabric" ? (
          <div className="stack">
            <fieldset className={styles.fieldset}>
              <legend className="visually-hidden">Fabric</legend>
              <div className={styles.options}>
                <label className="choice">
                  <input
                    type="radio"
                    name="fm"
                    checked={draft.fabricMode === "recommend"}
                    onChange={() => set("fabricMode", "recommend")}
                  />
                  <span className="choice-title">Recommend a fabric for me</span>
                  <span className="choice-meta">We will suggest options in the quote.</span>
                </label>
                <label className="choice">
                  <input
                    type="radio"
                    name="fm"
                    checked={draft.fabricMode === "catalog"}
                    onChange={() => set("fabricMode", "catalog")}
                  />
                  <span className="choice-title">Choose from the collection</span>
                </label>
                <label className="choice">
                  <input
                    type="radio"
                    name="fm"
                    checked={draft.fabricMode === "reference"}
                    onChange={() => set("fabricMode", "reference")}
                  />
                  <span className="choice-title">I have a fabric in mind</span>
                  <span className="choice-meta">Add a photo of it in the next step.</span>
                </label>
              </div>
            </fieldset>
            {draft.fabricMode === "catalog" ? (
              <div className="stack-sm">
                <ul className={styles.fabrics} aria-label="Fabrics">
                  {suitable.map((f) => (
                    <li key={f.key}>
                      <label className={styles.fabric}>
                        <input
                          type="radio"
                          name="fabric"
                          className="visually-hidden"
                          checked={draft.fabricKey === f.key}
                          onChange={() =>
                            setDraft((d) => ({ ...d, fabricKey: f.key, colorKey: f.colors[0]?.key ?? "" }))
                          }
                        />
                        <span className={styles.swatch}>
                          <FabricSwatch
                            fabric={f}
                            colorHex={
                              draft.fabricKey === f.key
                                ? f.colors.find((c) => c.key === draft.colorKey)?.hex
                                : f.colors[0]?.hex
                            }
                            sizes="96px"
                            round={false}
                          />
                        </span>
                        <span className={styles.fabricName}>{f.name}</span>
                        <span className="tiny muted">{f.composition}</span>
                        <span className="tiny">
                          {f.priceImpactMinor ? (
                            <>
                              + <Price minor={f.priceImpactMinor} />
                            </>
                          ) : (
                            "Included"
                          )}
                        </span>
                        {f.stockStatus === "low_stock" ? (
                          <span className="tiny" style={{ color: "var(--warning)" }}>
                            Limited stock
                          </span>
                        ) : null}
                        {f.stockStatus === "custom_order" ? (
                          <span className="tiny muted">Ordered in for you</span>
                        ) : null}
                      </label>
                    </li>
                  ))}
                </ul>
                {err("fabricKey") ? <p className="error small">{err("fabricKey")}</p> : null}
                {fabric && fabric.colors.length > 1 ? (
                  <fieldset className={styles.fieldset}>
                    <legend className="label">Colour of {fabric.name}</legend>
                    <div className="row-wrap">
                      {fabric.colors.map((c) => (
                        <label key={c.key} className={styles.color}>
                          <input
                            type="radio"
                            name="color"
                            className="visually-hidden"
                            checked={draft.colorKey === c.key}
                            onChange={() => set("colorKey", c.key)}
                          />
                          <span style={{ background: c.hex }} aria-hidden />
                          {c.name}
                        </label>
                      ))}
                    </div>
                  </fieldset>
                ) : null}
              </div>
            ) : null}
          </div>
        ) : null}

        {current === "References" ? (
          <div className="stack">
            <p className="muted">
              Photos of styles, details or fabrics you like. Say what each one shows so your tailor knows what to look
              at. They stay private between you and the atelier.
            </p>
            <ImageUploader
              purpose="reference"
              items={refs}
              onChange={setRefs}
              max={20}
              layout="list"
              reorderable
              label="Add reference photos"
              hint="JPEG, PNG or WebP, up to 15 MB each"
              renderDetail={(item) => {
                const meta = refMeta[item.key] ?? { tag: "overall", note: "" };
                const update = (m: Partial<typeof meta>) =>
                  setRefMeta((all) => ({ ...all, [item.key]: { ...meta, ...m } }));
                return (
                  <>
                    <div className="form-grid cols-2">
                      <Field label="This photo shows">
                        {(p) => (
                          <select
                            {...p}
                            className="select"
                            value={meta.tag}
                            onChange={(e) => update({ tag: e.target.value })}
                          >
                            {refTags.map(([k, l]) => (
                              <option key={k} value={k}>
                                {l}
                              </option>
                            ))}
                          </select>
                        )}
                      </Field>
                      <Field label="Note (optional)">
                        {(p) => (
                          <input
                            {...p}
                            className="input"
                            value={meta.note}
                            maxLength={300}
                            onChange={(e) => update({ note: e.target.value })}
                            placeholder="The collar shape, not the colour"
                          />
                        )}
                      </Field>
                    </div>
                    {cfg.flags.reference_analysis && item.status === "done" ? (
                      meta.hint ? (
                        <p className="small muted" style={{ margin: 0 }}>
                          Suggested description: {meta.hint}{" "}
                          <button
                            type="button"
                            className="link small"
                            onClick={() => update({ note: meta.hint!.slice(0, 300), hint: undefined })}
                          >
                            Use as note
                          </button>
                        </p>
                      ) : (
                        <button
                          type="button"
                          className="btn btn-ghost btn-sm"
                          style={{ justifySelf: "start" }}
                          onClick={() => analyse(item)}
                        >
                          Suggest a description
                        </button>
                      )
                    ) : null}
                  </>
                );
              }}
            />
            {err("references") ? <p className="error small">{err("references")}</p> : null}
          </div>
        ) : null}

        {current === "Details" ? (
          <Field
            label="Anything else your tailor should know? (optional)"
            error={err("notes")}
            hint="Details you care about, things to avoid, a budget range."
          >
            {(p) => (
              <textarea
                {...p}
                className="textarea"
                rows={6}
                maxLength={4000}
                value={draft.notes}
                onChange={(e) => set("notes", e.target.value)}
              />
            )}
          </Field>
        ) : null}

        {current === "Contact" ? (
          user?.customerId ? (
            <p className="lede">
              We will reply to {user.name} through your account and by{" "}
              {humanize(draft.contact.preferredContact).toLowerCase()}.
            </p>
          ) : (
            <div className="stack">
              <div className="form-grid cols-2">
                <Field label="Your name" error={err("contact.name")}>
                  {(p) => (
                    <input
                      {...p}
                      className="input"
                      autoComplete="name"
                      value={draft.contact.name}
                      onChange={(e) => set("contact", { ...draft.contact, name: e.target.value })}
                    />
                  )}
                </Field>
                <Field label="Phone" error={err("contact.phone")}>
                  {(p) => (
                    <input
                      {...p}
                      className="input"
                      inputMode="tel"
                      autoComplete="tel"
                      value={draft.contact.phone}
                      onChange={(e) => set("contact", { ...draft.contact, phone: e.target.value })}
                    />
                  )}
                </Field>
                <Field label="Email (optional)" error={err("contact.email")}>
                  {(p) => (
                    <input
                      {...p}
                      className="input"
                      type="email"
                      autoComplete="email"
                      value={draft.contact.email}
                      onChange={(e) => set("contact", { ...draft.contact, email: e.target.value })}
                    />
                  )}
                </Field>
                <Field label="Reply by" error={err("contact.preferredContact")}>
                  {(p) => (
                    <select
                      {...p}
                      className="select"
                      value={draft.contact.preferredContact}
                      onChange={(e) => set("contact", { ...draft.contact, preferredContact: e.target.value })}
                    >
                      <option value="whatsapp">WhatsApp</option>
                      <option value="phone">Phone call</option>
                      <option value="sms">SMS</option>
                      <option value="email">Email</option>
                    </select>
                  )}
                </Field>
              </div>
              <p className="small muted">
                We only use these details to reply about this request.{" "}
                <Link className="link" href="/account/sign-in?next=/custom-tailor/request">
                  Sign in
                </Link>{" "}
                to keep it in your account.
              </p>
            </div>
          )
        ) : null}

        {current === "Review" ? (
          <dl className={styles.review}>
            <ReviewRow label="Garment" value={garment?.name} onEdit={() => go(0)} />
            <ReviewRow
              label="Occasion"
              value={[
                occasions.find(([k]) => k === draft.occasion)?.[1],
                draft.occasionNote,
                draft.desiredDate ? `by ${draft.desiredDate}` : "",
              ]
                .filter(Boolean)
                .join(", ")}
              onEdit={() => go(1)}
            />
            <ReviewRow
              label="Fit"
              value={`${humanize(draft.bodyModel)} cut, ${draft.fitPreference} fit`}
              onEdit={() => go(2)}
            />
            <ReviewRow
              label="Measurements"
              value={
                draft.measurementMode === "in_store"
                  ? "Taken at the studio"
                  : draft.measurementMode === "saved_profile"
                    ? "Saved measurements"
                    : `${Object.values(draft.measure.values).filter((v) => v.trim()).length} entered in ${draft.measure.unit}`
              }
              onEdit={() => go(3)}
            />
            <ReviewRow
              label="Fabric"
              value={
                draft.fabricMode === "catalog"
                  ? `${fabric?.name ?? ""}${fabric?.colors.find((c) => c.key === draft.colorKey) ? `, ${fabric.colors.find((c) => c.key === draft.colorKey)!.name}` : ""}`
                  : draft.fabricMode === "reference"
                    ? "From my photo"
                    : "Recommended by the tailor"
              }
              onEdit={() => go(4)}
            />
            <ReviewRow
              label="Photos"
              value={`${refs.filter((r) => r.status === "done").length} attached`}
              onEdit={() => go(5)}
            />
            {draft.notes ? <ReviewRow label="Notes" value={draft.notes} onEdit={() => go(6)} /> : null}
            <ReviewRow
              label="Contact"
              value={
                user?.customerId ? user.name : [draft.contact.name, draft.contact.phone].filter(Boolean).join(", ")
              }
              onEdit={() => go(7)}
            />
          </dl>
        ) : null}

        <div className={styles.nav}>
          {step > 0 ? (
            <button type="button" className="btn btn-ghost" onClick={() => go(step - 1)}>
              Back
            </button>
          ) : (
            <span />
          )}
          {current === "Review" ? (
            <div className={styles.submit}>
              <button type="button" className="btn btn-primary" onClick={submit} disabled={busy}>
                {busy ? <span className="spinner" aria-hidden /> : null} Send request
              </button>
              <span className="tiny muted">Free and without commitment. You pay nothing until you accept a quote.</span>
            </div>
          ) : (
            <button type="button" className="btn btn-primary" onClick={next}>
              Continue
            </button>
          )}
        </div>
      </section>
    </div>
  );
}

const titles: Record<Step, string> = {
  Garment: "What would you like made?",
  Occasion: "Tell us about the occasion",
  Fit: "Cut and fit",
  Measurements: "Your measurements",
  Fabric: "Fabric",
  References: "Reference photos",
  Details: "Anything else",
  Contact: "How do we reach you?",
  Review: "Check and send",
};

function ReviewRow({ label, value, onEdit }: { label: string; value?: string; onEdit: () => void }) {
  return (
    <div className={styles.reviewRow}>
      <dt>{label}</dt>
      <dd>{value || "Not set"}</dd>
      <dd>
        <button type="button" className="link small" onClick={onEdit} aria-label={`Change ${label.toLowerCase()}`}>
          Change
        </button>
      </dd>
    </div>
  );
}
