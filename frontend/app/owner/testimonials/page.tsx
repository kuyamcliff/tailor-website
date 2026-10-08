import { redirect } from "next/navigation";

// Testimonials are managed alongside the portfolio.
export default function Page() {
  redirect("/owner/portfolio#t-h");
}
