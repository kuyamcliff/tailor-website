import type { Metadata } from "next";
import { OwnerQuotes } from "@/features/owner/quotes";

export const metadata: Metadata = { title: "Quotes" };

export default function Page() {
  return <OwnerQuotes />;
}
