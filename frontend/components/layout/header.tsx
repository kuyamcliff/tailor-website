"use client";

import { useHydrated } from "@/lib/client-hooks";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Heart, Menu, Search, ShoppingBag, User } from "lucide-react";
import { Wordmark } from "@/components/brand/wordmark";
import { Sheet } from "@/components/ui/sheet";
import { useCart, cartCount } from "@/stores/cart";
import { useSaved } from "@/stores/saved";
import { useSession } from "@/components/providers/session";
import { useConfig } from "@/components/providers/config";
import { SocialIcon, socialTitle } from "@/components/brand/brand-icon";
import styles from "./header.module.css";

export const navItems = [
  { href: "/", label: "Home" },
  { href: "/shop", label: "Shop" },
  { href: "/custom-tailor", label: "Custom Tailor" },
  { href: "/our-work", label: "Our Work" },
  { href: "/about", label: "About" },
  { href: "/contact", label: "Contact" },
];

export function Header() {
  const cfg = useConfig();
  const pathname = usePathname();
  const router = useRouter();
  const hydrated = useHydrated();
  const lines = useCart((s) => s.lines);
  const setCartOpen = useCart((s) => s.setOpen);
  const wishCount = useSaved((s) => s.wishlist.length);
  const { user } = useSession();
  // Overlays remember the path they were opened on, so navigating closes them without an effect.
  const [menuAt, setMenuAt] = useState<string | null>(null);
  const [searchAt, setSearchAt] = useState<string | null>(null);
  const menu = menuAt === pathname;
  const search = searchAt === pathname;
  const setMenu = (open: boolean) => setMenuAt(open ? pathname : null);
  const setSearch = (open: boolean) => setSearchAt(open ? pathname : null);
  const [q, setQ] = useState("");
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 8);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  const count = hydrated ? cartCount(lines) : 0;
  const accountHref = user ? (user.isStaff ? "/owner" : "/account") : "/account/sign-in";
  const isActive = (href: string) => (href === "/" ? pathname === "/" : pathname.startsWith(href));
  const name = cfg.business.name || "Atelier";

  return (
    <>
      <header className={`${styles.header} ${scrolled ? styles.scrolled : ""}`}>
        <div className={`container ${styles.bar}`}>
          <Wordmark name={name} logoUrl={cfg.business.logoUrl} />
          <nav className={styles.nav} aria-label="Main">
            {navItems.map((n) => (
              <Link
                key={n.href}
                href={n.href}
                className={styles.navLink}
                aria-current={isActive(n.href) ? "page" : undefined}
              >
                {n.label}
              </Link>
            ))}
          </nav>
          <div className={styles.actions}>
            <button className="icon-btn" onClick={() => setSearch(true)} aria-label="Search">
              <Search size={20} aria-hidden />
            </button>
            <Link href={accountHref} className="icon-btn" aria-label={user ? "Your account" : "Sign in"}>
              <User size={20} aria-hidden />
              {user ? <span className={styles.signedIn} aria-hidden /> : null}
            </Link>
            <Link
              href="/wishlist"
              className={`icon-btn ${styles.desktopOnly}`}
              aria-label={`Wishlist${hydrated && wishCount ? `, ${wishCount} items` : ""}`}
            >
              <Heart size={20} aria-hidden />
              {hydrated && wishCount > 0 ? <span className={styles.count}>{wishCount}</span> : null}
            </Link>
            <button
              className="icon-btn"
              onClick={() => setCartOpen(true)}
              aria-label={`Bag${count ? `, ${count} items` : ", empty"}`}
            >
              <ShoppingBag size={20} aria-hidden />
              {count > 0 ? <span className={styles.count}>{count}</span> : null}
            </button>
            <button
              className={`icon-btn ${styles.menuBtn}`}
              onClick={() => setMenu(true)}
              aria-label="Open menu"
              aria-expanded={menu}
            >
              <Menu size={22} aria-hidden />
            </button>
          </div>
        </div>
      </header>

      <Sheet open={menu} onClose={() => setMenu(false)} title="Menu" side="right" hideTitle>
        <nav aria-label="Mobile" className={styles.mobileNav}>
          {navItems.map((n) => (
            <Link
              key={n.href}
              href={n.href}
              className={styles.mobileLink}
              aria-current={isActive(n.href) ? "page" : undefined}
            >
              {n.label}
            </Link>
          ))}
        </nav>
        <div className={styles.mobileExtra}>
          <Link href="/studio" className="btn btn-primary btn-block">
            Open the fitting studio
          </Link>
          <Link href="/appointments" className="btn btn-block">
            Book a consultation
          </Link>
          <div className={styles.mobileLinks}>
            <Link href="/wishlist">Wishlist</Link>
            <Link href="/compare">Compare</Link>
            <Link href={accountHref}>{user ? "Account" : "Sign in"}</Link>
            <Link href="/support">Help</Link>
          </div>
          {cfg.business.social?.length ? (
            <div className="row-wrap">
              {cfg.business.social.map((s) => (
                <a
                  key={s.network}
                  href={s.url}
                  className="icon-btn"
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label={socialTitle[s.network]}
                >
                  <SocialIcon network={s.network} />
                </a>
              ))}
            </div>
          ) : null}
        </div>
      </Sheet>

      <Sheet open={search} onClose={() => setSearch(false)} title="Search" side="center">
        <form
          role="search"
          onSubmit={(e) => {
            e.preventDefault();
            router.push(`/shop?q=${encodeURIComponent(q.trim())}`);
          }}
          className="stack"
        >
          <label htmlFor="site-search" className="label">
            Search garments and fabrics
          </label>
          <div className="input-group">
            <input
              id="site-search"
              className="input"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Navy suit, linen shirt..."
              autoComplete="off"
            />
            <button className="btn btn-primary" type="submit">
              Search
            </button>
          </div>
          <div className="row-wrap small muted">
            <span>Popular:</span>
            {["Suit", "Shirt", "Linen", "Wedding"].map((t) => (
              <Link key={t} href={`/shop?q=${encodeURIComponent(t)}`} className="badge">
                {t}
              </Link>
            ))}
          </div>
        </form>
      </Sheet>
    </>
  );
}
