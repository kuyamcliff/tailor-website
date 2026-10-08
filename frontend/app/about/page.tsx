import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { getConfig, getContent } from "@/lib/server-data";
import { block } from "@/lib/content";

export const metadata: Metadata = { title: "About", alternates: { canonical: "/about" } };

export default async function AboutPage() {
  const [cfg, content] = await Promise.all([getConfig(), getContent()]);
  const about = block(content, "about");
  const process = block(content, "home.process");
  return (
    <>
      <section className="section-tight">
        <div
          className="container"
          style={{
            display: "grid",
            gap: 48,
            gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 380px), 1fr))",
            alignItems: "center",
          }}
        >
          <div className="stack-lg">
            <span className="eyebrow">{cfg.business.name || "The atelier"}</span>
            <h1 className="display-2">{about.title}</h1>
            {about.body
              .split(/\n\n+/)
              .filter(Boolean)
              .map((para, i) => (
                <p key={i} className="lede">
                  {para}
                </p>
              ))}
            <div className="row-wrap">
              <Link href="/our-work" className="btn">
                See our work
              </Link>
              <Link href="/appointments" className="btn btn-primary">
                Book a consultation
              </Link>
            </div>
          </div>
          {about.image ? (
            <div style={{ position: "relative", aspectRatio: "4 / 5", background: "var(--surface)" }}>
              <Image
                src={about.image}
                alt={about.imageAlt || ""}
                fill
                priority
                sizes="(max-width: 900px) 100vw, 50vw"
                style={{ objectFit: "cover" }}
              />
            </div>
          ) : null}
        </div>
      </section>
      {process.steps.length ? (
        <section className="section-tight" style={{ borderTop: "1px solid var(--line)" }}>
          <div className="container">
            <h2 className="display-3" style={{ marginBottom: 24 }}>
              {process.title}
            </h2>
            <ol
              style={{
                listStyle: "none",
                padding: 0,
                margin: 0,
                display: "grid",
                gap: 20,
                gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
              }}
            >
              {process.steps.map((s, i) => (
                <li key={s.title} style={{ borderTop: "1px solid var(--line-strong)", paddingTop: 16 }}>
                  <span className="serif" style={{ color: "var(--gold)", fontSize: "1.6rem" }}>
                    {String(i + 1).padStart(2, "0")}
                  </span>
                  <h3 className="title" style={{ margin: "6px 0" }}>
                    {s.title}
                  </h3>
                  <p className="muted small">{s.body}</p>
                </li>
              ))}
            </ol>
          </div>
        </section>
      ) : null}
    </>
  );
}
