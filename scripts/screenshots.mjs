/**
 * README 截图生成：起本地静态服务器 + 注入 mock ksu.exec，用无头 Edge/Chrome
 * 拍概览 / 历史记录 / 历史趋势 / 设置四页（zh+en），直接覆盖 .github/images/。
 * 依赖：webroot/ 构建产物（先 pnpm build）+ 本机 Edge/Chrome（或 CHROME_PATH）。
 * 视口 390×844、deviceScaleFactor=2；截图按 CSS 像素输出（390 宽），README 里
 * 缩到 200 宽仍清晰。截图里的电池数据全部是虚构演示数据，不代表真实设备。
 * 用法：pnpm run shots
 */
import { createServer } from 'node:http';
import { readFileSync, mkdirSync, existsSync } from 'node:fs';
import { extname, join, normalize, resolve } from 'node:path';
import { chromium } from 'playwright-core';

const ROOT = resolve(new URL('..', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'));
const WEBROOT = join(ROOT, 'webroot');
const OUT = join(ROOT, '.github', 'images');

if (!existsSync(join(WEBROOT, 'index.html'))) {
  console.error('webroot/index.html 不存在，请先运行 pnpm build');
  process.exit(1);
}
mkdirSync(OUT, { recursive: true });

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.txt': 'text/plain; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
};

const server = createServer((req, res) => {
  const url = new URL(req.url, 'http://localhost');
  if (url.pathname === '/favicon.ico') {
    res.writeHead(204);
    res.end();
    return;
  }
  if (url.pathname === '/internal/insets.css') {
    // 模拟宿主拦截：真实设备上由 KernelSU/APatch 注入，这里给空样式避免 404。
    res.writeHead(200, { 'content-type': 'text/css' });
    res.end(
      ':root{--safe-area-inset-top:0px;--safe-area-inset-right:0px;--safe-area-inset-bottom:0px;--safe-area-inset-left:0px}'
    );
    return;
  }
  let rel = decodeURIComponent(url.pathname);
  if (rel.endsWith('/')) rel += 'index.html';
  const file = normalize(join(WEBROOT, rel));
  if (!file.startsWith(WEBROOT) || !existsSync(file)) {
    res.writeHead(404);
    res.end('not found');
    return;
  }
  res.writeHead(200, { 'content-type': MIME[extname(file)] || 'application/octet-stream' });
  res.end(readFileSync(file));
});

const now = Math.floor(Date.now() / 1000);
const day = 86400;

// —— 虚构演示数据：数值自洽（估算 4520mAh / 设计 5000mAh ≈ 90%，当前 76% 充电中）——
const jsonDoc = {
  channel: 'stable',
  version: '1.3.8(37)',
  design_mah: 5000,
  full_mah: 4820,
  cycles: 232,
  pct: 90,
  est_mah: 4520,
  est_mah_sigma: 78,
  samples: 46,
  cycle_equiv: 1.68,
  r_moh: 92.5,
  temp_c: 33.2,
  trend_mah_per_week: -5.8,
  trend_state: 'significant',
  trend_span_day: 24,
  trend_from_mah: 4680,
  trend_to_mah: 4530,
  quality: { ok: true, spread_pct: 4, ema_mah: 4520, mid_mah: 4600, dis_mah: 4500, full_mah: 4820 },
  updated: '2026-11-07 00:30:00',
  samples_n: 1680,
  sessions_n: 42,
  calib: {
    current_node: '/sys/class/power_supply/battery/current_now',
    counter_node: '/sys/class/power_supply/battery/charge_counter',
    current_scale: 1000,
    current_unit: 'ma',
    anchor_scale: 0.001,
    raw_current: 3000,
    current_ua: 2450000,
  },
  live: {
    status: 'Charging',
    cap: 76,
    current_ua: 2450000,
    voltage_uv: 4210000,
    power_mw: 10314,
    charging: {
      start_ts: now - 720,
      start_cap: 61,
      cap: 76,
      acc_uah: 1234000,
      secs: 720,
      avg_current_ua: 6170000,
      eta_min: 42,
    },
  },
  daemon: { version: '1.3.8(37)', ts: now, age_sec: 12 },
  recent: [
    { ts: now - day * 24, mah: 4680 },
    { ts: now - day * 21, mah: 4655 },
    { ts: now - day * 18, mah: 4610 },
    { ts: now - day * 15, mah: 4585 },
    { ts: now - day * 12, mah: 4590 },
    { ts: now - day * 9, mah: 4550 },
    { ts: now - day * 6, mah: 4565 },
    { ts: now - day * 3, mah: 4528 },
    { ts: now - day, mah: 4520 },
  ],
  sessions: [],
  rest_points: [],
  ccct: [],
  ica_peaks: [],
  discharge_recent: [],
};

const sessions = [
  {
    start_ts: now - day * 6 - 4200,
    end_ts: now - day * 6,
    start_cap: 24,
    end_cap: 82,
    delta_pct: 58,
    est_mah: 4490,
    valid: true,
    temp_avg: 31,
    c_rate: 0.46,
  },
  {
    start_ts: now - day * 4 - 3900,
    end_ts: now - day * 4,
    start_cap: 33,
    end_cap: 91,
    delta_pct: 58,
    est_mah: 4535,
    valid: true,
    temp_avg: 32,
    c_rate: 0.48,
  },
  {
    start_ts: now - day * 2 - 3600,
    end_ts: now - day * 2,
    start_cap: 41,
    end_cap: 96,
    delta_pct: 55,
    est_mah: 4510,
    valid: true,
    temp_avg: 33,
    c_rate: 0.5,
  },
  {
    start_ts: now - 5400,
    end_ts: now - 1200,
    start_cap: 52,
    end_cap: 88,
    delta_pct: 36,
    est_mah: 4522,
    valid: true,
    temp_avg: 34,
    c_rate: 0.44,
  },
];
const discharges = [
  {
    ts: now - day * 5 - 7200,
    secs: 5400,
    uah_mah: 2240,
    start_cap: 84,
    end_cap: 58,
    implied_mah: 4560,
  },
  {
    ts: now - day * 3 - 7200,
    secs: 6100,
    uah_mah: 2380,
    start_cap: 79,
    end_cap: 51,
    implied_mah: 4480,
  },
  {
    ts: now - day - 7200,
    secs: 5800,
    uah_mah: 2300,
    start_cap: 77,
    end_cap: 53,
    implied_mah: 4510,
  },
];
const restPoints = [
  { ts: now - day * 5, uv: 3_880_000, cap: 54 },
  { ts: now - day * 3, uv: 3_905_000, cap: 50 },
  { ts: now - day, uv: 3_915_000, cap: 52 },
];
const ccct = [
  { ts: now - day * 5, secs: 700, vw_lo: 4_200_000, vw_hi: 4_300_000 },
  { ts: now - day * 3, secs: 745, vw_lo: 4_200_000, vw_hi: 4_300_000 },
  { ts: now - day, secs: 780, vw_lo: 4_200_000, vw_hi: 4_300_000 },
];
const ica = [
  { ts: now - day * 5, peak_uv: 4_200_000, peak_h_rel: 1.08 },
  { ts: now - day * 3, peak_uv: 4_208_000, peak_h_rel: 1.12 },
  { ts: now - day, peak_uv: 4_212_000, peak_h_rel: 1.09 },
];

const events = [
  { kind: 'charge', ts: sessions[3].end_ts, charge: sessions[3] },
  { kind: 'discharge', ts: discharges[2].ts, discharge: discharges[2] },
  { kind: 'charge', ts: sessions[2].end_ts, charge: sessions[2] },
  { kind: 'discharge', ts: discharges[1].ts, discharge: discharges[1] },
  { kind: 'charge', ts: sessions[1].end_ts, charge: sessions[1] },
  { kind: 'discharge', ts: discharges[0].ts, discharge: discharges[0] },
  { kind: 'charge', ts: sessions[0].end_ts, charge: sessions[0] },
];

const DEMO = {
  json: jsonDoc,
  diag:
    'CRecordEvolution diag（已脱敏：无序列号/IMEI/MAC/账号）\n' +
    'module=1.3.8(37) channel=stable root=KernelSU android=16(sdk36) abi=arm64\n' +
    'device=OnePlus PJE110 (aston)\n' +
    'calib=x1000 ma I_node=/sys/class/power_supply/battery/current_now\n' +
    'est=4520mAh samples=46 cycleEq=1.68\n' +
    'events7d=cur_cal=1 counter_scale=1 quality_warn=0 tick_error=0\n',
  events,
  summary: {
    days: 7,
    since: now - day * 7,
    until: now,
    sessions: sessions.length,
    valid_sessions: sessions.length,
    discharges: discharges.length,
    samples: 6420,
    oldest_ts: now - day * 6,
    newest_ts: now,
  },
  trends: {
    estimates: jsonDoc.recent.map((r) => ({ ...r })),
    sessions,
    discharge: discharges,
    rest_points: restPoints,
    ccct,
    ica_peaks: ica,
  },
  nextBefore: now - day * 7,
};

const HOST_STUB = (data) => `
var D = ${JSON.stringify(data)};
function crHistoryDoc(cmd) {
  var type = (/--type\\s+(\\w+)/.exec(cmd) || [])[1] || 'all';
  var before = Number((/--before\\s+(\\d+)/.exec(cmd) || [])[1] || 0);
  var events = before ? [] : D.events.filter(function (ev) { return type === 'all' || ev.kind === type; });
  return {
    events: events,
    has_more: !before,
    next_before: before ? undefined : D.nextBefore,
    summary: D.summary,
    trends: D.trends
  };
}
window.ksu = {
  enableEdgeToEdge() {},
  enableInsets() {},
  exec(cmd, opts, cb) {
    var out = '';
    var errno = 0;
    if (cmd.indexOf('batteryd json') >= 0) out = JSON.stringify(D.json);
    else if (cmd.indexOf('batteryd diag') >= 0) out = D.diag;
    else if (cmd.indexOf('batteryd history') >= 0) out = JSON.stringify(crHistoryDoc(cmd));
    else if (cmd.indexOf('cat ') === 0) { errno = 1; }
    setTimeout(function () {
      var fn = typeof cb === 'string' ? window[cb] : (typeof opts === 'function' ? opts : cb);
      if (typeof fn === 'function') fn(errno, out, errno ? 'no file' : '');
    }, 15);
  }
};
`;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function shoot(page, name) {
  await page.evaluate(() => window.scrollTo(0, 0));
  await sleep(350);
  // scale:'css'：按 CSS 像素输出 390 宽（DSF=2 只用于渲染锐度）
  await page.screenshot({ path: join(OUT, name), scale: 'css' });
}

async function launch() {
  const candidates = [
    process.env.CHROME_PATH,
    'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
    'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
    '/usr/bin/google-chrome',
    '/usr/bin/google-chrome-stable',
    '/opt/google/chrome/chrome',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
  ].filter(Boolean);
  const executablePath = candidates.find((p) => existsSync(p));
  if (!executablePath) throw new Error('找不到 Chrome/Edge，请设置 CHROME_PATH');
  return chromium.launch({ executablePath, headless: true, args: ['--no-sandbox'] });
}

const browser = await launch();
try {
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  const base = `http://127.0.0.1:${server.address().port}`;

  for (const lang of ['zh', 'en']) {
    const context = await browser.newContext({
      locale: lang === 'zh' ? 'zh-CN' : 'en-US',
      viewport: { width: 390, height: 844 },
      deviceScaleFactor: 2,
    });
    await context.addInitScript(HOST_STUB(DEMO));
    // boot.js 在首屏前读 localStorage：固定深色主题 + 目标语言
    await context.addInitScript((l) => {
      try {
        localStorage.setItem('cre_theme', 'dark');
        localStorage.setItem('cre_lang', l);
        localStorage.setItem('cre_barmode', 'floating');
      } catch (e) {
        /* 老浏览器忽略 */
      }
    }, lang);
    const page = await context.newPage();
    await page.goto(base + '/', { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('#page_overview.is-active', { timeout: 15000 });
    await page.waitForSelector('#page_overview .charge-card', { timeout: 15000 });
    await sleep(500);
    await shoot(page, `overview-${lang}.png`);

    await page.click('#nav_history');
    await page.waitForSelector('#page_history .tl-item', { timeout: 15000 });
    await page.locator('#page_history .tl-card').first().click();
    await page.waitForSelector('#page_history .tl-det', { timeout: 5000 });
    await shoot(page, `history-${lang}.png`);

    await page.locator('#hist_mode_seg .seg-btn').nth(1).click();
    await page.waitForSelector('#page_history .trend-wrap', { timeout: 15000 });
    await sleep(400);
    await shoot(page, `trends-${lang}.png`);

    await page.click('#nav_settings');
    await page.waitForSelector('#page_settings.is-active', { timeout: 15000 });
    await sleep(300);
    await shoot(page, `settings-${lang}.png`);

    await context.close();
    console.log(`已生成 ${lang} 四张截图`);
  }
} finally {
  await browser.close();
  server.close();
}

console.log(`截图输出目录：${OUT}`);
