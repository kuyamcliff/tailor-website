"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import { Eye, GitCompareArrows, Heart } from "lucide-react";
import { Price } from "@/components/ui/price";
import { useSaved } from "@/stores/saved";
import { useToast } from "@/components/providers/toast";
import type { Product } from "@/lib/types";
import { QuickView } from "./quick-view";
import styles from "./product-card.module.css";

export const availabilityLabel: Record<Product["availability"], string> = {
  in_stock: "In stock",
  low_stock: "Only a few left",
  made_to_order: "Made to order",
  out_of_stock: "Sold out",
};

export function ProductCard({ product, priority = false }: { product: Product; priority?: boolean }) {
  const [quick, setQuick] = useState(false);
  const wished = useSaved((s) => s.wishlist.some((w) => w.id === product.id));
  const compared = useSaved((s) => s.compare.some((c) => c.id === product.id));
  const toggleWish = useSaved((s) => s.toggleWish);
  const toggleCompare = useSaved((s) => s.toggleCompare);
  const toast = useToast();
  const img = product.media[0];
  const alt = product.media[1];
  const item = { type: "product" as const, id: product.id, slug: product.slug, name: product.name, image: img?.url ?? null };

  return (
    <article className={styles.card}>
      <Link href={`/shop/${product.slug}`} className={styles.media} aria-label={product.name}>
        {img ? (
          <Image src={img.url} alt={img.alt} fill sizes="(max-width: 640px) 50vw, (max-width: 1100px) 33vw, 25vw" priority={priority} className={styles.img} />
        ) : (
          <span className={styles.noImage}>{product.name}</span>
        )}
        {alt ? <Image src={alt.url} alt="" fill sizes="25vw" className={`${styles.img} ${styles.alt}`} aria-hidden /> : null}
        {product.availability !== "in_stock" ? (
          <span className={`badge ${product.availability === "out_of_stock" ? "" : "badge-gold"} ${styles.flag}`}>{availabilityLabel[product.availability]}</span>
        ) : null}
      </Link>
      <div className={styles.tools}>
        <button
          className="icon-btn"
          aria-pressed={wished}
          aria-label={wished ? `Remove ${product.name} from wishlist` : `Add ${product.name} to wishlist`}
          onClick={() => {
            toggleWish(item);
            toast(wished ? "Removed from your wishlist" : "Saved to your wishlist", "info");
          }}
        >
          <Heart size={18} aria-hidden fill={wished ? "currentColor" : "none"} />
        </button>
        <button
          className="icon-btn"
          aria-pressed={compared}
          aria-label={compared ? `Remove ${product.name} from comparison` : `Compare ${product.name}`}
          onClick={() => {
            if (!toggleCompare(item)) toast("You can compare up to four pieces. Remove one first.", "error");
            else toast(compared ? "Removed from comparison" : "Added to comparison", "info");
          }}
        >
          <GitCompareArrows size={18} aria-hidden />
        </button>
        <button className="icon-btn" aria-label={`Quick view of ${product.name}`} onClick={() => setQuick(true)}>
          <Eye size={18} aria-hidden />
        </button>
      </div>
      <div className={styles.body}>
        <Link href={`/shop/${product.slug}`} className={styles.name}>
          {product.name}
        </Link>
        <p className={styles.meta}>
          {product.fabric ? product.fabric.name : product.category?.name}
          {product.customizable ? " · Customisable" : ""}
        </p>
        <Price minor={product.priceMinor} maxMinor={product.priceMaxMinor} className={styles.price} />
      </div>
      {quick ? <QuickView slug={product.slug} onClose={() => setQuick(false)} /> : null}
    </article>
  );
}
