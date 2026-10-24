'use strict';

function contentFP(doc) {
    return JSON.stringify(doc, function (k, v) {
      return (k === 'updated' || k === 'temp_c') ? undefined : v;
    });
  }

  function storedThemeMode() {
    var s = null;
    try { s = localStorage.getItem(THEME_KEY); } catch (e) {}
    return (s === 'light' || s === 'dark') ? s : 'auto';
  }

  function systemTheme() {
    try {
      if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) return 'light';
    } catch (e) {}
    return 'dark';
  }

  function resolvedTheme(mode) { return mode === 'auto' ? systemTheme() : mode; }

  function applyThemeDocument(mode) {
    var theme = resolvedTheme(mode);
    document.documentElement.setAttribute('data-theme', theme);
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute('content', theme === 'light' ? '#f6f8fa' : '#0b0e14');
    try {
      var api = window.$CRecordEvolution;
      if (api && typeof api.setLightStatusBars === 'function') {
        api.setLightStatusBars(theme === 'light');
      }
    } catch (e) { /* 非 MMRL 宿主 */ }
  }

  function storedBarMode() {
    var s = null;
    try { s = localStorage.getItem(BAR_KEY); } catch (e) {}
    return s === 'classic' ? 'classic' : 'floating';
  }

  function storedHistMode() {
    var s = null;
    try { s = localStorage.getItem(HIST_MODE_KEY); } catch (e) {}
    return s === 'trends' ? 'trends' : 'records';
  }

  function storedHistDays() {
    var s = null;
    try { s = Number(localStorage.getItem(HIST_DAYS_KEY)); } catch (e) {}
    return (s === 7 || s === 30 || s === 90) ? s : 30;
  }

  function initialLang() {
    return document.documentElement.getAttribute('data-lang') === 'en' ? 'en' : 'zh';
  }

  var LangCtx = createContext({ lang: 'zh', t: function (k) { return k; } });
  function useTr() { return useContext(LangCtx); }

  // 轻量元素工厂：直接使用 Preact h()，避免引入额外模板库（htm 是 Apache-2.0，
  // 与本项目 GPL-2.0-only 不兼容，因此不打包 htm）
  function E(tag, props) {
    var children = Array.prototype.slice.call(arguments, 2);
    return h.apply(null, [tag, props == null ? null : props].concat(children));
  }

  function ErrorBanner(props) {
    var c = useTr();
    if (!props.err) return null;
    return E('section', { class: 'err', role: 'alert' }, translate(c.lang, props.err.key, props.err.vars));
  }

  function IconRefresh() {
    return E('svg', { viewBox: '0 0 24 24', 'aria-hidden': 'true' },
      E('path', { d: 'M20 11a8 8 0 1 0-2.3 5.7' }),
      E('path', { d: 'M20 5v6h-6' }));
  }

  function TopBar(props) {
    var c = useTr();
    var title = props.page === 0 ? 'ChargingRecord Evolution'
      : translate(c.lang, props.page === 1 ? 'page_history' : 'page_settings');
    var showRefresh = !props.gateMode && props.page !== 2;
    return E('header', { class: 'topbar' },
      E('div', { class: 'topbar-inner' },
        E('h1', { id: 'pageTitle' }, title),
        showRefresh ? E('button', {
          id: 'refreshTop',
          type: 'button',
          class: 'iconbtn' + (props.busy ? ' spinning' : ''),
          title: translate(c.lang, 'refresh'),
          'aria-label': translate(c.lang, 'refresh'),
          disabled: props.busy,
          onClick: props.onRefresh
        }, IconRefresh()) : null));
  }

  function navIcon(name) {
    if (name === 'overview') {
      return E('svg', { viewBox: '0 0 24 24', 'aria-hidden': 'true' },
        E('path', { d: 'M4 19V5' }), E('path', { d: 'M4 19h16' }),
        E('path', { d: 'M8 16l3.5-5 3 2.4L20 7' }));
    }
    if (name === 'history') {
      return E('svg', { viewBox: '0 0 24 24', 'aria-hidden': 'true' },
        E('circle', { cx: '12', cy: '12', r: '8.5' }),
        E('path', { d: 'M12 7.5V12l3 2' }));
    }
    return E('svg', { viewBox: '0 0 24 24', 'aria-hidden': 'true' },
      E('circle', { cx: '12', cy: '12', r: '3' }),
      E('path', { d: 'M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1.03 1.56V21a2 2 0 1 1-4 0v-.09A1.7 1.7 0 0 0 9 19.35a1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.64 15a1.7 1.7 0 0 0-1.56-1.03H3a2 2 0 1 1 0-4h.09A1.7 1.7 0 0 0 4.65 9a1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.7 1.7 0 0 0 9 4.64a1.7 1.7 0 0 0 1.03-1.56V3a2 2 0 1 1 4 0v.09A1.7 1.7 0 0 0 15 4.65a1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.7 1.7 0 0 0 19.36 9a1.7 1.7 0 0 0 1.56 1.03H21a2 2 0 1 1 0 4h-.09A1.7 1.7 0 0 0 19.4 15z' }));
  }

  function Dock(props) {
    var c = useTr();
    var items = [
      { id: 'overview', label: 'nav_overview' },
      { id: 'history', label: 'nav_history' },
      { id: 'settings', label: 'nav_settings' }
    ];
    return E('nav', { class: 'dock', 'aria-label': c.t('nav_aria') },
      E('div', { class: 'dock-inner', role: 'tablist' },
        E('span', {
          class: 'dock-ind', 'aria-hidden': 'true',
          style: { transform: 'translateX(' + (props.page * 100) + '%)' }
        }),
        items.map(function (item, i) {
          return E('button', {
            key: item.id,
            id: 'nav_' + item.id,
            'data-page': i,
            class: 'navitem' + (props.page === i ? ' is-active' : ''),
            type: 'button',
            role: 'tab',
            'aria-selected': props.page === i ? 'true' : 'false',
            onClick: function () { props.onNavigate(i); }
          }, navIcon(item.id), E('span', null, translate(c.lang, item.label)));
        })));
  }

  function ChartCard(props) {
    var c = useTr();
    var selected = useState(null);
    var idx = selected[0], setIdx = selected[1];
    useEffect(function () { setIdx(null); }, [props.svg]);
    if (props.hidden) return null;
    var hasSvg = !!props.svg && props.points && props.points.length > 0;
    function onClick(e) {
      var t = e.target;
      while (t && t !== e.currentTarget && !(t.getAttribute && t.getAttribute('data-i') != null)) t = t.parentNode;
      var attr = t && t.getAttribute ? t.getAttribute('data-i') : null;
      if (attr == null) return;
      var i = Number(attr);
      if (!isFinite(i) || i < 0 || i >= props.points.length) return;
      setIdx(i);
    }
    return E('section', { class: 'panel' },
      E('div', { class: 'ptitle' }, props.title),
      hasSvg
        ? E('div', { class: 'chart', onClick: onClick, dangerouslySetInnerHTML: { __html: props.svg } })
        : E('div', { class: 'row empty' }, props.empty),
      hasSvg ? E('div', { class: 'det' },
        idx == null
          ? E('span', { class: 'hint' }, c.t('tap_hint'))
          : props.format(props.points[idx].row)) : null,
      props.cap ? E('div', { class: 'cap' }, props.cap) : null);
  }

  function Cell(props) {
    return E('div', {
      class: 'cell' + (props.pt ? ' pt' : ''),
      style: props.style,
      title: props.title == null ? null : props.title,
      onClick: props.onClick
    },
      E('div', { class: 'label' }, props.label),
      E('div', { class: 'val' }, props.value),
      props.detail || null);
  }

  function Overview(props) {
    var c = useTr();
    var lang = c.lang;
    var d = props.doc;
    var qualState = useState(false);
    var qualOpen = qualState[0], setQualOpen = qualState[1];

    var design = d ? d.design_mah : null;
    var est = d ? d.est_mah : null;
    var hasEst = est != null && design != null && Number(design) > 0;
    var pctText = d ? (hasEst ? String(Math.round(est / design * 100)) : fmtInt(d.pct)) : '--';
    var badgeClass = 'badge ' + (hasEst ? 'meas' : 'kernel');
    var badgeText = hasEst ? c.t('badge_measured') : c.t('badge_kernel');
    var subText = !d ? c.t('loading')
      : (hasEst ? (est + ' / ' + design + ' mAh')
        : (d.full_mah != null ? c.t('sub_full', { mah: d.full_mah }) : '\u00a0'));

    var estText;
    if (est != null) {
      var sg = Number(d.est_mah_sigma);
      var hasSigma = d.channel === 'ml' && d.est_mah_sigma != null && isFinite(sg);
      estText = (lang === 'en' && hasSigma)
        ? (est + ' ±' + Math.round(sg) + ' mAh')
        : (est + ' mAh' + (hasSigma ? ' ±' + Math.round(sg) + ' mAh' : ''));
    } else {
      var nsamp = d && d.samples != null ? d.samples : 0;
      estText = plural(lang, nsamp, 'est_learning_one', 'est_learning_many', { n: fmtInt(nsamp) }, 'est_learning_zero');
    }

    var tw = d ? Number(d.trend_mah_per_week) : NaN;
    var hasTw = d && d.trend_mah_per_week != null && isFinite(tw);
    var trendText;
    if (d && d.trend_state === 'stable') trendText = c.t('trend_stable');
    else if (d && d.trend_state === 'insufficient') {
      trendText = c.t('trend_insufficient') + (d.trend_span_day != null ? c.t('trend_days', { n: fmtInt(d.trend_span_day) }) : '');
    } else if (hasTw) trendText = tw < 0 ? c.t('trend_down', { n: Math.abs(tw).toFixed(1) }) : c.t('trend_up', { n: tw.toFixed(1) });
    else trendText = '—';

    var q = d ? d.quality : null;
    var qOk = q != null && q.ok != null;
    var qDetail = [];
    if (qOk) {
      ['ema', 'mid', 'dis', 'full'].forEach(function (k) {
        if (q[k + '_mah'] != null) qDetail.push(c.t('ch_' + k) + ' ' + q[k + '_mah'] + ' mAh');
      });
    }
    var qDetailTxt = qOk ? c.t('qual_detail', { list: qDetail.join(' / ') }) : '';
    var qText = qOk ? c.t('qual_summary', { state: q.ok ? c.t('qual_ok') : c.t('qual_bad'), n: q.spread_pct }) : c.t('qual_insufficient');

    var recent = d && Array.isArray(d.recent) ? d.recent : [];
    var recentAsc = ascByTs(recent);
    var designOK = design != null && Number(design) > 0;
    var showEst = designOK || recent.length > 0;
    var estPoints = pickPairs(recentAsc, 'ts', 'mah');
    var latest = recent.length ? recent[0] : null;
    var estSvg = estPoints.length ? svgLine(recentAsc, CHART_W, CHART_H, 'var(--accent)', hasTw ? tw : null) : '';
    var capParts = [];
    if (latest != null) capParts.push(c.t('est_cap_latest', { mah: fmtInt(latest.mah) }));
    if (d && d.samples_n != null) capParts.push(plural(lang, d.samples_n, 'samples_one', 'samples_many', { n: fmtInt(d.samples_n) }, 'samples_zero'));
    var estCap = capParts.join(' · ');

    return E('section', { class: 'page' + (props.active ? ' is-active' : ''), id: 'page_overview', role: 'tabpanel' },
      E('section', { class: 'hero' },
        E('span', { class: badgeClass }, badgeText),
        E('div', { class: 'hero-line' }, E('span', null, pctText), E('span', { class: 'unit' }, '%')),
        E('div', { class: 'sub' }, subText)),
      E('section', { class: 'grid' },
        E(Cell, { label: c.t('label_design'), value: fmtInt(design) }),
        E(Cell, { label: c.t('label_full'), value: fmtInt(d ? d.full_mah : null) }),
        E(Cell, { label: c.t('label_cycles'), value: fmtInt(d ? d.cycles : null) }),
        E(Cell, { label: c.t('label_est'), value: estText }),
        E(Cell, { label: c.t('label_cyceq'), value: fmtFix(d ? d.cycle_equiv : null, 2) }),
        E(Cell, { label: c.t('label_rmoh'), value: d && d.r_moh != null ? Number(d.r_moh).toFixed(1) + ' mΩ' : c.t('rmoh_collecting') }),
        E('div', { class: 'cell', style: { gridColumn: 'span 2' } },
          E('div', { class: 'label' }, c.t('label_trend')),
          E('div', { class: 'val' }, trendText)),
        E(Cell, {
          label: c.t('label_qual'),
          value: qText,
          style: { gridColumn: 'span 2' },
          pt: qOk,
          title: qOk ? qDetailTxt : null,
          onClick: qOk ? function () { setQualOpen(!qualOpen); } : null,
          detail: qOk && qualOpen ? E('div', { class: 'det' }, qDetailTxt) : null
        })),
      E(ChartCard, {
        hidden: !showEst,
        title: c.t('panel_est'),
        svg: estSvg,
        points: estPoints,
        empty: c.t('est_empty'),
        cap: estCap,
        format: function (r) { return fmtTs(lang, r.ts) + ' · ' + fmtInt(r.mah) + ' mAh'; }
      }));
  }

  function HistStat(props) {
    return E('div', { class: 'hist-stat' },
      E('div', { class: 'n' }, props.value),
      E('div', { class: 'l' }, props.label));
  }

  function sessionStatus(c, s) {
    if (s && s.valid) return { text: c.t('session_counted'), ok: true };
    var r = s ? s.invalid_reason : null;
    return { text: (r && REASON_KEY[r]) ? c.t(REASON_KEY[r]) : c.t('session_not_counted'), ok: false };
  }

  function RecShell(props) {
    return E('div', { class: 'rec' },
      E('div', { class: 'rec-main' }, props.main),
      props.sub && props.sub.length
        ? E('div', { class: 'rec-sub' }, props.sub.map(function (p, i) { return E('span', { key: i }, p); }))
        : null);
  }

  function SessionRow(props) {
    var c = useTr();
    var s = props.row;
    var status = sessionStatus(c, s);
    var parts = [];
    if (s.delta_pct != null) parts.push(c.t('hist_session_gain', { n: fmtInt(s.delta_pct) }));
    if (s.est_mah != null) parts.push(c.t('hist_session_est', { n: fmtInt(s.est_mah) }));
    if (s.temp_avg != null) parts.push(c.t('hist_session_temp', { n: fmtInt(s.temp_avg) }));
    if (s.c_rate != null) parts.push(c.t('hist_session_crate', { n: Number(s.c_rate).toFixed(2) }));
    return E(RecShell, {
      main: [
        E('span', null, fmtRange(c.lang, s.start_ts, s.end_ts)),
        E('span', { class: status.ok ? 'ok' : 'bad' }, status.text)
      ],
      sub: parts
    });
  }

  function DischargeRow(props) {
    var c = useTr();
    var r = props.row;
    var parts = [];
    if (r.start_cap != null && r.end_cap != null) parts.push(fmtInt(r.start_cap) + '→' + fmtInt(r.end_cap) + '%');
    parts.push(c.t('hist_discharge_release', { n: fmtInt(r.uah_mah) }));
    var mins = Math.round(Number(r.secs) / 60);
    if (isFinite(mins)) parts.push(c.t('hist_duration', { n: mins }));
    if (r.implied_mah != null) parts.push(c.t('hist_implied', { n: fmtInt(r.implied_mah) }));
    return E(RecShell, { main: E('span', null, fmtTs(c.lang, r.ts)), sub: parts });
  }

  function RestRow(props) {
    var c = useTr();
    var r = props.row;
    var parts = [(Number(r.uv) / 1e6).toFixed(3) + ' V'];
    if (r.cap != null) parts.push(c.t('hist_rest_cap', { n: fmtInt(r.cap) }));
    return E(RecShell, { main: E('span', null, fmtTs(c.lang, r.ts)), sub: parts });
  }

  function CcctRow(props) {
    var c = useTr();
    var r = props.row;
    var parts = [c.t('hist_ccct_duration', { n: (Number(r.secs) / 60).toFixed(1) })];
    if (r.vw_lo && r.vw_hi) parts.push(c.t('hist_ccct_window', { lo: Math.round(r.vw_lo / 1000), hi: Math.round(r.vw_hi / 1000) }));
    return E(RecShell, { main: E('span', null, fmtTs(c.lang, r.ts)), sub: parts });
  }

  function IcaRow(props) {
    var c = useTr();
    var r = props.row;
    var parts = [c.t('hist_ica_peak', { v: (Number(r.peak_uv) / 1e6).toFixed(3) })];
    if (r.peak_h_rel != null) parts.push(c.t('hist_ica_height', { h: Number(r.peak_h_rel).toFixed(2) }));
    return E(RecShell, { main: E('span', null, fmtTs(c.lang, r.ts)), sub: parts });
  }

  function EstimateRow(props) {
    var c = useTr();
    var r = props.row;
    var parts = [fmtInt(r.mah) + ' mAh'];
    if (props.prevMah != null) {
      var delta = Number(r.mah) - Number(props.prevMah);
      if (isFinite(delta)) parts.push(c.t('hist_est_delta', { n: (delta >= 0 ? '+' : '') + Math.round(delta) }));
    }
    return E(RecShell, { main: E('span', null, fmtTs(c.lang, r.ts)), sub: parts });
  }

  function ChartCardInline(props) {
    var c = useTr();
    var selected = useState(null);
    var idx = selected[0], setIdx = selected[1];
    useEffect(function () { setIdx(null); }, [props.svg]);
    if (!props.svg || !props.points || !props.points.length) return null;
    function onClick(e) {
      var t = e.target;
      while (t && t !== e.currentTarget && !(t.getAttribute && t.getAttribute('data-i') != null)) t = t.parentNode;
      var attr = t && t.getAttribute ? t.getAttribute('data-i') : null;
      if (attr == null) return;
      var i = Number(attr);
      if (!isFinite(i) || i < 0 || i >= props.points.length) return;
      setIdx(i);
    }
    return E('div', null,
      E('div', { class: 'chart', onClick: onClick, dangerouslySetInnerHTML: { __html: props.svg } }),
      E('div', { class: 'det' },
        idx == null ? E('span', { class: 'hint' }, c.t('tap_hint')) : props.format(props.points[idx].row)),
      props.cap ? E('div', { class: 'cap' }, props.cap) : null);
  }

  function PanelTitle(props) {
    return E('div', { class: 'ptitle' }, props.title);
  }



