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
  // 趋势虚线端点：与 recent 同一份数据拟合（后端 trend_from/to），前端直接画线
  trend_from_mah: 4600,
  trend_to_mah: 4490,
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
  // 充电中实时数据：概览页实时卡与 15s 轮询都消费这两个字段；
  // daemon.stale 触发设置页「已升级，重启后生效」行。
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
  daemon: { version: '1.3.6(35)', ts: now, age_sec: 12, stale: true },
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
// 游标分页的第二页：比第一页更早的 5 条事件，用于断言「第 2 页记录」在切页后仍在、
// 以及分页失败/重试恢复；同时把历史页撑高，让滚动位置恢复可被断言。
const olderEvents = [
  {
    kind: 'charge',
    ts: now - day * 8,
    charge: { ...sessions[0], start_ts: now - day * 8 - 5400, end_ts: now - day * 8 },
  },
  {
    kind: 'discharge',
    ts: now - day * 9,
    discharge: { ...discharges[0], ts: now - day * 9 },
  },
  {
    kind: 'charge',
    ts: now - day * 10,
    charge: { ...sessions[1], start_ts: now - day * 10 - 6000, end_ts: now - day * 10 },
  },
  {
    kind: 'discharge',
    ts: now - day * 11,
    discharge: { ...discharges[1], ts: now - day * 11 },
  },
  {
    kind: 'charge',
    ts: now - day * 12,
    charge: { ...sessions[2], start_ts: now - day * 12 - 4000, end_ts: now - day * 12 },
  },
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
  olderEvents,
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
var D = (window.__crData = ${JSON.stringify(data)});
window.__crCommands = [];
window.__crHistoryFail = false;
function crHistoryDoc(cmd) {
  var type = (/--type\\s+(\\w+)/.exec(cmd) || [])[1] || 'all';
  var before = Number((/--before\\s+(\\d+)/.exec(cmd) || [])[1] || 0);
  // 第一页返回 D.events，带 before 游标的第二页返回更早的 D.olderEvents。
  var list = before ? D.olderEvents : D.events;
  var events = list.filter(function (ev) { return type === 'all' || ev.kind === type; });
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
    else if (cmd.indexOf('batteryd history') >= 0) {
      // 冒烟可打开 __crHistoryFail 模拟「加载更早」失败，用于错误行与重试断言。
      if (window.__crHistoryFail) { errno = 1; out = 'history failed'; }
      else out = JSON.stringify(crHistoryDoc(cmd));
    }
    else if (cmd.indexOf('cat ') === 0) { errno = 1; }
    setTimeout(function () {
      var fn = typeof cb === 'string' ? window[cb] : (typeof opts === 'function' ? opts : cb);
      if (typeof fn === 'function') fn(errno, out, errno ? (out || 'no file') : '');
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
  check(
    (await page.locator('#page_overview .chart svg path[stroke-dasharray]').count()) === 1,
    '概览：估算趋势虚线'
  );
  check((await page.locator('#page_overview .hero-temp').count()) === 1, '概览：温度行在主卡内');
  const heroTemp = await page.locator('#page_overview .hero-temp').innerText();
  check(heroTemp.includes('31.5') && heroTemp.includes('℃'), '概览：温度数值与单位', heroTemp);
  // 充电中实时卡：状态 / 电流 / 功率 / 电压 / 会话进度 / 预计充满
  check(
    (await page.locator('#page_overview .charge-card').count()) === 1,
    '概览：充电中实时卡渲染'
  );
  const chargeText = await page.locator('#page_overview .charge-card').innerText();
  check(chargeText.includes('2.45 A'), '概览：实时卡电流与单位', chargeText);
  check(chargeText.includes('10.3 W'), '概览：实时卡功率与单位');
  check(chargeText.includes('4.21 V'), '概览：实时卡电压与单位');
  check(
    chargeText.includes('61%→76%') &&
      chargeText.includes('1,234 mAh') &&
      chargeText.includes('12 min'),
    '概览：实时卡本次会话进度'
  );
  check(chargeText.includes('≈42 min to full'), '概览：实时卡预计充满');
  // 会话进度与预计充满合并成一行：nowrap + 390 宽下不溢出 + 单行高度
  const metaFit = await page.evaluate(() => {
    const el = document.querySelector('#page_overview .charge-meta');
    if (!el) return null;
    return {
      ws: getComputedStyle(el).whiteSpace,
      overflow: el.scrollWidth - el.clientWidth,
      height: el.offsetHeight,
    };
  });
  check(
    metaFit != null && metaFit.ws === 'nowrap' && metaFit.overflow <= 1 && metaFit.height <= 32,
    '概览：会话信息与预计充满同一行不溢出',
    JSON.stringify(metaFit)
  );
  check(
    (await page.locator('#page_overview .chart svg[aria-hidden="true"]').count()) === 1,
    '概览：图表按装饰性处理'
  );
  check(
    (await page.locator('#page_overview .cell[role="button"]').count()) === 1,
    '概览：可信度格可交互语义'
  );
  check(
    (await page.locator('#page_overview .cell[role="button"] .hint').innerText()).length > 0,
    '概览：可信度格点按提示'
  );
  check((await page.locator('#nav_overview').count()) === 1, '底栏：三页导航存在');
  await shot(page, 'overview.png');

  await page.click('#nav_history');
  await page.waitForSelector('#page_history.is-active');
  await page.waitForSelector('#page_history .tl-item', { timeout: 15000 });
  check((await page.locator('#page_history .hist-stat').count()) === 3, '历史：三张摘要卡');
  check((await page.locator('#page_history .tl-item').count()) >= 3, '历史：时间轴记录');
  check((await page.locator('#page_history .hist-chips--single').count()) === 1, '历史：筛选单行');
  check(
    (await page.locator('#page_history .hist-chips').getAttribute('aria-label')) ===
      'Time range and record type',
    '历史：筛选组 aria-label'
  );
  check(
    (await page.locator('#hist_mode_seg .seg-btn[aria-pressed="true"]').count()) === 1,
    '历史：分段选中语义 aria-pressed'
  );
  const tlCounts = await page.locator('#page_history .tl-count').allInnerTexts();
  check(
    !tlCounts.includes('1 records') && tlCounts.includes('1 record'),
    '历史：记录数英文单复数',
    tlCounts.join(',')
  );
  // 触控目标按布局高度（offsetHeight）量：getBoundingClientRect 会被 :active 的
  // scale(0.97) 与切页淡入 transform 影响，不能用测量值断言实际布局尺寸。
  const targets = await page.evaluate(
    () =>
      new Promise((resolve) => {
        // 等两帧，避开切页淡入动画与 :active 状态的残留
        requestAnimationFrame(() =>
          requestAnimationFrame(() => {
            const chip = document.querySelector('#page_history .chip');
            const seg = document.querySelector('#hist_mode_seg .seg-btn');
            resolve({
              chip: chip ? chip.offsetHeight : 0,
              seg: seg ? seg.offsetHeight : 0,
            });
          })
        );
      })
  );
  check(
    targets.chip >= 40 && targets.seg >= 40,
    '历史：chip 与分段触控目标 ≥40px',
    JSON.stringify(targets)
  );
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
    check((await page.locator('#page_history .tl-item').count()) === 10, '历史：第二页记录已追加');
  } else {
    check(false, '历史：加载更早记录按钮存在');
  }
  await shot(page, 'history.png');

  // ① 切页返回保持分页 / 展开详情 / 滚动位置（不重置、不重新拉第一页）
  const histCallsBeforeSwitch = (
    await page.evaluate(() => window.__crCommands.filter((c) => c.indexOf('history') >= 0))
  ).length;
  const itemsBeforeSwitch = await page.locator('#page_history .tl-item').count();
  const detsBeforeSwitch = await page.locator('#page_history .tl-det').count();
  await page.evaluate(() => window.scrollTo(0, 320));
  await sleep(120);
  const scrollBeforeSwitch = await page.evaluate(() => window.scrollY);
  check(scrollBeforeSwitch > 100, '历史：列表滚动到中部', String(scrollBeforeSwitch));
  await page.click('#nav_settings');
  await page.waitForSelector('#page_settings.is-active');
  await page.click('#nav_history');
  await page.waitForSelector('#page_history.is-active');
  await sleep(400);
  const itemsAfterSwitch = await page.locator('#page_history .tl-item').count();
  const detsAfterSwitch = await page.locator('#page_history .tl-det').count();
  const histCallsAfterSwitch = (
    await page.evaluate(() => window.__crCommands.filter((c) => c.indexOf('history') >= 0))
  ).length;
  const scrollAfterSwitch = await page.evaluate(() => window.scrollY);
  check(
    itemsBeforeSwitch === 10 && itemsAfterSwitch === itemsBeforeSwitch,
    '历史：切页返回保留第 2 页记录',
    itemsBeforeSwitch + '→' + itemsAfterSwitch
  );
  check(detsBeforeSwitch === 1 && detsAfterSwitch === 1, '历史：切页返回保留展开详情');
  check(histCallsAfterSwitch === histCallsBeforeSwitch, '历史：切页返回不重新加载第一页');
  check(
    Math.abs(scrollAfterSwitch - scrollBeforeSwitch) <= 4,
    '历史：切页返回恢复滚动位置',
    scrollBeforeSwitch + '→' + scrollAfterSwitch
  );

  // ②「加载更早」失败：底部错误行 + 重试恢复；切 7 天会重新拉第一页（has_more 恢复）
  await page.locator('#page_history .hist-chips .chip').first().click();
  await page.waitForFunction(
    () => document.querySelectorAll('#page_history .tl-item').length === 5,
    null,
    { timeout: 10000 }
  );
  await page.evaluate(() => {
    window.__crHistoryFail = true;
  });
  await page.locator('#page_history .hist-foot .load-more').click();
  await page.waitForSelector('#page_history .hist-more-err', { timeout: 5000 });
  check(
    (await page.locator('#page_history .hist-more-err').count()) === 1,
    '历史：分页失败在列表底部提示'
  );
  check(
    (await page.locator('#page_history .hist-more-err').innerText()).includes('failed'),
    '历史：分页失败错误文案'
  );
  await page.evaluate(() => {
    window.__crHistoryFail = false;
  });
  await page.locator('#page_history .hist-more-err .load-more').click();
  await page.waitForFunction(
    () =>
      document.querySelectorAll('#page_history .hist-more-err').length === 0 &&
      document.querySelectorAll('#page_history .tl-item').length === 10,
    null,
    { timeout: 10000 }
  );
  check(
    (await page.locator('#page_history .hist-more-err').count()) === 0,
    '历史：重试后错误行消失'
  );
  check((await page.locator('#page_history .tl-item').count()) === 10, '历史：重试后加载更早成功');

  // 手动刷新（reloadKey）仍必须重载：列表重置回第一页
  const histCallsBeforeRefresh = (
    await page.evaluate(() => window.__crCommands.filter((c) => c.indexOf('history') >= 0))
  ).length;
  await page.click('#refreshTop');
  await page.waitForFunction(
    (n) =>
      window.__crCommands.filter((c) => c.indexOf('history') >= 0).length > n &&
      document.querySelectorAll('#page_history .tl-item').length === 5,
    histCallsBeforeRefresh,
    { timeout: 10000 }
  );
  check(
    (await page.locator('#page_history .tl-item').count()) === 5,
    '历史：手动刷新重新加载第一页'
  );

  // 切换「记录/趋势」会改变查询参数并重新拉一次 history；必须等该响应落地再点图表，
  // 否则响应替换 data 会清掉刚选中的点，断言会读到空读数。
  const histCallsBeforeTrends = (
    await page.evaluate(() => window.__crCommands.filter((c) => c.indexOf('history') >= 0))
  ).length;
  await page.locator('#hist_mode_seg .seg-btn').nth(1).click();
  await page.waitForSelector('#page_history .trend-wrap', { timeout: 15000 });
  await page.waitForFunction(
    (n) => window.__crCommands.filter((c) => c.indexOf('history') >= 0).length > n,
    histCallsBeforeTrends,
    { timeout: 10000 }
  );
  await sleep(150);
  check((await page.locator('#page_history .trend-hero').count()) === 2, '趋势：两张主卡');
  check((await page.locator('#page_history .trend-tile').count()) === 4, '趋势：四张小图');

  // 容量大图：点按数据点出现读数 + 高亮圈；点图表空白处清除选中回到提示文案
  const capHero = page.locator('#page_history .trend-hero').first();
  // 命中圈是相互重叠的透明圆：真实坐标点击可能落到相邻圈/空白上（选择器与点击之间
  // 的重排会放大误差）。用 dispatchEvent('click') 验证「选中 → 读数/高亮」渲染链，
  // 命中测试由下面的空白处真实点击与顶部点点击覆盖。
  await capHero.locator('.chart circle[data-i="2"]').dispatchEvent('click');
  // Preact 的 setState 是异步的：派发后立刻读 innerText 会读到上一帧，必须等渲染完成再断言。
  await page.waitForFunction(
    () => {
      const el = document.querySelector('#page_history .trend-hero .trend-det');
      return el && /mAh/.test(el.textContent);
    },
    null,
    { timeout: 3000 }
  );
  check(
    (await capHero.locator('.trend-det').innerText()).includes('mAh'),
    '趋势：容量大图点按读数'
  );
  check((await capHero.locator('circle.pt-sel').count()) === 1, '趋势：容量大图选中高亮');
  await shot(page, 'trends-tap.png');
  await capHero.locator('.chart').click({ position: { x: 4, y: 4 } });
  await page.waitForFunction(
    () => {
      const el = document.querySelector('#page_history .trend-hero .trend-det');
      return el && !/mAh/.test(el.textContent);
    },
    null,
    { timeout: 3000 }
  );
  check(!(await capHero.locator('.trend-det').innerText()).includes('mAh'), '趋势：点空白清除选中');

  // 顶部（最高值）点的高亮圈不能被 SVG 视口裁切：选中圈半径要按到画布四边的距离收小
  const topPoint = await capHero.evaluate((el) => {
    const circles = [...el.querySelectorAll('.chart circle.pt[data-i]')];
    let best = null;
    for (const c of circles) {
      if (best === null || Number(c.getAttribute('cy')) < Number(best.getAttribute('cy'))) best = c;
    }
    return best ? Number(best.getAttribute('data-i')) : null;
  });
  check(topPoint != null, '趋势：存在可点按数据点');
  await capHero.locator('.chart circle[data-i="' + topPoint + '"]').dispatchEvent('click');
  await page.waitForFunction(
    () => document.querySelectorAll('#page_history .trend-hero circle.pt-sel').length === 1,
    null,
    { timeout: 3000 }
  );
  const ringInside = await capHero.evaluate((el) => {
    const ring = el.querySelector('circle.pt-sel');
    const svg = ring && ring.ownerSVGElement;
    if (!ring || !svg) return false;
    const cy = Number(ring.getAttribute('cy'));
    const r = Number(ring.getAttribute('r'));
    const h = svg.viewBox.baseVal.height;
    return cy - r - 1 >= 0 && cy + r + 1 <= h;
  });
  check(ringInside, '趋势：顶部点高亮圈不被裁切');
  await shot(page, 'trends-top-tap.png');
  await capHero.locator('.chart').click({ position: { x: 4, y: 4 } });

  await page.locator('#page_history .trend-tile').first().click();
  await page.waitForSelector('#page_history .trend-tile.is-open .trend-big', { timeout: 5000 });
  check(true, '趋势：小图展开大图');
  const openTile = page.locator('#page_history .trend-tile.is-open');
  await openTile.locator('.trend-big circle[data-i="0"]').dispatchEvent('click');
  await page.waitForFunction(
    () => {
      const el = document.querySelector('#page_history .trend-tile.is-open .trend-det');
      return el && /mAh/.test(el.textContent);
    },
    null,
    { timeout: 3000 }
  );
  check(
    (await openTile.locator('.trend-det').innerText()).includes('mAh'),
    '趋势：展开大图点按读数'
  );
  check((await openTile.locator('circle.pt-sel').count()) === 1, '趋势：展开大图选中高亮');
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
  check(
    (await page.locator('#page_settings [data-field="daemon"]').count()) === 1,
    '设置：daemon 待重启提示行'
  );
  check(
    (await page.locator('#page_settings [data-field="daemon"]').innerText()).includes('restart'),
    '设置：daemon 待重启文案'
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
  check(
    (await page.locator('#page_settings [data-field="daemon"]').innerText()).includes('重启后生效'),
    '设置：daemon 待重启中文文案'
  );
  check(
    (
      await page.evaluate(() => {
        const el = document.querySelector('#page_overview .hero-temp');
        return el ? el.textContent : '';
      })
    ).includes('温度'),
    '概览：温度行中文文案'
  );
  const chargeZh = await page.evaluate(() => {
    const el = document.querySelector('#page_overview .charge-card');
    return el ? el.textContent : '';
  });
  check(
    chargeZh.includes('充电中') && chargeZh.includes('电流') && chargeZh.includes('分钟充满'),
    '概览：实时卡中文文案'
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

  // 无 live/daemon 场景：同一页面删掉 mock 的实时字段并强制刷新，实时卡与待重启行都必须消失。
  await page.evaluate(() => {
    delete window.__crData.json.live;
    delete window.__crData.json.daemon;
  });
  await page.click('#nav_overview');
  await page.waitForSelector('#page_overview.is-active');
  await page.click('#refreshTop');
  await page.waitForFunction(
    () => document.querySelectorAll('#page_overview .charge-card').length === 0,
    null,
    { timeout: 10000 }
  );
  check(
    (await page.locator('#page_overview .charge-card').count()) === 0,
    '概览：无 live 时充电卡隐藏'
  );
  await page.click('#nav_settings');
  await page.waitForSelector('#page_settings.is-active');
  check(
    (await page.locator('#page_settings [data-field="daemon"]').count()) === 0,
    '设置：无 daemon 时不显示待重启行'
  );

  // 中文文案：重新激活历史页后切回记录视图，校验温度单位空格与记录数文案
  await page.click('#nav_history');
  await page.waitForSelector('#page_history.is-active');
  await page.locator('#hist_mode_seg .seg-btn').nth(0).click();
  await page.waitForSelector('#page_history .tl-item', { timeout: 15000 });
  const zhSub = await page.locator('#page_history .tl-sub').first().innerText();
  check(/温度 \d+ ℃/.test(zhSub), '历史：中文温度单位空格', zhSub);
  const zhCounts = await page.locator('#page_history .tl-count').allInnerTexts();
  check(
    zhCounts.every((s) => /^共 \d+ 条$/.test(s)),
    '历史：中文记录数文案',
    zhCounts.join(',')
  );

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
