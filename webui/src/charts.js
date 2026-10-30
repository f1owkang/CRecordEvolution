function spanPad(p) {
  return 0.05 + p * 0.9;
}

// pts: [{x,y}] 已归一化到 [0,1]
function ptsToPath(pts, w, h) {
  return pts
    .map(function (p, i) {
      return (i ? 'L' : 'M') + (p.x * w).toFixed(1) + ',' + ((1 - p.y) * h).toFixed(1);
    })
    .join('');
}

// 单调三次插值（Fritsch–Carlson）：曲线平滑但不过冲，适合容量/耗时这类
// 不该在相邻数据点之间冒出虚假峰谷的序列；两点时退化为直线，重复 x 或
// 非正间隔的段落回退为直线段。输入 pts 已归一化到 [0,1]。
function smoothPath(pts, w, h) {
  const n = pts.length;
  if (n < 2) return '';
  let xs = [],
    ys = [],
    i;
  for (i = 0; i < n; i++) {
    xs.push(pts[i].x * w);
    ys.push((1 - pts[i].y) * h);
  }
  const d = [],
    m = [];
  for (i = 0; i < n - 1; i++) {
    let dx = xs[i + 1] - xs[i];
    d.push(dx > 0 ? (ys[i + 1] - ys[i]) / dx : 0);
  }
  m[0] = d[0];
  for (i = 1; i < n - 1; i++) {
    // 极值点切线归零，避免曲线在点间冲出数据范围
    m[i] = d[i - 1] * d[i] <= 0 ? 0 : (d[i - 1] + d[i]) / 2;
  }
  m[n - 1] = d[n - 2];
  // Fritsch–Carlson 限幅：|m| ≤ 3·|delta|
  for (i = 0; i < n - 1; i++) {
    if (d[i] === 0) {
      m[i] = 0;
      m[i + 1] = 0;
      continue;
    }
    let a = m[i] / d[i],
      b = m[i + 1] / d[i];
    let s = a * a + b * b;
    if (s > 9) {
      let sc = 3 / Math.sqrt(s);
      m[i] = sc * a * d[i];
      m[i + 1] = sc * b * d[i];
    }
  }
  let out = 'M' + xs[0].toFixed(1) + ',' + ys[0].toFixed(1);
  for (i = 0; i < n - 1; i++) {
    let dx2 = xs[i + 1] - xs[i];
    if (!(dx2 > 0)) {
      out += 'L' + xs[i + 1].toFixed(1) + ',' + ys[i + 1].toFixed(1);
      continue;
    }
    let c1x = xs[i] + dx2 / 3,
      c1y = ys[i] + (m[i] * dx2) / 3;
    let c2x = xs[i + 1] - dx2 / 3,
      c2y = ys[i + 1] - (m[i + 1] * dx2) / 3;
    out +=
      'C' +
      c1x.toFixed(1) +
      ',' +
      c1y.toFixed(1) +
      ' ' +
      c2x.toFixed(1) +
      ',' +
      c2y.toFixed(1) +
      ' ' +
      xs[i + 1].toFixed(1) +
      ',' +
      ys[i + 1].toFixed(1);
  }
  return out;
}

function svgOpen(w, h) {
  return '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ' + w + ' ' + h + '" role="img">';
}

function chartBase(w, h) {
  return (
    '<line x1="0" y1="' +
    (h - 0.5) +
    '" x2="' +
    w +
    '" y2="' +
    (h - 0.5) +
    '" style="stroke:var(--chart-base)" stroke-width="1"/>'
  );
}

function dotEl(x, y, colorVar) {
  return (
    '<circle cx="' +
    x.toFixed(1) +
    '" cy="' +
    y.toFixed(1) +
    '" r="3" style="fill:' +
    colorVar +
    '" fill-opacity=".9"/>'
  );
}

