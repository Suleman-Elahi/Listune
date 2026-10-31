#!/usr/bin/env node
// NDJSON bridge between the Listune Go backend and the Soulseek network.
// Speaks the same protocol library and keychain as aeyoll/soulseek-cli
// (slsk-client + keytar service "soulseek-cli"), so `soulseek login`
// credentials keep working.
//
// Modes:
//   node bridge.mjs serve
//       Daemon mode (used by the Go backend). Reads NDJSON commands on
//       stdin, writes NDJSON events on stdout. One shared slsk connection:
//       downloads only work for peers discovered by a prior search in the
//       same session, so search and download must share this process.
//   node bridge.mjs status | login | search
//       One-shot helpers for manual debugging (search only; download
//       requires a session).
//
// Commands (serve mode), one JSON object per stdin line:
//   {"id":"1","cmd":"status"}
//   {"id":"2","cmd":"login","account":"u","password":"p"}
//   {"id":"3","cmd":"search","query":"...","mode":"mp3","quality":"320","timeoutMs":8000}
//   {"id":"4","cmd":"download","dest":"/path","items":[{"name","folder","raw"}]}
//
// Events always carry the originating "id". Terminal events are
// "done" / "ok" / "error".

import process from 'node:process';
import path from 'node:path';
import fs from 'node:fs';
import readline from 'node:readline';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

function loadDeps() {
  try {
    return {
      slsk: require('slsk-client'),
      keytar: require('keytar'),
    };
  } catch (err) {
    const candidates = [
      '/home/ilfs/.nvm/versions/node/v24.15.0/lib/node_modules/soulseek-cli/node_modules',
      '/usr/lib/node_modules/soulseek-cli/node_modules',
      '/usr/local/lib/node_modules/soulseek-cli/node_modules',
    ];
    for (const dir of candidates) {
      try {
        return {
          slsk: require(path.join(dir, 'slsk-client')),
          keytar: require(path.join(dir, 'keytar')),
        };
      } catch {
        // try next
      }
    }
    emit({ type: 'error', code: 'deps', message: `bridge dependencies unavailable: ${err.message}` });
    process.exit(2);
  }
}

const KEYCHAIN_SERVICE = 'soulseek-cli';

function emit(obj) {
  process.stdout.write(JSON.stringify(obj) + '\n');
}

function fail(message, code = 1) {
  emit({ type: 'error', message });
  process.exit(code);
}

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith('--')) {
      const key = a.slice(2);
      const next = argv[i + 1];
      if (next === undefined || next.startsWith('--')) {
        args[key] = true;
      } else {
        args[key] = next;
        i++;
      }
    } else {
      args._.push(a);
    }
  }
  return args;
}

async function getCredentials(keytar) {
  const account = process.env.SOULSEEK_ACCOUNT;
  const password = process.env.SOULSEEK_PASSWORD;
  if (account && password) {
    return { account, password };
  }
  const credentials = await keytar.findCredentials(KEYCHAIN_SERVICE);
  if (!credentials || credentials.length === 0) {
    return null;
  }
  return credentials[0];
}

// Mirrors soulseek-cli's FilterResult: free slots only, extension filter,
// optional bitrate filter, fastest peers first.
function filterResults(res, mode, quality) {
  let out = res.filter((r) => r.slots === true && r.speed > 0);
  if (mode === 'flac') {
    out = out.filter((r) => path.extname(r.file) === '.flac');
  } else {
    out = out.filter((r) => path.extname(r.file) === '.mp3');
  }
  if (quality) {
    out = out.filter((r) => r.bitrate === parseInt(quality, 10));
  }
  return out.sort((a, b) => b.speed - a.speed);
}

function groupByFolder(res) {
  const groups = new Map();
  for (const item of res) {
    const parts = item.file.split('\\');
    const folder = parts.length >= 2 ? parts[parts.length - 2] : '';
    const key = `${folder}||${item.user}`;
    if (!groups.has(key)) {
      groups.set(key, { folder, user: item.user, files: [] });
    }
    groups.get(key).files.push(item);
  }
  return [...groups.values()];
}

function displayName(file) {
  const parts = file.file.split('\\');
  return parts[parts.length - 1] || file.file;
}

