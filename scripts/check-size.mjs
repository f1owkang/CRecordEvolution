import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { gzipSync } from 'node:zlib';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');
const OUT = join(ROOT, 'webroot');
const LIMIT = 50 * 1024;
const CODE_EXT = new Set(['.js', '.css']);

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

const rows = [];
let totalRaw = 0;
let totalGzip = 0;
for (const p of walk(OUT)) {
  const ext = p.slice(p.lastIndexOf('.'));
  if (!CODE_EXT.has(ext)) continue;
  const raw = readFileSync(p);
  const gz = gzipSync(raw, { level: 9 }).length;
  rows.push({ file: relative(OUT, p).replaceAll('\\', '/'), raw: raw.length, gzip: gz });
  totalRaw += raw.length;
  totalGzip += gz;
}

rows.sort((a, b) => b.gzip - a.gzip);
console.log('WebUI 代码资源体积（gzip）:');
for (const r of rows) {
  console.log(`  ${r.file.padEnd(38)} ${String(r.raw).padStart(8)} B raw  ${String(r.gzip).padStart(7)} B gz`);
}
console.log(`  ${'TOTAL'.padEnd(38)} ${String(totalRaw).padStart(8)} B raw  ${String(totalGzip).padStart(7)} B gz`);
console.log(`上限 ${LIMIT} B gzip（${(totalGzip / LIMIT * 100).toFixed(1)}% 已用）`);

if (totalGzip > LIMIT) {
  console.error(`::error::WebUI gzip ${totalGzip} B 超过上限 ${LIMIT} B`);
  process.exit(1);
}
