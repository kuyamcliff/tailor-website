import { siFacebook, siInstagram, siTiktok, siWhatsapp, siX, siYoutube } from "simple-icons";
import type { SocialLink } from "@/lib/types";

// Social marks are rendered from the official Simple Icons path data (see public/brand/SOURCES.md),
// never approximated with glyphs or substitute icons.
const icons = {
  whatsapp: siWhatsapp,
  instagram: siInstagram,
  tiktok: siTiktok,
  facebook: siFacebook,
  youtube: siYoutube,
  x: siX,
} as const;

export function SocialIcon({
  network,
  size = 20,
  brandColor = false,
}: {
  network: SocialLink["network"];
  size?: number;
  brandColor?: boolean;
}) {
  const icon = icons[network];
  return (
    <svg
      role="img"
      viewBox="0 0 24 24"
      width={size}
      height={size}
      aria-hidden="true"
      fill={brandColor ? `#${icon.hex}` : "currentColor"}
    >
      <path d={icon.path} />
    </svg>
  );
}

export const socialTitle: Record<SocialLink["network"], string> = {
  whatsapp: "WhatsApp",
  instagram: "Instagram",
  tiktok: "TikTok",
  facebook: "Facebook",
  youtube: "YouTube",
  x: "X",
};

export function PaymentMark({ provider, size = 32 }: { provider: string; size?: number }) {
  if (provider !== "mtn" && provider !== "orange") return null;
  return (
    // eslint-disable-next-line @next/next/no-img-element -- small static SVG brand mark
    <img
      src={`/brand/payments/${provider}.svg`}
      width={size}
      height={size}
      alt=""
      aria-hidden="true"
      style={{ borderRadius: 3 }}
    />
  );
}

export function whatsappLink(number: string, text?: string) {
  const digits = number.replace(/\D/g, "");
  return `https://wa.me/${digits}${text ? `?text=${encodeURIComponent(text)}` : ""}`;
}
