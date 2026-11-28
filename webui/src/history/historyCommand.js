import { CMD_HISTORY, HIST_PAGE_LIMIT, parseJsonResult } from '../core.js';

// history 子命令的合法枚举与默认值：这些值会拼进 root shell 的固定模板，
// 只接受白名单内的值，其余一律回退默认，绝不透传任意字符串。
const HISTORY_DAYS = [7, 30, 90];
const HISTORY_TYPES = ['all', 'charge', 'discharge'];
const HISTORY_DEFAULT_DAYS = 30;
const HISTORY_DEFAULT_TYPE = 'all';

function normalizeDays(days) {
  const n = Number(days);
  return HISTORY_DAYS.indexOf(n) >= 0 ? n : HISTORY_DEFAULT_DAYS;
}

function normalizeType(type) {
  return HISTORY_TYPES.indexOf(type) >= 0 ? type : HISTORY_DEFAULT_TYPE;
}

// 游标必须来自响应 JSON 且是安全正整数：NaN/Infinity/负数/字符串一律视为无游标。
function normalizeBefore(before) {
  return Number.isSafeInteger(before) && before > 0 ? before : 0;
}

// 只从已加载数据里读游标：非法 next_before 按无游标处理（不拼进命令）。
function historyBefore(previous) {
  return normalizeBefore(previous && previous.next_before);
}

// history 命令拼装：天数/类型白名单 + 游标数值校验后填入固定模板。
function buildHistoryCmd(params) {
  const p = params || {};
  let cmd =
    CMD_HISTORY +
    ' --days ' +
    normalizeDays(p.days) +
    ' --limit ' +
    HIST_PAGE_LIMIT +
    ' --type ' +
    normalizeType(p.type);
  const before = normalizeBefore(p.before);
  if (before) cmd += ' --before ' + before;
  return cmd;
}

// 事件身份：kind + 结束时刻不足以唯一标识一条记录——同一秒结束的两条会话
// （多 daemon 遗留重复行清理前的窗口期）或同 ts 的放电行会并存，只按 kind:ts
// 去重会静默丢掉其中一条。带上各自的起始信息（charge.start_ts /
// discharge.secs+start_cap）区分；字段缺失时退回 kind:ts（兼容旧 payload）。
function historyEventKey(event) {
  const base = event.kind + ':' + event.ts;
  if (event.charge) {
    return base + ':c' + (event.charge.start_ts != null ? event.charge.start_ts : '');
  }
  if (event.discharge) {
    const secs = event.discharge.secs != null ? event.discharge.secs : '';
    const cap = event.discharge.start_cap != null ? event.discharge.start_cap : '';
    return base + ':d' + secs + ':' + cap;
  }
  return base;
}

// 追加分页时按身份去重：游标边界可能返回与上一页重叠的事件（服务端同秒延展与
// 数据修复期间都可能出现）。每个已见事件只保留首次出现的一份。
function mergeHistoryEvents(previous, next) {
  if (!previous) return next;
  const seen = new Set();
  const merged = [];
  previous.events.concat(next.events).forEach((event) => {
    const key = historyEventKey(event);
    if (seen.has(key)) return;
    seen.add(key);
    merged.push(event);
  });
  return {
    events: merged,
    has_more: next.has_more,
    next_before: next.next_before,
    summary: next.summary,
    trends: next.trends,
  };
}

// history 结果 → {doc} 或 {error:{key,vars}}：共用 core.parseJsonResult 的
// errno/stdout/JSON/结构校验，再补一层「events 必须是数组」。
function parseHistoryResult(result) {
  const parsed = parseJsonResult(result);
  if (parsed.error) return parsed;
  if (!Array.isArray(parsed.doc.events)) return { error: { key: 'err_struct' } };
  return parsed;
}

// 纯映射：失败返回统一错误模型 {key, vars}（err_run/err_noout/err_json/err_struct），
// 成功返回 null。与 useDocPolling 的错误模型一致，组件渲染时走 t(err.key, err.vars)。
function mapHistoryError(result) {
  return parseHistoryResult(result).error || null;
}

// 历史取数的请求代际：响应落地必须同时匹配「代际 + 请求参数键」。
// - 整页重载（reset）递增代际，作废此前所有在途请求（旧的追加分页、旧的整页请求），
//   避免旧游标页拼到新筛选结果上；
// - 参数键在每次渲染时同步（setKey），切天数/筛选/视图后落地迟到的响应会被丢弃；
// - 追加分页在整页重载在途或上一次追加未回包时由调用方直接忽略。
function createHistoryRequestTracker() {
  let gen = 0;
  let key = '';
  let pendingReset = false;
  let appendInFlight = false;

  const tracker = {
    setKey: function (next) {
      key = next == null ? '' : String(next);
    },
    getKey: function () {
      return key;
    },
    isPendingReset: function () {
      return pendingReset;
    },
    isAppendInFlight: function () {
      return appendInFlight;
    },
    // 开始一次请求并返回令牌；reset 会作废所有在途请求。
    begin: function (reset) {
      if (reset) {
        gen += 1;
        pendingReset = true;
        appendInFlight = false;
      } else {
        appendInFlight = true;
      }
      return { gen: gen, key: key, reset: !!reset };
    },
    // 代际与参数键都匹配才允许落地。
    accept: function (token) {
      return !!token && token.gen === gen && token.key === key;
    },
    // 落地后清掉对应在途标记；过期令牌返回 false（调用方应直接丢弃响应）。
    settle: function (token) {
      if (!tracker.accept(token)) return false;
      if (token.reset) pendingReset = false;
      else appendInFlight = false;
      return true;
    },
  };
  return tracker;
}

export {
  HISTORY_DAYS,
  HISTORY_TYPES,
  HISTORY_DEFAULT_DAYS,
  HISTORY_DEFAULT_TYPE,
  buildHistoryCmd,
  historyBefore,
  historyEventKey,
  mergeHistoryEvents,
  parseHistoryResult,
  mapHistoryError,
  createHistoryRequestTracker,
};
