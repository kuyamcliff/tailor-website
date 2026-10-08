import type { Metadata } from "next";
import { OwnerPayments } from "@/features/owner/payments";

export const metadata: Metadata = { title: "Payments" };

export default function Page() {
  return <OwnerPayments />;
}
