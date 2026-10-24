'use strict';
// Local launch adapter. Upstream repositories remain unchanged.
const http = require('node:http');
const net = require('node:net');
const fs = require('node:fs/promises');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const mime = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.xml': 'application/xml; charset=utf-8',
  '.txt': 'text/plain; charset=utf-8',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.svg': 'image/svg+xml',
  '.wav': 'audio/wav',
  '.mp3': 'audio/mpeg',
  '.ogg': 'audio/ogg',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.wasm': 'application/wasm',
  '.ico': 'image/x-icon',
  '.map': 'application/json; charset=utf-8',
  '.swf': 'application/x-shockwave-flash'
};
const roomTypes = ['studio_model_b','studio_model_c','studio_model_a','studio_model_d','studio_model_rooftop','studio_wayne','pokemon'];
const roomNames = ['Studio #1','Studio #2','Studio #3','Studio #4','Rooftop Studio','Studio Wayne','Pokemon'];

const HOST = process.env.HOST || '0.0.0.0';
const DEFAULT_PORT = parseInt(process.env.PORT || '24902', 10);
const BACKEND_PORT = parseInt(process.env.BACKEND_PORT || '8080', 10);
const COKEPHASE_PORT = parseInt(process.env.COKEPHASE_PORT || '8081', 10);
const MUSICMIXER_PORT = parseInt(process.env.MUSICMIXER_PORT || '8082', 10);

function send(res, status, body, type = 'text/plain; charset=utf-8') {
  res.writeHead(status, {
    'Content-Type': type,
    'Cache-Control': 'no-store',
    'X-Content-Type-Options': 'nosniff'
  });
  res.end(body);
}

async function fields(req, url) {
  const params = new URLSearchParams(url.search);
  if (req.method === 'POST') {
    let text = '';
    for await (const chunk of req) {
      text += chunk;
      if (text.length > 16384) throw new Error('Request too large');
    }
    for (const [k, v] of new URLSearchParams(text)) params.set(k, v);
  }
  return params;
}

