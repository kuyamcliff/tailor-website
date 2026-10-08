"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { api, ApiError } from "@/lib/api";
import { safeNext } from "@/lib/safe-next";
import { Field } from "@/components/ui/field";
import { useSession } from "@/components/providers/session";
import styles from "./auth.module.css";

function useNext() {
  const sp = useSearchParams();
  return sp.get("next");
}

function FormError({ message }: { message: string }) {
  return message ? (
    <p className="notice notice-danger" role="alert">
      {message}
    </p>
  ) : null;
}

export function SignInForm() {
  const { signIn } = useSession();
  const router = useRouter();
  const next = useNext();
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const u = await signIn(identifier.trim(), password);
      router.replace(safeNext(next, u.isStaff ? "/owner" : "/account"));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "We could not sign you in. Please try again.");
      setBusy(false);
    }
  }

  return (
    <form className={`${styles.card} stack`} onSubmit={submit} noValidate>
      <header className="stack-sm">
        <h1 className="display-3">Sign in</h1>
        <p className="muted">Follow your orders, saved designs and measurements.</p>
      </header>
      <FormError message={error} />
      <Field label="Email or phone">
        {(p) => <input {...p} className="input" value={identifier} onChange={(e) => setIdentifier(e.target.value)} autoComplete="username" required />}
      </Field>
      <Field label="Password">
        {(p) => <input {...p} className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />}
      </Field>
      <button className="btn btn-primary btn-block" type="submit" disabled={busy || !identifier || !password}>
        {busy ? <span className="spinner" aria-hidden /> : null} Sign in
      </button>
      <div className="spread small">
        <Link className="link" href="/account/forgot-password">
          Forgot your password?
        </Link>
        <Link className="link" href={`/account/sign-up${next ? `?next=${encodeURIComponent(next)}` : ""}`}>
          Create an account
        </Link>
      </div>
    </form>
  );
}

export function SignUpForm() {
  const { signUp } = useSession();
  const router = useRouter();
  const next = useNext();
  const [form, setForm] = useState({ name: "", email: "", phone: "", password: "", preferredContact: "whatsapp", marketingConsent: false });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (k: "name" | "email" | "phone" | "password" | "preferredContact") => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErrors({});
    setError("");
    try {
      await signUp({ ...form, phone: form.phone || undefined });
      router.replace(safeNext(next, "/account"));
    } catch (err) {
      if (err instanceof ApiError) {
        setErrors(err.fields);
        setError(Object.keys(err.fields).length ? "" : err.message);
      } else setError("Please try again.");
      setBusy(false);
    }
  }

  return (
    <form className={`${styles.card} stack`} onSubmit={submit} noValidate>
      <header className="stack-sm">
        <h1 className="display-3">Create an account</h1>
        <p className="muted">Orders, requests and designs you made on this device move into your account.</p>
      </header>
      <FormError message={error} />
      <Field label="Full name" error={errors.name}>
        {(p) => <input {...p} className="input" value={form.name} onChange={set("name")} autoComplete="name" required />}
      </Field>
      <Field label="Email" error={errors.email}>
        {(p) => <input {...p} className="input" type="email" value={form.email} onChange={set("email")} autoComplete="email" required />}
      </Field>
      <Field label="Phone (optional)" error={errors.phone} hint="Used for order updates and mobile money.">
        {(p) => <input {...p} className="input" type="tel" value={form.phone} onChange={set("phone")} autoComplete="tel" inputMode="tel" />}
      </Field>
      <Field label="Password" error={errors.password} hint="At least 10 characters.">
        {(p) => <input {...p} className="input" type="password" value={form.password} onChange={set("password")} autoComplete="new-password" minLength={10} required />}
      </Field>
      <Field label="Preferred contact" error={errors.preferredContact}>
        {(p) => (
          <select {...p} className="select" value={form.preferredContact} onChange={set("preferredContact")}>
            <option value="whatsapp">WhatsApp</option>
            <option value="phone">Phone call</option>
            <option value="sms">SMS</option>
            <option value="email">Email</option>
          </select>
        )}
      </Field>
      <label className="check">
        <input type="checkbox" checked={form.marketingConsent} onChange={(e) => setForm((f) => ({ ...f, marketingConsent: e.target.checked }))} />
        <span>Send me occasional news about new fabrics and collections.</span>
      </label>
      <p className="small muted">
        By creating an account you agree to our{" "}
        <Link className="link" href="/policies/terms">
          terms
        </Link>{" "}
        and{" "}
        <Link className="link" href="/policies/privacy">
          privacy policy
        </Link>
        .
      </p>
      <button className="btn btn-primary btn-block" type="submit" disabled={busy}>
        {busy ? <span className="spinner" aria-hidden /> : null} Create account
      </button>
      <p className="small">
        Already have an account?{" "}
        <Link className="link" href={`/account/sign-in${next ? `?next=${encodeURIComponent(next)}` : ""}`}>
          Sign in
        </Link>
      </p>
    </form>
  );
}

export function ForgotPasswordForm() {
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/auth/password-reset", { body: { email: email.trim() } });
      setSent(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Please try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className={`${styles.card} stack`} onSubmit={submit} noValidate>
      <header className="stack-sm">
        <h1 className="display-3">Reset your password</h1>
        <p className="muted">Enter the email on your account. We will send a link that is valid for one hour.</p>
      </header>
      {sent ? (
        <p className="notice notice-success" role="status">
          If an account uses this email, a reset link is on its way. Check your inbox and spam folder.
        </p>
      ) : (
        <>
          <FormError message={error} />
          <Field label="Email">
            {(p) => <input {...p} className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" required />}
          </Field>
          <button className="btn btn-primary btn-block" type="submit" disabled={busy || !email}>
            {busy ? <span className="spinner" aria-hidden /> : null} Send reset link
          </button>
        </>
      )}
      <Link className="link small" href="/account/sign-in">
        Back to sign in
      </Link>
    </form>
  );
}

export function ResetPasswordForm() {
  const sp = useSearchParams();
  const token = sp.get("token") ?? "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (password !== confirm) {
      setError("The two passwords do not match.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api("/auth/password-reset/confirm", { body: { token, password } });
      setDone(true);
    } catch (err) {
      setError(err instanceof ApiError ? (err.fields.password ?? err.message) : "Please try again.");
    } finally {
      setBusy(false);
    }
  }

  if (!token)
    return (
      <div className={`${styles.card} stack`}>
        <h1 className="display-3">This link is incomplete</h1>
        <p className="muted">Open the link from your email again, or request a new one.</p>
        <Link className="btn btn-primary" href="/account/forgot-password">
          Request a new link
        </Link>
      </div>
    );

  return (
    <form className={`${styles.card} stack`} onSubmit={submit} noValidate>
      <h1 className="display-3">Choose a new password</h1>
      {done ? (
        <>
          <p className="notice notice-success" role="status">
            Your password has been changed. For safety, every device was signed out.
          </p>
          <Link className="btn btn-primary" href="/account/sign-in">
            Sign in
          </Link>
        </>
      ) : (
        <>
          <FormError message={error} />
          <Field label="New password" hint="At least 10 characters.">
            {(p) => <input {...p} className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" minLength={10} required />}
          </Field>
          <Field label="Repeat new password">
            {(p) => <input {...p} className="input" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" required />}
          </Field>
          <button className="btn btn-primary btn-block" type="submit" disabled={busy || password.length < 10}>
            {busy ? <span className="spinner" aria-hidden /> : null} Save password
          </button>
        </>
      )}
    </form>
  );
}
