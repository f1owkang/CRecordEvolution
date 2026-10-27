// 历史页摘要区的统计卡（HistStat）：上数值、下标题的两行布局，三张卡横排。
// WebUI 组件统一使用 JSX（@preact/preset-vite automatic runtime）。
export function HistStat({ value, label }) {
  return (
    <div class="hist-stat">
      <div class="n">{value}</div>
      <div class="l">{label}</div>
    </div>
  );
}
