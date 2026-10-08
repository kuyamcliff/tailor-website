import Image from "next/image";
import Link from "next/link";
import { ArrowRight, Clock, MapPin } from "lucide-react";
import { serverApiOr } from "@/lib/server";
import { getConfig, getContent } from "@/lib/server-data";
import { block } from "@/lib/content";
import type { Fabric, ListResponse, PortfolioProject, Product, Testimonial } from "@/lib/types";
import { ProductCard } from "@/features/shop/product-card";
import { Reveal } from "@/components/ui/reveal";
import { FabricSwatch } from "@/components/ui/fabric-swatch";
import { StudioPreview } from "@/features/home/studio-preview";
import { SocialIcon, whatsappLink } from "@/components/brand/brand-icon";
import styles from "./home.module.css";

export const revalidate = 60;

export default async function HomePage() {
  const [config, content, featured, fabrics, work, testimonials] = await Promise.all([
    getConfig(),
    getContent(),
    serverApiOr<ListResponse<Product>>("/products?featured=true&limit=8", { items: [], total: 0, limit: 8, offset: 0 }),
    serverApiOr<Fabric[]>("/fabrics", []),
    serverApiOr<PortfolioProject[]>("/portfolio?limit=6", []),
    serverApiOr<Testimonial[]>("/testimonials", []),
  ]);
  const hero = block(content, "home.hero");
  const signature = block(content, "home.signature");
  const studio = block(content, "home.studio");
  const process = block(content, "home.process");
  const services = block(content, "services");
  const b = config.business;

  return (
    <>
      {/* 1. Hero */}
      <section className={styles.hero}>
        {hero.image ? (
          <Image src={hero.image} alt={hero.imageAlt ?? ""} fill priority sizes="100vw" className={styles.heroImg} />
        ) : null}
        <div className={styles.heroShade} aria-hidden />
        <div className={`container ${styles.heroInner}`}>
          <span className="eyebrow">{hero.eyebrow}</span>
          <h1 className="display-1">{hero.title}</h1>
          <p className={`lede ${styles.heroLede}`}>{hero.subtitle}</p>
          <div className="row-wrap">
            <Link href={hero.primaryCta.href} className="btn btn-primary">
              {hero.primaryCta.label}
            </Link>
            {config.flags.appointments ? (
              <Link href={hero.secondaryCta.href} className="btn">
                {hero.secondaryCta.label}
              </Link>
            ) : null}
          </div>
        </div>
      </section>

      {/* 2. Signature custom tailoring */}
      <section className={`section ${styles.signature}`}>
        <div className={`container ${styles.signatureGrid}`}>
          <Reveal>
            <span className="eyebrow">Custom tailoring</span>
            <h2 className="display-2" style={{ marginTop: 16 }}>
              {signature.title}
            </h2>
          </Reveal>
          <Reveal delay={80} className="stack-lg">
            <p className="lede">{signature.body}</p>
            <Link href={signature.cta.href} className="text-link">
              {signature.cta.label} <ArrowRight size={16} aria-hidden />
            </Link>
          </Reveal>
        </div>
      </section>

      {/* 3. Featured garments */}
      {featured.items.length ? (
        <section className="section-tight">
          <div className="container">
            <div className="section-head">
              <div>
                <span className="eyebrow">Ready to wear</span>
                <h2 className="display-2">Featured pieces</h2>
              </div>
              <Link href="/shop" className="text-link">
                Shop all <ArrowRight size={16} aria-hidden />
              </Link>
            </div>
            <div className={styles.productGrid}>
              {featured.items.slice(0, 4).map((p, i) => (
                <ProductCard key={p.id} product={p} priority={i < 2} />
              ))}
            </div>
          </div>
        </section>
      ) : null}

      {/* 4. 3D fitting studio preview */}
      {config.flags.studio ? (
        <section className={`section ${styles.studioSection}`}>
          <div className={`container ${styles.studioGrid}`}>
            <Reveal className="stack-lg">
              <span className="eyebrow">Fitting studio</span>
              <h2 className="display-2">{studio.title}</h2>
              <p className="lede">{studio.body}</p>
              <p className="small muted">{studio.note}</p>
              <div className="row-wrap">
                <Link href={studio.cta.href} className="btn btn-primary">
                  {studio.cta.label}
                </Link>
                <Link href="/custom-tailor" className="btn btn-ghost">
                  How it works
                </Link>
              </div>
            </Reveal>
            <Reveal delay={100}>
              <StudioPreview />
            </Reveal>
          </div>
        </section>
      ) : null}

      {/* 5. Fabric collection */}
      {fabrics.length ? (
        <section className="section-tight">
          <div className="container">
            <div className="section-head">
              <div>
                <span className="eyebrow">The cloth</span>
                <h2 className="display-2">Fabric collection</h2>
              </div>
              <p className="muted" style={{ maxWidth: 420 }}>
                Every fabric can be seen and touched at the studio. Choose one in the fitting studio to see it on your garment.
              </p>
            </div>
            <ul className={styles.fabrics}>
              {fabrics.slice(0, 6).map((f) => (
                <li key={f.key}>
                  <Link href={`/studio?fabric=${f.key}`} className={styles.fabric}>
                    <span className={styles.swatch}>
                      <FabricSwatch fabric={f} sizes="(max-width: 640px) 50vw, 16vw" />
                    </span>
                    <span className={styles.fabricName}>{f.name}</span>
                    <span className="tiny muted">
                      {f.composition}
                      {f.stockStatus !== "available" ? ` · ${f.stockStatus.replaceAll("_", " ")}` : ""}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        </section>
      ) : null}

      {/* 6. Portfolio */}
      {work.length ? (
        <section className="section">
          <div className="container">
            <div className="section-head">
              <div>
                <span className="eyebrow">Our work</span>
                <h2 className="display-2">From the atelier</h2>
              </div>
              <Link href="/our-work" className="text-link">
                See all work <ArrowRight size={16} aria-hidden />
              </Link>
            </div>
            <div className={styles.work}>
              {work.slice(0, 5).map((p, i) => (
                <Link key={p.id} href={`/our-work/${p.slug}`} className={`${styles.workItem} ${i === 0 ? styles.workLead : ""}`}>
                  {p.media[0] ? (
                    <Image src={p.media[0].url} alt={p.media[0].alt} fill sizes={i === 0 ? "(max-width: 900px) 100vw, 50vw" : "(max-width: 900px) 50vw, 25vw"} style={{ objectFit: "cover" }} />
                  ) : null}
                  <span className={styles.workCaption}>
                    <span className="tiny eyebrow">{p.category}</span>
                    <span className={styles.workTitle}>{p.title}</span>
                  </span>
                </Link>
              ))}
            </div>
          </div>
        </section>
      ) : null}

      {/* 7. Process */}
      {process.steps.length ? (
        <section className={`section ${styles.processSection}`}>
          <div className="container">
            <div className="section-head">
              <div>
                <span className="eyebrow">Process</span>
                <h2 className="display-2">{process.title}</h2>
              </div>
            </div>
            <ol className={styles.process}>
              {process.steps.map((s, i) => (
                <Reveal as="li" key={s.title} delay={i * 50} className={styles.step}>
                  <span className={styles.stepNo}>{String(i + 1).padStart(2, "0")}</span>
                  <h3 className="title">{s.title}</h3>
                  <p className="muted small">{s.body}</p>
                </Reveal>
              ))}
            </ol>
          </div>
        </section>
      ) : null}

      {/* 8. Services */}
      {services.items.length ? (
        <section className="section-tight">
          <div className="container">
            <div className="section-head">
              <div>
                <span className="eyebrow">Services</span>
                <h2 className="display-2">{services.title}</h2>
              </div>
            </div>
            <ul className={styles.services}>
              {services.items.map((s) => (
                <li key={s.title}>
                  <Link href={s.href} className={styles.service}>
                    <span className="display-3">{s.title}</span>
                    <span className="muted small">{s.body}</span>
                    <ArrowRight size={18} aria-hidden className={styles.serviceArrow} />
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        </section>
      ) : null}

      {/* 9. Testimonials: only real, consented testimonials are ever shown */}
      {testimonials.length ? (
        <section className="section">
          <div className="container">
            <div className="section-head">
              <div>
                <span className="eyebrow">Clients</span>
                <h2 className="display-2">In their words</h2>
              </div>
            </div>
            <div className={styles.quotes}>
              {testimonials.slice(0, 3).map((t) => (
                <figure key={t.id} className={styles.quote}>
                  <blockquote className="serif">“{t.quote}”</blockquote>
                  <figcaption className="small muted">
                    {t.customerName}
                    {t.context ? `, ${t.context}` : ""}
                  </figcaption>
                </figure>
              ))}
            </div>
          </div>
        </section>
      ) : null}

      {/* 10. Appointment CTA */}
      {config.flags.appointments ? (
        <section className={styles.appointment}>
          <div className={`container ${styles.appointmentInner}`}>
            <div>
              <span className="eyebrow">Appointments</span>
              <h2 className="display-2" style={{ marginTop: 12 }}>
                Request a consultation.
              </h2>
              <p className="lede" style={{ marginTop: 16 }}>
                Talk through your garment, see the fabrics and have your measurements taken by a tailor.
              </p>
            </div>
            <div className="row-wrap">
              <Link href="/appointments" className="btn btn-cream">
                Book a time
              </Link>
              {b.whatsapp ? (
                <a href={whatsappLink(b.whatsapp, "Hello, I would like to book a consultation.")} className="btn" target="_blank" rel="noopener noreferrer">
                  <SocialIcon network="whatsapp" size={16} /> WhatsApp
                </a>
              ) : null}
            </div>
          </div>
        </section>
      ) : null}

      {/* 11. Location and contact */}
      {b.address.line1 || b.phone ? (
        <section className="section-tight">
          <div className={`container ${styles.visit}`}>
            <div>
              <span className="eyebrow">Visit</span>
              <h2 className="display-3" style={{ marginTop: 12 }}>
                {b.name || "The atelier"}
              </h2>
            </div>
            {b.address.line1 ? (
              <p className={styles.visitLine}>
                <MapPin size={18} aria-hidden />
                <span>{[b.address.line1, b.address.line2, b.address.city].filter(Boolean).join(", ")}</span>
              </p>
            ) : null}
            {b.openingHours?.length ? (
              <p className={styles.visitLine}>
                <Clock size={18} aria-hidden />
                <span>{b.openingHours.map((h) => `${h.days} ${h.hours}`).join(" · ")}</span>
              </p>
            ) : null}
            <Link href="/contact" className="text-link">
              Directions and contact <ArrowRight size={16} aria-hidden />
            </Link>
          </div>
        </section>
      ) : null}
    </>
  );
}
