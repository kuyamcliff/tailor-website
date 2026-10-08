import type { Metadata } from "next";
import { AccountShell } from "@/features/account/account-shell";

export const metadata: Metadata = { title: { default: "Your account", template: "%s | Your account" }, robots: { index: false } };

export default function Layout({ children }: { children: React.ReactNode }) {
  return <AccountShell>{children}</AccountShell>;
}
