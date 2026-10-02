import { test } from 'node:test';
import assert from 'node:assert/strict';
import { episodeFile, episodeCandidates, episodeQualities, fileResolution } from '../src/lib/episode-file.ts';
const source={episode_number:3,season_number:2};
const file=(index,path)=>({index,path,is_video:true});
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
