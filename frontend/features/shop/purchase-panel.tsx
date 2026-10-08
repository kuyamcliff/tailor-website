"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { Ruler, Scissors } from "lucide-react";
import { Price } from "@/components/ui/price";
import { useCart } from "@/stores/cart";
import type { Product } from "@/lib/types";
import styles from "./purchase-panel.module.css";

// PurchasePanel lets the customer pick a size and colour and add the variant to the bag.
// Availability comes from the server; sold-out sizes stay visible but cannot be chosen.
export function PurchasePanel({ product, compact = false }: { product: Product; compact?: boolean }) {
  const add = useCart((s) => s.add);
  const colors = useMemo(() => [...new Set(product.variants.map((v) => v.colorName))], [product.variants]);
  const [color, setColor] = useState(colors[0] ?? "");
  const sizes = product.variants.filter((v) => v.colorName === color);
  const firstAvailable = sizes.find((v) => v.available);
  const [variantId, setVariantId] = useState(firstAvailable?.id ?? "");
  const variant = product.variants.find((v) => v.id === variantId);
  const [error, setError] = useState("");

  function addToBag() {
    if (!variant) {
      setError("Choose a size first.");
      return;
    }
    add({
      variantId: variant.id,
      productSlug: product.slug,
      name: product.name,
      size: variant.sizeLabel,
      color: variant.colorName,
      image: product.media[0]?.url ?? null,
      unitPriceMinor: variant.priceMinor,
      quantity: 1,
      requiresFitting: product.requiresFitting,
    });
  }

  if (product.variants.length === 0) {
    return <p className="notice">This piece is not available to order online. Contact us and we will help.</p>;
  }

  return (
    <div className={`stack ${styles.panel}`}>
      <Price minor={variant?.priceMinor ?? product.priceMinor} className={styles.price} />
      {colors.length > 1 || (colors[0] && colors[0] !== "") ? (
        <fieldset className={styles.fieldset}>
          <legend className="label">Colour{color ? `: ${color}` : ""}</legend>
          <div className="row-wrap">
            {colors.map((c) => (
              <button
                key={c}
                type="button"
                className={`choice ${styles.sizeChoice}`}
                aria-pressed={c === color}
                onClick={() => {
                  setColor(c);
                  setVariantId(product.variants.find((v) => v.colorName === c && v.available)?.id ?? "");
                }}
              >
                <span className="choice-title">{c || "Standard"}</span>
              </button>
            ))}
          </div>
        </fieldset>
      ) : null}
      <fieldset className={styles.fieldset}>
        <legend className="label">Size</legend>
        <div className={styles.sizes}>
          {sizes.map((v) => (
            <button
              key={v.id}
              type="button"
              className={`choice ${styles.sizeChoice}`}
              aria-pressed={v.id === variantId}
              disabled={!v.available}
              aria-disabled={!v.available}
              onClick={() => {
                setVariantId(v.id);
                setError("");
              }}
              title={v.available ? undefined : "Sold out"}
            >
              <span className="choice-title">{v.sizeLabel}</span>
              <span className="choice-meta">{!v.available ? "Sold out" : v.madeToOrder && !v.lowStock ? "Made to order" : v.lowStock ? "Few left" : ""}</span>
            </button>
          ))}
        </div>
        {error ? (
          <p className="error small" role="alert">
            {error}
          </p>
        ) : null}
      </fieldset>
      <button className="btn btn-primary btn-block" onClick={addToBag} disabled={!sizes.some((v) => v.available)}>
        {sizes.some((v) => v.available) ? "Add to bag" : "Sold out"}
      </button>
      {!compact ? (
        <div className={styles.extras}>
          {product.requiresFitting ? (
            <p className="small">
              <Ruler size={15} aria-hidden /> This piece includes a fitting. We will contact you to book it after your order.
            </p>
          ) : null}
          {product.customizable ? (
            <p className="small">
              <Scissors size={15} aria-hidden /> Want it cut to your measurements?{" "}
              <Link className="link" href={`/studio?garment=${product.garmentTypeKey ?? "suit"}${product.fabric ? `&fabric=${product.fabric.key}` : ""}`}>
                Customise in the fitting studio
              </Link>
            </p>
          ) : null}
          <Link href="/appointments?type=fitting" className="text-link">
            Book a fitting
          </Link>
        </div>
      ) : null}
    </div>
  );
}
