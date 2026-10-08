import type { Metadata } from "next";
import { Suspense } from "react";
import { OwnerAppointments } from "@/features/owner/appointments";

export const metadata: Metadata = { title: "Appointments" };

export default function Page() {
  return (
    <Suspense>
      <OwnerAppointments />
    </Suspense>
  );
}
