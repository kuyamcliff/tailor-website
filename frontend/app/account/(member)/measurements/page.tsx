import type { Metadata } from "next";
import { AccountMeasurements } from "@/features/account/measurements-page";

export const metadata: Metadata = { title: "Measurements" };

export default function Page() {
  return <AccountMeasurements />;
}
