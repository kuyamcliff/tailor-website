import { OwnerQuoteEditor } from "@/features/owner/quotes";

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  return <OwnerQuoteEditor id={(await params).id} />;
}
