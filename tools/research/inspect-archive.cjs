'use strict';
// Static preservation work only. Never executes recovered programs or scripts.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),cp=require('node:child_process');
const ROOT=path.resolve(__dirname,'../..'),OUT=path.join(ROOT,'docs/knowledge/generated');
const PACK='archives/Coke Studios Archive',EXPANDED='archives/research-expanded';
const seven=process.env.RESEARCH_7Z||(process.platform==='win32'?'C:/Program Files/7-Zip/7z.exe':'7z');
const slash=p=>p.replaceAll('\\','/'),rel=p=>slash(path.relative(ROOT,p));
const hash=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const walk=d=>!fs.existsSync(d)?[]:fs.readdirSync(d,{withFileTypes:true}).flatMap(e=>e.isSymbolicLink()?[]:e.isDirectory()?walk(path.join(d,e.name)):[path.join(d,e.name)]).sort();
const run=args=>cp.execFileSync(seven,args,{encoding:'utf8',maxBuffer:64*1024*1024,windowsHide:true});
function members(file){
 return run(['l','-slt','-ba','-sccUTF-8',file]).trim().split(/\r?\n\s*\r?\n/).filter(Boolean).map(block=>{
  const fields=Object.fromEntries(block.split(/\r?\n/).map(l=>{const i=l.indexOf(' = ');return i<0?null:[l.slice(0,i),l.slice(i+3)]}).filter(Boolean));
  return {path:slash(fields.Path||''),bytes:Number(fields.Size||0),directory:fields.Folder==='+'||/^D/.test(fields.Attributes||''),modified:fields.Modified||null,crc:fields.CRC||null,encrypted:fields.Encrypted==='+',attributes:fields.Attributes||null,link:fields['Symbolic Link']||fields['Hard Link']||null};
 });
}
function validate(entries,dest){
 const names=new Set();let bytes=0;
 for(const e of entries){
  const segments=e.path.split('/'),target=path.resolve(dest,e.path);
  if(!e.path||e.path.startsWith('/')||segments.some(s=>s==='..'||s==='.'||s.includes(':'))||!target.startsWith(dest+path.sep)||e.link||e.encrypted||/^l/.test(e.attributes||''))throw Error('Unsafe archive entry '+e.path);
  const key=e.path.toLowerCase();if(names.has(key))throw Error('Colliding archive path '+e.path);names.add(key);bytes+=e.bytes;
 }
 if(bytes>2*1024**3||entries.length>100000)throw Error('Archive exceeds research extraction bounds');
}
function extract(source,subdir){
 const file=path.resolve(ROOT,source),dest=path.resolve(ROOT,EXPANDED,subdir),base=path.resolve(ROOT,EXPANDED);
 if(!dest.startsWith(base+path.sep))throw Error('Extraction outside research directory');
 const entries=members(file);validate(entries,dest);
 const marker=path.join(dest,'.extraction.json'),sha256=hash(file);
 if(fs.existsSync(marker)){
  if(JSON.parse(fs.readFileSync(marker,'utf8')).sha256!==sha256)throw Error('Container changed: '+source);
 }else{
  if(fs.existsSync(dest)&&fs.readdirSync(dest).length)throw Error('Refusing populated unrecorded destination '+dest);
  fs.mkdirSync(dest,{recursive:true});run(['x','-y','-aos','-bd','-bb0','-o'+dest,file]);
  fs.writeFileSync(marker,JSON.stringify({source,sha256,destination:rel(dest),entries:entries.length,extractedAt:new Date().toISOString()},null,2)+'\n');
 }
 for(const e of entries.filter(e=>!e.directory)){
  const target=path.resolve(dest,e.path);
  if(!fs.existsSync(target)||fs.lstatSync(target).isSymbolicLink()||fs.statSync(target).size!==e.bytes)throw Error('Incomplete extraction '+e.path);
 }
 console.log(JSON.stringify({source,destination:rel(dest),files:entries.filter(e=>!e.directory).length}));
}
if(process.argv.includes('--extract')){
 extract(PACK+'/Client Source Code/March 2006 Client.rar','march-2006-source');
 extract(PACK+'/Original Client Assets/March 15, 2006.zip','march-15-2006-assets');
 const children=walk(path.join(ROOT,EXPANDED,'march-2006-source')).filter(p=>/\.(7z|rar|zip)$/i.test(p));
 for(const p of children)extract(rel(p),'march-2006-source-unpacked/'+path.basename(p).replace(/\.(7z|rar|zip)$/i,'').replace(/[^a-zA-Z0-9_-]+/g,'-'));
}
fs.mkdirSync(OUT,{recursive:true});
const containers=[path.join(ROOT,'archives/Coke Studios Archive.zip'),...walk(path.join(ROOT,PACK)),...walk(path.join(ROOT,EXPANDED))].filter(p=>/\.(zip|rar|7z)$/i.test(p));
const records=containers.map(p=>({path:rel(p),bytes:fs.statSync(p).size,sha256:hash(p),members:members(p)}));
fs.writeFileSync(path.join(OUT,'archive-containers.json'),JSON.stringify({generatedAt:new Date().toISOString(),limits:['Container timestamps are preservation metadata, not authenticated release dates.','SDK/private-server payloads are listed but not run.','Only client source and asset packages are additionally unpacked.'],containers:records},null,2)+'\n');
console.log(JSON.stringify({containers:records.length,members:records.reduce((n,r)=>n+r.members.filter(m=>!m.directory).length,0)}));