// 选中点高亮：只拼内部坐标，颜色引用主题变量。
// pointer-events 关闭，避免高亮圈盖住下面带 data-i 的透明命中圈。
// SVG 根节点默认按 viewBox 裁剪，最高/最低点若贴着上下边，固定 r=6.5 的圈会被
// 切掉一截（真机点选顶部点可见）：按到四条边的距离收小半径，保证整圈都在画布内。
function selRing(cx, cy, w, h) {
  const margin = 2; // 预留 stroke-width/2 再加 1px 余量
  const r = Math.max(2.5, Math.min(6.5, Math.min(cx, w - cx, cy, h - cy) - margin)).toFixed(1);
  return (
    '<circle class="pt-sel" cx="' +
    cx +
    '" cy="' +
    cy +
    '" r="' +
    r +
    '" fill="none" style="stroke:var(--fg)" stroke-width="2" stroke-opacity=".85" pointer-events="none"/>'
  );
}

// 选中下标只有在整数且落在数据范围内才生效；越界/未选返回 -1。
function selIndex(sel, count) {
  const i = Number(sel);
  if (sel == null || !Number.isInteger(i) || i < 0 || i >= count) return -1;
  return i;
}

function pickPairs(rows, kx, ky) {
  let out = [],
    i,
    r,
    x,
    y;
  for (i = 0; i < rows.length; i++) {
    r = rows[i];
    x = Number(r == null ? null : r[kx]);
    y = Number(r == null ? null : r[ky]);
    if (isFinite(x) && isFinite(y)) out.push({ x: x, y: y, row: r });
  }
  return out;
}

// 图表内部按 ts 升序绘制；点按详情必须用同一份升序数组，否则 data-i 会
// 与后端倒序数组错位（点最左的数据点却显示最右那条记录）。
function ascByTs(rows) {
  return (Array.isArray(rows) ? rows : []).slice().sort(function (a, b) {
    return Number(a && a.ts) - Number(b && b.ts);
  });
}

function minMax(vals) {
  let mn = Infinity,
    mx = -Infinity,
    i;
  for (i = 0; i < vals.length; i++) {
    if (vals[i] < mn) mn = vals[i];
    if (vals[i] > mx) mx = vals[i];
  }
  return { min: mn, max: mx };
}

function normPt(v, mn, mx) {
  if (!(mx > mn)) return 0.5;
  return (v - mn) / (mx - mn);
}

