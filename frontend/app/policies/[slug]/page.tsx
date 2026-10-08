import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { getContent } from "@/lib/server-data";
import { policy, policyKeys } from "@/lib/content";

export function generateStaticParams() {
  return policyKeys.map((slug) => ({ slug }));
}

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const p = policy(await getContent(), (await params).slug);
  return p ? { title: p.title, alternates: { canonical: `/policies/${(await params).slug}` } } : { title: "Not found" };
}

export default async function PolicyPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const p = policy(await getContent(), slug);
  if (!p) notFound();
  return (
    <article className="container-narrow section-tight prose">
      <h1 className="display-2" style={{ marginBottom: 24 }}>
        {p.title}
      </h1>
      {p.sections.map((s) => (
        <section key={s.heading}>
          <h2>{s.heading}</h2>
          <p>{s.body}</p>
        </section>
      ))}
    </article>
  );
}