function safeFolderName(folder) {
  const cleaned = String(folder || '')
    .replace(/[/\\:*?"<>|\u0000]/g, '_')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/^\.+/, '');
  return cleaned || 'soulseek-downloads';
}

// --- session: one slsk connection shared by search and download -------------

function createSession(slsk) {
  let clientPromise = null;

  function connect() {
    if (!clientPromise) {
      clientPromise = (async () => {
        const credentials = await getCredentials(session.keytar);
        if (!credentials) {
          throw new Error('not logged in');
        }
        return new Promise((resolve, reject) => {
          slsk.connect({ user: credentials.account, pass: credentials.password }, (err, client) => {
            if (err) reject(err);
            else resolve(client);
          });
        });
      })().catch((err) => {
        clientPromise = null;
        throw err;
      });
    }
    return clientPromise;
  }

  const session = {
    keytar: null,

    async status() {
      const credentials = await getCredentials(session.keytar);
      return {
        type: 'status',
        loggedIn: !!credentials,
        account: credentials ? credentials.account : null,
      };
    },

    async login(account, password) {
      await session.keytar.setPassword(KEYCHAIN_SERVICE, account, password);
      return { type: 'ok', account };
    },

    async search(query, mode, quality, timeoutMs) {
      const events = [];
      const client = await connect();
      const results = await new Promise((resolve, reject) => {
        client.search({ req: query, timeout: timeoutMs }, (err, res) => {
          if (err) reject(err);
          else resolve(res || []);
        });
      });

      const filtered = filterResults(results, mode, quality);
      const groups = groupByFolder(filtered);
      groups.forEach((group, resultIndex) => {
        const files = group.files.map((f, fileIndex) => ({
          fileIndex,
          name: displayName(f),
          size: f.size || 0,
          bitrate: f.bitrate ?? null,
        }));
        const totalSize = group.files.reduce((a, f) => a + (f.size || 0), 0);
        const bitrateSum = group.files.reduce((a, f) => a + (f.bitrate || 0), 0);
        events.push({
          type: 'result',
          resultIndex,
          user: group.user,
          folder: group.folder,
          files,
          avgBitrate: files.length ? Math.round(bitrateSum / files.length) : 0,
          sizeMb: Math.round(totalSize / 1024 / 1024),
          speedKbs: Math.round((group.files[0].speed || 0) / 1024),
          slots: true,
          // Raw slsk-client objects, echoed back verbatim for download.
          raw: group.files,
        });
      });
      events.push({ type: 'done', count: groups.length });
      return events;
    },

    async download(dest, items, onEvent) {
      const client = await connect();
      onEvent({ type: 'starting', count: items.length });

      const perFileTimeoutMs = parseInt(process.env.SLSK_DOWNLOAD_TIMEOUT_MS || '', 10) || 10 * 60 * 1000;
      let downloaded = 0;
      let failed = 0;
      for (const item of items) {
        const name = item.name || displayName(item.raw || item);
        const folder = safeFolderName(item.folder);
        const destPath = path.join(dest, folder, name);

        if (fs.existsSync(destPath)) {
          onEvent({ type: 'file-skip', name, path: destPath });
          downloaded++;
          continue;
        }

        onEvent({ type: 'file-start', name, path: destPath });
        try {
          fs.mkdirSync(path.dirname(destPath), { recursive: true });
          await new Promise((resolve, reject) => {
            let settled = false;
            const timer = setTimeout(() => {
              if (!settled) {
                settled = true;
                reject(new Error(`timed out after ${Math.round(perFileTimeoutMs / 1000)}s`));
              }
            }, perFileTimeoutMs);
            client.download({ file: item.raw || item, path: destPath }, (err) => {
              if (settled) return;
              settled = true;
              clearTimeout(timer);
              if (err) reject(err);
              else resolve(destPath);
            });
          });
          downloaded++;
          onEvent({ type: 'file-done', name, path: destPath });
        } catch (err) {
          failed++;
          onEvent({ type: 'file-error', name, message: String(err.message || err) });
        }
      }

      onEvent({ type: 'done', downloaded, failed });
      return { downloaded, failed };
    },
  };

  return session;
}

// --- serve mode -------------------------------------------------------------

function cmdServe(slsk, keytar) {
  const session = createSession(slsk);
  session.keytar = keytar;

  const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });

  rl.on('line', (line) => {
    line = line.trim();
    if (!line) return;
    let req;
    try {
      req = JSON.parse(line);
    } catch {
      emit({ type: 'error', message: 'invalid command JSON' });
      return;
    }
    const id = req.id ?? '';
    const done = (obj) => emit({ id, ...obj });

    (async () => {
      switch (req.cmd) {
        case 'status':
          done(await session.status());
          break;
        case 'login': {
          const res = await session.login(req.account, req.password);
          done(res);
          break;
        }
        case 'search': {
          if (!String(req.query || '').trim()) throw new Error('search requires query');
          const events = await session.search(
            req.query,
            req.mode === 'flac' ? 'flac' : 'mp3',
            req.quality || '',
            req.timeoutMs || 8000,
          );
          for (const ev of events) done(ev);
          break;
        }
        case 'download': {
          if (!req.dest) throw new Error('download requires dest');
          const items = Array.isArray(req.items) ? req.items : [];
          if (items.length === 0) throw new Error('download requires items');
          await session.download(req.dest, items, (ev) => done(ev));
          break;
        }
        default:
          throw new Error(`unknown command: ${req.cmd}`);
      }
    })().catch((err) => {
      done({ type: 'error', message: String(err.message || err) });
    });
  });

  rl.on('close', () => process.exit(0));
}

// --- one-shot helpers (manual debugging) ------------------------------------

async function main() {
  const [command, ...rest] = process.argv.slice(2);
  const args = parseArgs(rest);
  const deps = loadDeps();
  const session = createSession(deps.slsk);
  session.keytar = deps.keytar;

  switch (command) {
    case 'serve':
      return cmdServe(deps.slsk, deps.keytar);
    case 'status':
      emit(await session.status());
      return process.exit(0);
    case 'login': {
      const account = args.account;
      const password = process.env.BRIDGE_LOGIN_PASSWORD ?? args.password;
      if (typeof account !== 'string' || typeof password !== 'string') {
        fail('login requires --account and a password (BRIDGE_LOGIN_PASSWORD or --password)');
      }
      emit(await session.login(account, password));
      return process.exit(0);
    }
    case 'search': {
      const query = typeof args.query === 'string' ? args.query : '';
      if (!query.trim()) fail('search requires --query');
      const mode = args.mode === 'flac' ? 'flac' : 'mp3';
      const quality = typeof args.quality === 'string' ? args.quality : '';
      const timeout = typeof args.timeout === 'string' ? parseInt(args.timeout, 10) || 8000 : 8000;
      emit({ type: 'searching', query, mode, timeout });
      const events = await session.search(query, mode, quality, timeout);
      for (const ev of events) emit(ev);
      return process.exit(0);
    }
    case 'download':
      fail('download is only available in a session: use "serve" mode');
      break;
    default:
      fail(`unknown command: ${command || '(none)'}`);
  }
}

main().catch((err) => fail(`bridge crashed: ${err.stack || err}`));
