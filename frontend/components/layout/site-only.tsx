"use client";

import { usePathname } from "next/navigation";

// SiteOnly hides storefront chrome (header, footer, bag) inside the owner dashboard.
export function SiteOnly({ children }: { children: React.ReactNode }) {
  const path = usePathname();
  return path?.startsWith("/owner") ? null : <>{children}</>;
}
