import type { Metadata } from "next";
import { WishlistView } from "@/features/shop/wishlist-view";

export const metadata: Metadata = { title: "Wishlist", robots: { index: false } };

export default function WishlistPage() {
  return (
    <div className="container section-tight">
      <h1 className="display-2" style={{ marginBottom: 24 }}>
        Wishlist
      </h1>
      <WishlistView />
    </div>
  );
}
