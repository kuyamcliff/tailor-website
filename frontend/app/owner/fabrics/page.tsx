import type { Metadata } from "next";
import { OwnerFabrics } from "@/features/owner/fabrics";

export const metadata: Metadata = { title: "Fabrics" };

export default function Page() {
  return <OwnerFabrics />;
}
