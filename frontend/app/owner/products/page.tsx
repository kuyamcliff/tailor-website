import type { Metadata } from "next";
import { OwnerProducts } from "@/features/owner/products";

export const metadata: Metadata = { title: "Products" };

export default function Page() {
  return <OwnerProducts />;
}
