import { test } from 'node:test';
import assert from 'node:assert/strict';
import { playbackSources } from '../src/lib/playback-sources.ts';
const source = (id, score, seeders, is_batch=false) => ({id,info_hash:id,score_rank:score,seeders,is_batch});
test('available sources are attempted by descending score, without duplicates',()=>{
 const result=playbackSources([source('dead',100,0),source('low',20,8),source('best',80,2),source('BEST',70,1)]);
 assert.deepEqual(result.map(s=>s.id),['best','low','dead']);
});
test('equal scores and swarm strength prefer an individual episode over a pack',()=>{
 const result=playbackSources([source('pack',50,5,true),source('single',50,5),source('higher-pack',60,5,true)]);
 assert.deepEqual(result.map(s=>s.id),['higher-pack','single','pack']);
});
test('selection never modifies the provider list',()=>{
 const list=[source('low',20,5),source('best',80,5)]; playbackSources(list);
 assert.equal(list[0].id,'low');
});
