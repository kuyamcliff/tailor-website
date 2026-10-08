"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { Smartphone, FlaskConical } from "lucide-react";
import { api, ApiError, newIdempotencyKey } from "@/lib/api";
import type { Order, Payment } from "@/lib/types";
import { Price } from "@/components/ui/price";
import { PaymentMark, whatsappLink } from "@/components/brand/brand-icon";
import { useConfig } from "@/components/providers/config";
import styles from "./payment.module.css";

type Methods = { methods: { provider: string; name: string; simulated: boolean }[]; testMode: boolean };

const active = (s: Payment["status"]) => ["created", "pending", "customer_action_required", "processing"].includes(s);

// PaymentPanel starts a Mobile Money payment and follows it until the provider confirms the final
// state. Success is only shown when the server reports a verified "succeeded" status.
export function PaymentPanel({
  order,
  accessToken,
  online,
  onSettled,
}: {
  order: Order;
  accessToken?: string;
  online: boolean;
  onSettled: () => void;
}) {
  const cfg = useConfig();
  const methods = useQuery({
    queryKey: ["payment-methods"],
    queryFn: () => api<Methods>("/payments/methods"),
    enabled: online,
  });
  const inflight = order.payments.find((p) => active(p.status));
  const [chosen, setProvider] = useState("");
  const [msisdn, setMsisdn] = useState(order.contact.phone?.replace(/^237/, "") ?? "");
  const [paymentId, setPaymentId] = useState<string | null>(inflight?.id ?? null);
  const [payment, setPayment] = useState<Payment | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const provider = chosen || methods.data?.methods[0]?.provider || "";
  const keyRef = useRef(newIdempotencyKey());
  const depositDue =
    order.depositRequiredMinor > order.amountPaidMinor && order.depositRequiredMinor < order.totalMinor;
  const [purpose, setPurpose] = useState<"deposit" | "balance" | "full">(
    depositDue ? "deposit" : order.amountPaidMinor > 0 ? "balance" : "full",
  );
  const amount =
    purpose === "deposit"
      ? order.depositRequiredMinor - order.amountPaidMinor
      : order.totalMinor - order.amountPaidMinor;

  // Poll the payment while it is in flight. The server re-checks with the provider on each read.
  useEffect(() => {
    if (!paymentId) return;
    let stop = false;
    const tick = async () => {
      try {
        const p = await api<Payment>(`/payments/${paymentId}`, { accessToken });
        if (stop) return;
        setPayment(p);
        if (!active(p.status)) {
          onSettled();
          keyRef.current = newIdempotencyKey();
          return;
        }
      } catch {
        // transient network error: keep polling
      }
      if (!stop) setTimeout(tick, 3000);
    };
    void tick();
    return () => {
      stop = true;
    };
  }, [paymentId, accessToken, onSettled]);

  async function pay(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const p = await api<Payment>("/payments/intent", {
        body: { orderId: order.id, provider, msisdn, purpose },
        idempotencyKey: keyRef.current,
        accessToken,
        timeoutMs: 60000,
      });
      setPayment(p);
      setPaymentId(p.id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The payment could not be started.");
      keyRef.current = newIdempotencyKey();
    } finally {
      setBusy(false);
    }
  }

  const b = cfg.business;
  if (!online || (methods.data && methods.data.methods.length === 0)) {
    return (
      <section className={`panel panel-pad ${styles.panel}`} aria-labelledby="pay-title">
        <h2 id="pay-title" className="display-3">
          Payment
        </h2>
        <p className="muted">
          Amount due: <Price minor={amount} currency={order.currency} />. Online payment is not available right now. You
          can pay at the studio, or contact us and we will send payment details.
        </p>
        {b.whatsapp ? (
          <a
            className="btn btn-sm"
            href={whatsappLink(b.whatsapp, `Hello, I would like to pay for order ${order.number}.`)}
            target="_blank"
            rel="noopener noreferrer"
          >
            Message us about payment
          </a>
        ) : null}
      </section>
    );
  }

  const status = payment?.status;
  return (
    <section className={`panel panel-pad ${styles.panel}`} aria-labelledby="pay-title" aria-live="polite">
      <div className="spread">
        <h2 id="pay-title" className="display-3">
          {status && active(status) ? "Waiting for your approval" : "Pay with Mobile Money"}
        </h2>
        {methods.data?.testMode ? (
          <span className="badge badge-warning">
            <FlaskConical size={12} aria-hidden /> Test mode
          </span>
        ) : null}
      </div>
      {methods.data?.testMode ? (
        <p className="tiny muted">Payments are simulated in this environment. No real money moves.</p>
      ) : null}

      {status && active(status) ? (
        <div className={styles.waiting}>
          <Smartphone size={36} aria-hidden className={styles.phoneIcon} />
          <div className="stack-sm">
            <p>
              We sent a request for{" "}
              <strong>
                <Price minor={payment!.amountMinor} currency={payment!.currency} />
              </strong>{" "}
              to {payment!.payer} on {payment!.providerName}.
            </p>
            <p className="small muted">
              {payment!.provider === "orange"
                ? "Confirm with your Orange Money PIN. If you do not see a prompt, dial #150*50#."
                : "Approve it on your phone. If you do not see a prompt, dial *126#."}{" "}
              You can leave this page; we will confirm the payment as soon as the provider does.
            </p>
            <span className="row small muted">
              <span className="spinner" aria-hidden /> Checking with {payment!.providerName}...
            </span>
          </div>
        </div>
      ) : status === "succeeded" ? (
        <p className="notice notice-success">
          Payment received: <Price minor={payment!.amountMinor} currency={payment!.currency} />. Thank you.
        </p>
      ) : (
        <form onSubmit={pay} className="stack">
          {status && !active(status) ? (
            <p className="notice notice-danger" role="alert">
              {payment!.failureMessage ?? "The payment was not completed."} You can try again.
            </p>
          ) : null}
          {error ? (
            <p className="notice notice-danger" role="alert">
              {error}
            </p>
          ) : null}
          {depositDue ? (
            <div className="choices" role="radiogroup" aria-label="What to pay">
              <label className="choice">
                <input
                  type="radio"
                  name="purpose"
                  checked={purpose === "deposit"}
                  onChange={() => setPurpose("deposit")}
                />
                <span className="choice-title">Deposit</span>
                <span className="choice-meta">
                  <Price minor={order.depositRequiredMinor - order.amountPaidMinor} currency={order.currency} />
                </span>
              </label>
              <label className="choice">
                <input type="radio" name="purpose" checked={purpose === "full"} onChange={() => setPurpose("full")} />
                <span className="choice-title">Pay in full</span>
                <span className="choice-meta">
                  <Price minor={order.totalMinor - order.amountPaidMinor} currency={order.currency} />
                </span>
              </label>
            </div>
          ) : (
            <p>
              Amount due:{" "}
              <strong>
                <Price minor={amount} currency={order.currency} />
              </strong>
            </p>
          )}
          <div className="choices" role="radiogroup" aria-label="Payment method">
            {methods.data?.methods.map((m) => (
              <label key={m.provider} className={`choice ${styles.method}`}>
                <input
                  type="radio"
                  name="provider"
                  value={m.provider}
                  checked={provider === m.provider}
                  onChange={() => setProvider(m.provider)}
                />
                <PaymentMark provider={m.provider} size={34} />
                <span className="choice-title">{m.name}</span>
              </label>
            ))}
          </div>
          <div className="field">
            <label htmlFor="msisdn">Mobile Money number</label>
            <div className="input-group">
              <span
                className="input-addon"
                style={{ borderLeft: "1px solid var(--line-strong)", borderRight: 0, borderRadius: "2px 0 0 2px" }}
              >
                +{b.countryCode}
              </span>
              <input
                id="msisdn"
                className="input"
                inputMode="tel"
                autoComplete="tel-national"
                value={msisdn}
                onChange={(e) => setMsisdn(e.target.value)}
                required
                style={{ borderRadius: "0 2px 2px 0" }}
              />
            </div>
            <span className="hint">The number that will approve the payment. We never ask for your PIN.</span>
          </div>
          <button className="btn btn-primary" type="submit" disabled={busy || !provider || !msisdn}>
            {busy ? <span className="spinner" aria-hidden /> : null} Pay{" "}
            <Price
              minor={purpose === "full" ? order.totalMinor - order.amountPaidMinor : amount}
              currency={order.currency}
            />
          </button>
        </form>
      )}
    </section>
  );
}
