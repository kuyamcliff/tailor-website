import { OwnerProductEditor } from "@/features/owner/products";

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <OwnerProductEditor id={id === "new" ? null : id} />;
}
