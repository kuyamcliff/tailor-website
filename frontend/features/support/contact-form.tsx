"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { api, ApiError, newIdempotencyKey } from "@/lib/api";
import { Field } from "@/components/ui/field";
import { useSession } from "@/components/providers/session";

const categories = [
  ["general", "General question"],
  ["order_issue", "An order"],
  ["measurement_help", "Measurements"],
  ["payment_issue", "A payment"],
  ["appointment_issue", "An appointment"],
  ["alteration_request", "An alteration"],
  ["other", "Something else"],
] as const;

export function ContactForm({ orderId, defaultCategory = "general" }: { orderId?: string; defaultCategory?: string }) {
  const { user } = useSession();
  const key = useRef(newIdempotencyKey());
  const [form, setForm] = useState({ subject: "", category: defaultCategory, message: "", name: "", phone: "", email: "", preferredContact: "whatsapp" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<{ id: string; number: string; accessToken: string } | null>(null);
  const [error, setError] = useState("");

  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => setForm((f) => ({ ...f, [k]: e.target.value }));

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErrors({});
    setError("");
    try {
      const r = await api<{ id: string; number: string; accessToken: string }>("/support", {
        body: {
          subject: form.subject,
          category: form.category,
          message: form.message,
          orderId,
          contact: user?.customerId ? undefined : { name: form.name, phone: form.phone, email: form.email || null, preferredContact: form.preferredContact },
        },
        idempotencyKey: key.current,
      });
      setDone(r);
    } catch (err) {
      if (err instanceof ApiError) {
        setErrors(Object.fromEntries(Object.entries(err.fields).map(([k, v]) => [k.replace("contact.", ""), v])));
        setError(err.message);
      }
    } finally {
      setBusy(false);
    }
  }

  if (done) {
    return (
      <div className="notice notice-success" role="status">
        <span>
          Thank you. Your message reference is <strong>{done.number}</strong>.{" "}
          <Link className="link" href={`/support/${done.id}?token=${done.accessToken}`}>
            Follow the conversation
          </Link>
          . Keep this link to read our reply.
        </span>
      </div>
    );
  }

  return (
    <form onSubmit={submit} className="stack" noValidate>
      {error ? (
        <p className="notice notice-danger" role="alert">
          {error}
        </p>
      ) : null}
      {!user?.customerId ? (
        <div className="form-grid cols-2">
          <Field label="Your name" error={errors.name}>
            {(p) => <input {...p} className="input" value={form.name} onChange={set("name")} autoComplete="name" required />}
          </Field>
          <Field label="Phone" error={errors.phone}>
            {(p) => <input {...p} className="input" value={form.phone} onChange={set("phone")} autoComplete="tel" inputMode="tel" required />}
          </Field>
          <Field label="Email (optional)" error={errors.email}>
            {(p) => <input {...p} className="input" type="email" value={form.email} onChange={set("email")} autoComplete="email" />}
          </Field>
          <Field label="Reply by" error={errors.preferredContact}>
            {(p) => (
              <select {...p} className="select" value={form.preferredContact} onChange={set("preferredContact")}>
                <option value="whatsapp">WhatsApp</option>
                <option value="phone">Phone call</option>
                <option value="sms">SMS</option>
                <option value="email">Email</option>
              </select>
            )}
          </Field>
        </div>
      ) : null}
      <div className="form-grid cols-2">
        <Field label="About" error={errors.category}>
          {(p) => (
            <select {...p} className="select" value={form.category} onChange={set("category")}>
              {categories.map(([v, l]) => (
                <option key={v} value={v}>
                  {l}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label="Subject" error={errors.subject}>
          {(p) => <input {...p} className="input" value={form.subject} onChange={set("subject")} maxLength={200} required />}
        </Field>
      </div>
      <Field label="Message" error={errors.message}>
        {(p) => <textarea {...p} className="textarea" value={form.message} onChange={set("message")} maxLength={5000} required />}
      </Field>
      <button className="btn btn-primary" disabled={busy} type="submit">
        {busy ? <span className="spinner" aria-hidden /> : null} Send message
      </button>
    </form>
  );
}
