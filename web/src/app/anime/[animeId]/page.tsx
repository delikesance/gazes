import { AnimeBrowser } from "@/components/AnimeBrowser";
import { getFranchise } from "@/lib/api";

export default async function Page({
  params,
}: {
  params: Promise<{ animeId: string }>;
}) {
  const { animeId } = await params;
  const id = Number(animeId);
  let initialFranchise = null;
  try {
    if (id) initialFranchise = await getFranchise(id);
  } catch (e) {
    console.error("SSR getFranchise error:", e);
  }
  return <AnimeBrowser animeId={id} initialFranchise={initialFranchise} />;
}
