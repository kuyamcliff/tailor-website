"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { formatDate, humanize } from "@/lib/format";
import { formatLength } from "@/lib/units";
import type { GarmentType, MeasurementField, MeasurementProfile, MeasurementVersion } from "@/lib/types";
import { Field } from "@/components/ui/field";
import { useToast } from "@/components/providers/toast";
import { MeasurementForm, measureErrors, measurePayload, stateFromMM, type MeasureState } from "@/features/measurements/measurement-form";

const sourceLabel: Record<MeasurementVersion["source"], string> = {
  customer_entered: "Entered by you",
  tailor_verified: "Verified by your tailor",
  imported: "Imported",
  estimated: "Estimate",
};

const ageRanges = [
  ["", "Prefer not to say"],
  ["under_18", "Under 18"],
  ["18_29", "18 to 29"],
  ["30_44", "30 to 44"],
  ["45_59", "45 to 59"],
  ["60_plus", "60 or over"],
] as const;

type ProfileForm = { name: string; bodyModel: string; fitPreference: string; unit: string; ageRange: string; isDefault: boolean };

export function AccountMeasurements() {
  const qc = useQueryClient();
  const toast = useToast();
  const profiles = useQuery({ queryKey: ["me", "profiles"], queryFn: () => api<MeasurementProfile[]>("/me/measurement-profiles") });
  const [editing, setEditing] = useState<string | null>(null);
  const [history, setHistory] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const refresh = () => qc.invalidateQueries({ queryKey: ["me", "profiles"] });

  async function remove(p: MeasurementProfile) {
    if (!confirm(`Delete the profile "${p.name}"? Orders already placed keep their measurements.`)) return;
    try {
      await api(`/me/measurement-profiles/${p.id}`, { method: "DELETE" });
      toast("Profile deleted.");
      await refresh();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    }
  }

  return (
    <>
      <header className="stack-sm">
        <h1 className="display-2">Measurements</h1>
        <p className="lede">Keep measurements for yourself or the people you order for. Every change is saved as a new version, so past orders keep the numbers they were made with.</p>
      </header>
      {profiles.isLoading ? (
        <div className="skeleton" style={{ height: 200 }} />
      ) : (
        <div className="stack">
          {(profiles.data ?? []).map((p) => (
            <section key={p.id} className="panel panel-pad stack" aria-labelledby={`p-${p.id}`}>
              <div className="spread" style={{ flexWrap: "wrap", gap: 12 }}>
                <div className="stack-xs">
                  <h2 id={`p-${p.id}`} className="title row">
                    {p.name} {p.isDefault ? <span className="badge badge-gold">Default</span> : null}
                  </h2>
                  <span className="small muted">
                    {humanize(p.bodyModel)} · {humanize(p.fitPreference)} fit · {p.current ? `${sourceLabel[p.current.source]}, ${formatDate(p.current.createdAt)}` : "No measurements yet"}
                  </span>
                </div>
                <div className="row-wrap">
                  <button className="btn btn-sm" onClick={() => setEditing(editing === p.id ? null : p.id)} aria-expanded={editing === p.id}>
                    {p.current ? "Update measurements" : "Add measurements"}
                  </button>
                  {p.current ? (
                    <button className="btn btn-ghost btn-sm" onClick={() => setHistory(history === p.id ? null : p.id)} aria-expanded={history === p.id}>
                      History
                    </button>
                  ) : null}
                  <button className="btn btn-ghost btn-sm" onClick={() => remove(p)}>
                    Delete
                  </button>
                </div>
              </div>
              {p.current && editing !== p.id ? <VersionValues version={p.current} unit={p.unit} /> : null}
              {editing === p.id ? (
                <VersionEditor
                  profile={p}
                  onDone={async () => {
                    setEditing(null);
                    await refresh();
                  }}
                />
              ) : null}
              {history === p.id ? <History profileId={p.id} unit={p.unit} /> : null}
            </section>
          ))}
          {creating ? (
            <ProfileEditor
              onDone={async () => {
                setCreating(false);
                await refresh();
              }}
            />
          ) : (profiles.data?.length ?? 0) < 10 ? (
            <button className="btn" onClick={() => setCreating(true)} style={{ justifySelf: "start" }}>
              New measurement profile
            </button>
          ) : null}
        </div>
      )}
    </>
  );
}

