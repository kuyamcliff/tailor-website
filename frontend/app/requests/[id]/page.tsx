import type { Metadata } from "next";
import { RequestStatus } from "@/features/quotes/request-status";

export const metadata: Metadata = { title: "Your request", robots: { index: false } };

export default async function RequestPage({ params }: { params: Promise<{ id: string }> }) {
  return <RequestStatus id={(await params).id} />;
}
