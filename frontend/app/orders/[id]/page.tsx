import type { Metadata } from "next";
import { OrderView } from "@/features/orders/order-view";

export const metadata: Metadata = { title: "Your order", robots: { index: false } };

export default async function OrderPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <OrderView id={id} />;
}
