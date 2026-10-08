import type { Metadata } from "next";
import { ManageAppointment } from "@/features/appointments/manage";

export const metadata: Metadata = { title: "Your appointment", robots: { index: false } };

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  return <ManageAppointment id={(await params).id} />;
}