function makeServer(project) {
  let base;
  if (project === 'gocoke') {
    base = path.join(root, 'sources', 'gocoke', 'web');
  } else if (project === 'cokephase') {
    base = path.join(root, 'sources', 'cokephase');
  } else if (project === 'musicmixer') {
    base = path.join(root, 'sources', 'music-mixer', 'dist');
  } else if (project === 'portal') {
    base = path.join(root, 'tools', 'portal');
  } else {
    base = path.join(root, 'sources', project);
  }

  return http.createServer(async (req, res) => {
    try {
      const url = new URL(req.url, 'http://127.0.0.1');

      if (url.pathname === '/health') {
        return send(res, 200, JSON.stringify({
          project,
          status: 'ok',
          mode: project === 'gocoke' ? 'offline-preview' : 'local-demo',
          backend: true
        }), 'application/json');
      }

      if (project === 'portal' && (url.pathname.startsWith('/sf/') || url.pathname.startsWith('/api/'))) {
        const proxyReq = http.request({
          hostname: '127.0.0.1',
          port: BACKEND_PORT,
          path: req.url,
          method: req.method,
          headers: req.headers
        }, proxyRes => {
          res.writeHead(proxyRes.statusCode, proxyRes.headers);
          proxyRes.pipe(res);
        });
        proxyReq.on('error', () => {
          send(res, 502, `Game server backend is offline (check port ${BACKEND_PORT})`);
        });
        req.pipe(proxyReq);
        return;
      }

      if (url.pathname.startsWith('/sf/') || url.pathname.startsWith('/ws/')) {
        return send(res, 503, 'No game backend is included in this local preview.');
      }

      if (project === 'portal' && url.pathname.startsWith('/game/')) {
        const sub = url.pathname.slice(6);
        const rel = (!sub || sub === '/') ? 'index.html' : sub;
        const file = path.resolve(root, 'sources', 'gocoke', 'web', rel);
        if (file.startsWith(path.join(root, 'sources', 'gocoke', 'web'))) {
          const ext = path.extname(file).toLowerCase();
          if (mime[ext]) {
            const body = await fs.readFile(file);
            return send(res, 200, body, mime[ext]);
          }
        }
      }

      if (project === 'portal' && url.pathname.startsWith('/Lingo/')) {
        const audioFile = path.resolve(root, 'sources', 'gocoke', 'web', url.pathname.slice(1));
        const ext = path.extname(audioFile).toLowerCase();
        if (mime[ext]) {
          const body = await fs.readFile(audioFile);
          return send(res, 200, body, mime[ext]);
        }
      }

      if (project === 'portal' && url.pathname.startsWith('/mixer/')) {
        const sub = url.pathname.slice(7);
        const rel = (!sub || sub === '/') ? 'index.html' : sub;
        const file = path.resolve(root, 'sources', 'music-mixer', 'dist', rel);
        if (file.startsWith(path.join(root, 'sources', 'music-mixer', 'dist'))) {
          const ext = path.extname(file).toLowerCase();
          if (mime[ext]) {
            const body = await fs.readFile(file);
            return send(res, 200, body, mime[ext]);
          }
        }
      }

      if (project === 'portal' && url.pathname.startsWith('/sounds/')) {
        const audioFile = path.resolve(root, 'sources', 'gocoke', 'web', 'Lingo', 'soundlibrary', url.pathname.slice(8));
        const ext = path.extname(audioFile).toLowerCase();
        if (mime[ext]) {
          const body = await fs.readFile(audioFile);
          return send(res, 200, body, mime[ext]);
        }
      }

      if (project === 'cokephase' && url.pathname === '/jsonreply/room.php') {
        const params = await fields(req, url);
        const id = Number(params.get('id'));
        const data = Number.isInteger(id) && id >= 1 && id <= 7
          ? { room_id: String(id), room_name: roomNames[id - 1], room_owner: 'CokePhase', room_type: roomTypes[id - 1] }
          : {};
        return send(res, 200, JSON.stringify(data), 'application/json');
      }

      if (project === 'cokephase' && url.pathname === '/jsonreply/catalog.php') {
        const params = await fields(req, url);
        if (!params.has('page') && params.has('page-number')) {
          return send(res, 200, '{"pages":15}', 'application/json');
        }
        const page = params.get('page') || '0';
        if (!/^(?:[0-9]|1[0-5])$/.test(page)) return send(res, 400, 'Invalid catalogue page');
        // Disabled by the upstream PHP file.
        if (page === '12' || page === '13') return send(res, 200, '', 'application/json');
        const content = await fs.readFile(path.join(base, 'jsonreply', 'catalog', 'page-' + page + '.json'), 'utf8');
        const cleaned = content.replace(/^\s*\/\/.*$/gm, '').replace(/,\s*]/g, ']');
        return send(res, 200, JSON.stringify(JSON.parse(cleaned)), 'application/json');
      }

      if (req.method !== 'GET' && req.method !== 'HEAD') {
        return send(res, 405, 'Method not allowed');
      }

      const decoded = decodeURIComponent(url.pathname);
      if (decoded.includes('\\') || decoded.includes('\0') || decoded.split('/').some(p => p.startsWith('.') || p.includes(':'))) {
        return send(res, 403, 'Forbidden');
      }

      const rel = decoded === '/' ? 'index.html' : decoded.slice(1);
      if (project === 'cokephase' && !['index.html', 'ClientConfig.js', 'client.js', 'index.js'].includes(rel) && !/^(assets|css|js|engine|gameClasses)\//.test(rel)) {
        return send(res, 403, 'Not a public client asset');
      }

      const file = path.resolve(base, rel);
      if (!file.startsWith(base + path.sep) && file !== base) {
        return send(res, 403, 'Forbidden');
      }

      const ext = path.extname(file).toLowerCase();
      if (!mime[ext]) return send(res, 403, 'File type not served');

      let body = await fs.readFile(file);
      if (project === 'cokephase' && rel === 'index.html') {
        body = body.toString('utf8')
          .replace('https://ajax.googleapis.com/ajax/libs/jquery/2.1.3/jquery.min.js', '/engine/components/editor/vendor/jquery.2.0.3.min.js')
          .replace(/<script>\s*\(function\(i,s,o,g,r,a,m\)[\s\S]*?<\/script>/, '');
      }

      if (req.method === 'HEAD') {
        res.writeHead(200, {
          'Content-Type': mime[ext],
          'Content-Length': Buffer.byteLength(body)
        });
        res.end();
      } else {
        send(res, 200, body, mime[ext]);
      }
    } catch (error) {
      send(res, error.code === 'ENOENT' ? 404 : 400, error.code === 'ENOENT' ? 'Not found' : 'Unable to serve request');
    }
  });
}

