// localStorage 偏好读写：部分管理器的 WebView 会禁用 storage 或在隐私模式下抛异常，
// 读写统一 try/catch，出错一律按「没有存过 / 没写成功」处理，不影响首屏渲染。
// allowed 为数组时按白名单筛选（数组外的值回退 fallback），非数组时只做空值回退。

function readPref(key, allowed, fallback) {
  let raw = null;
  try {
    raw = localStorage.getItem(key);
  } catch (e) {
    raw = null;
  }
  if (Array.isArray(allowed)) {
    return raw != null && allowed.indexOf(raw) >= 0 ? raw : fallback;
  }
  return raw == null ? fallback : raw;
}

function writePref(key, value) {
  try {
    localStorage.setItem(key, value);
    return true;
  } catch (e) {
    return false;
  }
}

export { readPref, writePref };
