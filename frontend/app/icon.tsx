import { ImageResponse } from "next/og";
import { businessName, getConfig } from "@/lib/server-data";

export const size = { width: 64, height: 64 };
export const contentType = "image/png";

// Until the owner uploads a logo, the icon is the business initial set in type (no invented artwork).
export default async function Icon() {
  const initial =
    businessName(await getConfig())
      .replace(/^the\s+/i, "")
      .charAt(0)
      .toUpperCase() || "A";
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#0b0b0c",
        color: "#c2a165",
        fontSize: 44,
        fontFamily: "serif",
      }}
    >
      {initial}
    </div>,
    size,
  );
}
