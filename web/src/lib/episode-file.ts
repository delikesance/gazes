import type { EpisodeSource, FileInfo } from "../types/api";

function normalized(value: string): string {
 return value.normalize('NFKD').replace(/(\p{Script=Latin})\p{M}+/gu,'$1').normalize('NFC').toLowerCase().replace(/['’`´]/g,"").replace(/uu/g,"u").replace(/\b(?:season|saison|part|cour)\s*\d+\b/gi," ").replace(/[^\p{L}\p{N}]+/gu," ").trim();
}

/** Avoid original-series files in mixed packs while accepting numbered-only filenames. */
function matchesSeries(path: string, source: EpisodeSource): boolean {
 const aliases = source.anime_aliases?.length ? source.anime_aliases : source.anime_title ? [source.anime_title] : [];
 // Catalog aliases use "3rd Season" while releases often use S03E01.
 // Compare the series title here; episodeCandidates checks the season separately.
 const seriesKey = (value: string) => normalized(value.replace(/\b\d+(?:st|nd|rd|th)\s+season\b/gi, " "));
 // Releases often drop a catalog subtitle ("KAMUI ---He's behind you" -> "Kamui - S01E01").
 const withHeads = aliases.flatMap(alias => [alias, alias.split(/\s*(?:-{2,}|[:：]|\s-\s)/)[0]]);
 const keys = [...new Set(withHeads.map(seriesKey).filter(Boolean))];
 if (!keys.length) return true;
 const firstWords = keys.map(key=>key.split(" ")[0]).filter(word=>word.length>=3);
 for (const segment of path.replace(/\\/g,"/").split("/").reverse()) {
  const key = ` ${seriesKey(segment)} `;
  if (firstWords.some(word=>key.includes(` ${word} `))) {
   return keys.some(alias=>key.includes(` ${alias} `));
  }
 }
 return true;
}

/** Pure filename matching: no media download or probe is needed to select an episode. */
export function episodeCandidates(files: FileInfo[], source: EpisodeSource): FileInfo[] {
 const seasonNum = source.season_number || 1;
 const epNum = source.tagged_episode || source.episode_number;
 const absNum = source.absolute_episode || source.episode_number;

 return files.filter(file => {
  if (!file.is_video) return false;
  const path = file.path.replace(/[_.]/g," ");
  const name = (file.path.replace(/\\/g,"/").split("/").pop() || file.path).replace(/[_.]/g," ");
	  const rawName = file.path.replace(/\\/g,"/").split("/").pop() || file.path;
	  // Decimal episodes are recaps/specials, not the preceding integer episode.
	  if (/\bS\d+E\d+\.\d+\b/i.test(rawName)) return false;

  const normalizedName=` ${normalized(name)} `;
  // Naruto recuts and spin-offs do not share the catalog's TV episode numbering.
  if (/\bnaruto\s+(?:shippuden\s+)?(?:yaba[iï]|kai|full\s*edit|sd|spin\s*off)(?=\s|$)/i.test(normalized(file.path)) &&
      !(source.anime_aliases?.length ? source.anime_aliases : [source.anime_title || ""]).some(alias => /\b(?:yaba[iï]|kai|full\s*edit|sd|spin\s*off)(?=\s|$)/i.test(normalized(alias)))) return false;
  if(source.excluded_titles?.some(alias=>{const key=normalized(alias);return key&&normalizedName.includes(` ${key} `);}))return false;

  // Reject OVA / OAD / Special files and folders unless target is explicitly an OVA
  if (!("is_ova" in source && Boolean(source.is_ova)) &&
      (/\b(oad|ova|oav|sp|special|specials|ncop|nced|op|ed|openings?|endings?|oped|ost|sample|trailer|bonus|extra)\b/i.test(name) ||
	       /\b(?:S\d+)?(?:OAD|OVA|OAV)\d+\b/i.test(name) ||
	       path.replace(/\\/g,"/").split("/").slice(0,-1).some(segment => /^(?:oads?|ovas?|oavs?|movies?|films?|extras?|bonus)$/.test(normalized(segment).replace(/^\d+\s+/,""))) ||
       path.replace(/\\/g,"/").split("/").slice(0,-1).some(segment => /^(?:(?:openings?|endings?|op|ed|ncop|nced|oped|ost|soundtrack|extras?|bonus|samples?|trailers?)\s*)+(?:\d+)?$/i.test(normalized(segment))) ||
       /(?:^|[/\\])(?:oads?|ovas?|oavs?|movies?|films?|extras|bonus|openings?|endings?|ost|nc)(?:[/\\]|$)/i.test(path))) {
    return false;
  }

  // Reject movies if not standalone
  if (!("standalone" in source && Boolean(source.standalone)) && (/\b(movies?|films?|gekijouban)\b/i.test(name) || /(?:^|[/\\])(?:movies?|films?)(?:[/\\]|$)/i.test(path))) {
    return false;
  }

  if (!matchesSeries(file.path, source)) {
    return false;
  }

  // 1. Tagged SxxExx or SxEE
  const tagged = name.match(/\bS0*(\d+)\s*E0*(\d+)(?:v\d+)?\b/i);
  if (tagged) {
    return Number(tagged[1]) === seasonNum && (Number(tagged[2]) === epNum || Number(tagged[2]) === absNum);
  }

  // 2. Scene format: 1x02, 01x02
  const scene = name.replace(/\b\d{3,4}x\d{3,4}\b/gi," ").match(/\b0*(\d+)x0*(\d+)(?:v\d+)?\b/i);
  if (scene) {
    return Number(scene[1]) === seasonNum && (Number(scene[2]) === epNum || Number(scene[2]) === absNum);
  }

  // Directory season check: if in "Season 02/..." but looking for Season 1, reject
  const directorySeason = path.match(/\b(?:S|season|saison)\s*0*(\d+)\b|\b0*(\d+)(?:st|nd|rd|th)\s+season\b/i);
  if (directorySeason && Number(directorySeason[1] || directorySeason[2]) !== seasonNum) {
    return false;
  }

  // 3. Explicit "Episode 02" or "EP 02" or "E02"
  const explicit = name.match(/\b(?:EP?|episode)\s*[- ]?\s*0*(\d+)(?:v\d+)?\b/i);
  if (explicit) {
    const n = Number(explicit[1]);
    return n === epNum || n === absNum;
  }

  // 3b. Explicit CJK markers: 第19话 / 第19話 / 第19集 / 第19回 (full-width digits allowed)
  const cjk = name.replace(/[０-９]/g, digit => String(digit.charCodeAt(0) - 0xff10)).match(/第\s*0*(\d+)\s*[话話集回]/);
  if (cjk) {
    const n = Number(cjk[1]);
    return n === epNum || n === absNum;
  }

  // 4. Pre-clean CRC, resolution, codec, and bit-depth tokens before bare number matching
  const cleanedName = name
    .replace(/^\s*\[[^\]]*\]/, " ") // leading release-group tag: a numeric group such as "[224]" is not an episode
    .replace(/\[[0-9A-Fa-f]{8}\]/g, " ")
	    .replace(/\b(?:8|10|12)[ -]?bits?\b/gi, " ")
    .replace(/\b\d{3,4}x\d{3,4}\b/gi, " ") // WxH resolution
    .replace(/\b(?:ddp?|aac|e?ac3|opus|flac|dts(?: hd)?|truehd)\s*\d\s\d\b/gi, " ") // audio channels "DDP 2.0" (dots already spaced)
    .replace(/\b(10bit|8bit|12bit|x264|x265|h264|h265|hevc|avc|2160p|1080p|810p|720p|576p|480p|360p|4k|aac|flac|dts|ac3)\b/gi, " ");

  const candidates = [...cleanedName.matchAll(/(?:^|[\s\-\[(])0*(\d{1,4})(?:v\d+)?(?=$|[\s\[\(\)\]-])/gi)]
    .map(match => Number(match[1]))
    .filter(number => ![360, 480, 576, 720, 810, 1080, 2160, 264, 265].includes(number) && (number < 1900 || number > 2099));

  // A lone unnumbered video is a single-episode release (special/OVA-length TV entry): it is episode 1.
  if (!candidates.length) return epNum === 1 && files.filter(f => f.is_video).length === 1;
  return candidates.length === 1 && (candidates[0] === epNum || candidates[0] === absNum);
 });
}

export function episodeFile(files: FileInfo[], source: EpisodeSource): number | null {
 const matches = episodeCandidates(files, source);
 console.log(`[Gazes:EpisodeMatcher] Searching file for Season ${source.season_number || 1} Ep ${source.episode_number}: found ${matches.length} matches out of ${files.length} files`);
 if (matches.length === 1) {
  console.log(`[Gazes:EpisodeMatcher] -> Selected exact match [${matches[0].index}]: ${matches[0].path}`);
  return matches[0].index;
 }
 const heights = matches.map(file => fileResolution(file.path));
 if (matches.length > 1 && heights.every(Boolean) && new Set(heights).size === matches.length) {
  const best = [...matches].sort((a,b) => (fileResolution(b.path) || 0) - (fileResolution(a.path) || 0))[0];
  console.log(`[Gazes:EpisodeMatcher] -> Selected highest resolution variant [${best.index}]: ${best.path}`);
  return best.index;
 }
 if (matches.length > 1) console.log(`[Gazes:EpisodeMatcher] -> Ambiguous episode; viewer selection required`);
 return null;
}

export function fileResolution(path: string): number | null {
 const match = path.match(/(?:^|[^\d])(2160|1080|720|480|360)p(?:\b|_)/i);
 return match ? Number(match[1]) : null;
}

/** Only encoded variants of this episode; no upscaling or invented renditions. */
export function episodeQualities(files: FileInfo[], source: EpisodeSource): {height:number; file:FileInfo}[] {
 const unique = new Map<number,FileInfo>();
 for (const file of episodeCandidates(files,source)) {
  const height = fileResolution(file.path);
  if (height && !unique.has(height)) unique.set(height,file);
 }
 return [...unique].map(([height,file])=>({height,file})).sort((a,b)=>b.height-a.height);
}
