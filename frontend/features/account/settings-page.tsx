"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { Field } from "@/components/ui/field";
import { useSession } from "@/components/providers/session";
import { useToast } from "@/components/providers/toast";

type Profile = {
  id: string;
  name: string;
  phone: string | null;
  email: string | null;
  preferredContact: string;
  notificationPrefs: Record<string, boolean>;
  marketingConsent: boolean;
  createdAt: string;
};
type SessionRow = { id: string; ip: string | null; userAgent: string | null; createdAt: string; lastSeenAt: string; current: boolean };

export function AccountSettings() {
  return (
    <>
      <header className="stack-sm">
        <h1 className="display-2">Settings and privacy</h1>
      </header>
      <ProfileSection />
      <PasswordSection />
      <SessionsSection />
      <PrivacySection />
    </>
  );
}

function ProfileSection() {
  const { refresh } = useSession();
  const q = useQuery({ queryKey: ["me", "profile"], queryFn: () => api<Profile>("/me/profile") });
  if (!q.data) return <div className="skeleton" style={{ height: 240 }} />;
  return <ProfileForm initial={q.data} onSaved={refresh} />;
}

function ProfileForm({ initial, onSaved }: { initial: Profile; onSaved: () => Promise<unknown> }) {
  const toast = useToast();
  const [f, setF] = useState<Profile>(initial);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErrors({});
    try {
      await api("/me/profile", { method: "PUT", body: { ...f, phone: f.phone ?? "" } });
      toast("Profile saved.");
      await onSaved();
    } catch (err) {
      if (err instanceof ApiError) setErrors(err.fields);
      toast(err instanceof ApiError ? err.message : "Please try again.", "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="panel panel-pad stack" onSubmit={save} aria-labelledby="profile-h">
      <h2 id="profile-h" className="title">
        Profile
      </h2>
      <div className="form-grid cols-2">
        <Field label="Full name" error={errors.name}>
          {(p) => <input {...p} className="input" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} autoComplete="name" />}
        </Field>
        <Field label="Email" hint="Contact the atelier to change the email you sign in with.">
          {(p) => <input {...p} className="input" value={f.email ?? ""} readOnly />}
        </Field>
        <Field label="Phone" error={errors.phone}>
          {(p) => <input {...p} className="input" value={f.phone ?? ""} onChange={(e) => setF({ ...f, phone: e.target.value })} inputMode="tel" autoComplete="tel" />}
        </Field>
        <Field label="Preferred contact" error={errors.preferredContact}>
          {(p) => (
            <select {...p} className="select" value={f.preferredContact} onChange={(e) => setF({ ...f, preferredContact: e.target.value })}>
              <option value="whatsapp">WhatsApp</option>
              <option value="phone">Phone call</option>
              <option value="sms">SMS</option>
              <option value="email">Email</option>
            </select>
          )}
        </Field>
      </div>
      <fieldset className="stack-sm" style={{ border: 0, padding: 0, margin: 0 }}>
        <legend className="label">Order updates</legend>
        <label className="check">
          <input type="checkbox" checked={Boolean(f.notificationPrefs.email)} onChange={(e) => setF({ ...f, notificationPrefs: { ...f.notificationPrefs, email: e.target.checked } })} />
          <span>Email me when my order or request changes</span>
        </label>
        <label className="check">
          <input type="checkbox" checked={Boolean(f.notificationPrefs.sms)} onChange={(e) => setF({ ...f, notificationPrefs: { ...f.notificationPrefs, sms: e.target.checked } })} />
          <span>Send me SMS updates</span>
        </label>
        <label className="check">
          <input type="checkbox" checked={f.marketingConsent} onChange={(e) => setF({ ...f, marketingConsent: e.target.checked })} />
          <span>Occasional news about new fabrics and collections</span>
        </label>
      </fieldset>
      <button className="btn btn-primary" style={{ justifySelf: "start" }} disabled={busy}>
        Save profile
      </button>
    </form>
  );
}

