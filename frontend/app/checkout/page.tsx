import type { Metadata } from "next";
import { CheckoutForm } from "@/features/checkout/checkout-form";

export const metadata: Metadata = { title: "Checkout", robots: { index: false } };

export default function CheckoutPage() {
  return (
    <div className="container section-tight">
      <h1 className="display-2" style={{ marginBottom: 32 }}>
        Checkout
      </h1>
      <CheckoutForm />
    </div>
  );
}
