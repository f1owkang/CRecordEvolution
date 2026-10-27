import { createContext } from 'preact';
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

export { LangCtx, useTr };
