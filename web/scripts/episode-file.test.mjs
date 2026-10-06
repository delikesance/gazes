import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { episodeFile, episodeCandidates, episodeQualities, fileResolution } from '../src/lib/episode-file.ts';
const source={episode_number:3,season_number:2};
const file=(index,path)=>({index,path,is_video:true});
test('ordinal catalog season titles match the real Apothecary Diaries release filename',()=>{
 const identity={episode_number:1,season_number:3,anime_aliases:['The Apothecary Diaries Season 3','Kusuriya no Hitorigoto 3rd Season','薬屋のひとりごと 第3期'],excluded_titles:['Kusuriya no Hitorigoto 2nd Season Mini Anime']};
 const path='[Trix] Kusuriya no Hitorigoto S03E01 [WEBRip 1080p AV1] (Multi Subs).mkv';
 assert.equal(episodeFile([file(0,path)],identity),0);
 for(const wrong of [path.replace('S03E01','S02E01'),path.replace('S03E01','S03E02'),'Kusuriya no Hitorigoto 2nd Season - 01.mkv','Kusuriya no Hitorigoto 2nd Season/01.mkv','Kusuriya no Hitorigoto 2nd Season Mini Anime S03E01.mkv']) {
  assert.equal(episodeFile([file(0,wrong)],identity),null,wrong);
 }
 assert.equal(episodeFile([file(0,'Kusuriya no Hitorigoto 3rd Season - 01.mkv')],identity),0);
});
test('accented sibling titles cannot become original Naruto episodes',()=>{
 const identity={episode_number:1,season_number:1,anime_aliases:['Naruto'],excluded_titles:['Naruto Shippuden','Boruto Naruto Next Generations']};
 for (const path of ['Naruto Shippûden 001.mkv','Boruto - Naruto Next Générations 001.mkv']) assert.equal(episodeFile([file(0,path)],identity),null,path);
 assert.equal(episodeFile([file(0,'Naruto 001.mkv')],identity),0);
});
const narutoPack = JSON.parse(readFileSync(new URL('./fixtures/naruto-dvdrip.json', import.meta.url), 'utf8'));

const tensuraPacks = JSON.parse(readFileSync(new URL('./fixtures/tensura-files.json', import.meta.url), 'utf8'));
test('all 24 Tensura season one episodes select the correct file in real mixed and 10-bit packs',()=>{
 for(let episode=1;episode<=24;episode++) {
  const identity={episode_number:episode,season_number:1,anime_aliases:['That Time I Got Reincarnated as a Slime','Tensei Shitara Slime Datta Ken']};
  for(const [hash,files] of Object.entries(tensuraPacks)) {
   const candidates=episodeCandidates(files,identity);
   assert.equal(candidates.length,1,`${hash} episode ${episode}`);
   assert.equal(candidates[0].index,episode-1,`${hash} episode ${episode}`);
  }
 }
});

test('all 220 Naruto episodes match the real Nyaa DVDRIP file list, excluding its 24 extras',()=>{
 for (let episode=1;episode<=220;episode++) {
  const identity={episode_number:episode,season_number:1,anime_aliases:['Naruto']};
  const expected=narutoPack.files.find(f=>f.path===`Naruto ${String(episode).padStart(3,'0')}.mkv`);
  assert.ok(expected,`fixture episode ${episode}`);
  assert.deepEqual(episodeCandidates(narutoPack.files,identity).map(f=>f.index),[expected.index],`episode ${episode}`);
  assert.equal(episodeFile(narutoPack.files,identity),expected.index,`episode ${episode}`);
 }
 assert.equal(episodeFile(narutoPack.files,{episode_number:221,season_number:1,anime_aliases:['Naruto']}),null);
});

test('combined extra folders and full opening/ending names cannot become episodes',()=>{
 const identity={episode_number:1,season_number:1,anime_aliases:['Naruto']};
 for(const path of ['Opening & Ending/Naruto - EndinG 1.mkv','Opening & Ending/Naruto - OpeninG 1.mkv','OP & ED/Naruto 001.mkv','NCOP + NCED/Naruto 001.mkv','OPED/Naruto 001.mkv','Naruto Ending 1.mkv','Naruto Opening 1.mkv']) {
  assert.equal(episodeFile([file(0,path)],identity),null,path);
 }
});

