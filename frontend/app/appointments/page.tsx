import type { Metadata } from "next";
import { getConfig } from "@/lib/server-data";
import { BookingFlow } from "@/features/appointments/booking-flow";

export const metadata: Metadata = {
  title: "Book an appointment",
  description: "Book a consultation, measuring session, fitting or pickup with the atelier.",
  alternates: { canonical: "/appointments" },
};

export default async function AppointmentsPage({ searchParams }: { searchParams: Promise<{ type?: string; order?: string; request?: string }> }) {
  const [cfg, sp] = await Promise.all([getConfig(), searchParams]);
  return (
    <div className="container section-tight">
      <header className="stack" style={{ marginBottom: 32, maxWidth: 720 }}>
        <span className="eyebrow">Appointments</span>
        <h1 className="display-2">Book a time with your tailor</h1>
        <p className="lede">Choose what you would like to do and a time that suits you. Times are shown in the studio's time zone.</p>
      </header>
      {cfg.flags.appointments ? (
        <BookingFlow initialType={sp.type} orderId={sp.order} requestId={sp.request} />
      ) : (
        <p className="notice">Online booking is paused. Please call or message us to arrange a time.</p>
      )}
    </div>
  );
}
