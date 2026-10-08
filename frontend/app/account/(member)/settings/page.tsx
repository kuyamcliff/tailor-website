import type { Metadata } from "next";
import { AccountSettings } from "@/features/account/settings-page";

export const metadata: Metadata = { title: "Settings and privacy" };

export default function Page() {
  return <AccountSettings />;
}
