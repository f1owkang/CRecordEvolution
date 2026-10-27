import { h, createContext } from 'preact';
import { useContext } from 'preact/hooks';

const LangCtx = createContext({
  lang: 'zh',
  t: function (k) {
    return k;
  },
});
function useTr() {
  return useContext(LangCtx);
}

// 轻量元素工厂：直接使用 Preact h()，避免引入额外模板库（htm 是 Apache-2.0，
// 与本项目 GPL-2.0-only 不兼容，因此不打包 htm）
function E(tag, props) {
  const children = Array.prototype.slice.call(arguments, 2);
  return h.apply(null, [tag, props == null ? null : props].concat(children));
}

export { LangCtx, useTr, E };