test('ambiguous same-episode files do not silently select the first candidate',()=>{
 assert.equal(episodeFile([file(0,'Naruto 001 VF.mkv'),file(1,'Naruto 001 VOSTFR.mkv')],{episode_number:1,season_number:1}),null);
});

test('Naruto Yabai and Kai recuts do not use standard TV episode numbering',()=>{
 for(const path of ['[Triggerforce]Naruto Yabaï 01 - L\'épreuve de survie.mkv','Naruto Yabai 01.mkv','Naruto Kai 01.mkv','Naruto FullEdit 01.mkv','Naruto SD 01.mkv','Naruto Spin-Off - Rock Lee 01.mkv']) {
  assert.equal(episodeFile([file(0,path)],{episode_number:1,season_number:1,anime_aliases:['Naruto']}),null,path);
 }
 assert.equal(episodeFile([file(0,'Naruto Kai 01.mkv')],{episode_number:1,season_number:1,anime_aliases:['Naruto Kai']}),0);
});
test('season packs select the requested file rather than the largest',()=>{
 assert.equal(episodeFile([file(0,'Example S02E01.mkv'),file(1,'Example S01E03.mkv'),file(2,'Example S02E03.mkv')],source),2);
});
test('ambiguous and absent episode matches require viewer selection',()=>{
 assert.equal(episodeFile([file(0,'Example - 03 720p.mkv'),file(1,'Example - 03 1080p.mkv')],source),1);
 assert.equal(episodeFile([file(0,'movie.mkv')],source),null);
});
test('absolute numbering and sample exclusion',()=>{
 assert.equal(episodeFile([file(0,'Sample - 27.mkv'),file(1,'Example - 27.mkv')],{...source,absolute_episode:27}),1);
});

test('renamed sequel packs use release season rather than franchise position',()=>{
 assert.equal(episodeFile([file(0,'Naruto Shippuden S02E01.mkv'),file(1,'Naruto Shippuden S01E01.mkv')],{episode_number:1,season_number:1}),1);
});

test('episode 14 is selected automatically from a 500-episode pack',()=>{
 const files=Array.from({length:500},(_,i)=>file(i,`Naruto Shippuden Complete (001-500 + Movies)/[AnimeRG]_Naruto_Shippuuden_${String(i+1).padStart(3,'0')}_1080p.mkv`));
 assert.equal(episodeFile(files,{episode_number:14,season_number:1,anime_aliases:['Naruto Shippuden']}),13);
});
test('mixed packs exclude original Naruto, movies and endings',()=>{
 const source={episode_number:14,season_number:1,anime_aliases:['Naruto Shippuden']};
 assert.equal(episodeFile([file(0,'Naruto/Naruto_014.mkv'),file(1,'Naruto Shippuden/Movies/Naruto Shippuden_014.mkv'),file(2,'Naruto Shippuden/Naruto_Shippuuden_ED_014.mkv'),file(3,'Naruto Shippuden/Naruto_Shippuuden_014.mkv')],source),3);
});
test('filename conventions with dots, parentheses, episode tags and versions',()=>{
 for(const path of ['Naruto.Shippuden.S01E14.mkv','Naruto Shippuden (014).mkv','Naruto Shippuden Ep.014.mkv','Naruto_Shippuuden_014v2_[1080p].mkv']){
  assert.equal(episodeFile([file(7,path)],{episode_number:14,season_number:1}),7,path);
 }
});
test('Steel Ball Run continuation maps local episode one to release episode two',()=>{
 const identity={episode_number:1,season_number:6,tagged_episode:2,absolute_episode:2,anime_aliases:['Steel Ball Run']};
 assert.equal(episodeFile([file(0,'Steel Ball Run S06E01.mkv'),file(1,'Steel Ball Run S06E02.mkv')],identity),1);
 assert.equal(episodeFile([file(0,'Steel Ball Run - 01.mkv'),file(1,'Steel Ball Run - 02.mkv')],identity),1);
});

