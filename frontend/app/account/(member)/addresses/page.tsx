import type { Metadata } from "next";
import { AccountAddresses } from "@/features/account/addresses-page";

export const metadata: Metadata = { title: "Addresses" };

export default function Page() {
  return <AccountAddresses />;
}
