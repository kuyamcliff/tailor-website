import type { Metadata } from "next";
import { OwnerSupport } from "@/features/owner/support";

export const metadata: Metadata = { title: "Messages" };

export default function Page() {
  return <OwnerSupport />;
}
