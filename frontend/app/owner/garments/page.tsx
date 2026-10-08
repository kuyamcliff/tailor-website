import type { Metadata } from "next";
import { OwnerGarments } from "@/features/owner/garments";

export const metadata: Metadata = { title: "Garments and fit" };

export default function Page() {
  return <OwnerGarments />;
}
