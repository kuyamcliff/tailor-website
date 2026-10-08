import type { Metadata } from "next";
import { SupportThreadView } from "@/features/support/thread-view";

export const metadata: Metadata = { title: "Conversation", robots: { index: false } };

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  return <SupportThreadView id={(await params).id} />;
}
