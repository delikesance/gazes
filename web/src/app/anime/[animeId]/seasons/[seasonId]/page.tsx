import { AnimeBrowser } from "@/components/AnimeBrowser";
import { getFranchise, getSeason } from "@/lib/api";

export default async function Page({
  params,
}: {
  params: Promise<{ animeId: string; seasonId: string }>;
}) {
  const { animeId, seasonId } = await params;
  const id = Number(animeId);
  const sId = Number(seasonId);
  let initialFranchise = null;
  let initialSeason = null;
  try {
    if (id) {
      initialFranchise = await getFranchise(id);
      if (sId) initialSeason = await getSeason(id, sId);
    }
  } catch (e) {
    console.error("SSR getSeason error:", e);
  }
  return <AnimeBrowser animeId={id} seasonId={sId} initialFranchise={initialFranchise} initialSeason={initialSeason} />;
}
