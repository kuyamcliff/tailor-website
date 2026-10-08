"use client";

import Image from "next/image";
import Link from "next/link";
import { Minus, Plus, Ruler, Trash2 } from "lucide-react";
import { Sheet } from "@/components/ui/sheet";
import { Price } from "@/components/ui/price";
import { useCart, cartSubtotal } from "@/stores/cart";
import styles from "./cart-drawer.module.css";

export function CartDrawer() {
  const { lines, open, setOpen, setQuantity, remove } = useCart();
  const subtotal = cartSubtotal(lines);
  return (
    <Sheet
      open={open}
      onClose={() => setOpen(false)}
      title="Your bag"
      footer={
        lines.length ? (
          <div className="stack">
            <div className="spread">
              <span className="muted">Subtotal</span>
              <Price minor={subtotal} className="title" />
            </div>
            <p className="tiny muted">
              Delivery is calculated at checkout. Prices are confirmed when you place the order.
            </p>
            <Link href="/checkout" className="btn btn-primary btn-block" onClick={() => setOpen(false)}>
              Checkout
            </Link>
          </div>
        ) : null
      }
    >
      {lines.length === 0 ? (
        <div className="empty">
          <p className="display-3">Your bag is empty.</p>
          <p className="muted">Browse ready-made pieces, or design something made for you.</p>
          <div className="row-wrap">
            <Link href="/shop" className="btn btn-sm" onClick={() => setOpen(false)}>
              Shop
            </Link>
            <Link href="/custom-tailor" className="btn btn-sm btn-primary" onClick={() => setOpen(false)}>
              Custom tailoring
            </Link>
          </div>
        </div>
      ) : (
        <ul className={styles.list}>
          {lines.map((l) => (
            <li key={l.variantId} className={styles.line}>
              <div className={styles.thumb}>
                {l.image ? <Image src={l.image} alt="" fill sizes="88px" style={{ objectFit: "cover" }} /> : null}
              </div>
              <div className={styles.info}>
                <Link href={`/shop/${l.productSlug}`} className={styles.name} onClick={() => setOpen(false)}>
                  {l.name}
                </Link>
                <p className="small muted">
                  Size {l.size}
                  {l.color ? `, ${l.color}` : ""}
                </p>
                {l.requiresFitting ? (
                  <p className={`tiny ${styles.fitting}`}>
                    <Ruler size={13} aria-hidden /> Includes a fitting appointment
                  </p>
                ) : null}
                <div className="spread">
                  <div className={styles.qty} role="group" aria-label={`Quantity for ${l.name}`}>
                    <button
                      onClick={() => setQuantity(l.variantId, l.quantity - 1)}
                      aria-label="Decrease quantity"
                      disabled={l.quantity <= 1}
                    >
                      <Minus size={14} aria-hidden />
                    </button>
                    <span aria-live="polite" className="tabular">
                      {l.quantity}
                    </span>
                    <button
                      onClick={() => setQuantity(l.variantId, l.quantity + 1)}
                      aria-label="Increase quantity"
                      disabled={l.quantity >= 20}
                    >
                      <Plus size={14} aria-hidden />
                    </button>
                  </div>
                  <Price minor={l.unitPriceMinor * l.quantity} />
                </div>
              </div>
              <button className="icon-btn" onClick={() => remove(l.variantId)} aria-label={`Remove ${l.name}`}>
                <Trash2 size={16} aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      )}
    </Sheet>
  );
}
