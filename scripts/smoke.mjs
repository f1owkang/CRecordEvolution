/**
 * 无头冒烟：起本地静态服务器、注入 mock ksu.exec，用无头 Edge/Chrome 走一遍
 * 概览 / 历史 / 趋势 / 设置 / gate 页面，并把截图写到 artifacts/smoke/。
 * 依赖：webroot/ 构建产物 + 本机 Edge/Chrome（或用 CHROME_PATH 指定）。
 * 退出码：0 = 全部检查通过；1 = 有检查失败或找不到浏览器。
 */
import { createServer } from 'node:http';
import { readFileSync, mkdirSync, existsSync } from 'node:fs';
import { extname, join, normalize, resolve } from 'node:path';
import { chromium } from 'playwright-core';

const ROOT = resolve(new URL('..', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'));
const WEBROOT = join(ROOT, 'webroot');
const SHOTS = join(ROOT, 'artifacts', 'smoke');

if (!existsSync(join(WEBROOT, 'index.html'))) {
  console.error('webroot/index.html 不存在，请先运行 pnpm build');
  process.exit(1);
}
mkdirSync(SHOTS, { recursive: true });

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
    // 模拟宿主拦截：真实设备上由 KernelSU/APatch 注入，这里给空样式避免 404 噪音。
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

const jsonDoc = {
  channel: 'stable',
  version: '1.3.6(35)',
  design_mah: 5000,
  full_mah: 4800,
  cycles: 210,
  pct: 96,
  est_mah: 4500,
  samples: 42,
  cycle_equiv: 1.42,
  r_moh: 88.4,
  temp_c: 31.5,
  trend_mah_per_week: -6.2,
  trend_state: 'significant',
  trend_span_day: 21,
  quality: { ok: true, spread_pct: 4, ema_mah: 4500, mid_mah: 4600, dis_mah: 4480, full_mah: 4800 },
  updated: '2026-10-27 19:00',
  samples_n: 1420,
  sessions_n: 37,
  calib: {
    current_node: '/sys/class/power_supply/battery/current_now',
    counter_node: '/sys/class/power_supply/battery/charge_counter',
    current_scale: 1000,
    current_unit: 'ma',
    raw_current: 3000,
    current_ua: 3000000,
  },
  recent: [
    { ts: now - day * 4, mah: 4620 },
    { ts: now - day * 3, mah: 4560 },
    { ts: now - day * 2, mah: 4510 },
    { ts: now - day, mah: 4500 },
  ],
  sessions: [],
  rest_points: [],
  ccct: [],
  ica_peaks: [],
  discharge_recent: [],
};

const sessions = [
  {
    start_ts: now - day * 2 - 3600,
    end_ts: now - day * 2,
    start_cap: 30,
    end_cap: 68,
    delta_pct: 38,
    est_mah: 4480,
    valid: true,
    temp_avg: 30,
    c_rate: 0.42,
  },
  {
    start_ts: now - day - 2800,
    end_ts: now - day,
    start_cap: 30,
    end_cap: 76,
    delta_pct: 46,
    est_mah: 4510,
    valid: true,
    temp_avg: 32,
    c_rate: 0.5,
  },
  {
    start_ts: now - 3600,
    end_ts: now,
    start_cap: 44,
    end_cap: 96,
    delta_pct: 52,
    est_mah: 4530,
    valid: true,
    temp_avg: 31,
    c_rate: 0.48,
  },
];
const discharges = [
  {
    ts: now - day * 2 - 7200,
    secs: 5400,
    uah_mah: 2100,
    start_cap: 80,
    end_cap: 58,
    implied_mah: 4560,
  },
  {
    ts: now - day - 7200,
    secs: 6200,
    uah_mah: 2350,
    start_cap: 76,
    end_cap: 51,
    implied_mah: 4480,
  },
];
const restPoints = [
  { ts: now - day * 2, uv: 3_890_000, cap: 55 },
  { ts: now - day, uv: 3_910_000, cap: 52 },
];
const ccct = [
  { ts: now - day * 2, secs: 720, vw_lo: 4_200_000, vw_hi: 4_300_000 },
  { ts: now - day, secs: 780, vw_lo: 4_200_000, vw_hi: 4_300_000 },
];
const ica = [
  { ts: now - day * 2, peak_uv: 4_200_000, peak_h_rel: 1.08 },
  { ts: now - day, peak_uv: 4_210_000, peak_h_rel: 1.12 },
];

const events = [
  { kind: 'charge', ts: sessions[2].end_ts, charge: sessions[2] },
  { kind: 'discharge', ts: discharges[1].ts, discharge: discharges[1] },
  { kind: 'charge', ts: sessions[1].end_ts, charge: sessions[1] },
  { kind: 'discharge', ts: discharges[0].ts, discharge: discharges[0] },
  { kind: 'charge', ts: sessions[0].end_ts, charge: sessions[0] },
];

const SMOKE_DATA = {
  json: jsonDoc,
  diag:
    'CRecordEvolution diag（已脱敏：无序列号/IMEI/MAC/账号）\n' +
    'module=1.3.6(35) channel=stable root=KernelSU android=16(sdk36) abi=arm64\n' +
    'device=OnePlus PJE110 (aston)\n' +
    'calib=x1000 ma I_node=/sys/class/power_supply/battery/current_now\n' +
    'est=5273mAh samples=12 cycleEq=1.42\n' +
    'events7d=cur_cal=1 counter_scale=2 quality_warn=0 tick_error=0\n',
  events,
  summary: {
    days: 30,
    since: now - day * 30,
    until: now,
    sessions: sessions.length,
    valid_sessions: sessions.length,
    discharges: discharges.length,
    samples: 15432,
    oldest_ts: now - day * 29,
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
window.__crCommands = [];
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
    window.__crCommands.push(cmd);
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

// 截图前回到顶部，避免 Playwright 的 fullPage 把 fixed 底栏画在滚动后的位置
async function shot(page, name) {
  await page.evaluate(() => window.scrollTo(0, 0));
  await sleep(250);
  await page.screenshot({ path: join(SHOTS, name), fullPage: true });
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

const failures = [];
function check(ok, label, extra = '') {
  console.log(`${ok ? 'PASS' : 'FAIL'} ${label}${extra ? ' — ' + extra : ''}`);
  if (!ok) failures.push(label);
}

const browser = await launch();
const external = [];
const consoleErrors = [];
let base = '';

try {
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  const port = server.address().port;
  base = `http://127.0.0.1:${port}`;

  const context = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 2,
  });
  await context.addInitScript(HOST_STUB(SMOKE_DATA));
  // 无头环境没有真实剪贴板权限：注入 writeText 桩，让“复制诊断”成功路径可断言。
  await context.addInitScript(() => {
    try {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: { writeText: () => Promise.resolve() },
      });
    } catch (e) {
      /* 老浏览器忽略，走 execCommand 回退 */
    }
  });
  const page = await context.newPage();
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
  });
  page.on('pageerror', (e) => consoleErrors.push(String(e)));
  page.on('request', (req) => {
    const u = req.url();
    if (!u.startsWith(base) && !u.startsWith('data:') && !u.startsWith('blob:')) external.push(u);
  });

  await page.goto(base + '/', { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('#page_overview.is-active', { timeout: 15000 });
  await page.waitForSelector('#page_overview .hero-line', { timeout: 15000 });
  await sleep(300);
  check((await page.locator('#page_overview .cell').count()) >= 6, '概览：指标格渲染');
  check(
    (await page.locator('#page_overview .hero-line').innerText()).includes('%'),
    '概览：健康度百分比'
  );
  check((await page.locator('#nav_overview').count()) === 1, '底栏：三页导航存在');
  await shot(page, 'overview.png');

  await page.click('#nav_history');
  await page.waitForSelector('#page_history.is-active');
  await page.waitForSelector('#page_history .tl-item', { timeout: 15000 });
  check((await page.locator('#page_history .hist-stat').count()) === 3, '历史：三张摘要卡');
  check((await page.locator('#page_history .tl-item').count()) >= 3, '历史：时间轴记录');
  check((await page.locator('#page_history .hist-chips--single').count()) === 1, '历史：筛选单行');
  await page.locator('#page_history .tl-card').first().click();
  await page.waitForSelector('#page_history .tl-det', { timeout: 5000 });
  check(true, '历史：记录展开详情');
  const beforeCalls = (
    await page.evaluate(() => window.__crCommands.filter((c) => c.indexOf('history') >= 0))
  ).length;
  if (await page.locator('#page_history .hist-foot .load-more').count()) {
    await page.locator('#page_history .hist-foot .load-more').click();
    await page.waitForFunction(
      () => document.querySelectorAll('#page_history .hist-foot .load-more').length === 0,
      null,
      { timeout: 10000 }
    );
    const afterCalls = (
      await page.evaluate(() => window.__crCommands.filter((c) => c.indexOf('history') >= 0))
    ).length;
    check(afterCalls > beforeCalls, '历史：加载更早记录（游标分页）');
  } else {
    check(false, '历史：加载更早记录按钮存在');
  }
  await shot(page, 'history.png');

  await page.locator('#hist_mode_seg .seg-btn').nth(1).click();
  await page.waitForSelector('#page_history .trend-wrap', { timeout: 15000 });
  check((await page.locator('#page_history .trend-hero').count()) === 2, '趋势：两张主卡');
  check((await page.locator('#page_history .trend-tile').count()) === 4, '趋势：四张小图');
  await page.locator('#page_history .trend-tile').first().click();
  await page.waitForSelector('#page_history .trend-tile.is-open .trend-big', { timeout: 5000 });
  check(true, '趋势：小图展开大图');
  await shot(page, 'trends.png');

  await page.click('#nav_settings');
  await page.waitForSelector('#page_settings.is-active');
  check((await page.locator('#theme_seg .seg-btn').count()) === 3, '设置：主题三档');
  check(
    (await page.locator('#page_settings [data-field="current_node"]').count()) === 1,
    '设置：电流采样节点行'
  );
  check(
    (await page.locator('#page_settings [data-field="current_calib"]').count()) === 1,
    '设置：电流刻度行'
  );
  check(
    (await page.locator('#page_settings [data-field="current_node"]').innerText()).includes(
      'battery/current_now'
    ),
    '设置：电流采样节点值'
  );
  const calibText = await page.locator('#page_settings [data-field="current_calib"]').innerText();
  check(calibText.includes('mA'), '设置：电流刻度单位');
  check(calibText.includes('1000'), '设置：电流刻度数值');
  check(
    (await page.locator('#page_settings [data-field="sessions"]').innerText()).includes('37'),
    '设置：会话数使用 sessions_n 总数'
  );
  check((await page.locator('#page_settings #diag').count()) === 1, '设置：诊断复制按钮');
  await page.click('#page_settings #diag');
  await page.waitForSelector('#page_settings #diag-status', { timeout: 5000 });
  check(
    (await page.locator('#page_settings #diag-status').innerText()).length > 0,
    '设置：诊断复制结果提示'
  );
  check(
    (await page.evaluate(
      () => window.__crCommands.filter((c) => c.indexOf('batteryd diag') >= 0).length
    )) === 1,
    '设置：诊断命令已执行'
  );
  check(
    (await page.locator('#page_settings #diag-text').count()) === 0,
    '设置：复制成功无手动文本'
  );
  check((await page.locator('#page_settings .about-hero').count()) === 1, '设置：关于品牌卡');
  check(
    (await page.locator('#page_settings .about-chip').count()) === 2,
    '设置：关于通道/版本胶囊'
  );
  check(
    (await page.locator('#page_settings [data-field="version"]').innerText()).includes('1.3.6'),
    '设置：关于版本信息'
  );
  await page.locator('#theme_seg .seg-btn').nth(1).click();
  await page.waitForFunction(() => document.documentElement.getAttribute('data-theme') === 'light');
  check(true, '设置：切换浅色主题');
  await page.locator('#theme_seg .seg-btn').nth(2).click();
  await page.waitForFunction(() => document.documentElement.getAttribute('data-theme') === 'dark');
  check(true, '设置：切换深色主题');

  await page.locator('#lang_seg .seg-btn').nth(0).click();
  await page.waitForFunction(() => document.documentElement.lang === 'zh-CN');
  check(
    (await page.locator('#page_settings .settings-title').first().innerText()) === '外观',
    '设置：切换中文'
  );
  await page.waitForFunction(
    () => window.__crCommands.some((c) => c.indexOf('batteryd once') >= 0),
    null,
    { timeout: 5000 }
  );
  check(
    await page.evaluate(() => window.__crCommands.some((c) => c.indexOf('batteryd once') >= 0)),
    '设置：切换语言后刷新模块描述'
  );
  await shot(page, 'settings.png');

  check(consoleErrors.length === 0, '控制台无报错', consoleErrors.join(' | '));
  check(external.length === 0, '无远程网络请求', external.join(' | '));

  await context.close();

  // 普通浏览器（无 ksu）：必须显示 gate 页而不是白屏
  const gateCtx = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 390, height: 844 },
  });
  const gatePage = await gateCtx.newPage();
  const gateErrors = [];
  gatePage.on('pageerror', (e) => gateErrors.push(String(e)));
  await gatePage.goto(base + '/', { waitUntil: 'domcontentloaded' });
  await gatePage.waitForSelector('.gate', { timeout: 10000 });
  check((await gatePage.locator('.gate-title').innerText()).length > 0, '无宿主：显示 gate 页');
  check((await gatePage.locator('#page_overview').count()) === 0, '无宿主：不渲染数据页');
  await shot(gatePage, 'gate.png');
  check(gateErrors.length === 0, 'gate 页无 JS 报错', gateErrors.join(' | '));
  await gateCtx.close();
} finally {
  await browser.close();
  server.close();
}

if (failures.length) {
  console.error(`\n${failures.length} 项失败：${failures.join('、')}`);
  process.exit(1);
}
console.log(`\n冒烟检查通过，截图在 ${SHOTS}`);
