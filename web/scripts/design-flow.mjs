// Exercise the responsive reference layout without relying on a live catalog.
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
const base=process.env.TEST_BASE_URL||'http://127.0.0.1:4391';
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/nix/store/smy016lwammg4q19qxcadh5svm7c8f9k-chromium-148.0.7778.215/bin/chromium',args:['--no-sandbox']});
const page=await browser.newPage({locale:'fr-FR'});
const errors=[];page.on('pageerror',error=>errors.push(error.message));
const image=(color,width,height)=>`data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}"><rect width="100%" height="100%" fill="${color}"/></svg>`)}`;
const title='Re:Zero kara Hajimeru Isekai Seikatsu 4th Season';
const items=Array.from({length:16},(_,index)=>({id:index+1,media_id:index===0?100:undefined,display_title:index===0?'Re:Zero':`Anime ${index}`,media_title:index===0?title:`Anime ${index} — Saison 2`,title_romaji:index===0?title:undefined,banner_image:index===0?image('#806638',1900,400):undefined,poster_image:image('#354064',460,650),media_poster_image:index===0?image('#4f284f',460,610):undefined,episodes:26,season_year:2026,status:'RELEASING'}));
const featuredTitle='Naruto Shippuden';
const classic={...items[0],id:900,media_id:901,media_title:featuredTitle,title_romaji:featuredTitle,status:'FINISHED'};
let fail=false,empty=false,upcoming=false;
await page.route('**/api/v1/**',async route=>{
 if(fail){await route.fulfill({status:502,body:'offline'});return;}
 await route.fulfill({contentType:'application/json',body:JSON.stringify({page:1,has_next_page:true,items:new URL(route.request().url()).pathname.endsWith('/popular')?[classic]:empty?[]:items.map(item=>upcoming?{...item,status:'NOT_YET_RELEASED',start_date:'2026-10-04'}:item)})});
});
try {
 for(const width of [1280,430,320]){
  await page.setViewportSize({width,height:900});await page.goto(base);await page.getByRole('heading',{name:featuredTitle,exact:true}).first().waitFor();await page.evaluate(()=>document.fonts.ready);
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,`page overflow at ${width}`);
  for(const selector of ['.site-wordmark','.catalog-search','.theme-toggle']) {
   assert.notEqual(await page.locator(selector).evaluate(el=>getComputedStyle(el).backdropFilter),'none','compiled CSS must retain the standard backdrop-filter');
  }
  assert.equal(await page.getByRole('link',{name:'Regarder',exact:true}).getAttribute('href'),'/anime/900/seasons/901/episodes/1','play the featured season, not the canonical franchise ID');
  const hero=await page.locator('.anime-hero').boundingBox(), heading=await page.locator('.anime-hero h1').boundingBox(), rail=await page.locator('.seasonal-grid').boundingBox();
  assert.ok(heading.y>=0 && heading.y+heading.height<=hero.y+hero.height,'heading must fit its artwork');
  assert.ok(rail.y>=hero.y+hero.height,'posters must not overlap the featured content');
  const columns=await page.locator('.seasonal-grid').evaluate(el=>getComputedStyle(el).gridTemplateColumns.split(' ').length);
  assert.equal(columns,width===1280?4:2);
  assert.equal(items.length % columns,0);
  assert.equal(await page.locator(".seasonal-card").first().locator(".poster-overlay > div").evaluate(el=>el.firstElementChild.tagName),"P");
  const seasonalCard=page.locator('.seasonal-card').first();
  assert.equal(await seasonalCard.getAttribute('href'),'/anime/1/seasons/100');
  assert.equal(await seasonalCard.locator('img').getAttribute('src'),items[0].media_poster_image);
  assert.equal(await page.locator('.poster-rail').count(),0);
  assert.equal(await page.locator('.hero-portrait').isVisible(),width<=600);
  assert.equal(await page.locator('.hero-wide').isVisible(),width>600);
 }
 await page.locator('.header-search-toggle').click();await page.getByRole('textbox',{name:'Rechercher un anime'}).fill('Example');await page.getByRole('button',{name:'Rechercher',exact:true}).click();await page.waitForURL('**/?q=Example&page=1');await page.getByRole('heading',{name:'Résultats pour « Example »'}).waitFor();assert.equal(await page.locator('.anime-hero').count(),0);
 const poster=page.locator('.poster-card').first();await poster.focus();await page.keyboard.press('Tab');await page.keyboard.press('Shift+Tab');await page.waitForFunction(()=>getComputedStyle(document.querySelector('.poster-overlay')).opacity==='1');
 await page.goto(`${base}/?tab=popular`);await page.getByRole('heading',{name:'Les incontournables'}).waitFor();
 upcoming=true;await page.goto(base);await page.locator('.premiere-date').first().waitFor();assert.equal(await page.getByRole('link',{name:'Regarder',exact:true}).count(),1);
 empty=true;await page.reload();await page.getByText('Aucun anime trouvé.').waitFor();assert.equal(await page.locator('.seasonal-card').count(),0);
 fail=true;await page.reload();await page.locator('main [role="alert"]').waitFor();fail=false;empty=false;upcoming=false;await page.getByRole('button',{name:'Réessayer'}).click();await page.getByRole('heading',{name:featuredTitle,exact:true}).first().waitFor();
 assert.deepEqual(errors,[]);console.log('PASS: desktop/mobile layout, overflow, seasonal grid and posters, season-specific playback link, search, keyboard focus, popular route, upcoming, empty and retry.');
}finally{await browser.close();}
