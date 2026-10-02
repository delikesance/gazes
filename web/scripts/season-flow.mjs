import { chromium } from 'playwright';
import assert from 'node:assert/strict';
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium',args:['--no-sandbox','--disable-gpu']});
const base=process.env.TEST_BASE_URL||'http://127.0.0.1:4391';
const page=await browser.newPage({locale:'fr-FR'});
const assertShell=async()=>{assert.equal(await page.locator('header.catalog-header').count(),1);assert.equal(await page.locator('.header-search-toggle').count(),1);assert.equal(await page.getByRole('button',{name:'Changer de thème clair/sombre'}).count(),1);assert.equal(await page.locator('footer.site-footer').count(),1);};
const errors=[];page.on('pageerror',e=>errors.push(e.message));
const seasons=[{id:1,title:'Example',season_name:'Saison 1',group:'main',season_number:1,episodes:2},{id:2,title:'Example Season 2',season_name:'Saison 2',group:'main',season_number:2,episodes:2},{id:3,title:'Movie',season_name:'Film',group:'movies'}];
const source={id:'abc',info_hash:'abc',title:'Example S02 Complete VOSTFR 1080p',magnet_uri:'magnet:?xt=urn:btih:abc',seeders:5,leechers:2,size_display:'2 GB',is_batch:true,episode_number:1,season_number:2,language_tag:'VOSTFR',language_flags:['VOSTFR'],is_french:true,quality:'1080p',score_rank:56,score_breakdown:{french:30,multi:0,swarm:21,leechers:2,quality:3}};
let fail=false,streams=0;
await page.route('**/api/v1/**',async route=>{
 const path=new URL(route.request().url()).pathname;let data;
 if(path.endsWith('/franchise')) data={id:1,title:'Example',complete:true,seasons};
 else if(path.endsWith('/sources')) {if(fail){await route.fulfill({status:502,body:'offline'});return;}data={total_sources:1,french_sources:1,sources:[source]};}
 else if(/\/seasons\/\d+$/.test(path)) data={id:2,display_title:'Example Season 2',episodes:2,episode_list:[{episode_number:1,title:'Épisode 1'},{episode_number:2,title:'Épisode 2'}]};
 else if(path.endsWith('/torrent/load')) data={info_hash:'abc',main_video_index:0,files:[{index:0,path:'a.mkv',is_video:true},{index:1,path:'b.mkv',is_video:true}]};
 else if(path.endsWith('/torrent/stats')) data={download_rate_bps:0,active_seeders:0,total_peers:0,progress_pct:0};
 else if(path.endsWith('/metadata')) data={duration_sec:0};
 else if(path.endsWith('/stream')) {streams++;await route.fulfill({body:''});return;}
 else data={page:1,per_page:24,has_next_page:true,items:[{id:1,display_title:'Example',status:'FINISHED',banner_image:'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22/%3E'}]};
 await route.fulfill({contentType:'application/json',body:JSON.stringify(data)});
});
try {
 await page.goto(base);await page.getByRole('link',{name:'En savoir plus',exact:true}).click();await page.waitForURL('**/anime/1');await assertShell();
 await page.getByRole('heading',{name:'Films',exact:true}).waitFor();await page.getByRole('link',{name:/Saison 2/}).click();await page.waitForURL('**/seasons/2');await assertShell();
 await page.getByRole('link',{name:'Épisode 1'}).click();await page.waitForURL('**/episodes/1');await page.getByText('Aucune source disponible ne permet de lire cet épisode.').waitFor();assert.equal(streams,0);assert.equal(await page.getByRole('button',{name:'Lire',exact:true}).count(),0);await page.getByRole('button',{name:'Fermer',exact:true}).click();
 await page.waitForURL('**/seasons/2');await page.goto(`${base}/anime/2`);await page.waitForURL('**/anime/1');
 fail=true;await page.goto(`${base}/anime/1/seasons/2/episodes/1`);await page.locator('main [role="alert"]').waitFor();await page.waitForLoadState('networkidle');await page.getByRole('button',{name:'Réessayer'}).waitFor();fail=false;
 await page.getByRole('button',{name:'Réessayer'}).click();await page.getByText('Aucune source disponible ne permet de lire cet épisode.').waitFor();await page.getByRole('button',{name:'Fermer',exact:true}).click();
 await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth),false);
 await page.goto(`${base}/?q=Example&genre=Action&page=2`);await page.getByRole('heading',{name:'Example',exact:true}).last().click();await page.waitForURL('**/anime/1');await page.goBack();assert.match(page.url(),/q=Example/);assert.match(page.url(),/page=2/);
 await assertShell();assert.deepEqual(errors,[]);console.log('PASS: complete navigation, pack safety, canonical URLs, Back, retry, mobile, search state.');
}catch(error){console.log('PAGE:',await page.locator('body').innerText());console.log('ERRORS:',errors);throw error;}finally{await browser.close();}