function ProfileEditor({ onDone }: { onDone: () => void }) {
  const toast = useToast();
  const [f, setF] = useState<ProfileForm>({ name: "", bodyModel: "masculine", fitPreference: "regular", unit: "cm", ageRange: "", isDefault: false });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await api("/me/measurement-profiles", { body: { ...f, ageRange: f.ageRange || null } });
      toast("Profile created.");
      onDone();
    } catch (err) {
      if (err instanceof ApiError) setErrors(err.fields);
      toast(err instanceof ApiError ? err.message : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }
  const set = (k: keyof ProfileForm) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setF((x) => ({ ...x, [k]: e.target.value }));
  return (
    <form className="panel panel-pad stack" onSubmit={save} aria-label="New measurement profile">
      <h2 className="title">New profile</h2>
      <div className="form-grid cols-2">
        <Field label="Name" error={errors.name} hint="For example: Me, or My son">
          {(p) => <input {...p} className="input" value={f.name} onChange={set("name")} maxLength={60} required />}
        </Field>
        <Field label="Body model" error={errors.bodyModel}>
          {(p) => (
            <select {...p} className="select" value={f.bodyModel} onChange={set("bodyModel")}>
              <option value="masculine">Masculine</option>
              <option value="feminine">Feminine</option>
            </select>
          )}
        </Field>
        <Field label="Preferred fit" error={errors.fitPreference}>
          {(p) => (
            <select {...p} className="select" value={f.fitPreference} onChange={set("fitPreference")}>
              <option value="slim">Slim</option>
              <option value="regular">Regular</option>
              <option value="relaxed">Relaxed</option>
            </select>
          )}
        </Field>
        <Field label="Units" error={errors.unit}>
          {(p) => (
            <select {...p} className="select" value={f.unit} onChange={set("unit")}>
              <option value="cm">Centimetres</option>
              <option value="in">Inches</option>
            </select>
          )}
        </Field>
        <Field label="Age range (optional)" error={errors.ageRange}>
          {(p) => (
            <select {...p} className="select" value={f.ageRange} onChange={set("ageRange")}>
              {ageRanges.map(([v, l]) => (
                <option key={v} value={v}>
                  {l}
                </option>
              ))}
            </select>
          )}
        </Field>
      </div>
      <label className="check">
        <input type="checkbox" checked={f.isDefault} onChange={(e) => setF((x) => ({ ...x, isDefault: e.target.checked }))} />
        <span>Use as my default profile</span>
      </label>
      <div className="row-wrap">
        <button className="btn btn-primary" disabled={busy} type="submit">
          Create profile
        </button>
        <button className="btn btn-ghost" type="button" onClick={onDone}>
          Cancel
        </button>
      </div>
    </form>
  );
}

