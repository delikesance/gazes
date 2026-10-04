import { AnimeBrowser } from "@/components/AnimeBrowser";
import { getFranchise, getSeason } from "@/lib/api";

export default async function Page({
  params,
}: {
  params: Promise<{ animeId: string; seasonId: string; episodeNumber: string }>;
}) {
  const { animeId, seasonId, episodeNumber } = await params;
  const id = Number(animeId);
  const sId = Number(seasonId);
  const ep = Number(episodeNumber);
  let initialFranchise = null;
  let initialSeason = null;

  // Sources are resolved by the client (they can take seconds on a cold swarm);
  // awaiting them here would hold the navigation on a blank screen.
  try {
    if (id) {
      [initialFranchise, initialSeason] = await Promise.all([
        getFranchise(id),
        sId ? getSeason(id, sId) : Promise.resolve(null),
      ]);
    }
  } catch (e) {
    console.error("SSR getEpisode error:", e);
  }

  return (
    <AnimeBrowser
      animeId={id}
      seasonId={sId}
      episodeNumber={ep}
      initialFranchise={initialFranchise}
      initialSeason={initialSeason}
    />
  );
}
