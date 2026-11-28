/**
 * 产物离线守卫：扫描 webroot/ 构建产物，禁止 fetch / XMLHttpRequest / WebSocket /
 * 远程 import 与外部 URL（唯一放行 SVG 的 xmlns 命名空间）。
 * 依赖：先运行 `pnpm run build` 生成 webroot/。
 * 退出码：0 = 通过；1 = webroot/ 缺失或发现违规内容。
 */
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { ROOT } from './lib/root.mjs';

const OUT = join(ROOT, 'webroot');
// .svg 也要扫描：图标/图表里可以内嵌 <image href>、<script> 或远程 URL，
// 之前只扫 js/css/html/txt/json 会让构建产物里的 SVG 成为离线守卫盲区。
const TEXT_EXT = new Set(['.js', '.css', '.html', '.txt', '.json', '.svg']);

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
  // 协议相对 URL（//host/path）：既不是 https?:// 也不一定带 src/href 属性，
  // 放在引号/括号/等号之后、紧跟域名才算，避免把 JS 行注释（// ...）当命中。
  ['protocol-relative url', /['"(=]\s*\/\/[A-Za-z0-9][A-Za-z0-9.-]*\.[A-Za-z]{2,}/g],
  ['external html attr', /\b(?:src|href)\s*=\s*['"](?:https?:)?\/\//g],
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

if (
  !(() => {
    try {
      return statSync(OUT).isDirectory();
    } catch {
      return false;
    }
  })()
) {
  console.error('webroot/ 不存在，请先运行 pnpm build');
  process.exit(1);
}

const violations = [];
const files = walk(OUT);
let scanned = 0;
for (const p of files) {
  const ext = p.slice(p.lastIndexOf('.'));
  if (!TEXT_EXT.has(ext)) continue;
  scanned++;
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
console.log(
  `离线检查通过：${files.length} 个产物文件（扫描 ${scanned} 个文本/SVG），无远程请求/外部 URL/远程 import`
);
