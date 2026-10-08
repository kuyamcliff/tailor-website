import type { Metadata } from "next";
import { OwnerCustomers } from "@/features/owner/customers";

export const metadata: Metadata = { title: "Customers" };

export default function Page() {
  return <OwnerCustomers />;
}
