import type { Metadata } from "next";
import { QuoteView } from "@/features/quotes/quote-view";

export const metadata: Metadata = { title: "Your quote", robots: { index: false } };

export default async function QuotePage({ params }: { params: Promise<{ id: string }> }) {
  return <QuoteView id={(await params).id} />;
}
