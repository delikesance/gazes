`subtitle-window.mkv` is a synthetic three-minute, 64×64 video with PGS and
SubRip tracks. Both tracks contain cues around 70 and 150 seconds, so extracting
60–100 seconds must retain the original 70-second timestamp and omit the later cue.
The video was generated with FFmpeg; it contains no anime footage.
The PGS sample is adapted from `libpgs/tests/files/test.sup` (MIT, David Schulte);
see `libpgs-LICENSE`. Text cues are generated locally.
