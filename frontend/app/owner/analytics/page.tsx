import type { Metadata } from "next";
import { OwnerAnalytics } from "@/features/owner/analytics";

export const metadata: Metadata = { title: "Analytics" };

export default function Page() {
  return <OwnerAnalytics />;
}
