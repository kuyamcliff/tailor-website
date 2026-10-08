import type { Metadata } from "next";
import { getConfig } from "@/lib/server-data";
import { ContactForm } from "@/features/support/contact-form";
import { ConversationList } from "@/features/support/conversation-list";

export const metadata: Metadata = { title: "Help and messages", robots: { index: false } };

const allowed = new Set([
  "general",
  "order_issue",
  "measurement_help",
  "payment_issue",
  "appointment_issue",
  "alteration_request",
  "other",
]);

export default async function SupportPage({
  searchParams,
}: {
  searchParams: Promise<{ order?: string; category?: string }>;
}) {
  const [cfg, sp] = await Promise.all([getConfig(), searchParams]);
  const orderId = sp.order && /^[0-9a-f-]{36}$/i.test(sp.order) ? sp.order : undefined;
  const category = sp.category && allowed.has(sp.category) ? sp.category : orderId ? "order_issue" : "general";
  return (
    <div
      className="container section-tight"
      style={{
        display: "grid",
        gap: 48,
        gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 380px), 1fr))",
        alignItems: "start",
      }}
    >
      <section className="stack-lg">
        <header className="stack-sm">
          <span className="eyebrow">Help</span>
          <h1 className="display-2">How can we help?</h1>
          <p className="lede">
            Ask about an order, your measurements, a payment or an appointment. A tailor reads every message.
          </p>
        </header>
        <ConversationList />
      </section>
      <section className="panel panel-pad stack" aria-labelledby="support-form-title">
        <h2 id="support-form-title" className="display-3">
          {orderId ? "Message about your order" : "New message"}
        </h2>
        {cfg.flags.support_inbox ? (
          <ContactForm orderId={orderId} defaultCategory={category} />
        ) : (
          <p className="notice">Online messages are paused. Please call or message us on WhatsApp.</p>
        )}
      </section>
    </div>
  );
}
