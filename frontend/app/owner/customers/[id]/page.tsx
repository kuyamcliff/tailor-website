import { OwnerCustomer } from "@/features/owner/customers";

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  return <OwnerCustomer id={(await params).id} />;
}
