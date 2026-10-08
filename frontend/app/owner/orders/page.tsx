import type { Metadata } from "next";
import { OwnerOrders } from "@/features/owner/orders";

export const metadata: Metadata = { title: "Orders" };

export default function Page() {
  return <OwnerOrders />;
}
