// 历史页摘要区的统计卡（HistStat）：上数值、下标题的两行布局，三张卡横排。
// 单位统一写在标题里（如「总充入（mAh）」），数值列只放纯数字，窄屏不会断行。
export function HistStat({ value, label }) {
  return (
    <div class="hist-stat">
      <div class="n">{value}</div>
      <div class="l">{label}</div>
    </div>
  );
}
