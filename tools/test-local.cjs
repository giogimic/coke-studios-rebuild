'use strict';
const assert = require('node:assert/strict');
const {Readable} = require('node:stream');
const {makeServer} = require('./serve-local.cjs');
const servers = {gocoke:makeServer('gocoke'),cokephase:makeServer('cokephase')};
function request(project,url,{method='GET',body=''}={}) {
  return new Promise((resolve,reject)=>{
    const req = Readable.from(body?[Buffer.from(body)]:[]);
    Object.assign(req,{url,method,headers:{}});
    const res = {
      writeHead(status,headers){this.status=status;this.headers=headers;},
      end(data){resolve({status:this.status,headers:this.headers,body:data?Buffer.from(data):Buffer.alloc(0)});}
    };
    servers[project].emit('request',req,res);
    req.on('error',reject);
  });
}
(async()=>{
  for(const project of Object.keys(servers)) {
    let r=await request(project,'/health');assert.equal(r.status,200);assert.equal(JSON.parse(r.body).project,project);
    r=await request(project,'/');assert.equal(r.status,200);assert.match(r.body.toString(),/<!doctype html>/i);
    r=await request(project,'/.git/config');assert.equal(r.status,403);
    r=await request(project,'/%2e%2e%5c%2e%2e%5cStart-Local.ps1');assert.equal(r.status,403);
    r=await request(project,'/sf/gateway',{method:'POST',body:'test'});assert.equal(r.status,503);
    r=await request(project,'/assets/missing-file.png');assert.equal(r.status,404);
  }
  assert.equal((await request('cokephase','/sftp-config.json')).status,403);
  assert.equal((await request('cokephase','/package.json')).status,403);
  const index=(await request('cokephase','/')).body.toString();
  assert.ok(!index.includes('google-analytics.com'));assert.ok(!index.includes('ajax.googleapis.com'));
  for(let id=1;id<=7;id++) {
    const r=await request('cokephase','/jsonreply/room.php',{method:'POST',body:'id='+id});
    assert.equal(JSON.parse(r.body).room_id,String(id));
  }
  assert.deepEqual(JSON.parse((await request('cokephase','/jsonreply/room.php',{method:'POST',body:'id=900'})).body),{});
  assert.equal(JSON.parse((await request('cokephase','/jsonreply/catalog.php',{method:'POST',body:'page-number=1'})).body).pages,15);
  for(const page of [0,1,2,3,4,5,6,7,8,9,10,11,14,15]) {
    const r=await request('cokephase','/jsonreply/catalog.php?page='+page);
    assert.equal(r.status,200);assert.equal(typeof JSON.parse(r.body),'object');
  }
  for(const page of [12,13]) assert.equal((await request('cokephase','/jsonreply/catalog.php?page='+page)).body.length,0);
  assert.equal((await request('cokephase','/jsonreply/catalog.php?page=../../secret')).status,400);
  const wasm=await request('gocoke','/main.wasm');
  assert.equal(wasm.headers['Content-Type'],'application/wasm');
  assert.equal(wasm.body.subarray(0,4).toString('hex'),'0061736d');
  assert.ok(WebAssembly.validate(wasm.body));
  const head=await request('gocoke','/main.wasm',{method:'HEAD'});
  assert.equal(head.body.length,0);assert.equal(head.headers['Content-Length'],wasm.body.length);
  console.log('PASS: both document roots, bundled jQuery/analytics handling, seven room fixtures, catalogue pages, WASM validity/MIME/HEAD, backend status and path restrictions.');
  console.log('Transport-free request tests; this does not establish browser gameplay or multiplayer.');
})().catch(e=>{console.error(e);process.exitCode=1;});
