"use client";

import Image from "next/image";
import { useState } from "react";
import type { Media } from "@/lib/types";
import styles from "./gallery.module.css";
import { SampleTag } from "@/components/ui/sample-tag";

export function Gallery({ media, name }: { media: Media[]; name: string }) {
  const [active, setActive] = useState(0);
  const current = media[active];
  if (!current) return <div className={styles.main} aria-label={`No photos of ${name} yet`} />;
  return (
    <div className={styles.gallery}>
      <div className={styles.main}>
        <Image
          src={current.url}
          alt={current.alt}
          fill
          priority
          sizes="(max-width: 960px) 100vw, 55vw"
          className={styles.img}
        />
        <SampleTag show={current.sample} />
      </div>
      {media.length > 1 ? (
        <div className={styles.thumbs} role="group" aria-label="Photos">
          {media.map((m, i) => (
            <button
              key={m.id}
              className={styles.thumb}
              aria-pressed={i === active}
              aria-label={`Photo ${i + 1} of ${media.length}: ${m.alt}`}
              onClick={() => setActive(i)}
            >
              <Image src={m.url} alt="" fill sizes="96px" style={{ objectFit: "cover" }} />
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