function VersionEditor({ profile, onDone }: { profile: MeasurementProfile; onDone: () => void }) {
  const toast = useToast();
  const [garment, setGarment] = useState("");
  const garments = useQuery({ queryKey: ["garments"], queryFn: () => api<GarmentType[]>("/garments") });
  const fields = useQuery({ queryKey: ["measure-fields", garment], queryFn: () => api<MeasurementField[]>(`/measurements/fields${garment ? `?garment=${garment}` : ""}`) });
  const [state, setState] = useState<MeasureState>(() =>
    profile.current ? stateFromMM(profile.current.valuesMm, profile.current.heightMm, profile.unit) : { unit: profile.unit, height: "", values: {} },
  );
  const [notes, setNotes] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  async function save() {
    const local = measureErrors(state, fields.data ?? [], false);
    setErrors(local);
    if (Object.keys(local).length) return;
    setBusy(true);
    try {
      const r = await api<{ id: string; reviewFlags: { message: string }[] | null }>(`/me/measurement-profiles/${profile.id}/versions`, {
        body: { garment, ...measurePayload(state), notes },
      });
      toast(r.reviewFlags?.length ? "Saved. Your tailor will double-check a few values." : "Measurements saved as a new version.");
      onDone();
    } catch (err) {
      if (err instanceof ApiError) setErrors(err.fields);
      toast(err instanceof ApiError ? err.message : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="stack">
      <Field label="Show measurements for" hint="Each garment uses a different set of measurements.">
        {(p) => (
          <select {...p} className="select" value={garment} onChange={(e) => setGarment(e.target.value)} style={{ maxWidth: 360 }}>
            <option value="">All measurements</option>
            {(garments.data ?? []).map((g) => (
              <option key={g.key} value={g.key}>
                {g.name}
              </option>
            ))}
          </select>
        )}
      </Field>
      {fields.isLoading ? <div className="skeleton" style={{ height: 240 }} /> : <MeasurementForm fields={fields.data ?? []} state={state} onChange={setState} serverErrors={errors} />}
      {errors.values ? (
        <p className="notice notice-danger" role="alert">
          {errors.values}
        </p>
      ) : null}
      <Field label="Notes for your tailor (optional)">
        {(p) => <textarea {...p} className="textarea" value={notes} onChange={(e) => setNotes(e.target.value)} maxLength={1000} rows={3} />}
      </Field>
      <div className="row-wrap">
        <button className="btn btn-primary" onClick={save} disabled={busy}>
          {busy ? <span className="spinner" aria-hidden /> : null} Save new version
        </button>
        <button className="btn btn-ghost" onClick={onDone}>
          Cancel
        </button>
      </div>
    </div>
  );
}

function VersionValues({ version, unit }: { version: MeasurementVersion; unit: "cm" | "in" }) {
  const fields = useQuery({ queryKey: ["measure-fields", ""], queryFn: () => api<MeasurementField[]>("/measurements/fields") });
  const label = (k: string) => fields.data?.find((f) => f.key === k)?.label ?? humanize(k);
  const entries = Object.entries(version.valuesMm);
  return (
    <div className="stack-sm">
      <dl style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))", gap: "10px 24px", margin: 0 }}>
        {version.heightMm ? (
          <div>
            <dt className="small muted">Height</dt>
            <dd className="tabular" style={{ margin: 0 }}>
              {formatLength(version.heightMm, unit)}
            </dd>
          </div>
        ) : null}
        {entries.map(([k, mm]) => (
          <div key={k}>
            <dt className="small muted">{label(k)}</dt>
            <dd className="tabular" style={{ margin: 0 }}>
              {formatLength(mm, unit)}
            </dd>
          </div>
        ))}
      </dl>
      {version.reviewFlags?.length ? (
        <ul className="notice notice-warning small" style={{ display: "block", paddingLeft: 32 }}>
          {version.reviewFlags.map((f) => (
            <li key={f.message}>{f.message}</li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function History({ profileId, unit }: { profileId: string; unit: "cm" | "in" }) {
  const q = useQuery({ queryKey: ["me", "profile-versions", profileId], queryFn: () => api<MeasurementVersion[]>(`/me/measurement-profiles/${profileId}/versions`) });
  if (q.isLoading) return <div className="skeleton" style={{ height: 120 }} />;
  return (
    <ol className="list-rows" aria-label="Version history">
      {(q.data ?? []).map((v) => (
        <li key={v.id}>
          <details style={{ padding: "12px 4px" }}>
            <summary className="spread" style={{ cursor: "pointer" }}>
              <span>
                Version {v.versionNo} · {sourceLabel[v.source]}
              </span>
              <span className="small muted">{formatDate(v.createdAt)}</span>
            </summary>
            <div style={{ marginTop: 12 }}>
              <VersionValues version={v} unit={unit} />
              {v.notes ? <p className="small muted">Note: {v.notes}</p> : null}
            </div>
          </details>
        </li>
      ))}
    </ol>
  );
}
