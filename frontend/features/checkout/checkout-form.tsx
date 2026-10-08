"use client";

import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Lock, Ruler } from "lucide-react";
import { api, ApiError, newIdempotencyKey } from "@/lib/api";
import { useCart, cartSubtotal } from "@/stores/cart";
import { useConfig } from "@/components/providers/config";
import { useSession } from "@/components/providers/session";
import { Field } from "@/components/ui/field";
import { Price } from "@/components/ui/price";
import { rememberLink } from "@/lib/links";
import { useHydrated } from "@/lib/client-hooks";
import type { Product, SavedAddress } from "@/lib/types";
import styles from "./checkout.module.css";

const KEY = "atelier.checkoutKey";

// The idempotency key survives a refresh, so pressing Place order twice or reloading mid-request
// never creates two orders. It is reset when the bag changes.
function checkoutKey(signature: string) {
  try {
    const saved = JSON.parse(sessionStorage.getItem(KEY) ?? "null") as { sig: string; key: string } | null;
    if (saved && saved.sig === signature) return saved.key;
    const key = newIdempotencyKey();
    sessionStorage.setItem(KEY, JSON.stringify({ sig: signature, key }));
    return key;
  } catch {
    return newIdempotencyKey();
  }
}

export function CheckoutForm() {
  const router = useRouter();
  const cfg = useConfig();
  const { user } = useSession();
  const { lines, clear, setQuantity, remove } = useCart();
  const ready = useHydrated();
  const zones = cfg.business.delivery;
  const [zoneKey, setZoneKey] = useState(zones[0]?.key ?? "pickup");
  const zone = zones.find((z) => z.key === zoneKey);
  const [contact, setContact] = useState({ name: "", phone: "", email: "", preferredContact: "whatsapp" });
  const [address, setAddress] = useState({ recipient: "", phone: "", line1: "", line2: "", city: "", region: "", notes: "" });
  const [notes, setNotes] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  useEffect(() => {
    // Prefill once the session loads; the customer can still edit every field.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (user) setContact((c) => ({ ...c, name: c.name || user.name, email: c.email || user.email }));
  }, [user]);
  const [saved, setSaved] = useState<SavedAddress[]>([]);
  useEffect(() => {
    if (!user?.customerId) return;
    api<SavedAddress[]>("/me/addresses")
      .then((list) => {
        setSaved(list);
        const d = list.find((a) => a.isDefault);
        if (d) setAddress((a) => (a.line1 ? a : { recipient: d.recipient, phone: d.phone, line1: d.line1, line2: d.line2, city: d.city, region: d.region, notes: d.notes }));
      })
      .catch(() => setSaved([]));
  }, [user?.customerId]);

  const subtotal = cartSubtotal(lines);
  const delivery = zone?.feeMinor ?? 0;
  const tax = cfg.business.pricesIncludeTax ? 0 : Math.round(((subtotal + delivery) * cfg.business.taxRateBp) / 10000);
  const total = subtotal + delivery + tax;

  // Refresh prices and availability from the server (used after a price_changed or stock conflict).
  async function refreshBag() {
    setRefreshing(true);
    try {
      const slugs = [...new Set(lines.map((l) => l.productSlug))];
      const products = await Promise.all(slugs.map((s) => api<{ product: Product }>(`/products/${s}`).then((r) => r.product).catch(() => null)));
      for (const l of lines) {
        const p = products.find((x) => x?.slug === l.productSlug);
        const v = p?.variants.find((x) => x.id === l.variantId);
        if (!v || !v.available) remove(l.variantId);
        else useCart.setState((s) => ({ lines: s.lines.map((x) => (x.variantId === l.variantId ? { ...x, unitPriceMinor: v.priceMinor } : x)) }));
      }
    } finally {
      setRefreshing(false);
    }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    setErrors({});
    const signature = JSON.stringify(lines.map((l) => [l.variantId, l.quantity]));
    try {
      const r = await api<{ orderId: string; number: string; accessToken?: string; totalMinor: number }>("/checkout", {
        idempotencyKey: checkoutKey(signature),
        body: {
          items: lines.map((l) => ({ variantId: l.variantId, quantity: l.quantity })),
          contact: { ...contact, email: contact.email || null },
          fulfillment: {
            method: zone?.method,
            zoneKey,
            note: address.notes,
            address: zone && zone.method !== "pickup" ? { ...address, label: "Delivery", country: cfg.business.address.country || "CM" } : null,
          },
          notes,
          expectedTotalMinor: total,
        },
      });
      if (r.accessToken) rememberLink({ kind: "order", id: r.orderId, number: r.number, token: r.accessToken });
      try {
        sessionStorage.removeItem(KEY);
      } catch {}
      clear();
      router.push(`/orders/${r.orderId}${r.accessToken ? `?token=${r.accessToken}&` : "?"}placed=1`);
    } catch (err) {
      if (err instanceof ApiError) {
        setErrors(err.fields);
        setError(err.message);
        if (err.code === "price_changed" || err.code === "out_of_stock" || err.code === "insufficient_stock" || err.code === "item_unavailable") {
          await refreshBag();
        }
      }
      setBusy(false);
    }
  }

  if (!ready) return <div className="skeleton" style={{ height: 400 }} />;
  if (!lines.length)
    return (
      <div className="empty">
        <p className="display-3">Your bag is empty.</p>
        <Link href="/shop" className="btn btn-sm">
          Continue shopping
        </Link>
      </div>
    );
  if (!cfg.flags.guest_checkout && !user)
    return (
      <div className="empty">
        <p className="display-3">Please sign in to check out.</p>
        <Link href="/account/sign-in?next=/checkout" className="btn btn-primary btn-sm">
          Sign in
        </Link>
      </div>
    );

  const err = (k: string) => errors[k];

  return (
    <form onSubmit={submit} className={styles.layout} noValidate>
      <div className="stack-lg">
        {error ? (
          <div className="notice notice-danger" role="alert">
            <span>
              {error}
              {refreshing ? " Updating your bag..." : ""}
            </span>
          </div>
        ) : null}
        <fieldset className={styles.section}>
          <legend className="display-3">Your details</legend>
          <div className="form-grid cols-2">
            <Field label="Full name" error={err("contact.name")}>
              {(p) => <input {...p} className="input" autoComplete="name" value={contact.name} onChange={(e) => setContact({ ...contact, name: e.target.value })} />}
            </Field>
            <Field label="Phone" error={err("contact.phone")} hint="We use this to arrange delivery or pickup.">
              {(p) => <input {...p} className="input" autoComplete="tel" inputMode="tel" value={contact.phone} onChange={(e) => setContact({ ...contact, phone: e.target.value })} />}
            </Field>
            <Field label="Email (optional)" error={err("contact.email")}>
              {(p) => <input {...p} className="input" type="email" autoComplete="email" value={contact.email} onChange={(e) => setContact({ ...contact, email: e.target.value })} />}
            </Field>
            <Field label="Contact me by" error={err("contact.preferredContact")}>
              {(p) => (
                <select {...p} className="select" value={contact.preferredContact} onChange={(e) => setContact({ ...contact, preferredContact: e.target.value })}>
                  <option value="whatsapp">WhatsApp</option>
                  <option value="phone">Phone call</option>
                  <option value="sms">SMS</option>
                  <option value="email">Email</option>
                </select>
              )}
            </Field>
          </div>
        </fieldset>

        <fieldset className={styles.section}>
          <legend className="display-3">Delivery or pickup</legend>
          <div className="choices" role="radiogroup" aria-label="Delivery method">
            {zones.map((z) => (
              <label key={z.key} className="choice">
                <input type="radio" name="zone" value={z.key} checked={z.key === zoneKey} onChange={() => setZoneKey(z.key)} />
                <span className="choice-title">{z.label}</span>
                <span className="choice-meta">{z.feeMinor ? <Price minor={z.feeMinor} /> : "Free"}</span>
                {z.description ? <span className="choice-meta">{z.description}</span> : null}
              </label>
            ))}
          </div>
          {err("fulfillment.zoneKey") ? <p className="error small">{err("fulfillment.zoneKey")}</p> : null}
          {zone && zone.method !== "pickup" && saved.length > 1 ? (
            <Field label="Saved address" className="span-2">
              {(p) => (
                <select
                  {...p}
                  className="select"
                  style={{ marginTop: 16 }}
                  defaultValue=""
                  onChange={(e) => {
                    const d = saved.find((a) => a.id === e.target.value);
                    if (d) setAddress({ recipient: d.recipient, phone: d.phone, line1: d.line1, line2: d.line2, city: d.city, region: d.region, notes: d.notes });
                  }}
                >
                  <option value="">Choose a saved address</option>
                  {saved.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.label || a.recipient}: {a.line1}, {a.city}
                    </option>
                  ))}
                </select>
              )}
            </Field>
          ) : null}
          {zone && zone.method !== "pickup" ? (
            <div className="form-grid cols-2" style={{ marginTop: 16 }}>
              <Field label="Recipient" error={err("fulfillment.address.recipient")}>
                {(p) => <input {...p} className="input" value={address.recipient} onChange={(e) => setAddress({ ...address, recipient: e.target.value })} />}
              </Field>
              <Field label="Recipient phone" error={err("fulfillment.address.phone")}>
                {(p) => <input {...p} className="input" inputMode="tel" value={address.phone} onChange={(e) => setAddress({ ...address, phone: e.target.value })} />}
              </Field>
              <Field label="Street or landmark" error={err("fulfillment.address.line1")} className="span-2">
                {(p) => <input {...p} className="input" autoComplete="address-line1" value={address.line1} onChange={(e) => setAddress({ ...address, line1: e.target.value })} />}
              </Field>
              <Field label="Neighbourhood (optional)">
                {(p) => <input {...p} className="input" autoComplete="address-line2" value={address.line2} onChange={(e) => setAddress({ ...address, line2: e.target.value })} />}
              </Field>
              <Field label="City" error={err("fulfillment.address.city")}>
                {(p) => <input {...p} className="input" autoComplete="address-level2" value={address.city} onChange={(e) => setAddress({ ...address, city: e.target.value })} />}
              </Field>
              <Field label="Directions for the courier (optional)" className="span-2">
                {(p) => <input {...p} className="input" value={address.notes} onChange={(e) => setAddress({ ...address, notes: e.target.value })} />}
              </Field>
            </div>
          ) : null}
        </fieldset>

        <fieldset className={styles.section}>
          <legend className="display-3">Notes</legend>
          <Field label="Anything we should know (optional)" error={err("notes")}>
            {(p) => <textarea {...p} className="textarea" maxLength={2000} value={notes} onChange={(e) => setNotes(e.target.value)} />}
          </Field>
        </fieldset>
      </div>

      <aside className={`panel panel-pad ${styles.summary}`} aria-labelledby="summary-title">
        <h2 id="summary-title" className="display-3">
          Order summary
        </h2>
        <ul className={styles.items}>
          {lines.map((l) => (
            <li key={l.variantId}>
              <span className={styles.thumb}>{l.image ? <Image src={l.image} alt="" fill sizes="56px" style={{ objectFit: "cover" }} /> : null}</span>
              <span>
                <span className={styles.itemName}>{l.name}</span>
                <span className="tiny muted">
                  Size {l.size} ·{" "}
                  <label>
                    <span className="visually-hidden">Quantity</span>
                    <select className={styles.qty} value={l.quantity} onChange={(e) => setQuantity(l.variantId, Number(e.target.value))}>
                      {Array.from({ length: 10 }, (_, i) => i + 1).map((n) => (
                        <option key={n} value={n}>
                          Qty {n}
                        </option>
                      ))}
                    </select>
                  </label>
                </span>
                {l.requiresFitting ? (
                  <span className="tiny" style={{ color: "var(--gold)", display: "flex", gap: 4, alignItems: "center" }}>
                    <Ruler size={12} aria-hidden /> Fitting included
                  </span>
                ) : null}
              </span>
              <Price minor={l.unitPriceMinor * l.quantity} />
            </li>
          ))}
        </ul>
        <dl className={styles.totals}>
          <div>
            <dt>Subtotal</dt>
            <dd>
              <Price minor={subtotal} />
            </dd>
          </div>
          <div>
            <dt>{zone?.method === "pickup" ? "Pickup" : "Delivery"}</dt>
            <dd>{delivery ? <Price minor={delivery} /> : "Free"}</dd>
          </div>
          {tax ? (
            <div>
              <dt>{cfg.business.taxLabel}</dt>
              <dd>
                <Price minor={tax} />
              </dd>
            </div>
          ) : null}
          <div className={styles.total}>
            <dt>Total</dt>
            <dd>
              <Price minor={total} />
            </dd>
          </div>
        </dl>
        <p className="tiny muted">
          {cfg.flags.online_payments
            ? "You will pay by Mobile Money on the next page. Your order is confirmed once payment is verified."
            : "We will contact you to arrange payment. Nothing is charged now."}
        </p>
        <button className="btn btn-primary btn-block" type="submit" disabled={busy || refreshing}>
          {busy ? <span className="spinner" aria-hidden /> : <Lock size={15} aria-hidden />} Place order
        </button>
        <p className="tiny faint">
          By placing an order you agree to our <Link href="/policies/terms" className="link">terms</Link> and{" "}
          <Link href="/policies/delivery" className="link">delivery policy</Link>.
        </p>
      </aside>
    </form>
  );
}
