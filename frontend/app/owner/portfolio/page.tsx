import type { Metadata } from "next";
import { OwnerPortfolio } from "@/features/owner/portfolio";

export const metadata: Metadata = { title: "Portfolio" };

export default function Page() {
  return <OwnerPortfolio />;
}
