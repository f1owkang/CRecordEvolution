// 颜色解析与对比度工具：纯函数，供取色主题在应用宿主调色板时纠偏。
// 宿主的 M3 调色板由各管理器自己生成，角色取值不保证符合规范：实测
// KernelSU(Miuix) + Monochrome 下 --tertiary 为 #000000、--secondary 为
// #474747，作为图表线/柱颜色与 #0b0b0d 的卡片底色几乎同色（issue #13）。
// 这里只做配色计算，不读写 DOM。

function parseCssColor(value) {
  const s = String(value == null ? '' : value)
    .trim()
    .toLowerCase();
  if (!s) return null;
  if (s.charAt(0) === '#') {
    let h = s.slice(1);
    if (h.length === 3 || h.length === 4) {
      h = h
        .split('')
        .map(function (c) {
          return c + c;
        })
        .join('');
    }
    if (h.length !== 6 && h.length !== 8) return null;
    const r = parseInt(h.slice(0, 2), 16);
    const g = parseInt(h.slice(2, 4), 16);
    const b = parseInt(h.slice(4, 6), 16);
    if (isNaN(r) || isNaN(g) || isNaN(b)) return null;
    return { r: r, g: g, b: b };
  }
  // rgb()/rgba()：兼容逗号与空格分隔（含 `rgb(1 2 3 / .5)`）
  const m = s.match(
    /^rgba?\(\s*(\d+(?:\.\d+)?)\s*[\s,]\s*(\d+(?:\.\d+)?)\s*[\s,]\s*(\d+(?:\.\d+)?)/
  );
  if (!m) return null;
  const r = Number(m[1]);
  const g = Number(m[2]);
  const b = Number(m[3]);
  if (!isFinite(r) || !isFinite(g) || !isFinite(b)) return null;
  return { r: r, g: g, b: b };
}

function channelLum(v) {
  const n = v / 255;
  return n <= 0.04045 ? n / 12.92 : Math.pow((n + 0.055) / 1.055, 2.4);
}

// WCAG 相对亮度（0=黑，1=白）
function relativeLuminance(rgb) {
  return 0.2126 * channelLum(rgb.r) + 0.7152 * channelLum(rgb.g) + 0.0722 * channelLum(rgb.b);
}

// WCAG 对比度（1~21）；任一颜色解析失败返回 null。
function contrastRatio(a, b) {
  const ca = typeof a === 'object' && a ? a : parseCssColor(a);
  const cb = typeof b === 'object' && b ? b : parseCssColor(b);
  if (!ca || !cb) return null;
  const la = relativeLuminance(ca);
  const lb = relativeLuminance(cb);
  const hi = Math.max(la, lb);
  const lo = Math.min(la, lb);
  return (hi + 0.05) / (lo + 0.05);
}

function mixRgb(a, b, t) {
  return {
    r: a.r + (b.r - a.r) * t,
    g: a.g + (b.g - a.g) * t,
    b: a.b + (b.b - a.b) * t,
  };
}

function rgbToHex(rgb) {
  function hex(v) {
    return Math.round(Math.max(0, Math.min(255, v)))
      .toString(16)
      .padStart(2, '0');
  }
  return '#' + hex(rgb.r) + hex(rgb.g) + hex(rgb.b);
}

// 让 fg 相对 bg 至少达到 minRatio：已达标的保持原值（返回 null 表示“不用改”），
// 未达标的沿 toward（通常是宿主的 onSurface）混合到刚过阈值为止，保留原色相；
// toward 本身也不达标时改用黑/白兜底（任意底色的黑/白对比度上限都 ≥4.58）。
function ensureContrast(fg, bg, toward, minRatio) {
  const f = typeof fg === 'object' && fg ? fg : parseCssColor(fg);
  const b = typeof bg === 'object' && bg ? bg : parseCssColor(bg);
  const min = Number(minRatio);
  if (!f || !b || !isFinite(min) || min <= 1) return null;
  if (contrastRatio(f, b) >= min) return null;
  let target = typeof toward === 'object' && toward ? toward : parseCssColor(toward);
  if (!target || contrastRatio(target, b) < min) {
    target = relativeLuminance(b) > 0.4 ? { r: 0, g: 0, b: 0 } : { r: 255, g: 255, b: 255 };
  }
  if (contrastRatio(target, b) < min) return null;
  let lo = 0;
  let hi = 1;
  for (let i = 0; i < 24; i++) {
    const mid = (lo + hi) / 2;
    if (contrastRatio(mixRgb(f, target, mid), b) >= min) hi = mid;
    else lo = mid;
  }
  let out = mixRgb(f, target, hi);
  if (contrastRatio(out, b) < min) out = target;
  if (contrastRatio(out, b) < min) return null;
  return rgbToHex(out);
}

// 把 fg 朝 bg 方向混合到「对比度刚好不低于 minRatio」的最深一档，用作低强调色
// （例如未选中的柱）：既最大化与主色的层次差，又守住非文本图形 3:1 的下限。
// fg 本身已低于阈值（再由调用方负责纠偏）或颜色无法解析时返回 null。
function dimToContrast(fg, bg, minRatio) {
  const f = typeof fg === 'object' && fg ? fg : parseCssColor(fg);
  const b = typeof bg === 'object' && bg ? bg : parseCssColor(bg);
  const min = Number(minRatio);
  if (!f || !b || !isFinite(min) || min <= 1) return null;
  if (contrastRatio(f, b) < min) return null;
  let lo = 0;
  let hi = 1;
  for (let i = 0; i < 24; i++) {
    const mid = (lo + hi) / 2;
    if (contrastRatio(mixRgb(f, b, mid), b) >= min) lo = mid;
    else hi = mid;
  }
  // 8 位取整可能把结果压到阈值下一点，逐步回退到原始色，保证返回值仍 ≥ min。
  for (let i = 0; i < 12; i++) {
    const hex = rgbToHex(mixRgb(f, b, lo));
    if (contrastRatio(hex, b) >= min) return hex;
    lo = Math.max(0, lo - 0.02);
  }
  return rgbToHex(f);
}

export { parseCssColor, relativeLuminance, contrastRatio, ensureContrast, dimToContrast };
