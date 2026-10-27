// 历史页摘要区的统计卡（HistStat）：上数值、下标题的两行布局，三张卡横排。
// 本文件用 JSX 写，是 @preact/preset-vite 的 JSX 构建路径的参照实现；
// 其余组件仍用 h() 工厂（见 ui/context.js 的 E）。
export function HistStat({ value, label }) {
  return (
    <div class="hist-stat">
      <div class="n">{value}</div>
      <div class="l">{label}</div>
    </div>
  );
}