function Seg(props) {
    return E('div', { class: 'seg', id: props.id || null, role: 'group', 'aria-label': props.label || null },
      props.options.map(function (o) {
        return E('button', {
          key: o.value,
          type: 'button',
          class: 'seg-btn' + (o.value === props.value ? ' is-active' : ''),
          onClick: function () { props.onChange(o.value); }
        }, o.label);
      }));
  }

  function SetStack(props) {
    return E('div', { class: 'set-item set-item--stack' },
      E('div', { class: 'set-text' },
        E('div', { class: 'set-title' }, props.title),
        E('div', { class: 'set-desc' }, props.desc)),
      props.control);
  }

  function SetRow(props) {
    return E('div', { class: 'set-item' },
      E('span', { class: 'set-title' }, props.label),
      E('span', { class: 'set-value' + (props.cls ? ' ' + props.cls : ''), 'data-field': props.field || null }, props.value));
  }

  function Divider() { return E('div', { class: 'set-divider' }); }

  function Settings(props) {
    var c = useTr();
    var lang = c.lang;
    var d = props.doc || {};
    var channelKey = CHANNEL_KEY[d.channel];
    var channel = channelKey ? c.t(channelKey) : (d.channel || '—');
    var themeOptions = [
      { value: 'auto', label: c.t('theme_auto') },
      { value: 'light', label: c.t('theme_light') },
      { value: 'dark', label: c.t('theme_dark') }
    ];
    var langOptions = [
      { value: 'zh', label: c.t('lang_zh') },
      { value: 'en', label: c.t('lang_en') }
    ];
    var barOptions = [
      { value: 'floating', label: c.t('bar_floating') },
      { value: 'classic', label: c.t('bar_classic') }
    ];
    return E('section', { class: 'page' + (props.active ? ' is-active' : ''), id: 'page_settings', role: 'tabpanel' },
      E('div', { class: 'settings-section' },
        E('div', { class: 'settings-title' }, c.t('settings_appearance')),
        E('div', { class: 'set-group' },
          E(SetStack, {
            title: c.t('settings_theme'), desc: c.t('settings_theme_desc'),
            control: E(Seg, { id: 'theme_seg', label: c.t('settings_theme'), value: props.themeMode, options: themeOptions, onChange: props.onThemeChange })
          }),
          E(Divider),
          E(SetStack, {
            title: c.t('settings_language'), desc: c.t('settings_language_desc'),
            control: E(Seg, { id: 'lang_seg', label: c.t('settings_language'), value: lang, options: langOptions, onChange: props.onLangChange })
          }),
          E(Divider),
          E(SetStack, {
            title: c.t('settings_bar_mode'), desc: c.t('settings_bar_mode_desc'),
            control: E(Seg, { id: 'bar_mode_seg', label: c.t('settings_bar_mode'), value: props.barMode, options: barOptions, onChange: props.onBarChange })
          }))),
      E('div', { class: 'settings-section' },
        E('div', { class: 'settings-title' }, c.t('settings_refresh')),
        E('div', { class: 'set-group' },
          E(SetStack, {
            title: c.t('settings_manual_refresh'), desc: c.t('settings_refresh_desc'),
            control: E('button', {
              id: 'refresh', type: 'button', disabled: props.busy, onClick: props.onRefresh
            }, props.busy ? c.t('refreshing') : c.t('refresh_now'))
          }))),
      E('div', { class: 'settings-section' },
        E('div', { class: 'settings-title' }, c.t('settings_data')),
        E('div', { class: 'set-group' },
          E(SetRow, { label: c.t('settings_channel'), value: channel, field: 'channel' }),
          E(Divider),
          E(SetRow, { label: c.t('settings_updated'), value: d.updated || '—', field: 'updated' }),
          E(Divider),
          E(SetRow, { label: c.t('settings_samples'), value: fmtInt(d.samples_n), field: 'samples' }),
          E(Divider),
          E(SetRow, { label: c.t('settings_sessions'), value: fmtInt(Array.isArray(d.sessions) ? d.sessions.length : null), field: 'sessions' }),
          E(Divider),
          E(SetRow, { label: c.t('settings_retention'), value: c.t('settings_retention_value') }),
          E(Divider),
          E(SetRow, { label: c.t('settings_storage'), value: '/data/adb/modules/CRecordEvolution/data', cls: 'set-value--path' }))),
      E('div', { class: 'settings-section' },
        E('div', { class: 'settings-title' }, c.t('settings_about')),
        E('div', { class: 'set-group' },
          E('div', { class: 'set-item set-item--stack' },
            E('div', { class: 'set-text' },
              E('div', { class: 'set-title' }, 'ChargingRecord Evolution'),
              E('div', { class: 'set-desc' }, c.t('settings_about_desc')))),
          E(Divider),
          E(SetRow, { label: c.t('settings_webui'), value: c.t('settings_webui_value') }))));
  }

  function Gate() {
    var c = useTr();
    return E('section', { class: 'gate' },
      E('div', { class: 'gate-title' }, c.t('gate_title')),
      E('div', { class: 'gate-desc' }, c.t('gate_desc')));
  }

