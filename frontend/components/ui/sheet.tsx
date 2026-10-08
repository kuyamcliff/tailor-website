"use client";

import { useEffect, useRef } from "react";
import { X } from "lucide-react";
import styles from "./sheet.module.css";

type Props = {
  open: boolean;
  onClose: () => void;
  title: string;
  side?: "right" | "left" | "bottom" | "center";
  size?: "sm" | "md" | "lg";
  children: React.ReactNode;
  footer?: React.ReactNode;
  hideTitle?: boolean;
};

// Sheet uses the native <dialog> element: focus is trapped, Escape closes it and the rest of the
// page is inert while it is open.
export function Sheet({ open, onClose, title, side = "right", size = "md", children, footer, hideTitle }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      d.showModal();
      document.documentElement.style.overflow = "hidden";
    } else if (!open && d.open) {
      d.close();
    }
    return () => {
      document.documentElement.style.overflow = "";
    };
  }, [open]);

  return (
    <dialog
      ref={ref}
      className={`${styles.sheet} ${styles[side]} ${styles[size]}`}
      aria-label={title}
      onClose={() => {
        document.documentElement.style.overflow = "";
        onClose();
      }}
      onClick={(e) => {
        if (e.target === ref.current) onClose();
      }}
    >
      <div className={styles.inner}>
        <header className={styles.header}>
          <h2 className={hideTitle ? "visually-hidden" : styles.title}>{title}</h2>
          <button className="icon-btn" onClick={onClose} aria-label="Close" autoFocus>
            <X size={20} aria-hidden />
          </button>
        </header>
        <div className={styles.body}>{children}</div>
        {footer ? <footer className={styles.footer}>{footer}</footer> : null}
      </div>
    </dialog>
  );
}
