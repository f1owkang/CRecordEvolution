import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');
const OUT = join(ROOT, 'webroot');
const TEXT_EXT = new Set(['.js', '.css', '.html', '.txt', '.json']);

// SVG 命名空间不是网络请求；只放行这一处固定字符串。
const ALLOWED_URL = /^https?:\/\/www\.w3\.org\//;

const RULES = [
  ['fetch()', /\bfetch\s*\(/g],
  ['XMLHttpRequest', /\bXMLHttpRequest\b/g],
  ['WebSocket', /\bnew\s+WebSocket\b/g],
  ['sendBeacon', /\bsendBeacon\s*\(/g],
  ['importScripts', /\bimportScripts\s*\(/g],
  ['remote import', /\b(?:import|from)\s*\(?\s*['"]https?:\/\//g],
  ['remote url', /https?:\/\/[^\s"'`<>)\\]+/g],
  ['external html attr', /\b(?:src|href)\s*=\s*['"](?:https?:)?\/\//g]
];

function walk(dir) {
  const files = [];
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) files.push(...walk(p));
    else files.push(p);
  }
  return files;
}

if (!(() => { try { return statSync(OUT).isDirectory(); } catch { return false; } })()) {
  console.error('webroot/ 不存在，请先运行 pnpm build');
  process.exit(1);
}

const violations = [];
for (const p of walk(OUT)) {
  const ext = p.slice(p.lastIndexOf('.'));
  if (!TEXT_EXT.has(ext)) continue;
  const text = readFileSync(p, 'utf8');
  for (const [name, re] of RULES) {
    re.lastIndex = 0;
    let m;
    while ((m = re.exec(text))) {
      const hit = m[0];
      if (name === 'remote url' && ALLOWED_URL.test(hit)) continue;
      const line = text.slice(0, m.index).split('\n').length;
      violations.push({ file: relative(OUT, p).replaceAll('\\', '/'), line, name, hit });
    }
  }
}

if (violations.length) {
  console.error('发现离线约束禁止的内容：');
  for (const v of violations) {
    console.error(`  ${v.file}:${v.line} [${v.name}] ${v.hit.slice(0, 120)}`);
  }
  process.exit(1);
}
console.log(`离线检查通过：${walk(OUT).length} 个产物文件，无远程请求/外部 URL/远程 import`);
