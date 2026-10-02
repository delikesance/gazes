import { AnimeBrowser } from "@/components/AnimeBrowser";
import { getFranchise, getSeason, getSeasonSources } from "@/lib/api";

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
  let initialSources = null;

  try {
    if (id) {
      initialFranchise = await getFranchise(id);
      if (sId) {
        initialSeason = await getSeason(id, sId);
        if (ep) {
          // Fast timeout for SSR sources so the page shell renders instantly without blocking
          initialSources = await Promise.race([
            getSeasonSources(id, sId, ep),
            new Promise<null>((resolve) => setTimeout(() => resolve(null), 2500)),
          ]).catch(() => null);
        }
      }
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
      initialSources={initialSources}
    />
  );
}
