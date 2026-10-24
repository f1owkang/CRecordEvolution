'use strict';

function histEvent(c, ev) {
    var lang = c.lang;
    var ch = ev && ev.charge;
    var dis = ev && ev.discharge;
    var invalid = false;
    var title, primary, status, statusOk = false;
    var sub = [], detail = [];

    function addDetail(k, v) { if (v != null && v !== '') detail.push([k, v]); }

    if (ch) {
      title = c.t('panel_ses');
      primary = ch.delta_pct != null ? c.t('hist_session_gain', { n: fmtInt(ch.delta_pct) }) : c.t('panel_ses');
      invalid = ch.valid === false;
      if (invalid) {
        status = (ch.invalid_reason && REASON_KEY[ch.invalid_reason]) ? c.t(REASON_KEY[ch.invalid_reason]) : c.t('session_not_counted');
      } else {
        status = c.t('session_counted');
        statusOk = true;
      }
      if (ch.est_mah != null) sub.push(c.t('hist_session_est', { n: fmtInt(ch.est_mah) }));
      if (ch.temp_avg != null) sub.push(c.t('hist_session_temp', { n: fmtInt(ch.temp_avg) }));
      var mins = Math.round((Number(ch.end_ts) - Number(ch.start_ts)) / 60);
      if (isFinite(mins) && mins > 0) sub.push(c.t('hist_duration', { n: mins }));
      addDetail(c.t('hist_detail_time'), fmtRange(lang, ch.start_ts, ch.end_ts));
      if (ch.temp_avg != null) addDetail(c.t('hist_detail_temp'), fmtInt(ch.temp_avg) + ' ℃');
      if (ch.c_rate != null) addDetail(c.t('hist_detail_rate'), Number(ch.c_rate).toFixed(2) + ' C');
      if (ch.est_mah != null) addDetail(c.t('hist_detail_estimate'), fmtInt(ch.est_mah) + ' mAh');
    } else if (dis) {
      title = c.t('panel_dis');
      primary = fmtInt(dis.uah_mah) + ' mAh';
      if (dis.invalid_reason && REASON_KEY[dis.invalid_reason]) {
        status = c.t(REASON_KEY[dis.invalid_reason]);
        invalid = true;
      } else {
        status = c.t('hist_status_recorded');
        statusOk = true;
      }
      if (dis.start_cap != null && dis.end_cap != null) sub.push(fmtInt(dis.start_cap) + '→' + fmtInt(dis.end_cap) + '%');
      var dmins = Math.round(Number(dis.secs) / 60);
      if (isFinite(dmins)) sub.push(c.t('hist_duration', { n: dmins }));
      if (dis.implied_mah != null) sub.push(c.t('hist_implied', { n: fmtInt(dis.implied_mah) }));
      addDetail(c.t('hist_detail_time'), fmtTs(lang, dis.ts));
      addDetail(c.t('hist_detail_duration'), dmins + ' min');
      if (dis.start_cap != null && dis.end_cap != null) addDetail(c.t('hist_detail_soc'), fmtInt(dis.start_cap) + '→' + fmtInt(dis.end_cap) + '%');
      addDetail(c.t('hist_detail_released'), fmtInt(dis.uah_mah) + ' mAh');
      if (dis.implied_mah != null) addDetail(c.t('hist_detail_implied'), fmtInt(dis.implied_mah) + ' mAh');
    } else {
      return null;
    }

    return {
      title: title,
      primary: primary,
      status: status,
      statusOk: statusOk,
      invalid: invalid,
      sub: sub,
      detail: detail
    };
  }

  // 时间轴一行：默认只显示标题/时间/主指标/两个副指标，点按展开详情。
  function TimelineEvent(props) {
    var c = useTr();
    var info = histEvent(c, props.event);
    if (!info) return null;
    var detailEls = [];
    if (props.open) {
      info.detail.forEach(function (row, i) {
        detailEls.push(E('div', { key: 'k' + i, class: 'k' }, row[0]));
        detailEls.push(E('div', { key: 'v' + i, class: 'v' }, row[1]));
      });
    }
    return E('div', { class: 'tl-item ' + props.kind + (info.invalid ? ' is-invalid' : '') },
      E('div', { class: 'tl-rail' }, E('span', { class: 'tl-dot' })),
      E('div', {
        class: 'tl-card' + (props.open ? ' is-open' : '') + (info.invalid ? ' is-invalid' : ''),
        role: 'button',
        tabIndex: 0,
        'aria-expanded': props.open ? 'true' : 'false',
        'aria-label': info.title + ' ' + info.primary,
        onClick: props.onToggle,
        onKeyDown: function (e) {
          if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); props.onToggle(); }
        }
      },
        E('div', { class: 'tl-top' },
          E('span', { class: 'tl-kind' }, info.title),
          E('span', { class: 'tl-time' }, fmtClock(c.lang, props.event.ts)),
          info.status ? E('span', { class: 'pill ' + (info.statusOk ? 'ok' : 'warn') }, info.status) : null,
          E('span', { class: 'tl-chev', 'aria-hidden': 'true' })),
        E('div', { class: 'tl-value' }, info.primary),
        info.sub.length ? E('div', { class: 'tl-sub' }, info.sub.map(function (s, i) { return E('span', { key: i }, s); })) : null,
        props.open
          ? E('div', { class: 'tl-det' }, detailEls)
          : null));
  }

  function TrendTile(props) {
    var clickable = !!props.small;
    function onKey(e) {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (clickable) props.onToggle(); }
    }
    return E('div', {
      class: 'trend-tile' + (props.open ? ' is-open' : ''),
      role: clickable ? 'button' : null,
      tabIndex: clickable ? 0 : null,
      'aria-expanded': clickable ? (props.open ? 'true' : 'false') : null,
      onClick: clickable ? props.onToggle : null,
      onKeyDown: clickable ? onKey : null
    },
      E('div', { class: 'trend-head' },
        E('span', { class: 'trend-title' }, props.title),
        E('span', { class: 'trend-value' }, props.value)),
      clickable
        ? E('div', { class: 'trend-spark', dangerouslySetInnerHTML: { __html: props.small } })
        : E('div', { class: 'trend-empty' }, props.empty || '—'),
      clickable && props.open && props.big
        ? E('div', { class: 'trend-big', dangerouslySetInnerHTML: { __html: props.big } })
        : null,
      clickable && props.open && props.detail ? E('div', { class: 'trend-det' }, props.detail) : null,
      props.cap ? E('div', { class: 'trend-cap' }, props.cap) : null);
  }

  function History(props) {
    var c = useTr();
    var lang = c.lang;
    var modeS = useState(storedHistMode());
    var mode = modeS[0], setMode = modeS[1];
    var filterS = useState('all');
    var filter = filterS[0], setFilter = filterS[1];
    var daysS = useState(storedHistDays());
    var days = daysS[0], setDays = daysS[1];
    var dataS = useState(null);
    var data = dataS[0], setData = dataS[1];
    var loadingS = useState(false);
    var loading = loadingS[0], setLoading = loadingS[1];
    var moreS = useState(false);
    var loadingMore = moreS[0], setLoadingMore = moreS[1];
    var errS = useState(null);
    var err = errS[0], setErr = errS[1];
    var openS = useState(null);
    var open = openS[0], setOpen = openS[1];
    var openTileS = useState(null);
    var openTile = openTileS[0], setOpenTile = openTileS[1];
    var reqSeq = useRef(0);
    var dataRef = useRef(null);

    useEffect(function () { dataRef.current = data; }, [data]);

    function load(reset) {
      if (!props.active) return;
      var prev = dataRef.current;
      var before = reset ? 0 : (prev && prev.has_more ? prev.next_before : 0);
      if (!reset && !before) return;
      var seq = ++reqSeq.current;
      var type = mode === 'records' ? filter : 'all';
      if (reset) { setLoading(true); setErr(null); setOpen(null); }
      else setLoadingMore(true);
      var cmd = CMD_HISTORY + ' --days ' + days + ' --limit ' + HIST_PAGE_LIMIT + ' --type ' + type;
      if (before) cmd += ' --before ' + before;
      run(cmd).then(function (r) {
        if (seq !== reqSeq.current) return;
        if (r.errno !== 0) {
          setErr({ detail: r.stderr || ('errno ' + r.errno) });
          setLoading(false); setLoadingMore(false);
          return;
        }
        var stdout = String(r.stdout || '').trim();
        if (!stdout) { setErr({ detail: 'empty' }); setLoading(false); setLoadingMore(false); return; }
        var doc;
        try { doc = JSON.parse(stdout); } catch (e) { setErr({ detail: 'json' }); setLoading(false); setLoadingMore(false); return; }
        if (!doc || !Array.isArray(doc.events)) { setErr({ detail: 'struct' }); setLoading(false); setLoadingMore(false); return; }
        if (reset) {
          setData(doc);
        } else {
          setData(function (old) {
            if (!old) return doc;
            var seen = {}, merged = [];
            old.events.concat(doc.events).forEach(function (ev) {
              var key = String(ev.kind) + ':' + String(ev.ts);
              if (seen[key]) return;
              seen[key] = 1; merged.push(ev);
            });
            return { events: merged, has_more: doc.has_more, next_before: doc.next_before, summary: doc.summary, trends: doc.trends };
          });
        }
        setErr(null); setLoading(false); setLoadingMore(false);
      }, function (e) {
        if (seq !== reqSeq.current) return;
        setErr({ detail: String(e && e.message || e) });
        setLoading(false); setLoadingMore(false);
      });
    }

    useEffect(function () {
      if (!props.active) return;
      load(true);
    }, [props.active, props.reloadKey, days, filter, mode]);

    function changeMode(next) {
      if (next !== 'records' && next !== 'trends') return;
      if (next === mode) return;
      setMode(next);
      try { localStorage.setItem(HIST_MODE_KEY, next); } catch (e) {}
    }

    function changeFilter(next) {
      if (next !== 'all' && next !== 'charge' && next !== 'discharge') return;
      if (next !== filter) setFilter(next);
    }

    function changeDays(next) {
      if (next !== 7 && next !== 30 && next !== 90) return;
      if (next === days) return;
      setDays(next);
      try { localStorage.setItem(HIST_DAYS_KEY, String(next)); } catch (e) {}
    }

    var events = data && Array.isArray(data.events) ? data.events : [];
    var summary = data ? data.summary : null;
    var trends = data && data.trends ? data.trends : {};

    var groups = [];
    var groupMap = {};
    events.forEach(function (ev, i) {
      var day = dayStart(ev.ts);
      if (!groupMap[day]) {
        groupMap[day] = { day: day, events: [] };
        groups.push(groupMap[day]);
      }
      groupMap[day].events.push({ ev: ev, key: String(ev.kind) + ':' + String(ev.ts) + ':' + i });
    });

    var chips = [];
    [7, 30, 90].forEach(function (d) {
      chips.push(E('button', {
        key: d, type: 'button',
        class: 'chip' + (days === d ? ' is-active' : ''),
        'aria-pressed': days === d ? 'true' : 'false',
        onClick: function () { changeDays(d); }
      }, c.t('hist_range_' + d)));
    });
    if (mode === 'records') {
      chips.push(E('span', { key: 'sep', class: 'chip-sep', 'aria-hidden': 'true' }));
      [['all', 'hist_filter_all'], ['charge', 'hist_filter_charge'], ['discharge', 'hist_filter_discharge']].forEach(function (it) {
        chips.push(E('button', {
          key: it[0], type: 'button',
          class: 'chip' + (filter === it[0] ? ' is-active' : ''),
          'aria-pressed': filter === it[0] ? 'true' : 'false',
          onClick: function () { changeFilter(it[0]); }
        }, c.t(it[1])));
      });
    }

    var controls = E('div', { class: 'hist-controls' },
      E(Seg, {
        id: 'hist_mode_seg', label: c.t('page_history'), value: mode,
        options: [
          { value: 'records', label: c.t('hist_mode_records') },
          { value: 'trends', label: c.t('hist_mode_trends') }
        ],
        onChange: changeMode
      }),
      E('div', { class: 'hist-chips hist-chips--single', 'aria-label': c.t('hist_range_30') }, chips));

    var summaryCards = E('section', { class: 'hist-summary' },
      E(HistStat, { value: summary ? fmtInt(summary.valid_sessions) : '—', label: c.t('history_summary_valid') }),
      E(HistStat, { value: summary ? fmtInt(summary.discharges) : '—', label: c.t('history_summary_discharges') }),
      E(HistStat, { value: summary ? fmtInt(summary.samples) : '—', label: c.t('history_summary_samples') }));

    var body;
    if (err && !data) {
      body = E('div', { class: 'hist-empty' },
        E('div', { class: 't' }, c.t('err_run', { detail: err.detail })),
        E('button', { type: 'button', class: 'load-more', onClick: function () { setData(null); load(true); } }, c.t('hist_retry')));
    } else if (!data && loading) {
      body = E('div', null,
        E('div', { class: 'skl skl-summary' }),
        E('div', { class: 'skl skl-row' }),
        E('div', { class: 'skl skl-row' }),
        E('div', { class: 'skl skl-chart' }));
    } else if (mode === 'records') {
      if (!events.length) {
        body = E('div', { class: 'hist-empty' },
          E('div', { class: 't' }, filter === 'all' ? c.t('hist_empty_records') : c.t('hist_empty_filter')),
          filter !== 'all' ? E('button', { type: 'button', class: 'load-more', onClick: function () { changeFilter('all'); } }, c.t('hist_filter_all')) : null);
      } else {
        var timeline = E('div', { class: 'timeline' }, groups.map(function (g) {
          return E('section', { class: 'tl-group', key: g.day },
            E('div', { class: 'tl-date' },
              E('span', null, fmtDayLabel(lang, g.events[0].ev.ts)),
              E('span', { class: 'tl-count' }, c.t('hist_records_count', { n: g.events.length }))),
            g.events.map(function (item) {
              return E(TimelineEvent, {
                key: item.key, event: item.ev, kind: item.ev.kind,
                open: open === item.key,
                onToggle: function () { setOpen(open === item.key ? null : item.key); }
              });
            }));
        }));
        var foot;
        if (data && data.has_more) {
          foot = E('div', { class: 'hist-foot' },
            E('button', { type: 'button', class: 'load-more', disabled: loadingMore, onClick: function () { load(false); } },
              loadingMore ? c.t('loading') : c.t('hist_load_more')));
        } else {
          foot = E('div', { class: 'hist-foot' }, E('div', { class: 'note' }, c.t('hist_all_loaded')));
        }
        body = E('div', null, timeline, foot);
      }
    } else {
      body = trendsView(c, trends, openTile, setOpenTile);
    }

    return E('section', { class: 'page' + (props.active ? ' is-active' : ''), id: 'page_history', role: 'tabpanel' },
      summaryCards, controls, body);
  }

  function trendsView(c, tr, openTile, setOpenTile) {
    var lang = c.lang;
    tr = tr || {};
    function toggleTile(key) {
      setOpenTile(openTile === key ? null : key);
    }
    var estRows = Array.isArray(tr.estimates) ? tr.estimates : [];
    var estPoints = pickPairs(estRows, 'ts', 'mah');
    var capSvg = estPoints.length ? svgLine(estRows, CHART_W, CHART_H, 'var(--accent)') : '';
    var latestEst = estRows.length ? estRows[estRows.length - 1] : null;
    var capVal = latestEst ? fmtInt(latestEst.mah) + ' mAh' : '—';

    var sessions = (Array.isArray(tr.sessions) ? tr.sessions : []).filter(function (s) { return s && s.valid; });
    var sesBig = sessions.length ? svgBars(sessions, CHART_W, CHART_H, 'var(--blue)') : '';
    var sesVal = sessions.length ? plural(lang, sessions.length, 'hist_point_one', 'hist_point_many', { n: sessions.length }, 'hist_point_zero') : '—';
    var latestSes = sessions.length ? sessions[sessions.length - 1] : null;
    if (latestSes && latestSes.delta_pct != null) {
      sesVal = c.t('hist_session_gain', { n: fmtInt(latestSes.delta_pct) });
    }
    var sesCap = plural(lang, sessions.length, 'hist_point_one', 'hist_point_many', { n: sessions.length }, 'hist_point_zero');
    if (sessions.length >= 2) {
      sesCap += ' · ' + fmtShortDate(lang, sessions[0].end_ts) + ' – ' + fmtShortDate(lang, sessions[sessions.length - 1].end_ts);
    }

    var disRows = Array.isArray(tr.discharge) ? tr.discharge : [];
    var disPoints = pickPairs(disRows, 'ts', 'uah_mah');
    var disSvg = disPoints.length ? svgTsLine(disRows, CHART_W, 64, 'var(--accent)', 'uah_mah') : '';
    var disBig = disPoints.length ? svgTsLine(disRows, CHART_W, CHART_H, 'var(--accent)', 'uah_mah') : '';
    var disSum = 0;
    disRows.forEach(function (r) { var v = Number(r && r.uah_mah); if (isFinite(v)) disSum += v; });
    var disVal = disRows.length ? fmtInt(Math.round(disSum)) + ' mAh' : '—';

    var restRows = Array.isArray(tr.rest_points) ? tr.rest_points : [];
    var restPoints = pickPairs(restRows, 'ts', 'uv');
    var restSvg = restPoints.length ? svgScatter(restRows, CHART_W, 64, 'var(--rest-dot)') : '';
    var restBig = restPoints.length ? svgScatter(restRows, CHART_W, CHART_H, 'var(--rest-dot)') : '';
    var latestRest = restRows.length ? restRows[restRows.length - 1] : null;
    var restVal = latestRest ? (Number(latestRest.uv) / 1e6).toFixed(3) + ' V' : '—';

    var ccctRows = Array.isArray(tr.ccct) ? tr.ccct : [];
    var ccctPoints = pickPairs(ccctRows, 'ts', 'secs');
    var ccctSvg = ccctPoints.length ? svgTsLine(ccctRows, CHART_W, 64, 'var(--purple)', 'secs') : '';
    var ccctBig = ccctPoints.length ? svgTsLine(ccctRows, CHART_W, CHART_H, 'var(--purple)', 'secs') : '';
    var latestCcct = ccctRows.length ? ccctRows[ccctRows.length - 1] : null;
    var ccctVal = latestCcct ? (Number(latestCcct.secs) / 60).toFixed(1) + ' min' : '—';

    var icaRows = (Array.isArray(tr.ica_peaks) ? tr.ica_peaks : []).filter(function (r) { return r && r.peak_h_rel != null; });
    var icaPoints = pickPairs(icaRows, 'ts', 'peak_h_rel');
    var icaSvg = icaPoints.length ? svgTsLine(icaRows, CHART_W, 64, 'var(--orange)', 'peak_h_rel', 1) : '';
    var icaBig = icaPoints.length ? svgTsLine(icaRows, CHART_W, CHART_H, 'var(--orange)', 'peak_h_rel', 1) : '';
    var latestIca = icaRows.length ? icaRows[icaRows.length - 1] : null;
    var icaVal = latestIca ? Number(latestIca.peak_h_rel).toFixed(2) : '—';

    var any = estPoints.length || sessions.length || disPoints.length || restPoints.length || ccctPoints.length || icaPoints.length;
    if (!any) {
      return E('div', { class: 'hist-empty' }, E('div', { class: 't' }, c.t('hist_empty_trends')));
    }

    var pointsCap = function (points) {
      var txt = plural(lang, points.length, 'hist_point_one', 'hist_point_many', { n: points.length }, 'hist_point_zero');
      if (points.length >= 2) {
        txt += ' · ' + fmtShortDate(lang, points[0].x) + ' – ' + fmtShortDate(lang, points[points.length - 1].x);
      }
      return txt;
    };

    return E('div', { class: 'trend-wrap' },
      E('section', { class: 'trend-hero' },
        E('div', { class: 'trend-head' },
          E('span', { class: 'trend-title' }, c.t('hist_trend_capacity')),
          E('span', { class: 'trend-value' }, capVal)),
        capSvg
          ? E('div', { class: 'chart', dangerouslySetInnerHTML: { __html: capSvg } })
          : E('div', { class: 'trend-empty' }, c.t('hist_empty_trends')),
        E('div', { class: 'trend-cap' }, pointsCap(estPoints)),
        latestEst && estPoints.length >= 2 ? E('div', { class: 'trend-cap' },
          c.t('hist_est_delta', { n: (latestEst.mah - estRows[estRows.length - 2].mah >= 0 ? '+' : '') + fmtInt(latestEst.mah - estRows[estRows.length - 2].mah) })) : null),
      E('section', { class: 'trend-hero' },
        E('div', { class: 'trend-head' },
          E('span', { class: 'trend-title' }, c.t('hist_trend_sessions')),
          E('span', { class: 'trend-value' }, sesVal)),
        sesBig
          ? E('div', { class: 'chart', dangerouslySetInnerHTML: { __html: sesBig } })
          : E('div', { class: 'trend-empty' }, c.t('hist_empty_records')),
        E('div', { class: 'trend-cap' }, sesCap)),
      E('div', { class: 'trend-grid' },
        E(TrendTile, {
          title: c.t('hist_trend_discharge'), value: disVal, small: disSvg, big: disBig,
          open: openTile === 'discharge', onToggle: function () { toggleTile('discharge'); },
          cap: pointsCap(disPoints),
          empty: c.t('hist_empty_records')
        }),
        E(TrendTile, {
          title: c.t('hist_trend_rest'), value: restVal, small: restSvg, big: restBig,
          open: openTile === 'rest', onToggle: function () { toggleTile('rest'); },
          cap: pointsCap(restPoints),
          empty: c.t('hist_empty_trends')
        }),
        E(TrendTile, {
          title: c.t('hist_trend_ccct'), value: ccctVal, small: ccctSvg, big: ccctBig,
          open: openTile === 'ccct', onToggle: function () { toggleTile('ccct'); },
          cap: pointsCap(ccctPoints),
          empty: c.t('hist_empty_trends')
        }),
        E(TrendTile, {
          title: c.t('hist_trend_ica'), value: icaVal, small: icaSvg, big: icaBig,
          open: openTile === 'ica', onToggle: function () { toggleTile('ica'); },
          cap: pointsCap(icaPoints),
          empty: c.t('hist_empty_trends')
        })));
  }

