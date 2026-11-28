// data/lang 文件内容解析：只接受 zh/en 前缀（旧安装器/手动写入可能带换行或后缀）。
function parseModuleLang(result) {
  if (!result || Number(result.errno) !== 0) return '';
  const value = String(result.stdout || '')
    .trim()
    .toLowerCase();
  if (value.indexOf('zh') === 0) return 'zh';
  if (value.indexOf('en') === 0) return 'en';
  return '';
}

// 挂载时读到的 data/lang 是否应该覆盖当前语言：用户已经手动切过语言（langTouched）
// 时，迟到的 cat 响应必须作废，否则会覆盖用户刚选的档位（也不能再写 localStorage）。
function adoptModuleLang(touched, detected) {
  if (touched) return false;
  return detected === 'zh' || detected === 'en';
}

export { parseModuleLang, adoptModuleLang };
