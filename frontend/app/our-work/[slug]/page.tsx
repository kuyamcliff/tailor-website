import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { serverApi, ServerApiError } from "@/lib/server";
import type { PortfolioProject } from "@/lib/types";
import { SampleTag } from "@/components/ui/sample-tag";

async function load(slug: string) {
  try {
    return await serverApi<PortfolioProject>(`/portfolio/${encodeURIComponent(slug)}`);
  } catch (e) {
    if (e instanceof ServerApiError && e.status === 404) return null;
    throw e;
  }
}

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const p = await load((await params).slug);
  return p
    ? { title: p.title, description: p.description, alternates: { canonical: `/our-work/${p.slug}` } }
    : { title: "Not found" };
}

export default async function ProjectPage({ params }: { params: Promise<{ slug: string }> }) {
  const p = await load((await params).slug);
  if (!p) notFound();
  return (
    <article className="container section-tight">
      <nav aria-label="Breadcrumb" className="tiny muted">
        <Link href="/our-work">Our work</Link> / <span aria-current="page">{p.title}</span>
      </nav>
      <header className="stack" style={{ margin: "24px 0 32px", maxWidth: 760 }}>
        <span className="eyebrow">{p.category}</span>
        <h1 className="display-2">{p.title}</h1>
        {p.description ? <p className="lede">{p.description}</p> : null}
        {p.materials ? <p className="muted">Materials: {p.materials}</p> : null}
      </header>
      <div style={{ display: "grid", gap: 16, gridTemplateColumns: "repeat(auto-fit, minmax(min(100%, 420px), 1fr))" }}>
        {p.media.map((m, i) => (
          <div
            key={m.id}
            style={{
              position: "relative",
              aspectRatio: m.width && m.height ? `${m.width} / ${m.height}` : "4 / 5",
              background: "var(--surface)",
            }}
          >
            <Image
              src={m.url}
              alt={m.alt}
              fill
              priority={i === 0}
              sizes="(max-width: 900px) 100vw, 50vw"
              style={{ objectFit: "cover" }}
            />
            <SampleTag show={m.sample} />
          </div>
        ))}
      </div>
      {p.videoUrl ? (
        <p style={{ marginTop: 24 }}>
          <a className="link" href={p.videoUrl} target="_blank" rel="noopener noreferrer">
            Watch the video
          </a>
        </p>
      ) : null}
      <div className="notice" style={{ marginTop: 48 }}>
        <span>
          Would you like something similar?{" "}
          <Link href="/custom-tailor/request" className="link">
            Start a custom request
          </Link>{" "}
          and add this piece as a reference.
        </span>
      </div>
    </article>
  );
}
