'use strict';
// Explicit research download queue only. Never executes downloaded material.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const ROOT=path.resolve(__dirname,'../..'),KB=path.join(ROOT,'docs/knowledge'),CAP=path.join(KB,'sources/captures');
const queue=JSON.parse(fs.readFileSync(path.join(KB,'acquisition-queue.json'),'utf8')).entries;
const logPath=path.join(KB,'sources/retrievals.jsonl'),args=process.argv.slice(2);
const value=n=>{const i=args.indexOf(n);return i<0?undefined:args[i+1]};
const allowed=new Set(['markblottner.com','web.archive.org','decibel.fun']);
const hash=b=>crypto.createHash('sha256').update(b).digest('hex');
function safe(p){const a=path.resolve(CAP,p);if(!a.startsWith(CAP+path.sep))throw Error('Capture path leaves capture directory');return a}
function log(r){fs.appendFileSync(logPath,JSON.stringify(r)+'\n');console.log(r.id+': '+r.status+(r.error?' - '+r.error:''))}
function history(){if(!fs.existsSync(logPath))return [];return fs.readFileSync(logPath,'utf8').trim().split(/\r?\n/).filter(Boolean).map(l=>JSON.parse(l))}
async function main(){
 const limit=Number(value('--limit')??5);if(!Number.isInteger(limit)||limit<1||limit>100)throw Error('--limit must be 1..100');
 const id=value('--id');let entries=queue.filter(e=>e.enabled!==false&&(!id||e.id===id));if(id&&!entries.length)throw Error('Unknown queue id: '+id);
 if(args.includes('--list')){for(const e of entries)console.log(e.id+' | '+e.kind+' | '+e.url);return}
 fs.mkdirSync(CAP,{recursive:true});const previous=history();let tried=0,failed=false;
 for(const e of entries){
  if(tried>=limit)break;
  if(!/^[a-z0-9-]+$/.test(e.id)||!['.png','.html'].includes(e.extension))throw Error('Invalid queue path');
  const u=new URL(e.url);if(u.protocol!=='https:'||!allowed.has(u.hostname)||u.username||u.password)throw Error('URL outside explicit collection hosts');
  const saved=previous.filter(r=>r.id===e.id&&r.status==='downloaded').at(-1);
  if(saved){const p=path.resolve(ROOT,saved.localPath);if(!p.startsWith(CAP+path.sep))throw Error('Invalid recorded path');if(fs.existsSync(p)&&hash(fs.readFileSync(p))===saved.sha256){console.log(e.id+': already saved and hash verified');continue}throw Error('Previously saved payload missing or changed: '+e.id)}
  if(previous.some(r=>r.id===e.id&&r.status==='access-denied')){console.log(e.id+': access denied previously; skipped');continue}
  tried++;const r={id:e.id,sourceId:e.sourceId,requestedUrl:e.url,requestedCapture:e.requestedCapture??null,attemptedAt:new Date().toISOString(),status:'failed',localPath:null,sha256:null};
  try{
   let current=e.url,response;
   // Manual redirects retain provenance and never silently cross to another host.
   const redirects=[];const signal=AbortSignal.timeout(15000);
   for(let step=0;step<6;step++){
    response=await fetch(current,{redirect:'manual',signal,headers:{'User-Agent':'CokeMusicPreservationResearch/1.0','Accept':e.kind==='image'?'image/png':'text/html'}});
    if([301,302,303,307,308].includes(response.status)){
     const target=new URL(response.headers.get('location'),current);
     await response.body?.cancel();
     if(target.protocol!=='https:'||!allowed.has(target.hostname)||target.username||target.password)throw Error('Redirect outside collection hosts: '+target.hostname);
     redirects.push({from:current,to:target.href,status:response.status});current=target.href;continue;
    }break;
   }
   r.finalUrl=current;r.redirects=redirects;r.httpStatus=response.status;r.contentType=response.headers.get('content-type');r.lastModified=response.headers.get('last-modified');
   if([401,403].includes(response.status)){r.status='access-denied';await response.body?.cancel();log(r);failed=true;continue}
   if(!response.ok){await response.body?.cancel();throw Error('HTTP '+response.status)}
   const MAX=20*1024*1024,reader=response.body.getReader(),chunks=[];let size=0;
   try{while(true){const {done,value}=await reader.read();if(done)break;size+=value.length;if(size>MAX)throw Error('Response exceeds 20 MiB capture limit');chunks.push(Buffer.from(value));}}finally{await reader.cancel()}
   const bytes=Buffer.concat(chunks);
   if(e.kind==='image'&&(!bytes.subarray(0,8).equals(Buffer.from([137,80,78,71,13,10,26,10]))||!/image\/png/i.test(r.contentType??'')))throw Error('Expected PNG; response is not a validated image');
   if(e.kind==='html'&&(!/text\/html/i.test(r.contentType??'')||!/<!doctype|<html|<head|<body/i.test(bytes.toString('utf8',0,8192))))throw Error('Expected HTML');
   const digest=hash(bytes),dir=safe(e.id);fs.mkdirSync(dir,{recursive:true});const dest=safe(e.id+'/'+digest+e.extension);
   if(fs.existsSync(dest)){if(hash(fs.readFileSync(dest))!==digest)throw Error('Hash conflict at existing capture path')}else fs.writeFileSync(dest,bytes,{flag:'wx'});
   Object.assign(r,{status:'downloaded',bytes:bytes.length,sha256:digest,localPath:path.relative(ROOT,dest).split(path.sep).join('/'),historicalValidation:'pending: check capture date, archive errors, missing dependencies and version'});
   log(r);
  }catch(err){r.error=String(err.cause?.code||err.message);log(r);failed=true;if(/ENOTFOUND|EAI_AGAIN|ENETUNREACH|fetch failed/.test(r.error)){console.log('Network unavailable; remaining queue retained.');break}}
  if(tried<limit)await new Promise(resolve=>setTimeout(resolve,500));
 }
 if(failed)process.exitCode=1;
}
main().catch(e=>{console.error(e.message);process.exitCode=1});
