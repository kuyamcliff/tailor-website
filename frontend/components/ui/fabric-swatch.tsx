import Image from "next/image";
import type { Fabric } from "@/lib/types";

// FabricSwatch shows the fabric texture; for fabrics whose texture is tinted per colour, the chosen
// colour is multiplied over it exactly as the 3D material does.
export function FabricSwatch({ fabric, colorHex, sizes = "120px", round = true }: { fabric: Pick<Fabric, "name" | "swatchUrl" | "pbr" | "colors">; colorHex?: string; sizes?: string; round?: boolean }) {
  const tint = fabric.pbr?.tint ? (colorHex ?? fabric.colors[0]?.hex) : undefined;
  return (
    <span style={{ position: "absolute", inset: 0, borderRadius: round ? "50%" : 0, overflow: "hidden", background: colorHex ?? "var(--surface)" }}>
      {fabric.swatchUrl ? <Image src={fabric.swatchUrl} alt={`${fabric.name} swatch`} fill sizes={sizes} style={{ objectFit: "cover" }} /> : null}
      {tint ? <span aria-hidden style={{ position: "absolute", inset: 0, background: tint, mixBlendMode: "multiply" }} /> : null}
    </span>
  );
}