// data: [{ts,v}]，自动 min/max 归一化；0 点返回 ''（由调用方决定占位/隐藏），仅 1 点渲染单点
// trend: {from,to} 为后端拟合线在首/末数据点时刻上的容量（mAh）；给出时画虚线。
// 不要把斜率交给前端外推：只给斜率时只能拿首个实际点当锚点，虚线会整体偏移，
// 也和后端拟合窗口（与 data 同一份 estimates）对不上。
function svgLine(data, w, h, color, trend, sel) {
  const raw = pickPairs(Array.isArray(data) ? data : [], 'ts', 'mah');
  if (!raw.length) return '';
  raw.sort(function (a, b) {
    return a.x - b.x;
  });
  const si = selIndex(sel, raw.length);
  if (raw.length === 1) {
    const sx = spanPad(0.5) * w,
      sy = (1 - spanPad(0.5)) * h;
    return (
      svgOpen(w, h) +
      chartBase(w, h) +
      dotEl(sx, sy, color) +
      (si === 0 ? selRing(sx.toFixed(1), sy.toFixed(1), w, h) : '') +
      '<circle class="pt" data-i="0" cx="' +
      sx.toFixed(1) +
      '" cy="' +
      sy.toFixed(1) +
      '" r="12" fill="transparent"/></svg>'
    );
  }
  const tx = minMax(
    raw.map(function (p) {
      return p.x;
    })
  );
  const ys = raw.map(function (p) {
    return p.y;
  });
  let trendPts = null;
  if (trend && isFinite(trend.from) && isFinite(trend.to)) {
    trendPts = [
      { x: raw[0].x, y: trend.from },
      { x: tx.max, y: trend.to },
    ];
    ys.push(trendPts[0].y, trendPts[1].y);
  }
  const ty = minMax(ys);
  function toXY(p) {
    return { x: spanPad(normPt(p.x, tx.min, tx.max)), y: spanPad(normPt(p.y, ty.min, ty.max)) };
  }
  const pts = raw.map(toXY);
  // 可见数据点 + 透明命中圈：点按看该次明细（与 svgScatter 同口径）。
  // 长历史（>40 点）缩小可见点半径，避免点连成一片；命中圈保持 12px。
  const dotR = pts.length > 40 ? 2 : 3;
  let dots = '',
    ring = '';
  for (let i = 0; i < pts.length; i++) {
    let cx = (pts[i].x * w).toFixed(1);
    let cy = ((1 - pts[i].y) * h).toFixed(1);
    if (i === si) ring = selRing(cx, cy, w, h);
    dots +=
      '<circle class="pt" data-i="' +
      i +
      '" cx="' +
      cx +
      '" cy="' +
      cy +
      '" r="12" fill="transparent"/>' +
      '<circle cx="' +
      cx +
      '" cy="' +
      cy +
      '" r="' +
      dotR +
      '" style="fill:' +
      color +
      '" fill-opacity=".85" pointer-events="none"/>';
  }
  let overlay = '';
  if (trendPts) {
    overlay =
      '<path d="' +
      ptsToPath(trendPts.map(toXY), w, h) +
      '" fill="none" style="stroke:var(--blue)" stroke-width="1.5" stroke-dasharray="6,4" stroke-linecap="round"/>';
  }
  return (
    svgOpen(w, h) +
    chartBase(w, h) +
    overlay +
    '<path d="' +
    smoothPath(pts, w, h) +
    '" fill="none" style="stroke:' +
    color +
    '" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>' +
    dots +
    ring +
    '</svg>'
  );
}

// rest_points 散点：每点一个可见 <circle> 叠一层透明命中圈（点按看详情）；
// 仅 1 点也渲染（单点居中）
function svgScatter(data, w, h, color, sel) {
  const raw = pickPairs(Array.isArray(data) ? data : [], 'ts', 'uv');
  if (!raw.length) return '';
  const tx = minMax(
    raw.map(function (p) {
      return p.x;
    })
  );
  const ty = minMax(
    raw.map(function (p) {
      return p.y;
    })
  );
  const si = selIndex(sel, raw.length);
  let dots = '',
    ring = '';
  for (let i = 0; i < raw.length; i++) {
    let cx = (spanPad(normPt(raw[i].x, tx.min, tx.max)) * w).toFixed(1);
    let cy = ((1 - spanPad(normPt(raw[i].y, ty.min, ty.max))) * h).toFixed(1);
    if (i === si) ring = selRing(cx, cy, w, h);
    dots +=
      '<circle class="pt" data-i="' +
      i +
      '" cx="' +
      cx +
      '" cy="' +
      cy +
      '" r="12" fill="transparent"/>' +
      '<circle cx="' +
      cx +
      '" cy="' +
      cy +
      '" r="3" style="fill:' +
      color +
      '" fill-opacity=".85" pointer-events="none"/>';
  }
  return svgOpen(w, h) + chartBase(w, h) + dots + ring + '</svg>';
}

