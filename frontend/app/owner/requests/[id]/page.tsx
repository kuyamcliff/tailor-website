import { OwnerRequestDetail } from "@/features/owner/request-detail";

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  return <OwnerRequestDetail id={(await params).id} />;
}