function PasswordSection() {
  const toast = useToast();
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErrors({});
    try {
      await api("/auth/password", { body: { currentPassword: cur, newPassword: next } });
      setCur("");
      setNext("");
      toast("Password changed. Other devices were signed out.");
    } catch (err) {
      if (err instanceof ApiError) setErrors(Object.keys(err.fields).length ? err.fields : { currentPassword: err.message });
    } finally {
      setBusy(false);
    }
  }
  return (
    <form className="panel panel-pad stack" onSubmit={save} aria-labelledby="pw-h">
      <h2 id="pw-h" className="title">
        Password
      </h2>
      <div className="form-grid cols-2">
        <Field label="Current password" error={errors.currentPassword}>
          {(p) => <input {...p} className="input" type="password" value={cur} onChange={(e) => setCur(e.target.value)} autoComplete="current-password" />}
        </Field>
        <Field label="New password" error={errors.newPassword} hint="At least 10 characters.">
          {(p) => <input {...p} className="input" type="password" value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" />}
        </Field>
      </div>
      <button className="btn" style={{ justifySelf: "start" }} disabled={busy || !cur || next.length < 10}>
        Change password
      </button>
    </form>
  );
}

function SessionsSection() {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["me", "sessions"], queryFn: () => api<SessionRow[]>("/auth/sessions") });
  async function revoke() {
    await api("/auth/sessions/revoke-others", { body: {} });
    toast("Signed out of all other devices.");
    await qc.invalidateQueries({ queryKey: ["me", "sessions"] });
  }
  return (
    <section className="panel panel-pad stack" aria-labelledby="sess-h">
      <h2 id="sess-h" className="title">
        Signed-in devices
      </h2>
      <ul className="list-rows">
        {(q.data ?? []).map((s) => (
          <li key={s.id} className="list-row">
            <span className="stack-xs">
              <span>{describeAgent(s.userAgent)}</span>
              <span className="small muted">
                Signed in {formatDate(s.createdAt)} · last active {formatDate(s.lastSeenAt)}
              </span>
            </span>
            {s.current ? <span className="badge badge-gold">This device</span> : null}
          </li>
        ))}
      </ul>
      {(q.data ?? []).length > 1 ? (
        <button className="btn" style={{ justifySelf: "start" }} onClick={revoke}>
          Sign out of other devices
        </button>
      ) : null}
    </section>
  );
}

function describeAgent(ua: string | null): string {
  if (!ua) return "Unknown device";
  const os = /iPhone|iPad/.test(ua) ? "iOS" : /Android/.test(ua) ? "Android" : /Mac OS/.test(ua) ? "macOS" : /Windows/.test(ua) ? "Windows" : /Linux/.test(ua) ? "Linux" : "";
  const br = /Edg\//.test(ua) ? "Edge" : /Chrome\//.test(ua) ? "Chrome" : /Firefox\//.test(ua) ? "Firefox" : /Safari\//.test(ua) ? "Safari" : "Browser";
  return os ? `${br} on ${os}` : br;
}

function PrivacySection() {
  const router = useRouter();
  const { refresh } = useSession();
  const toast = useToast();
  const [confirming, setConfirming] = useState(false);
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function exportData() {
    try {
      const data = await api<unknown>("/me/export");
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "my-atelier-data.json";
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    }
  }

  async function del(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/me/delete-account", { body: { password } });
      await refresh();
      router.replace("/?account=deleted");
    } catch (err) {
      setError(err instanceof ApiError ? (err.fields.password ?? err.message) : "Please try again.");
      setBusy(false);
    }
  }

  return (
    <section className="panel panel-pad stack" aria-labelledby="privacy-h">
      <h2 id="privacy-h" className="title">
        Your data
      </h2>
      <p className="muted">Download a copy of your profile, addresses, measurements, designs, requests, orders, appointments and messages.</p>
      <button className="btn" style={{ justifySelf: "start" }} onClick={exportData}>
        Download my data
      </button>
      <hr className="rule" />
      <p className="muted">
        Deleting your account removes your measurements, designs, addresses and uploaded photos. Order and payment records are kept with your name removed, because we must keep accounting records.
      </p>
      {confirming ? (
        <form className="stack" onSubmit={del}>
          {error ? (
            <p className="notice notice-danger" role="alert">
              {error}
            </p>
          ) : null}
          <Field label="Enter your password to confirm">
            {(p) => <input {...p} className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" style={{ maxWidth: 360 }} />}
          </Field>
          <div className="row-wrap">
            <button className="btn btn-danger" disabled={busy || !password}>
              Delete my account permanently
            </button>
            <button className="btn btn-ghost" type="button" onClick={() => setConfirming(false)}>
              Keep my account
            </button>
          </div>
        </form>
      ) : (
        <button className="btn btn-danger" style={{ justifySelf: "start" }} onClick={() => setConfirming(true)}>
          Delete my account
        </button>
      )}
    </section>
  );
}
