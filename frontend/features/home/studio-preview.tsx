"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import styles from "./studio-preview.module.css";

// A lightweight preview of the fitting studio on the home page: pre-rendered frames of the studio's
// own 3D model at the standard view angles. The full WebGL studio only loads on /studio.
const views = [
  { key: "front", label: "Front" },
  { key: "45", label: "45°" },
  { key: "side", label: "Side" },
  { key: "135", label: "135°" },
  { key: "back", label: "Back" },
];

export function StudioPreview({ renders }: { renders: Record<string, string> }) {
  const [view, setView] = useState(0);
  return (
    <div className={styles.frame}>
      <div className={styles.stage}>
        {views.map((v, i) => (
          <Image
            unoptimized
            key={v.key}
            src={renders[v.key] ?? `/3d/renders/suit-${v.key}.webp`}
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
            <button key={v.key} className={styles.view} aria-pressed={i === view} onClick={() => setView(i)}>
              {v.label}
            </button>
          ))}
        </div>
      </div>
      <Link href="/studio" className={styles.overlayLink} aria-label="Open the fitting studio" tabIndex={-1} />
    </div>
  );
}
