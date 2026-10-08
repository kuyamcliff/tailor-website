import type { Metadata } from "next";
import { AccountOrders } from "@/features/account/orders-page";

export const metadata: Metadata = { title: "Orders and requests" };

export default function Page() {
  return <AccountOrders />;
}
