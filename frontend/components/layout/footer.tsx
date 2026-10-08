import Link from "next/link";
import { Mail, MapPin, Phone } from "lucide-react";
import { SocialIcon, socialTitle, whatsappLink, PaymentMark } from "@/components/brand/brand-icon";
import { block } from "@/lib/content";
import type { ContentBlocks, PublicConfig } from "@/lib/types";
import styles from "./footer.module.css";

export function Footer({ config, content }: { config: PublicConfig; content: ContentBlocks }) {
  const b = config.business;
  const name = b.name || "Atelier";
  const note = block(content, "footer").note;
  const addr = [b.address.line1, b.address.line2, b.address.city, b.address.country].filter(Boolean);
  const year = new Date().getFullYear();
  return (
    <footer className={styles.footer}>
      <div className={`container ${styles.grid}`}>
        <div className={styles.brand}>
          <p className={styles.name}>{name}</p>
          {note ? <p className="muted small">{note}</p> : null}
          {b.social?.length ? (
            <ul className={styles.social} aria-label="Social media">
              {b.social.map((s) => (
                <li key={s.network}>
                  <a
                    href={s.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    aria-label={`${name} on ${socialTitle[s.network]}`}
                    className={styles.socialLink}
                  >
                    <SocialIcon network={s.network} size={18} />
                  </a>
                </li>
              ))}
            </ul>
          ) : null}
        </div>

        <div>
          <h2 className={styles.heading}>Visit</h2>
          <ul className={styles.list}>
            {addr.length ? (
              <li className={styles.iconLine}>
                <MapPin size={16} aria-hidden />
                <span>
                  {b.address.mapUrl ? (
                    <a href={b.address.mapUrl} target="_blank" rel="noopener noreferrer" className={styles.plain}>
                      {addr.join(", ")}
                    </a>
                  ) : (
                    addr.join(", ")
                  )}
                </span>
              </li>
            ) : null}
            {(b.openingHours ?? []).map((h) => (
              <li key={h.days} className={styles.hours}>
                <span className="muted">{h.days}</span>
                <span className="tabular">{h.hours}</span>
              </li>
            ))}
          </ul>
        </div>

        <div>
          <h2 className={styles.heading}>Contact</h2>
          <ul className={styles.list}>
            {b.phone ? (
              <li className={styles.iconLine}>
                <Phone size={16} aria-hidden />
                <a href={`tel:${b.phone.replace(/\s/g, "")}`} className={styles.plain}>
                  {b.phone}
                </a>
              </li>
            ) : null}
            {b.whatsapp ? (
              <li className={styles.iconLine}>
                <SocialIcon network="whatsapp" size={16} />
                <a href={whatsappLink(b.whatsapp)} target="_blank" rel="noopener noreferrer" className={styles.plain}>
                  Message on WhatsApp
                </a>
              </li>
            ) : null}
            {b.email ? (
              <li className={styles.iconLine}>
                <Mail size={16} aria-hidden />
                <a href={`mailto:${b.email}`} className={styles.plain}>
                  {b.email}
                </a>
              </li>
            ) : null}
            <li>
              <Link href="/support" className={styles.plain}>
                Help with an order
              </Link>
            </li>
          </ul>
        </div>

        <div>
          <h2 className={styles.heading}>Atelier</h2>
          <ul className={styles.list}>
            <li>
              <Link href="/custom-tailor" className={styles.plain}>
                Custom tailoring
              </Link>
            </li>
            <li>
              <Link href="/studio" className={styles.plain}>
                Fitting studio
              </Link>
            </li>
            <li>
              <Link href="/shop" className={styles.plain}>
                Shop
              </Link>
            </li>
            <li>
              <Link href="/our-work" className={styles.plain}>
                Our work
              </Link>
            </li>
            <li>
              <Link href="/appointments" className={styles.plain}>
                Appointments
              </Link>
            </li>
          </ul>
        </div>
      </div>

      {config.demoContent ? (
        <p className={`container small faint ${styles.demo}`}>
          Photos marked &ldquo;Sample photo&rdquo; are licensed stock images used while this site is being set up. They
          do not show garments made by {name}.
        </p>
      ) : null}
      <div className={`container ${styles.bottom}`}>
        <p className="tiny faint">
          © {year} {name}
        </p>
        <nav aria-label="Legal" className={styles.legal}>
          <Link href="/policies/privacy">Privacy</Link>
          <Link href="/policies/terms">Terms</Link>
          <Link href="/policies/delivery">Delivery</Link>
          <Link href="/policies/alterations">Alterations and remakes</Link>
        </nav>
        {config.flags.online_payments ? (
          <div className={styles.payments} aria-label="Payment methods">
            <PaymentMark provider="mtn" size={26} />
            <PaymentMark provider="orange" size={26} />
          </div>
        ) : null}
      </div>
    </footer>
  );
}