const { spawn } = require('node:child_process');
const fsSync = require('node:fs');

if (require.main === module) {
  const isWin = process.platform === 'win32';
  const binName = isWin ? 'coke-server.exe' : 'coke-server-linux';
  const binPath = path.join(root, 'tools', 'bin', binName);
  const webDir = path.join(root, 'sources', 'gocoke', 'web');

  let cokeChild = null;
  const servers = [];

  // Start standalone Coke Server for GoCoke (port 8080)
  if (fsSync.existsSync(binPath)) {
    if (!isWin) {
      try { fsSync.chmodSync(binPath, 0o755); } catch (_) {}
    }
    console.log(`Starting native Go backend on port ${BACKEND_PORT} (${binName})...`);
    cokeChild = spawn(binPath, ['-addr', `${HOST}:${BACKEND_PORT}`, '-root', webDir], {
      stdio: ['ignore', 'inherit', 'inherit']
    });
    cokeChild.on('error', err => {
      console.error('Failed to spawn coke-server:', err.message);
    });
    cokeChild.on('exit', (code, sig) => {
      if (code !== null && code !== 0) console.warn(`coke-server exited with code ${code}`);
    });
  } else {
    console.warn(`[WARN] Native ${binName} not found, falling back to static preview on port ${BACKEND_PORT}.`);
    const gocokeServer = makeServer('gocoke');
    gocokeServer.listen(BACKEND_PORT, HOST, () => console.log(`gocoke: http://${HOST}:${BACKEND_PORT}/`));
    servers.push(gocokeServer);
  }

  // Node servers: Portal defaults to port 24902, plus secondary ports
  const nodeConfigs = [
    ['portal', DEFAULT_PORT],
    ['cokephase', COKEPHASE_PORT],
    ['musicmixer', MUSICMIXER_PORT]
  ];

  for (const [name, port] of nodeConfigs) {
    const s = makeServer(name);
    s.on('error', e => {
      console.error(name + ': ' + e.message);
      process.exit(1);
    });

    if (name === 'portal') {
      s.on('upgrade', (req, socket, head) => {
        if (req.url && req.url.startsWith('/ws/realtime')) {
          const proxySocket = net.connect(BACKEND_PORT, '127.0.0.1', () => {
            proxySocket.write(`${req.method} ${req.url} HTTP/1.1\r\n`);
            for (let i = 0; i < req.rawHeaders.length; i += 2) {
              proxySocket.write(`${req.rawHeaders[i]}: ${req.rawHeaders[i+1]}\r\n`);
            }
            proxySocket.write('\r\n');
            if (head && head.length) proxySocket.write(head);
            proxySocket.pipe(socket);
            socket.pipe(proxySocket);
          });
          proxySocket.on('error', () => socket.destroy());
          socket.on('error', () => proxySocket.destroy());
        } else {
          socket.destroy();
        }
      });
    }

    s.listen(port, HOST, () => console.log(`${name}: http://${HOST}:${port}/`));
    servers.push(s);
  }

  function cleanup() {
    for (const s of servers) {
      try { s.close(); } catch (_) {}
    }
    if (cokeChild) {
      try { cokeChild.kill(); } catch (_) {}
    }
  }

  for (const signal of ['SIGINT', 'SIGTERM']) {
    process.on(signal, () => {
      cleanup();
      process.exit(0);
    });
  }
}

module.exports = { makeServer };
