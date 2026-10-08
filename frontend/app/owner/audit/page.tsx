import type { Metadata } from "next";
import { OwnerAudit } from "@/features/owner/staff";

export const metadata: Metadata = { title: "Audit log" };

export default function Page() {
  return <OwnerAudit />;
}
