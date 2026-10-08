import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { serverApiOr } from "@/lib/server";
import { getConfig, getContent } from "@/lib/server-data";
import { block } from "@/lib/content";
import type { GarmentType } from "@/lib/types";
import { Price } from "@/components/ui/price";
import styles from "@/features/custom/landing.module.css";

export const metadata: Metadata = {
  title: "Custom tailoring",
  description:
    "Design a garment made to your measurements: choose the style and cloth, preview it in 3D, then request a quote.",
  alternates: { canonical: "/custom-tailor" },
};
export const revalidate = 60;

export default async function CustomTailorPage() {
  const [cfg, content, garments] = await Promise.all([
    getConfig(),
    getContent(),
    serverApiOr<GarmentType[]>("/garments", [], 60),
  ]);
  const landing = block(content, "custom.landing");
  const faqs = block(content, "faqs");
  const studio = cfg.flags.studio;
  return (
    <div className="container section-tight">
      <header className={styles.head}>
        <span className="eyebrow">Custom tailoring</span>
        <h1 className="display-1">{landing.title}</h1>
        {landing.subtitle ? <p className="lede">{landing.subtitle}</p> : null}
      </header>

      <section className={styles.index} aria-labelledby="garments-h">
        <h2 id="garments-h" className="eyebrow">
          Start with a garment
        </h2>
        <ul>
          {garments.map((g) => {
            const designable = studio && g.studioEnabled;
            return (
              <li key={g.key}>
                <Link
                  href={designable ? `/studio?garment=${g.key}` : `/custom-tailor/request?garment=${g.key}`}
                  className={styles.row}
                >
                  <span className={styles.name}>{g.name}</span>
                  <span className={styles.desc}>{g.description}</span>
                  <span className={styles.meta}>
                    {g.basePriceMinor ? (
                      <>
                        from <Price minor={g.basePriceMinor} />
                      </>
                    ) : (
                      "Quoted on request"
                    )}
                  </span>
                  <span className={styles.go}>
                    {designable ? "Design in 3D" : "Describe it"} <ArrowRight size={16} aria-hidden />
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
        <p className="small muted">
          Not sure yet?{" "}
          <Link className="link" href="/appointments?type=consultation">
            Book a consultation
          </Link>{" "}
          and we will work it out together, or{" "}
          <Link className="link" href="/custom-tailor/request">
            send a request with photos
          </Link>
          .
        </p>
      </section>

      {landing.steps.length ? (
        <section className={styles.steps} aria-labelledby="steps-h">
          <h2 id="steps-h" className="display-3">
            How it works
          </h2>
          <ol>
            {landing.steps.map((s) => (
              <li key={s.title}>
                <strong>{s.title}.</strong> {s.body}
              </li>
            ))}
          </ol>
        </section>
      ) : null}

      {faqs.items.length ? (
        <section className={styles.faq} aria-labelledby="faq-h">
          <h2 id="faq-h" className="display-3">
            {faqs.title}
          </h2>
          {faqs.items.map((f) => (
            <details key={f.q}>
              <summary>{f.q}</summary>
              <p>{f.a}</p>
            </details>
          ))}
        </section>
      ) : null}
    </div>
  );
}
