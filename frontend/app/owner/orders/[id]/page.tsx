import { OwnerOrderDetail } from "@/features/owner/orders";

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  return <OwnerOrderDetail id={(await params).id} />;
}
