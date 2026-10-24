'use strict';
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const ROOT=path.resolve(__dirname,'../..'),KB=path.join(ROOT,'docs/knowledge'),GEN=path.join(KB,'generated');
const sha=b=>crypto.createHash('sha256').update(b).digest('hex');
const read=(p)=>JSON.parse(fs.readFileSync(path.join(KB,p),'utf8'));
const failures=[];function check(v,m){if(!v)failures.push(m)}
const files=fs.readFileSync(path.join(GEN,'files.jsonl'),'utf8').trim().split('\n').map(JSON.parse),byPath=new Map(files.map(f=>[f.path,f]));
const summary=read('generated/summary.json'),windows=read('generated/interface-windows.json'),pages=read('generated/website-pages.json');
check(summary.files===files.length,'File count mismatch');check(byPath.size===files.length,'Duplicate inventory path');
let hashed=0;
for(const f of files){
 const p=path.resolve(ROOT,f.path);
 if(!p.startsWith(ROOT+path.sep)){failures.push('Path outside workspace: '+f.path);continue}
 if(!fs.existsSync(p)){failures.push('Missing indexed file: '+f.path);continue}
 const b=fs.readFileSync(p);check(b.length===f.bytes&&sha(b)===f.sha256,'Inventory hash/size mismatch: '+f.path);hashed++;
}
check(summary.windows===windows.length,'Window count mismatch');
check(summary.elements===windows.reduce((n,w)=>n+w.elements.length,0),'Element count mismatch');
let sourceLines=0;
for(const w of windows){
 check(byPath.get(w.path)?.sha256===w.sha256,'Window hash mismatch: '+w.path);
 const lines=fs.readFileSync(path.join(ROOT,w.path),'utf8').replace(/\r\n?/g,'\n').split('\n');
 for(const e of w.elements){check(lines[e.line-1]?.trim()===e.raw,'Window source line mismatch: '+w.path+':'+e.line);sourceLines++}
}
check(summary.websitePages===pages.length,'Website page count mismatch');
check(pages.some(p=>p.path.endsWith('RevealChallengeRules.htm')),'Uncover rules HTML missing from page index');
check(pages.every(p=>!p.path.includes('/Lingo/')),'Director text member misclassified as website page');
for(const p of pages)check(byPath.get(p.path)?.sha256===p.sha256,'Page hash mismatch: '+p.path);
const obs=read('observations.json').observations;
for(const o of obs)check(byPath.get(o.sourcePath)?.sha256===o.sha256,'Observation hash mismatch: '+o.id);
const sources=read('sources.json').sources,sourceIDs=new Set(sources.map(s=>s.id));
for(const s of sources)if(s.localPath)check(fs.existsSync(path.join(ROOT,s.localPath)),'Missing source path '+s.id);
const q=read('acquisition-queue.json').entries;
check(new Set(q.map(x=>x.id)).size===q.length,'Duplicate queue ID');
for(const e of q){check(sourceIDs.has(e.sourceId),'Unknown source ID '+e.id);check(!e.localPath,'Queue incorrectly claims a local capture '+e.id);check(/^https:\/\//.test(e.url),'Queue URL not HTTPS '+e.id)}
const visual=read('generated/visual-references.json');for(const v of visual)check(byPath.get(v.path)?.sha256===v.sha256,'Visual hash mismatch '+v.path);
const html=fs.readFileSync(path.join(GEN,'ASSET-GALLERY.html'),'utf8');let images=0;
for(const m of html.matchAll(/<img[^>]+src="([^"]+)"/g)){const u=decodeURI(m[1].replaceAll('&amp;','&'));check(fs.existsSync(path.resolve(GEN,u)),'Missing gallery image '+u);images++}
check(summary.galleryImages===images,'Gallery count mismatch');
let links=0;
for(const name of fs.readdirSync(KB).filter(x=>x.endsWith('.md'))){const md=fs.readFileSync(path.join(KB,name),'utf8');for(const m of md.matchAll(/!?\[[^\]]*\]\(([^)\n]+)\)/g)){const u=m[1];if(/^[a-z]+:|^#/i.test(u))continue;const p=path.resolve(KB,decodeURI(u.split('#')[0]));check(fs.existsSync(p),'Broken link in '+name+': '+u);links++}}
const logs=fs.existsSync(path.join(KB,'sources/retrievals.jsonl'))?fs.readFileSync(path.join(KB,'sources/retrievals.jsonl'),'utf8').trim().split('\n').filter(Boolean).map(JSON.parse):[];
for(const r of logs)if(r.status==='downloaded'){check(!!r.localPath&&!!r.sha256,'Incomplete capture record');if(r.localPath&&fs.existsSync(path.join(ROOT,r.localPath)))check(sha(fs.readFileSync(path.join(ROOT,r.localPath)))===r.sha256,'Capture hash mismatch '+r.id);else failures.push('Capture file missing '+r.id)}
const report={checkedAt:new Date().toISOString(),status:failures.length?'failed':'passed',inventoryFilesHashed:hashed,windowSourceLinesChecked:sourceLines,websitePages:pages.length,observations:obs.length,visualReferences:visual.length,visuallyInspected:visual.filter(v=>v.inspection.startsWith('visually-')).length,galleryImages:images,localDocumentationLinks:links,queuedCaptures:q.length,captureAttempts:logs.length,downloadedCaptures:logs.filter(r=>r.status==='downloaded').length,failures,limits:['Checks verify file integrity, source locators and index consistency; they do not authenticate historical versions.','No Go gameplay, browser or historical-parity tests run for this research-only change.']};
fs.writeFileSync(path.join(KB,'VERIFICATION.json'),JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report,null,2));if(failures.length)process.exitCode=1;
