'use strict';
const fs=require('node:fs');const path=require('node:path');const crypto=require('node:crypto');
const root=path.resolve(__dirname,'..');
const expected=JSON.parse(fs.readFileSync(path.join(root,'docs/FILE-HASHES.json'),'utf8').replace(/^\uFEFF/,''));
const bad=[];
for(const x of expected){try{const b=fs.readFileSync(path.join(root,x.path));const sha=crypto.createHash('sha1').update('blob '+b.length+'\0').update(b).digest('hex');if(sha!==x.sha)bad.push(x.path);}catch{bad.push(x.path);}}
console.log(JSON.stringify({files:expected.length,mismatches:bad},null,2));
if(bad.length)process.exitCode=1;
