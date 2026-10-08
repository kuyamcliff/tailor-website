import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { serverApiOr } from "@/lib/server";
import type { PortfolioProject } from "@/lib/types";
import styles from "./work.module.css";
import { SampleTag } from "@/components/ui/sample-tag";

export const metadata: Metadata = {
  title: "Our work",
  description: "Suits, dresses, gowns, shirts, traditional wear and alterations made in the atelier.",
  alternates: { canonical: "/our-work" },
};

const categories = [
  ["", "All"],
  ["suits", "Suits"],
  ["dresses", "Dresses"],
  ["gowns", "Gowns"],
  ["shirts", "Shirts"],
  ["trousers", "Trousers"],
  ["traditional", "Traditional"],
  ["wedding", "Wedding"],
  ["alterations", "Alterations"],
  ["other", "Other"],
] as const;

export default async function WorkPage({ searchParams }: { searchParams: Promise<{ category?: string }> }) {
  const { category = "" } = await searchParams;
  const valid = categories.some(([k]) => k === category) ? category : "";
  const projects = await serverApiOr<PortfolioProject[]>(`/portfolio${valid ? `?category=${valid}` : ""}`, []);
  return (
    <div className="container section-tight">
      <header className="stack" style={{ marginBottom: 32 }}>
        <span className="eyebrow">Our work</span>
        <h1 className="display-2">Made in the atelier</h1>
        <p className="lede">Each piece here was cut and finished for one client.</p>
      </header>
      <nav className={styles.filters} aria-label="Filter by category">
        {categories.map(([k, label]) => (
          <Link
            key={k}
            href={k ? `/our-work?category=${k}` : "/our-work"}
            className={styles.filter}
            aria-current={valid === k ? "page" : undefined}
            scroll={false}
          >
            {label}
          </Link>
        ))}
      </nav>
      {projects.length === 0 ? (
        <div className="empty">
          <p className="display-3">Nothing here yet.</p>
          <p className="muted">
            We are photographing recent work. In the meantime, visit the shop or start your own piece.
          </p>
          <Link href="/custom-tailor" className="btn btn-sm btn-primary">
            Create your outfit
          </Link>
        </div>
      ) : (
        <div className={styles.grid}>
          {projects.map((p, i) => (
            <Link key={p.id} href={`/our-work/${p.slug}`} className={styles.item}>
              <div className={styles.media} style={{ aspectRatio: i % 3 === 0 ? "4 / 5" : "1 / 1" }}>
                {p.media[0] ? (
                  <Image
                    src={p.media[0].url}
                    alt={p.media[0].alt}
                    fill
                    sizes="(max-width: 700px) 100vw, (max-width: 1100px) 50vw, 33vw"
                    style={{ objectFit: "cover" }}
                  />
                ) : null}
                <SampleTag show={p.media[0]?.sample} />
              </div>
              <span className="eyebrow tiny">{p.category}</span>
              <span className={styles.title}>{p.title}</span>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
