import type { Metadata } from "next";
import { AccountDesigns } from "@/features/account/designs-page";

export const metadata: Metadata = { title: "Saved designs" };

export default function Page() {
  return <AccountDesigns />;
}
