const LANGS = {
  zh: {
    // 首屏加载与刷新按钮
    loading: '正在获取数据…',
    refresh: '刷新',
    refresh_now: '立即刷新',
    refreshing: '刷新中…',
    // 底栏样式（悬浮胶囊 / 传统贴底）
    settings_bar_mode: '底栏样式',
    settings_bar_mode_desc: '悬浮胶囊或贴底传统样式',
    bar_floating: '悬浮底栏',
    bar_classic: '传统底栏',
    // 历史页：摘要统计
    history_summary_valid: '有效会话',
    history_summary_discharges: '放电记录',
    history_summary_samples: '采样数',
    // 历史页：记录 / 趋势视图切换与范围筛选
    hist_mode_records: '记录',
    hist_mode_trends: '趋势',
    hist_range_7: '7 天',
    hist_range_30: '30 天',
    hist_range_90: '90 天',
    hist_filter_all: '全部',
    hist_filter_charge: '充电',
    hist_filter_discharge: '放电',
    // 历史页：时间轴日期与列表状态
    hist_today: '今天',
    hist_yesterday: '昨天',
    hist_load_more: '加载更早记录',
    hist_all_loaded: '已显示全部记录',
    hist_empty_records: '还没有充电或放电记录',
    hist_empty_filter: '该筛选下暂无记录',
    hist_empty_trends: '趋势数据还不够，继续使用后会出现',
    hist_trend_capacity: '容量趋势',
    hist_trend_sessions: '充电会话',
    hist_trend_discharge: '放电记录',
    hist_trend_rest: '静置电压',
    hist_trend_ccct: '恒流时长',
    hist_trend_ica: '容量主峰',
    hist_point_zero: '暂无数据点',
    hist_point_one: '{n} 个点',
    hist_point_many: '{n} 个点',
    // 历史页：记录行与详情字段
    hist_records_count: '共 {n} 条',
    hist_status_recorded: '已记录',
    hist_retry: '重试',
    hist_detail_time: '时间',
    hist_detail_duration: '时长',
    hist_detail_temp: '平均温度',
    hist_detail_rate: '平均倍率',
    hist_detail_estimate: '估算容量',
    hist_detail_soc: '电量',
    hist_detail_implied: '隐含容量',
    hist_detail_released: '放出电量',
    // 历史页：记录详情与趋势卡片副标题
    hist_est_delta: '较上次 {n} mAh',
    hist_session_gain: '电量 +{n}%',
    hist_session_soc: '电量 {a}% → {b}%',
    hist_sessions_note: '柱高为本次充电提升的电量百分点，不是容量增长',
    hist_session_est: '估算 {n} mAh',
    hist_session_temp: '温度 {n}℃',
    hist_duration: '历时 {n} 分钟',
    hist_implied: '隐含 {n} mAh',

    // 导航与页面标题
    nav_overview: '概览',
    nav_history: '历史',
    nav_settings: '设置',
    nav_aria: '页面导航',
    page_history: '历史',
    page_settings: '设置',

    // 设置页：外观（主题 / 语言 / 底栏）
    settings_appearance: '外观',
    settings_theme: '主题',
    settings_theme_desc: '跟随系统，或手动选择浅色 / 深色',
    theme_auto: '跟随系统',
    theme_light: '浅色',
    theme_dark: '深色',
    settings_language: '语言',
    settings_language_desc: '模块描述与 WebUI 共用同一个设置',
    lang_zh: '中文',
    lang_en: 'English',
    // 设置页：手动刷新
    settings_refresh: '刷新',
    settings_manual_refresh: '手动刷新',
    settings_refresh_desc: '数据每 60 秒自动更新，失败后会退避重试',
    // 设置页：数据信息
    settings_data: '数据',
    settings_channel: '通道',
    settings_updated: '更新于',
    settings_samples: '采样数',
    settings_sessions: '会话数',
    settings_retention: '数据保留',
    settings_retention_value: '最近 90 天',
    settings_storage: '存储目录',
    // 设置页：电流刻度校准诊断
    settings_current_node: '采样节点',
    settings_current_calib: '电流刻度',
    calib_calibrated: '已按电量计校准 ×{scale}（{unit}）',
    calib_auto: '自动识别（{unit}）',
    calib_unit_auto: '量纲未知',
    // 设置页：诊断信息复制（已脱敏）
    settings_diag: '诊断信息',
    settings_diag_desc: '复制已脱敏的环境与诊断日志，方便反馈 issue',
    diag_copy: '复制诊断',
    diag_busy: '收集中…',
    diag_copied: '已复制，可直接粘贴到 issue',
    diag_copy_fail: '复制失败，请长按下方文本手动复制',
    diag_error: '收集失败，请稍后重试',
    // 设置页：关于
    settings_about: '关于',
    settings_about_desc: '通过 root 读取电量计，结合充电会话估算电池健康度与容量。',
    about_version: '版本',
    about_network: '联网',
    about_network_value: '无网络请求、无遥测',
    about_license: '开源许可',
    about_license_value: 'GPL-2.0-only',
    about_third_party: '第三方组件',
    about_third_party_value: 'Preact（MIT）',
    settings_webui: '界面',
    settings_webui_value: '管理器内置 WebUI',

    // 概览页：指标格（设计容量 / 满充 / 循环 / 估算 / 内阻）
    label_design: '设计容量',
    label_full: '满充容量',
    label_cycles: '循环次数',
    label_est: '实测估算',
    label_cyceq: '循环当量',
    label_rmoh: '内阻',
    label_trend: '容量趋势',
    label_qual: '数据可信度',

    // 概览页：面板标题
    panel_est: '估算趋势',
    panel_ses: '充电会话',
    panel_dis: '放电记录',

    // 无宿主时的 gate 页
    gate_title: '无法连接管理器',
    gate_desc: '需要 KernelSU/APatch/MMRL 管理器',
    tap_hint: '点按图上数据点查看详情',

    // 概览页：健康度徽章与容量副标题
    badge_measured: '实测',
    badge_kernel: '内核',
    sub_full: '满充容量 {mah} mAh',
    est_learning_zero: '学习中 · 暂未采样',
    est_learning_one: '学习中 · 已采集 1 次',
    est_learning_many: '学习中 · 已采集 {n} 次',
    rmoh_collecting: '采集中',
    // 概览页：容量趋势文案
    trend_stable: '无显著变化',
    trend_insufficient: '数据积累中',
    trend_days: ' · {n}/21 天',
    trend_down: '每周衰减 {n} mAh',
    trend_up: '每周回升 {n} mAh',

    // 质量哨兵（ema / 中段 / 放电 / 内核四路互检）
    qual_ok: '可信',
    qual_bad: '存疑',
    qual_summary: '{state} · 误差 {n}%',
    qual_detail: '四条独立测量的容量对照：{list}',
    qual_insufficient: '数据不足',
    ch_ema: '实测',
    ch_mid: '中段',
    ch_dis: '放电',
    ch_full: '内核',

    // 会话结算状态与不计入原因
    session_counted: '参与估算',
    session_not_counted: '未参与估算',
    reason_temp: '温度出界',
    reason_delta: '电量提升不足 20%',
    reason_window: '估算超界',
    reason_outlier: '离群丢弃',

    // 估算学习进度与采样计数
    est_empty: '等待首次有效结算',
    est_cap_latest: '最新 {mah} mAh',
    samples_one: '累计采样 1 次',
    samples_many: '累计采样 {n} 次',
    samples_zero: '累计采样 0 次',

    // 构建通道
    channel_stable: '稳定版',
    channel_ml: 'ML 实验版',

    // 错误提示
    err_run: '执行失败：{detail}',
    err_noout: '无输出：batteryd 未返回数据',
    err_json: '数据解析失败：输出不是有效 JSON',
    err_struct: '数据解析失败：意外的数据结构',
    err_render: '渲染失败：{msg}',
  },
  en: {
    // 首屏加载与刷新按钮
    loading: 'Loading data…',
    refresh: 'Refresh',
    refresh_now: 'Refresh now',
    refreshing: 'Refreshing…',
    // 底栏样式（悬浮胶囊 / 传统贴底）
    settings_bar_mode: 'Bottom bar',
    settings_bar_mode_desc: 'Floating pill or classic edge-to-edge bar',
    bar_floating: 'Floating',
    bar_classic: 'Classic',
    // 历史页：摘要统计
    history_summary_valid: 'Valid sessions',
    history_summary_discharges: 'Discharges',
    history_summary_samples: 'Samples',
    // 历史页：记录 / 趋势视图切换与范围筛选
    hist_mode_records: 'Records',
    hist_mode_trends: 'Trends',
    hist_range_7: '7 d',
    hist_range_30: '30 d',
    hist_range_90: '90 d',
    hist_filter_all: 'All',
    hist_filter_charge: 'Charge',
    hist_filter_discharge: 'Discharge',
    // 历史页：时间轴日期与列表状态
    hist_today: 'Today',
    hist_yesterday: 'Yesterday',
    hist_load_more: 'Load earlier records',
    hist_all_loaded: 'All records loaded',
    hist_empty_records: 'No charge or discharge records yet',
    hist_empty_filter: 'No records for this filter',
    hist_empty_trends: 'Not enough trend data yet; keep using the module',
    hist_trend_capacity: 'Capacity trend',
    hist_trend_sessions: 'Sessions',
    hist_trend_discharge: 'Discharge',
    hist_trend_rest: 'Resting V',
    hist_trend_ccct: 'CCCT',
    hist_trend_ica: 'ICA peak',
    hist_point_zero: 'No points',
    hist_point_one: '{n} point',
    hist_point_many: '{n} points',
    // 历史页：记录行与详情字段
    hist_records_count: '{n} records',
    hist_status_recorded: 'Recorded',
    hist_retry: 'Retry',
    hist_detail_time: 'Time',
    hist_detail_duration: 'Duration',
    hist_detail_temp: 'Avg temp',
    hist_detail_rate: 'Avg C-rate',
    hist_detail_estimate: 'Estimate',
    hist_detail_soc: 'State of charge',
    hist_detail_implied: 'Implied capacity',
    hist_detail_released: 'Released',
    // 历史页：记录详情与趋势卡片副标题
    hist_est_delta: '{n} mAh vs previous',
    hist_session_gain: 'SOC +{n}%',
    hist_session_soc: 'SOC {a}% → {b}%',
    hist_sessions_note: 'Bar height is the SOC gain in percentage points, not capacity growth',
    hist_session_est: 'Estimate {n} mAh',
    hist_session_temp: 'Temp {n}°C',
    hist_duration: '{n} min',
    hist_implied: 'Implied {n} mAh',

    // 导航与页面标题
    nav_overview: 'Overview',
    nav_history: 'History',
    nav_settings: 'Settings',
    nav_aria: 'Navigation',
    page_history: 'History',
    page_settings: 'Settings',

    // 设置页：外观（主题 / 语言 / 底栏）
    settings_appearance: 'Appearance',
    settings_theme: 'Theme',
    settings_theme_desc: 'Follow the system, or pick light / dark manually',
    theme_auto: 'System',
    theme_light: 'Light',
    theme_dark: 'Dark',
    settings_language: 'Language',
    settings_language_desc: 'Shared by the module description and WebUI',
    lang_zh: '中文',
    lang_en: 'English',
    // 设置页：手动刷新
    settings_refresh: 'Refresh',
    settings_manual_refresh: 'Manual refresh',
    settings_refresh_desc: 'Data refreshes every 60 seconds, backing off on failure',
    // 设置页：数据信息
    settings_data: 'Data',
    settings_channel: 'Channel',
    settings_updated: 'Updated',
    settings_samples: 'Samples',
    settings_sessions: 'Sessions',
    settings_retention: 'Retention',
    settings_retention_value: 'Last 90 days',
    settings_storage: 'Data directory',
    // 设置页：电流刻度校准诊断
    settings_current_node: 'Current node',
    settings_current_calib: 'Current scaling',
    calib_calibrated: 'Calibrated from fuel gauge ×{scale} ({unit})',
    calib_auto: 'Auto-detected ({unit})',
    calib_unit_auto: 'unknown unit',
    // Settings: diagnostics copy (redacted)
    settings_diag: 'Diagnostics',
    settings_diag_desc: 'Copy a redacted environment and diagnostic report for issue reports',
    diag_copy: 'Copy diagnostics',
    diag_busy: 'Collecting…',
    diag_copied: 'Copied — paste it into the issue',
    diag_copy_fail: 'Copy failed — long-press the text below to copy',
    diag_error: 'Collection failed, try again later',
    // 设置页：关于
    settings_about: 'About',
    settings_about_desc:
      'Reads the fuel gauge with root and estimates battery health and capacity from charge sessions.',
    about_version: 'Version',
    about_network: 'Network',
    about_network_value: 'No requests, no telemetry',
    about_license: 'License',
    about_license_value: 'GPL-2.0-only',
    about_third_party: 'Third-party',
    about_third_party_value: 'Preact (MIT)',
    settings_webui: 'Interface',
    settings_webui_value: 'Built-in manager WebUI',

    // 概览页：指标格（设计容量 / 满充 / 循环 / 估算 / 内阻）
    label_design: 'Design capacity',
    label_full: 'Full charge capacity',
    label_cycles: 'Cycle count',
    label_est: 'Measured estimate',
    label_cyceq: 'Cycle equivalent',
    label_rmoh: 'Internal resistance',
    label_trend: 'Capacity trend',
    label_qual: 'Data confidence',

    // 概览页：面板标题
    panel_est: 'Estimate trend',
    panel_ses: 'Charge sessions',
    panel_dis: 'Discharge records',

    // 无宿主时的 gate 页
    gate_title: 'Manager not connected',
    gate_desc: 'Requires a KernelSU, APatch, or MMRL manager',
    tap_hint: 'Tap a data point for details',

    // 概览页：健康度徽章与容量副标题
    badge_measured: 'Measured',
    badge_kernel: 'Kernel',
    sub_full: 'Full charge capacity {mah} mAh',
    est_learning_zero: 'Learning · No valid sessions yet',
    est_learning_one: 'Learning · 1 valid session',
    est_learning_many: 'Learning · {n} valid sessions',
    rmoh_collecting: 'Collecting',
    // 概览页：容量趋势文案
    trend_stable: 'No significant change',
    trend_insufficient: 'Collecting data',
    trend_days: ' · {n} of 21 days',
    trend_down: 'Down {n} mAh per week',
    trend_up: 'Up {n} mAh per week',

    // 质量哨兵（ema / 中段 / 放电 / 内核四路互检）
    qual_ok: 'Reliable',
    qual_bad: 'Questionable',
    qual_summary: '{state} · spread {n}%',
    qual_detail: 'Four independent capacity measurements: {list}',
    qual_insufficient: 'Not enough data',
    ch_ema: 'Measured',
    ch_mid: 'Mid-range',
    ch_dis: 'Discharge',
    ch_full: 'Kernel',

    // 会话结算状态与不计入原因
    session_counted: 'Counted',
    session_not_counted: 'Not counted',
    reason_temp: 'Temp out of range',
    reason_delta: 'SOC gain under 20%',
    reason_window: 'Estimate out of range',
    reason_outlier: 'Outlier discarded',

    // 估算学习进度与采样计数
    est_empty: 'Waiting for the first valid estimate',
    est_cap_latest: 'Latest {mah} mAh',
    samples_one: '1 sample',
    samples_many: '{n} samples',
    samples_zero: 'No samples yet',

    // 构建通道
    channel_stable: 'Stable',
    channel_ml: 'ML experimental',

    // 错误提示
    err_run: 'Command failed: {detail}',
    err_noout: 'No output: batteryd returned no data',
    err_json: 'Parse error: output is not valid JSON',
    err_struct: 'Parse error: unexpected data structure',
    err_render: 'Render error: {msg}',
  },
};

function translate(lang, key, vars) {
  const dict = LANGS[lang] || LANGS.zh;
  let s = dict[key];
  if (s == null) s = LANGS.zh[key];
  if (s == null) return key;
  if (vars) {
    s = s.replace(/\{(\w+)\}/g, function (m, name) {
      return vars[name] == null ? '' : String(vars[name]);
    });
  }
  return s;
}

function plural(lang, n, oneKey, manyKey, vars, zeroKey) {
  vars = vars || {};
  vars.n = n;
  if (Number(n) === 0 && zeroKey) return translate(lang, zeroKey, vars);
  return translate(lang, Number(n) === 1 ? oneKey : manyKey, vars);
}

export { translate, plural };
