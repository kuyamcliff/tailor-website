import type { Metadata } from "next";
import { OwnerContent } from "@/features/owner/content";

export const metadata: Metadata = { title: "Pages and policies" };

export default function Page() {
  return <OwnerContent />;
}
