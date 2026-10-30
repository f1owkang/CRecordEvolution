// 图表点按的公共规则：沿 target 向上找最近的 data-i（charts.js 生成的透明
// 命中圈），返回该点的下标；点到空白返回 null，由调用方清除选中。
// 概览 ChartCard 与历史趋势图共用，避免两处各写一套遍历逻辑。
function tapIndex(e, count) {
  let node = e.target;
  while (
    node &&
    node !== e.currentTarget &&
    !(node.getAttribute && node.getAttribute('data-i') != null)
  ) {
    node = node.parentNode;
  }
  const attr = node && node.getAttribute ? node.getAttribute('data-i') : null;
  const i = attr == null ? NaN : Number(attr);
  return Number.isInteger(i) && i >= 0 && i < count ? i : null;
}

// 选中下标 → 该点读数；未选中或下标越界时返回 null（调用方显示统一提示文案）。
function selectedText(rows, format, index) {
  if (index == null || index < 0 || index >= rows.length) return null;
  return format(rows[index]);
}

export { tapIndex, selectedText };
