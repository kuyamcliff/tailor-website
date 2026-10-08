"use client";

import { useEffect, useRef } from "react";

// Reveal fades content in as it scrolls into view. Server-rendered content is visible by default;
// only elements that start below the fold are hidden after hydration, and reduced-motion users are
// never animated.
export function Reveal({
  children,
  as: Tag = "div",
  className = "",
  delay = 0,
}: {
  children: React.ReactNode;
  as?: "div" | "section" | "li" | "article";
  className?: string;
  delay?: number;
}) {
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el || !("IntersectionObserver" in window) || window.matchMedia("(prefers-reduced-motion: reduce)").matches)
      return;
    if (el.getBoundingClientRect().top < window.innerHeight) return; // already on screen
    el.classList.add("reveal-pending");
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          el.classList.remove("reveal-pending");
          io.disconnect();
        }
      },
      { rootMargin: "0px 0px -6% 0px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);
  return (
    <Tag
      ref={ref as never}
      className={`reveal ${className}`}
      style={delay ? { transitionDelay: `${delay}ms` } : undefined}
    >
      {children}
    </Tag>
  );
}
