"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import type { SavedAddress } from "@/lib/types";
import { Field } from "@/components/ui/field";
import { useToast } from "@/components/providers/toast";

const blank: Omit<SavedAddress, "id"> = {
  label: "",
  recipient: "",
  phone: "",
  line1: "",
  line2: "",
  city: "",
  region: "",
  country: "",
  notes: "",
  isDefault: false,
};

export function AccountAddresses() {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["me", "addresses"], queryFn: () => api<SavedAddress[]>("/me/addresses") });
  const [edit, setEdit] = useState<(Omit<SavedAddress, "id"> & { id?: string }) | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!edit) return;
    setBusy(true);
    setErrors({});
    try {
      await api(edit.id ? `/me/addresses/${edit.id}` : "/me/addresses", {
        method: edit.id ? "PUT" : "POST",
        body: edit,
      });
      setEdit(null);
      toast("Address saved.");
      await qc.invalidateQueries({ queryKey: ["me", "addresses"] });
    } catch (err) {
      if (err instanceof ApiError) setErrors(err.fields);
    } finally {
      setBusy(false);
    }
  }
  async function remove(a: SavedAddress) {
    if (!confirm("Delete this address?")) return;
    await api(`/me/addresses/${a.id}`, { method: "DELETE" });
    await qc.invalidateQueries({ queryKey: ["me", "addresses"] });
  }
  const set = (k: keyof typeof blank) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setEdit((x) => (x ? { ...x, [k]: e.target.value } : x));

  return (
    <>
      <header className="stack-sm">
        <h1 className="display-2">Addresses</h1>
        <p className="lede">Saved delivery addresses appear at checkout.</p>
      </header>
      {q.isLoading ? (
        <div className="skeleton" style={{ height: 160 }} />
      ) : (
        <ul className="list-rows">
          {(q.data ?? []).map((a) => (
            <li key={a.id} className="list-row" style={{ flexWrap: "wrap" }}>
              <span className="stack-xs">
                <strong className="row">
                  {a.label || a.recipient} {a.isDefault ? <span className="badge badge-gold">Default</span> : null}
                </strong>
                <span className="small muted">
                  {[a.recipient, a.line1, a.line2, a.city, a.region].filter(Boolean).join(", ")}
                </span>
                <span className="small muted">{a.phone}</span>
              </span>
              <span className="row-wrap">
                <button className="btn btn-sm" onClick={() => setEdit(a)}>
                  Edit
                </button>
                <button className="btn btn-ghost btn-sm" onClick={() => remove(a)}>
                  Delete
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}
      {edit ? (
        <form className="panel panel-pad stack" onSubmit={save} aria-label={edit.id ? "Edit address" : "New address"}>
          <div className="form-grid cols-2">
            <Field label="Label (optional)" error={errors.label} hint="For example: Home, Office">
              {(p) => <input {...p} className="input" value={edit.label} onChange={set("label")} />}
            </Field>
            <Field label="Recipient" error={errors.recipient}>
              {(p) => (
                <input
                  {...p}
                  className="input"
                  value={edit.recipient}
                  onChange={set("recipient")}
                  autoComplete="name"
                />
              )}
            </Field>
            <Field label="Phone" error={errors.phone}>
              {(p) => (
                <input
                  {...p}
                  className="input"
                  value={edit.phone}
                  onChange={set("phone")}
                  autoComplete="tel"
                  inputMode="tel"
                />
              )}
            </Field>
            <Field label="Street or landmark" error={errors.line1}>
              {(p) => (
                <input
                  {...p}
                  className="input"
                  value={edit.line1}
                  onChange={set("line1")}
                  autoComplete="address-line1"
                />
              )}
            </Field>
            <Field label="Building, floor (optional)" error={errors.line2}>
              {(p) => (
                <input
                  {...p}
                  className="input"
                  value={edit.line2}
                  onChange={set("line2")}
                  autoComplete="address-line2"
                />
              )}
            </Field>
            <Field label="City" error={errors.city}>
              {(p) => (
                <input
                  {...p}
                  className="input"
                  value={edit.city}
                  onChange={set("city")}
                  autoComplete="address-level2"
                />
              )}
            </Field>
            <Field label="Region (optional)" error={errors.region}>
              {(p) => (
                <input
                  {...p}
                  className="input"
                  value={edit.region}
                  onChange={set("region")}
                  autoComplete="address-level1"
                />
              )}
            </Field>
            <Field label="Delivery notes (optional)" error={errors.notes}>
              {(p) => <input {...p} className="input" value={edit.notes} onChange={set("notes")} />}
            </Field>
          </div>
          <label className="check">
            <input
              type="checkbox"
              checked={edit.isDefault}
              onChange={(e) => setEdit({ ...edit, isDefault: e.target.checked })}
            />
            <span>Use as my default address</span>
          </label>
          <div className="row-wrap">
            <button className="btn btn-primary" type="submit" disabled={busy}>
              Save address
            </button>
            <button className="btn btn-ghost" type="button" onClick={() => setEdit(null)}>
              Cancel
            </button>
          </div>
        </form>
      ) : (
        <button
          className="btn"
          style={{ justifySelf: "start" }}
          onClick={() => setEdit({ ...blank, isDefault: !(q.data ?? []).length })}
        >
          Add an address
        </button>
      )}
    </>
  );
}
