// 新组件用 JSX 写，验证 @preact/preset-vite 的 automatic runtime 已生效。
// 老组件仍保留 h() 写法，迁移可以按文件逐步进行。
export function HistStat({ value, label }) {
  return (
    <div class="hist-stat">
      <div class="n">{value}</div>
      <div class="l">{label}</div>
    </div>
  );
}
