import { useState, useEffect, useRef } from 'preact/hooks';
import { E, useTr, HistStat, Seg } from '../ui/index.js';
import {
  CMD_HISTORY, HIST_PAGE_LIMIT, HIST_MODE_KEY, HIST_DAYS_KEY,
  fmtDayLabel, fmtInt, dayStart, storedHistMode, storedHistDays, run
} from '../core.js';
import { TimelineEvent } from './timeline.js';
import { trendsView } from './trends.js';

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
export { History };
