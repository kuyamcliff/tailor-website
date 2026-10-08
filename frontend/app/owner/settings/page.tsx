import type { Metadata } from "next";
import { OwnerSettings } from "@/features/owner/settings";

export const metadata: Metadata = { title: "Settings" };

export default function Page() {
  return <OwnerSettings />;
}
