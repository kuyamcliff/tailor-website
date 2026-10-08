"use client";

import Image from "next/image";
import Link from "next/link";
import { useEffect, useState } from "react";
import { RotateCcw } from "lucide-react";
import styles from "./studio-preview.module.css";

// A lightweight preview of the fitting studio on the home page: pre-rendered frames of the studio's
// own 3D model at the standard view angles. The full WebGL studio only loads on /studio.
const views = [
  { key: "front", label: "Front" },
  { key: "45", label: "45°" },
  { key: "side", label: "Side" },
  { key: "back", label: "Back" },
];

export function StudioPreview() {
  const [view, setView] = useState(0);
  const [auto, setAuto] = useState(true);
  useEffect(() => {
    if (!auto || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const t = setInterval(() => setView((v) => (v + 1) % views.length), 2600);
    return () => clearInterval(t);
  }, [auto]);
  return (
    <div className={styles.frame}>
      <div className={styles.stage}>
        {views.map((v, i) => (
          <Image
            key={v.key}
            src={`/3d/renders/suit-${v.key}.webp`}
            alt={i === view ? `Suit in the fitting studio, ${v.label} view` : ""}
            fill
            sizes="(max-width: 960px) 100vw, 50vw"
            className={`${styles.render} ${i === view ? styles.active : ""}`}
            aria-hidden={i !== view}
          />
        ))}
      </div>
      <div className={styles.bar}>
        <div className={styles.views} role="group" aria-label="View angle">
          {views.map((v, i) => (
            <button
              key={v.key}
              className={styles.view}
              aria-pressed={i === view}
              onClick={() => {
                setAuto(false);
                setView(i);
              }}
            >
              {v.label}
            </button>
          ))}
        </div>
        <button
          className="icon-btn"
          aria-label={auto ? "Pause rotation" : "Resume rotation"}
          aria-pressed={auto}
          onClick={() => setAuto((a) => !a)}
        >
          <RotateCcw size={18} aria-hidden />
        </button>
      </div>
      <Link href="/studio" className={styles.overlayLink} aria-label="Open the fitting studio" tabIndex={-1} />
    </div>
  );
}
