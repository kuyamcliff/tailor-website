import { OwnerGarment } from "@/features/owner/garments";

export default async function Page({ params }: { params: Promise<{ key: string }> }) {
  return <OwnerGarment garmentKey={(await params).key} />;
}
