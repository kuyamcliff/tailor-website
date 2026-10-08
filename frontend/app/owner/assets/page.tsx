import type { Metadata } from "next";
import { OwnerAssets } from "@/features/owner/assets";

export const metadata: Metadata = { title: "3D assets" };

export default function Page() {
  return <OwnerAssets />;
}
