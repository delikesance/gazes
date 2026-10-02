import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { frenchAudioEvidence, vfStatus, preferredAudioTrack } from '../src/lib/media-tracks.ts';
const audio=(index,language,title='')=>({index,language,title});
test('French codes and untagged explicit labels confirm audio, contradictory tags do not',()=>{
 for(const code of ['fr','fre','fra','FRA']) assert.equal(frenchAudioEvidence(audio(1,code)),'confirmed');
 for(const title of ['French','Français','VF','Audio Français']) assert.equal(frenchAudioEvidence(audio(1,'und',title)),'confirmed');
 assert.equal(frenchAudioEvidence(audio(1,'jpn','VF')),'unknown');
 assert.equal(frenchAudioEvidence(audio(1,'und','')),'unknown');
 assert.equal(frenchAudioEvidence(audio(1,'eng','VFR')),'other');
});
test('empty failed probes never prove that VF is absent; French subtitles do not confirm audio',()=>{
 assert.equal(vfStatus({probe_status:'timeout',audio_tracks:[]}),'inaccessible');
 assert.equal(vfStatus({probe_status:'failed',audio_tracks:[]}),'inaccessible');
 assert.equal(vfStatus({probe_status:'complete',audio_tracks:[audio(0,'jpn')],subtitle_tracks:[{language:'fre'}]}),'absent');
 assert.equal(vfStatus({probe_status:'complete',audio_tracks:[audio(0,'und')]}),'unknown');
 assert.equal(vfStatus({probe_status:'complete',audio_tracks:[audio(0,'jpn'),audio(1,'fre')]}),'confirmed');
});
test('late metadata chooses French audio after Japanese default and preserves a manual selection',()=>{
 assert.equal(preferredAudioTrack([],0,false),0);
 const tracks=[audio(0,'jpn'),audio(1,'fre')];
 assert.equal(preferredAudioTrack(tracks,0,false),1);
 assert.equal(preferredAudioTrack(tracks,0,true),0);
 assert.equal(preferredAudioTrack(tracks,1,false),1);
});

test('five real Naruto episode captures contain confirmed French audio on full-length episodes',()=>{
 const captures=JSON.parse(readFileSync(new URL('./fixtures/naruto-vf-metadata.json',import.meta.url),'utf8'));
 assert.deepEqual(captures.map(capture=>capture.episode),[1,5,10,15,20]);
 for(const {metadata} of captures){
  assert.equal(vfStatus(metadata),'confirmed');
  assert.ok(metadata.duration_sec>1000&&metadata.duration_sec<1800);
  const index=preferredAudioTrack(metadata.audio_tracks,0,false);
  assert.equal(frenchAudioEvidence(metadata.audio_tracks.find(track=>track.index===index)),'confirmed');
 }
});