test('French mixed-season packs select only the main series episode',()=>{
 const source={episode_number:1,season_number:1,is_french:true,anime_aliases:['Tensei Shitara Slime Datta Ken'],excluded_titles:['Tensura Nikki']};
 const root='Tensei Shitara Slime Datta Ken S01+02+OADs+Tensura Nikki/';
 assert.equal(episodeFile([
  file(0,root+'Tensura Nikki Tensei Shitara Slime Datta Ken S01E01.mkv'),
  file(1,root+'Tensei Shitara Slime Datta Ken S02E01.mkv'),
  file(2,root+'OADs/Tensei Shitara Slime Datta Ken 01.mkv'),
  file(3,root+'Tensei Shitara Slime Datta Ken S01E01.mkv')
 ],source),3);
});

test('qualities contain only real variants of the requested episode',()=>{
 const files=[file(7,'Example S02E03 720p.mkv'),file(2,'Example S02E03 1080p.mkv'),file(9,'Example S02E03 1080p duplicate.mkv'),file(3,'Example S02E04 2160p.mkv'),file(4,'Example S01E03 480p.mkv'),file(5,'Example S02E03 original.mkv'),{...file(6,'Example S02E03 2160p.txt'),is_video:false}];
 assert.deepEqual(episodeQualities(files,source).map(({height,file})=>[height,file.index]),[[1080,2],[720,7]]);
 assert.deepEqual(episodeQualities([file(0,'Example S02E03.mkv')],source),[]);
});

test('French markers never allow extras or wrong episodes into playback',()=>{
 const identity={episode_number:1,season_number:1,is_french:true,anime_aliases:['Tensei Shitara Slime Datta Ken'],excluded_titles:['Tensura Nikki']};
 for(const path of ['Tensura Nikki Tensei Shitara Slime Datta Ken S01E01.mkv','Tensei Shitara Slime Datta Ken OVA 01.mkv','OADs/Tensei Shitara Slime Datta Ken 01.mkv','Movies/Tensei Shitara Slime Datta Ken 01.mkv','Tensei Shitara Slime Datta Ken S02E01.mkv','Tensei Shitara Slime Datta Ken S01E02.mkv']){
  assert.deepEqual(episodeCandidates([file(0,path)],identity),[],path);
  assert.equal(episodeFile([file(0,path)],identity),null,path);
 }
});

test('mixed pack root mentioning spin-offs does not exclude the main series',()=>{
 const identity={episode_number:1,season_number:2,is_french:true,excluded_titles:['Tensura Nikki']};
 assert.equal(episodeFile([file(8,'S01+02+OADs+Tensura Nikki/Tensei Shitara Slime Datta Ken S02E01.mkv')],identity),8);
});

test('episode and quality matching handles small episode numbers without confusing bit depth',()=>{
 for(const episode of [1,8,10,12]) {
  assert.equal(episodeFile([file(7,`Example - ${String(episode).padStart(2,'0')} [1080p][x265][10bit][ABCDEF12].mkv`)],{episode_number:episode,season_number:1}),7);
 }
});

test('resolution parsing requires an explicit supported video height',()=>{
 for(const [name,want] of [['Example 2160p.mkv',2160],['Example_1080p_HEVC.mkv',1080],['Example 720p.mkv',720],['Example 1080.mkv',null],['Example 11080p.mkv',null],['Example original.mkv',null]]){
  assert.equal(fileResolution(name),want,name);
 }
});

test('Darki season-plus-extras pack selects TV episodes and excludes OAV/OAD folders',()=>{
 const identity={episode_number:1,season_number:1,anime_aliases:['One Punch Man'],excluded_titles:['One Punch Man OVA']};
 const root='One Punch Man - S01 MULTi [BD 1080p Opus] (Darki)/';
 const files=[file(0,root+'Extras/One Punch Man - S01E01.mkv'),file(1,root+'OAD/One Punch Man - S01E01.mkv'),file(2,root+'OAV/One Punch Man - S01E01.mkv'),file(3,root+'One Punch Man - S01E02 MULTi [BD 1080p Opus] (Darki).mkv'),file(4,root+'One Punch Man - S01E01 MULTi [BD 1080p Opus] (Darki).mkv')];
 assert.deepEqual(episodeCandidates(files,identity).map(f=>f.index),[4]);
 assert.equal(episodeFile(files,identity),4);
});

