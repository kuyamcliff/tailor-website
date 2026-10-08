import type { Metadata } from "next";
import { OwnerRequests } from "@/features/owner/requests";

export const metadata: Metadata = { title: "Requests" };

export default function Page() {
  return <OwnerRequests />;
}
