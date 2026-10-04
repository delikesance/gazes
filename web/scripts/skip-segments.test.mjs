import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mergeSkipSegments, activeSkipSegment, needsAniSkip, skipAction } from '../src/lib/skip-segments.ts';
const seg=(kind,start,end,source='chapters')=>({kind,start,end,source});
test('merge keeps chapter segments and lets AniSkip fill only missing kinds, sorted by start',()=>{
 const merged=mergeSkipSegments([seg('opening',60,150)],[seg('opening',70,160,'aniskip'),seg('ending',1300,1400,'aniskip')]);
 assert.deepEqual(merged,[seg('opening',60,150),seg('ending',1300,1400,'aniskip')]);
 assert.deepEqual(mergeSkipSegments([seg('ending',1300,1400)],[seg('opening',60,150,'aniskip')]).map((s)=>s.kind),['opening','ending']);
 assert.deepEqual(mergeSkipSegments([],[]),[]);
});
test('active segment covers start up to the last second before end',()=>{
 const s=[seg('opening',120,210)];
 for(const p of [120,150,208.99]) assert.equal(activeSkipSegment(s,p),s[0],String(p));
 for(const p of [119.9,209,210,300]) assert.equal(activeSkipSegment(s,p),null,String(p));
});
test('AniSkip is needed while either kind is missing',()=>{
 assert.equal(needsAniSkip([]),true);
 assert.equal(needsAniSkip([seg('opening',1,2)]),true);
 assert.equal(needsAniSkip([seg('opening',1,2),seg('ending',3,4)]),false);
});
test('only a closing ending with a next episode jumps to it',()=>{
 const end=seg('ending',1330,1420);
 assert.equal(skipAction(end,1422,true),'next-episode');
 assert.equal(skipAction(end,1422,false),'seek');
 assert.equal(skipAction(seg('ending',1290,1380),1420,true),'seek');
 assert.equal(skipAction(seg('opening',1330,1420),1422,true),'seek');
});