test('a numeric release-group tag is not mistaken for an episode number',()=>{
 const files=[file(0,'OPED/[224] Death Note - OP - 01 [BDRip.1080p.x265.FLAC].mkv'),file(1,'[224] Death Note - 05 [BDRip.1080p.x265.FLAC].mkv'),file(2,'[224] Death Note - 06 [BDRip.1080p.x265.FLAC].mkv')];
 assert.equal(episodeFile(files,{episode_number:6,season_number:1,anime_aliases:['Death Note']}),2);
});

test('KAMUI single-file releases match despite apostrophes and subtitle-less filenames',()=>{
 const identity={episode_number:1,season_number:1,anime_aliases:["KAMUI ---He's behind you",'Ushiro no Shoumen Kamui-san'],excluded_titles:['Ushiro no Shoumen Kamui-san Mini Anime']};
 for(const path of ['KAMUI.Hes.Behind.You.S01E01.REPACK.1080p.UNCENSORED.AMZN.WEB-DL.JPN.DDP2.0.H.264.MSubs-ToonsHub.mkv','[Judas] Kamui - S01E01.mkv','Ushiro no Shoumen Kamui-san - 01 (WEBRip D-Anime ver 1920x1080 x264 AAC RAW).mp4']) assert.equal(episodeFile([file(0,path)],identity),0,path);
 assert.equal(episodeFile([file(0,'KAMUI.Hes.Behind.You.S01E02.mkv')],identity),null);
});

test('KAMUI batch and encoder releases select episode 1 without audio-channel or resolution tokens',()=>{
 const identity={episode_number:1,season_number:1,anime_aliases:["KAMUI ---He's behind you",'Ushiro no Shoumen Kamui-san'],excluded_titles:[]};
 const batch=[2,1,4].map((n,i)=>file(i,`[Hentai] Ushiro no Shoumen Kamui-san - 0${n}v2 [WEB 1080p DDP 2.0. H 264] (Uncensored).mkv`));
 assert.equal(episodeFile(batch,identity),1);
 for(const path of ['[shincaps] Ushiro no Shoumen Kamui-san - 01 (AT-X 1440x1080 MPEG2 AAC).ts','[LoliHouse] Ushiro no Shoumen Kamui-san - 01 [WebRip 1080p HEVC-10bit AAC SRTx2].mkv']) assert.equal(episodeFile([file(0,path)],identity),0,path);
});

test('CJK episode markers (第19话 / 第19集) are read, and other CJK episodes are rejected',()=>{
 const rezero={episode_number:19,season_number:4,anime_aliases:['Re:Zero kara Hajimeru Isekai Seikatsu','Re:Zero − Starting Life in Another World']};
 const doomdos='[Doomdos] Re:Zero kara Hajimeru Isekai Seikatsu Season 4 - 第19话 - [1080p BILIBILI COM WEB-DL].mkv';
 assert.equal(episodeFile([file(0,doomdos)],rezero),0);
 assert.equal(episodeFile([file(0,doomdos.replace('第19话','第１９話'))],rezero),0);
 assert.equal(episodeFile([file(0,doomdos.replace('第19话','第18话'))],rezero),null);
 const cn='【喵萌奶茶屋】★04月新番★[Re：从零开始的异世界生活 第四季][第19集][1080p][简日双语].mp4';
 const cnSource={...rezero,anime_aliases:['Re:Zero','Re：从零开始的异世界生活']};
 assert.equal(episodeFile([file(0,cn)],cnSource),0);
 assert.equal(episodeFile([file(0,cn.replace('第19集','第18集'))],cnSource),null);
});
test('a one-file unnumbered release of a single-episode special is episode 1',()=>{
 const special={episode_number:1,season_number:1,anime_aliases:['Dragon Ball: Yo! Son Goku and His Friends Return!!'],excluded_titles:['Dragon Ball: Curse of the Blood Rubies']};
 const path='[AWGS] Dragon Ball Yo! Son Goku and His Friends Return!!.mp4';
 assert.equal(episodeFile([file(0,path)],special),0);
 assert.equal(episodeFile([file(0,path)],{...special,episode_number:2}),null);
 assert.equal(episodeFile([file(0,path),file(1,path.replace('.mp4','.sample.mp4'))],special),null);
});
