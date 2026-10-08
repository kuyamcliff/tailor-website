import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { OrderDocument } from "@/features/documents/order-document";

export const metadata: Metadata = { title: "Document", robots: { index: false } };

const kinds = ["summary", "invoice", "receipt", "measurements"] as const;

export default async function DocumentPage({ params }: { params: Promise<{ id: string; kind: string }> }) {
  const { id, kind } = await params;
  if (!kinds.includes(kind as (typeof kinds)[number])) notFound();
  return <OrderDocument id={id} kind={kind as (typeof kinds)[number]} />;
}