// sessions delta 条形：每条会话一个 <rect>，高度按 delta_pct 占比；仅 1 条也渲染。
// 可见柱不接事件，命中用铺满槽位的透明 <rect>（data-i 与传入的升序数组对齐）。
function svgBars(data, w, h, color, sel) {
  const rows = Array.isArray(data) ? data : [];
  let vals = [],
    i,
    v;
  for (i = 0; i < rows.length; i++) {
    v = Number(rows[i] == null ? null : rows[i].delta_pct);
    if (isFinite(v)) vals.push(v);
  }
  if (!vals.length) return '';
  let mx = minMax(vals).max;
  if (!(mx > 0)) mx = 1;
  const slot = w / vals.length;
  const bw = Math.min(slot * 0.62, 26);
  const si = selIndex(sel, vals.length);
  let rects = '',
    hits = '',
    ring = '';
  for (i = 0; i < vals.length; i++) {
    let bh = Math.max((vals[i] / mx) * (h - 6), 2);
    let bx = i * slot + (slot - bw) / 2;
    let by = h - bh;
    if (i === si) ring = selRing((i * slot + slot / 2).toFixed(1), by.toFixed(1), w, h);
    rects +=
      '<rect x="' +
      bx.toFixed(1) +
      '" y="' +
      by.toFixed(1) +
      '" width="' +
      bw.toFixed(1) +
      '" height="' +
      bh.toFixed(1) +
      '" rx="3" style="fill:' +
      color +
      '" pointer-events="none"/>';
    hits +=
      '<rect class="pt" data-i="' +
      i +
      '" x="' +
      (i * slot).toFixed(1) +
      '" y="0" width="' +
      slot.toFixed(1) +
      '" height="' +
      h +
      '" fill="transparent"/>';
  }
  return svgOpen(w, h) + chartBase(w, h) + rects + hits + ring + '</svg>';
}

// ts→key 通用曲线：自动 min/max 归一化；refY 有效时纳入取值范围并以虚线画出参考线；
// 仅 1 点渲染单点（含参考线）
function svgTsLine(data, w, h, color, key, refY, sel) {
  const raw = pickPairs(Array.isArray(data) ? data : [], 'ts', key);
  if (!raw.length) return '';
  raw.sort(function (a, b) {
    return a.x - b.x;
  });
  const tx = minMax(
    raw.map(function (p) {
      return p.x;
    })
  );
  const ys = raw.map(function (p) {
    return p.y;
  });
  const hasRef = typeof refY === 'number' && isFinite(refY);
  if (hasRef) ys.push(refY);
  const ty = minMax(ys);
  function toXY(p) {
    return { x: spanPad(normPt(p.x, tx.min, tx.max)), y: spanPad(normPt(p.y, ty.min, ty.max)) };
  }
  let refLine = '';
  if (hasRef) {
    const ry = ((1 - toXY({ x: 0, y: refY }).y) * h).toFixed(1);
    refLine =
      '<line x1="' +
      (w * 0.05).toFixed(1) +
      '" y1="' +
      ry +
      '" x2="' +
      (w * 0.95).toFixed(1) +
      '" y2="' +
      ry +
      '" style="stroke:var(--chart-ref)" stroke-width="1.5" stroke-dasharray="5,4"/>';
  }
  let body;
  // 数据点标记 + 透明命中圈（点按看详情），与折线同色
  const si = selIndex(sel, raw.length);
  let marks = '',
    ring = '';
  for (let i = 0; i < raw.length; i++) {
    let p = toXY(raw[i]);
    let cx = (p.x * w).toFixed(1),
      cy = ((1 - p.y) * h).toFixed(1);
    if (i === si) ring = selRing(cx, cy, w, h);
    marks +=
      '<circle class="pt" data-i="' +
      i +
      '" cx="' +
      cx +
      '" cy="' +
      cy +
      '" r="12" fill="transparent"/>' +
      '<circle cx="' +
      cx +
      '" cy="' +
      cy +
      '" r="2.5" style="fill:' +
      color +
      '" pointer-events="none"/>';
  }
  if (raw.length >= 2) {
    body =
      '<path d="' +
      smoothPath(raw.map(toXY), w, h) +
      '" fill="none" style="stroke:' +
      color +
      '" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>' +
      marks +
      ring;
  } else {
    body = marks + ring;
  }
  return svgOpen(w, h) + chartBase(w, h) + refLine + body + '</svg>';
}

export { pickPairs, ascByTs, svgLine, svgScatter, svgBars, svgTsLine };
