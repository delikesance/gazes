import type { AudioTrack, SubtitleTrack, VideoMetadata } from '../types/api';

/** Track titles only provide evidence when the language tag does not contradict them. */
export function frenchAudioEvidence(track: AudioTrack): 'confirmed' | 'unknown' | 'other' {
 const code = trackLanguageCode(track.language || '');
 if (code === 'fr') return 'confirmed';
 const frenchTitle = /\b(?:french|fran[cç]ais|vf)\b/i.test(track.title || '');
 if (!code || code === 'und') return frenchTitle ? 'confirmed' : 'unknown';
 return frenchTitle ? 'unknown' : 'other';
}

export function vfStatus(meta: VideoMetadata): 'confirmed' | 'absent' | 'unknown' | 'inaccessible' {
 if (meta.probe_status !== 'complete') return 'inaccessible';
 const evidence = (meta.audio_tracks || []).map(frenchAudioEvidence);
 if (evidence.includes('confirmed')) return 'confirmed';
 if (evidence.includes('unknown')) return 'unknown';
 return 'absent';
}

export function preferredAudioTrack(tracks: AudioTrack[], current: number, manual: boolean): number {
 if (manual) return current;
 return tracks.find(track => frenchAudioEvidence(track) === 'confirmed')?.index ?? current;
}

const languageCodes: Record<string, string> = {
  fre: 'fr', fra: 'fr', eng: 'en', jpn: 'ja', por: 'pt', spa: 'es', ara: 'ar',
  ita: 'it', rus: 'ru', ind: 'id', may: 'ms', msa: 'ms', vie: 'vi', tha: 'th',
  chi: 'zh', zho: 'zh', ger: 'de', deu: 'de', kor: 'ko', dut: 'nl', nld: 'nl',
};
export function trackLanguageCode(language: string): string {
  const code = language.trim().toLowerCase();
  return languageCodes[code] || code;
}

export function mediaTrackLabel(track: AudioTrack | SubtitleTrack, tracks: (AudioTrack | SubtitleTrack)[], locale: 'fr' | 'en'): string {
  let code = trackLanguageCode(track.language);
  if (code === 'pt' && /brazil/i.test(track.title)) code = 'pt-BR';
  if (code === 'es' && /latin.?america/i.test(track.title)) code = 'es-419';
  if (code === 'zh' && /simplified/i.test(track.title)) code = 'zh-Hans';
  if (code === 'zh' && /traditional/i.test(track.title)) code = 'zh-Hant';
  let language = locale === 'fr' ? 'Langue inconnue' : 'Unknown language';
  if (code && code !== 'und') {
    try {
      language = new Intl.DisplayNames([locale], { type: 'language' }).of(code) || language;
    } catch { /* Keep the unknown-language label for invalid metadata. */ }
  }
  language = language.charAt(0).toUpperCase() + language.slice(1);
  const details: string[] = [];
  if ('channels' in track) {
    if (track.codec) details.push(track.codec.toUpperCase());
    if (track.channels > 0) details.push(`${track.channels} ${locale === 'fr' ? 'canaux' : 'channels'}`);
  } else if (track.is_forced) {
    details.push(locale === 'fr' ? 'Forcés' : 'Forced');
  }
  // Preserve separate tracks when the file contains multiple versions of a language.
  if (tracks.filter(other => trackLanguageCode(other.language) === trackLanguageCode(track.language)).length > 1) {
    details.push(`${locale === 'fr' ? 'Piste' : 'Track'} ${track.index + 1}`);
  }
  return [language, ...details].join(' · ');
}
