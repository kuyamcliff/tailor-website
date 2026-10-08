import type { Metadata } from "next";
import { OwnerStaff } from "@/features/owner/staff";

export const metadata: Metadata = { title: "Staff" };

export default function Page() {
  return <OwnerStaff />;
}
