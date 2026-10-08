import Link from "next/link";
import styles from "./wordmark.module.css";

// The site identity is the owner's uploaded logo or, until one is configured, the business name set
// in type. No invented logo artwork is shipped.
export function Wordmark({ name, logoUrl, href = "/" }: { name: string; logoUrl?: string; href?: string }) {
  return (
    <Link href={href} className={styles.mark} aria-label={`${name}, home`}>
      {logoUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={logoUrl} alt="" className={styles.logo} />
      ) : (
        <span className={styles.text}>{name}</span>
      )}
    </Link>
  );
}
