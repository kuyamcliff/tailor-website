import { OwnerSupportThread } from "@/features/owner/support";

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  return <OwnerSupportThread id={(await params).id} />;
}
