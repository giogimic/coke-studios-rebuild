'use strict';
// Research-only: read preserved files; write docs/knowledge/generated; never execute recovered code.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const ROOT=path.resolve(__dirname,'../..'),OUT=path.join(ROOT,'docs/knowledge/generated');
fs.mkdirSync(OUT,{recursive:true});
const slash=p=>p.split(path.sep).join('/'),rel=p=>slash(path.relative(ROOT,p));
const sha=b=>crypto.createHash('sha256').update(b).digest('hex'),bt=String.fromCharCode(96);
const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const clean=s=>String(s??'').replace(/<!--[\s\S]*?-->/g,'').replace(/<script\b[\s\S]*?<\/script>/gi,'').replace(/<style\b[\s\S]*?<\/style>/gi,'').replace(/<[^>]+>/g,' ').replace(/&nbsp;/g,' ').replace(/&amp;/g,'&').replace(/\s+/g,' ').trim();
const specs=[
 ['sources/cache-finder','community-preservation-research','screenshots and known-file hashes; capture dates require authentication'],
 ['sources/music-mixer/assets','community-mixer-preserved-assets','OddCast and remake assets; classify each file before use'],
 ['sources/avatar-engine','community-avatar-reference','not original server behavior'],
 ['sources/uncover-the-music','community-minigame-preservation','preserved Flash and reconstruction code; mixed provenance'],
 ['sources/cs-website','community-website-reconstruction','pre-MyCoke recreation; not a dated original capture'],
 ['sources/gocoke/web/Lingo','recovered-client-assets','release association needs file-level verification'],
 ['sources/gocoke/web/data','client-data','release association needs verification'],
 ['sources/original-client-source','decompiled-original-client','dated directory is an archive label, not independently authenticated build'],
 ['sources/avatar-assets','community-preserved-avatar-assets','version unresolved'],
 ['sources/catalogue-parser','community-parser-and-data','version unresolved'],
 ['sources/cokeitems','community-item-reference','version unresolved'],
 ['sources/cs-ui','community-ui-reference','version unresolved'],
 ['sources/cs-chat','community-chat-reference','version unresolved'],
 ['sources/screensavers','preserved-downloads','version unresolved'],
 ['archives/extracted-complete-pack/Cokestudios Private Server','private-server-release-and-assets','mixed preservation pack; not original backend']
];
const files=[],windows=[],pages=[],urls=[],scripts=[],strings=[],members=[];
const skip=new Set(['.git','node_modules','.runtime','target','dist']);
function walk(dir){if(!fs.existsSync(dir))return [];let a=[];for(const e of fs.readdirSync(dir,{withFileTypes:true})){if(skip.has(e.name)||e.isSymbolicLink())continue;const p=path.join(dir,e.name);if(e.isDirectory())a.push(...walk(p));else if(e.isFile())a.push(p)}return a.sort()}
function dimensions(b){try{
 if(b.length>30&&b.toString('ascii',0,4)==='RIFF'&&b.toString('ascii',8,12)==='WEBP'){const t=b.toString('ascii',12,16);if(t==='VP8X')return{width:1+b.readUIntLE(24,3),height:1+b.readUIntLE(27,3)};if(t==='VP8L'){const n=b.readUInt32LE(21);return{width:1+(n&16383),height:1+((n>>>14)&16383)}}}
 if(b.length>24&&b.toString('ascii',1,4)==='PNG')return {width:b.readUInt32BE(16),height:b.readUInt32BE(20)};
 if(b.length>10&&b.toString('ascii',0,3)==='GIF')return {width:b.readUInt16LE(6),height:b.readUInt16LE(8)};
 if(b[0]===255&&b[1]===216){let i=2;while(i+9<b.length){if(b[i]!==255){i++;continue}const m=b[i+1];i+=2;if(m===217||m===218)break;if(m===216||m===1)continue;const n=b.readUInt16BE(i);if([192,193,194,195,197,198,199,201,202,203,205,206,207].includes(m))return {width:b.readUInt16BE(i+5),height:b.readUInt16BE(i+3)};if(n<2)break;i+=n}}
}catch{}return null}
function attrs(tag){const o={};for(const m of tag.matchAll(/([\w:-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))/g))o[m[1].toLowerCase()]=m[2]??m[3]??m[4];return o}
const getTag=(s,n)=>(s.match(new RegExp('<'+n+'>\\s*([\\s\\S]*?)\\s*</'+n+'>','i'))||[])[1]?.trim()??null;
for(const [folder,kind,releaseNote] of specs)for(const full of walk(path.join(ROOT,folder))){
 const buf=fs.readFileSync(full),p=rel(full),ext=path.extname(p).toLowerCase();
 const rec={path:p,bytes:buf.length,sha256:sha(buf),extension:ext,kind,releaseNote};
 if(/\.(png|gif|jpe?g|webp)$/.test(ext))Object.assign(rec,dimensions(buf)||{});
 files.push(rec);
 if(!/\.(txt|ls|html?|css|js|json|xml|csv|as|md)$/.test(ext)||buf.length>2*1024*1024)continue;
 const text=buf.toString('utf8').replace(/\r\n?/g,'\n'),lines=text.split(/\n/);
 for(let i=0;i<lines.length;i++)for(const m of lines[i].matchAll(/https?:\/\/[^\s"'<>\\\])]+/gi)){
  const url=m[0].replace(/[.,;]+$/,'');
  if(/cokemusic|mycoke|icoke|cokestudio|studiocom|maissito|markblottner|gamesthatwork|decibel\.fun|yourcoke/i.test(url))urls.push({url,path:p,line:i+1,kind});
 }
 if(ext==='.ls'){const handlers=[];for(let i=0;i<lines.length;i++){const m=lines[i].match(/^\s*on\s+([^\s,]+)(.*)$/i);if(m)handlers.push({name:m[1],parameters:m[2].trim(),line:i+1})}scripts.push({path:p,sha256:rec.sha256,handlers})}
 if(ext==='.csv'&&path.basename(p)==='Members.csv')members.push({path:p,sha256:rec.sha256,rows:Math.max(0,lines.filter(x=>x.trim()).length-1)});
 if(ext==='.txt'&&/<window>/i.test(text)){
  const elements=[];for(let i=0;i<lines.length;i++){const line=lines[i];if(!/#(?:member|id)\s*:/i.test(line))continue;const properties={};
   for(const m of line.matchAll(/#(\w+)\s*:\s*("(?:[^"]|"")*"|#[\w.]+|[-+]?\d+(?:\.\d+)?|\[[^\]]*\])/g)){let value=m[2];if(value.startsWith('"'))value=value.slice(1,-1);else if(/^[-+]?\d/.test(value))value=Number(value);properties[m[1].toLowerCase()]=value}
   elements.push({line:i+1,properties,raw:line.trim()});
  }
  windows.push({path:p,sha256:rec.sha256,name:getTag(text,'name'),embeddedDate:getTag(text,'date'),version:getTag(text,'version'),rect:getTag(text,'rect'),border:getTag(text,'border'),variant:/old|test|temp/i.test(path.basename(p))?'old-or-test-name':'active-status-unproven',elements});
 }
 if((folder==='sources/cs-website'||(folder==='sources/uncover-the-music'&&p.includes('/_resources/_other/html/')))&&/\.html?$/.test(ext)){
  const links=[],images=[],forms=[],controls=[],headings=[],popups=[];
  const html=text.replace(/<script\b[\s\S]*?<\/script>/gi,m=>' '.repeat(m.length)).replace(/<!--[\s\S]*?-->/g,m=>' '.repeat(m.length));
  for(const m of html.matchAll(/<a\b([^>]*)>([\s\S]*?)<\/a>/gi)){const a=attrs(m[1]);links.push({...a,label:clean(m[2]),line:text.slice(0,m.index).split('\n').length})}
  for(const m of html.matchAll(/<(?:img|embed|object)\b([^>]*)>/gi))images.push(attrs(m[1]));
  for(const m of html.matchAll(/<form\b([^>]*)>/gi))forms.push(attrs(m[1]));
  for(const m of html.matchAll(/<(?:input|select|textarea|button)\b([^>]*)>/gi))controls.push(attrs(m[1]));
  for(const m of html.matchAll(/<h[1-6]\b[^>]*>([\s\S]*?)<\/h[1-6]>/gi))headings.push(clean(m[1]));
  for(const m of text.matchAll(/(?:MM_openBrWindow|window\.open)\s*\(([^)]*)\)/g))popups.push(m[1]);
  pages.push({path:p,sha256:rec.sha256,kind,title:clean((text.match(/<title[^>]*>([\s\S]*?)<\/title>/i)||[])[1]),headings,links,images,forms,controls,popups,text:clean(text)});
 }
 if(ext==='.txt'&&/Text_English|FeatureSet|Catalogue_English/i.test(p))strings.push({path:p,sha256:rec.sha256,lineCount:lines.length});
}
files.sort((a,b)=>a.path.localeCompare(b.path));windows.sort((a,b)=>a.path.localeCompare(b.path));
const byHash=new Map();for(const f of files){if(!byHash.has(f.sha256))byHash.set(f.sha256,[]);byHash.get(f.sha256).push(f.path)}
const duplicates=[...byHash].filter(([,p])=>p.length>1).map(([sha256,paths])=>({sha256,paths}));
function json(name,data){fs.writeFileSync(path.join(OUT,name),JSON.stringify(data,null,2)+'\n')}
fs.writeFileSync(path.join(OUT,'files.jsonl'),files.map(f=>JSON.stringify(f)).join('\n')+'\n');
json('interface-windows.json',windows);json('website-pages.json',pages);json('historical-url-occurrences.json',urls);json('lingo-handlers.json',scripts);json('cast-members-index.json',members);json('duplicate-files.json',duplicates);json('text-datasets.json',strings);
const byKind={};for(const f of files){byKind[f.kind]??={files:0,bytes:0};byKind[f.kind].files++;byKind[f.kind].bytes+=f.bytes}
const summary={generatedAt:new Date().toISOString(),workspace:ROOT,scope:specs.map(([root,kind,releaseNote])=>({root,kind,releaseNote})),files:files.length,bytes:files.reduce((s,f)=>s+f.bytes,0),uniqueContentHashes:byHash.size,duplicateGroups:duplicates.length,windows:windows.length,elements:windows.reduce((s,w)=>s+w.elements.length,0),websitePages:pages.length,websiteLinks:pages.reduce((s,p)=>s+p.links.length,0),lingoScripts:scripts.length,lingoHandlers:scripts.reduce((s,p)=>s+p.handlers.length,0),historicalURLReferences:urls.length,uniqueHistoricalURLs:new Set(urls.map(x=>x.url)).size,byKind,limits:['No original release date inferred from extractor filenames or embedded window dates.','Text extraction limited to 2 MiB per file; binaries and LASM are inventoried, not parsed.','Window property extraction preserves raw lines; it is not a complete Lingo interpreter.','Website HTML is a community reconstruction; extracted navigation does not prove historical routing.','No original screenshot pixels recovered through web search are claimed as downloaded.']};
json('summary.json',summary);
let md='# Recovered interface definitions\n\nGenerated from local recovered source. Dates are embedded authoring metadata, not release authentication. OLD/test variants remain separate. Full controls, positions, fonts, keys and raw lines: [interface-windows.json](interface-windows.json).\n\n| Source file | Window | Embedded date | Rectangle | Elements |\n|---|---|---|---|---|\n';
for(const w of windows)md+='| '+bt+w.path+bt+' | '+String(w.name).replaceAll('|','\\|')+' | '+(w.embeddedDate||'unknown')+' | '+(w.rect||'unknown')+' | '+w.elements.length+' |\n';
fs.writeFileSync(path.join(OUT,'INTERFACE-INDEX.md'),md);
md='# Recovered website link inventory\n\nThese HTML files are preserved in community projects: CSWebsite reconstruction and the Uncover rules fragment. A link may point to a missing page. This is not a dated original crawl. See [website-pages.json](website-pages.json) for forms, images, embedded objects, popups and extracted text.\n';
for(const p of pages){md+='\n## '+path.basename(p.path)+'\n\nSource: '+bt+p.path+bt+'\n\n| Label | Target | Other behavior |\n|---|---|---|\n';for(const l of p.links)md+='| '+(l.label||'(image/blank)').replaceAll('|','\\|')+' | '+bt+(l.href||'')+bt+' | '+(l.onclick||l.target||'').replaceAll('|','\\|')+' |\n'}
fs.writeFileSync(path.join(OUT,'WEBSITE-LINKS.md'),md);
const gallery=files.filter(f=>f.width&&(/sources\/cache-finder\/docs\/img\//.test(f.path)||/sources\/cs-website\/www\/images\//.test(f.path)||/\/Lingo\/(cc_interface\[1\]|cc_navigator\[1\]|cc_messenger\[1\]|cc_room|mixer|entry)\//.test(f.path)));
const groups=new Map();for(const f of gallery){const g=slash(path.dirname(f.path));if(!groups.has(g))groups.set(g,[]);groups.get(g).push(f)}
let html='<!doctype html><meta charset="utf-8"><title>Coke Music recovered interface assets</title><style>body{font:15px system-ui;background:#eee;color:#181818;padding:24px}details{margin:16px 0}section{display:flex;flex-wrap:wrap;gap:12px}figure{margin:0;padding:12px;background:white;border:1px solid #bbb;max-width:260px}img{max-width:240px;max-height:160px;image-rendering:pixelated}figcaption{font-size:11px;overflow-wrap:anywhere}a{color:#183d80}</style><h1>Recovered website and interface assets</h1><p>Research catalogue only. Images remain in their preserved source directories. Asset existence and embedded dates do not prove a historical version or active UI state.</p><p>'+gallery.length+' images with measured dimensions. Open a source image at full size for inspection.</p>';
for(const [g,items]of groups){html+='<details><summary>'+esc(g)+' ('+items.length+')</summary><section>';for(const f of items){const href=encodeURI(slash(path.relative(OUT,path.join(ROOT,f.path))));html+='<figure><a href="'+esc(href)+'"><img loading="lazy" src="'+esc(href)+'" alt="'+esc(path.basename(f.path))+'"></a><figcaption>'+esc(path.basename(f.path))+'<br>'+f.width+' × '+f.height+'<br>SHA-256 '+f.sha256+'</figcaption></figure>'}html+='</section></details>'}
fs.writeFileSync(path.join(OUT,'ASSET-GALLERY.html'),html);


const locale=[], catalogue=[];
for(const f of strings){
 let section='';const lines=fs.readFileSync(path.join(ROOT,f.path),'utf8').replace(/\r\n?/g,'\n').split('\n');
 for(let i=0;i<lines.length;i++){
  const l=lines[i].trim(),h=l.match(/^\[\s*([^\]]+)\s*\]$/);if(h){section=h[1].trim();continue}
  const m=l.match(/^([A-Za-z0-9_.-]+)\s*=(.*)$/);if(!m)continue;
  const r={path:f.path,line:i+1,section,key:m[1],value:m[2]};
  (/Catalogue/.test(f.path)?catalogue:locale).push(r);
 }
}
json('text-keys.json',locale);json('catalogue-records.json',catalogue);
const keyUses=[];for(const w of windows)for(const e of w.elements)if(e.properties.key)keyUses.push({window:w.path,line:e.line,id:e.properties.id,key:e.properties.key,matches:locale.filter(t=>t.key.toLowerCase()===String(e.properties.key).toLowerCase()).map(t=>({path:t.path,line:t.line,value:t.value}))});
json('interface-text-links.json',keyUses);
const knownPath='sources/cache-finder/src/known.c';const known=[];
if(fs.existsSync(path.join(ROOT,knownPath))){const text=fs.readFileSync(path.join(ROOT,knownPath),'utf8');for(const m of text.matchAll(/\{"([a-f0-9]{40})",\s*(\d+)u,\s*"([^"]+)"\}/g))known.push({sha1:m[1],bytes:Number(m[2]),filename:m[3],source:knownPath});}
json('community-known-file-fingerprints.json',known);
summary.textKeys=locale.length;summary.catalogueRecords=catalogue.length;summary.windowTextReferences=keyUses.length;summary.knownFileFingerprints=known.length;summary.galleryImages=gallery.length;
summary.limits.push('HTML extraction is static and heuristic; unclosed tags or runtime JavaScript need manual review.','Community known-file fingerprints describe that project archive, not all surviving files globally.');
json('summary.json',summary);

const inspected=new Set(['cokemusic-2002.webp','sharktale-site.webp','beta-ingame.webp','v1-alaska.webp']);
json('visual-references.json',files.filter(f=>f.path.startsWith('sources/cache-finder/docs/img/')).map(f=>({...f,inspection:inspected.has(path.basename(f.path))?'visually-inspected-2026-10-24':'catalogued-not-visually-inspected',originalCaptureDate:null,originalURL:null,preservedBy:'Lost-Media-Hunt/Coke-Studios',labelStatus:'filename label; historical version unconfirmed'})));

const dispatcher='sources/gocoke/web/Lingo/entry/67_WindowInteractivity.ls';
const dispatchLines=fs.readFileSync(path.join(ROOT,dispatcher),'utf8').replace(/\r\n?/g,'\n').split('\n');
const actions=[];let current=null;
for(let i=0;i<dispatchLines.length;i++){
 const line=dispatchLines[i],m=line.match(/^\s*"([^"]+)":\s*$/);
 if(m){current={controlId:m[1],path:dispatcher,line:i+1,statements:[],windowReferences:[]};actions.push(current);continue}
 if(current&&line.trim()&&!/^\s*(end case|end)$/i.test(line))current.statements.push({line:i+1,text:line.trim()});
 if(/^\s*end case/i.test(line))current=null;
}
for(const a of actions)for(const w of windows)for(const e of w.elements)if(e.properties.id===a.controlId)a.windowReferences.push({path:w.path,line:e.line,member:e.properties.member});
json('interface-actions.json',actions);
summary.interfaceDispatchCases=actions.length;
json('summary.json',summary);
console.log(JSON.stringify(summary,null,2));
