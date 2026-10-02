import { test } from 'node:test';
import assert from 'node:assert/strict';
import { playbackSources, extendPlaybackSources } from '../src/lib/playback-sources.ts';
const source = (id, score, seeders, is_batch=false) => ({id,info_hash:id,score_rank:score,seeders,is_batch});
test('available sources are attempted by descending score, without duplicates',()=>{
 const result=playbackSources([source('dead',100,0),source('low',20,8),source('best',80,2),source('BEST',70,1)]);
 assert.deepEqual(result.map(s=>s.id),['best','low','dead']);
});
test('season packs are attempted before individual episodes',()=>{
 const result=playbackSources([source('pack',50,5,true),source('single',50,5),source('higher-pack',60,5,true)]);
 assert.deepEqual(result.map(s=>s.id),['higher-pack','pack','single']);
});
test('selection never modifies the provider list',()=>{
 const list=[source('low',20,5),source('best',80,5)]; playbackSources(list);
 assert.equal(list[0].id,'low');
});
test('VF takes priority over VOSTFR packs, and quality falls back within a format',()=>{
 const vf=(id,batch,quality,score)=>({...source(id,score,2,batch),language_tag:'VF',score_breakdown:{french:200,quality}});
 const sub={...source('sub-pack',164,100,true),language_tag:'VOSTFR',score_breakdown:{french:100,quality:4}};
 assert.deepEqual(playbackSources([vf('vf-single',false,4,264),sub,vf('vf-pack-sd',true,1,261),vf('vf-pack-hd',true,3,213)]).map(s=>s.id),['vf-pack-hd','vf-pack-sd','vf-single','sub-pack']);
});
test('full discovery adds fallbacks without changing the active source or retrying failed ones',()=>{
 const current=[source('failed',100,2),source('active',80,2),source('pending',20,2)];
 const full=[source('new',70,3),source('FAILED',110,3),source('active',80,2)];
 assert.deepEqual(extendPlaybackSources(current,full,1).map(s=>s.id),['failed','active','new','pending']);
 assert.deepEqual(extendPlaybackSources(current,full,3).map(s=>s.id),['failed','active','pending','new']);
});
