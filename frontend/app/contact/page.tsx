import type { Metadata } from "next";
import { Clock, Mail, MapPin, Phone } from "lucide-react";
import { getConfig, getContent } from "@/lib/server-data";
import { block } from "@/lib/content";
import { SocialIcon, socialTitle, whatsappLink } from "@/components/brand/brand-icon";
import { ContactForm } from "@/features/support/contact-form";

export const metadata: Metadata = { title: "Contact", alternates: { canonical: "/contact" } };

export default async function ContactPage() {
  const [cfg, content] = await Promise.all([getConfig(), getContent()]);
  const c = block(content, "contact");
  const b = cfg.business;
  const addr = [b.address.line1, b.address.line2, b.address.city, b.address.region, b.address.country].filter(Boolean).join(", ");
  return (
    <div className="container section-tight" style={{ display: "grid", gap: 48, gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 380px), 1fr))" }}>
      <section className="stack-lg">
        <span className="eyebrow">Contact</span>
        <h1 className="display-2">{c.title}</h1>
        {c.body ? <p className="lede">{c.body}</p> : null}
        <ul style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: 16 }}>
          {addr ? (
            <li className="row" style={{ alignItems: "flex-start" }}>
              <MapPin size={18} aria-hidden style={{ color: "var(--gold)", marginTop: 4 }} />
              <span>
                {addr}
                {b.address.mapUrl ? (
                  <>
                    <br />
                    <a className="link small" href={b.address.mapUrl} target="_blank" rel="noopener noreferrer">
                      Open in maps
                    </a>
                  </>
                ) : null}
              </span>
            </li>
          ) : null}
          {b.openingHours?.length ? (
            <li className="row" style={{ alignItems: "flex-start" }}>
              <Clock size={18} aria-hidden style={{ color: "var(--gold)", marginTop: 4 }} />
              <span>
                {b.openingHours.map((h) => (
                  <span key={h.days} style={{ display: "block" }}>
                    {h.days}: {h.hours}
                  </span>
                ))}
              </span>
            </li>
          ) : null}
          {b.phone ? (
            <li className="row">
              <Phone size={18} aria-hidden style={{ color: "var(--gold)" }} />
              <a href={`tel:${b.phone.replace(/\s/g, "")}`}>{b.phone}</a>
            </li>
          ) : null}
          {b.email ? (
            <li className="row">
              <Mail size={18} aria-hidden style={{ color: "var(--gold)" }} />
              <a href={`mailto:${b.email}`}>{b.email}</a>
            </li>
          ) : null}
        </ul>
        <div className="row-wrap">
          {b.whatsapp ? (
            <a className="btn" href={whatsappLink(b.whatsapp)} target="_blank" rel="noopener noreferrer">
              <SocialIcon network="whatsapp" size={16} /> WhatsApp
            </a>
          ) : null}
          {(b.social ?? [])
            .filter((s) => s.network !== "whatsapp")
            .map((s) => (
              <a key={s.network} className="icon-btn" href={s.url} target="_blank" rel="noopener noreferrer" aria-label={socialTitle[s.network]}>
                <SocialIcon network={s.network} />
              </a>
            ))}
        </div>
      </section>
      <section className="panel panel-pad" aria-labelledby="contact-form-title">
        <h2 id="contact-form-title" className="display-3" style={{ marginBottom: 8 }}>
          Send a message
        </h2>
        <p className="muted small" style={{ marginBottom: 24 }}>
          We reply during opening hours. For an existing order, use the help link on your order page so we can see the details.
        </p>
        {cfg.flags.support_inbox ? (
          <ContactForm />
        ) : (
          <p className="notice">Online messages are paused. Please call or message us on WhatsApp.</p>
        )}
      </section>
    </div>
  );
}
