import { useState, useEffect, useRef } from 'preact/hooks';
import { E, useTr, HistStat, Seg } from '../ui/index.js';
import {
  CMD_HISTORY,
  HIST_PAGE_LIMIT,
  HIST_MODE_KEY,
  HIST_DAYS_KEY,
  fmtDayLabel,
  fmtInt,
  dayStart,
  storedHistMode,
  storedHistDays,
  run,
} from '../core.js';
import { TimelineEvent } from './timeline.js';
import { trendsView } from './trends.js';

function History(props) {
  const c = useTr();
  const lang = c.lang;
  const [mode, setMode] = useState(storedHistMode());
  const [filter, setFilter] = useState('all');
  const [days, setDays] = useState(storedHistDays());
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [err, setErr] = useState(null);
  const [open, setOpen] = useState(null);
  const [openTile, setOpenTile] = useState(null);
  const reqSeq = useRef(0);
  const dataRef = useRef(null);

  useEffect(
    function () {
      dataRef.current = data;
    },
    [data]
  );

  function load(reset) {
    if (!props.active) return;
    const prev = dataRef.current;
    const before = reset ? 0 : prev && prev.has_more ? prev.next_before : 0;
    if (!reset && !before) return;
    const seq = ++reqSeq.current;
    const type = mode === 'records' ? filter : 'all';
    if (reset) {
      setLoading(true);
      setErr(null);
      setOpen(null);
    } else setLoadingMore(true);
    let cmd = CMD_HISTORY + ' --days ' + days + ' --limit ' + HIST_PAGE_LIMIT + ' --type ' + type;
    if (before) cmd += ' --before ' + before;
    run(cmd).then(
      function (r) {
        if (seq !== reqSeq.current) return;
        if (r.errno !== 0) {
          setErr({ detail: r.stderr || 'errno ' + r.errno });
          setLoading(false);
          setLoadingMore(false);
          return;
        }
        const stdout = String(r.stdout || '').trim();
        if (!stdout) {
          setErr({ detail: 'empty' });
          setLoading(false);
          setLoadingMore(false);
          return;
        }
        let doc;
        try {
          doc = JSON.parse(stdout);
        } catch (e) {
          setErr({ detail: 'json' });
          setLoading(false);
          setLoadingMore(false);
          return;
        }
        if (!doc || !Array.isArray(doc.events)) {
          setErr({ detail: 'struct' });
          setLoading(false);
          setLoadingMore(false);
          return;
        }
        if (reset) {
          setData(doc);
        } else {
          setData(function (old) {
            if (!old) return doc;
            const seen = {},
              merged = [];
            old.events.concat(doc.events).forEach(function (ev) {
              const key = String(ev.kind) + ':' + String(ev.ts);
              if (seen[key]) return;
              seen[key] = 1;
              merged.push(ev);
            });
            return {
              events: merged,
              has_more: doc.has_more,
              next_before: doc.next_before,
              summary: doc.summary,
              trends: doc.trends,
            };
          });
        }
        setErr(null);
        setLoading(false);
        setLoadingMore(false);
      },
      function (e) {
        if (seq !== reqSeq.current) return;
        setErr({ detail: String((e && e.message) || e) });
        setLoading(false);
        setLoadingMore(false);
      }
    );
  }

  useEffect(
    function () {
      if (!props.active) return;
      load(true);
    },
    [props.active, props.reloadKey, days, filter, mode]
  );

  function changeMode(next) {
    if (next !== 'records' && next !== 'trends') return;
    if (next === mode) return;
    setMode(next);
    try {
      localStorage.setItem(HIST_MODE_KEY, next);
    } catch (e) {}
  }

  function changeFilter(next) {
    if (next !== 'all' && next !== 'charge' && next !== 'discharge') return;
    if (next !== filter) setFilter(next);
  }

  function changeDays(next) {
    if (next !== 7 && next !== 30 && next !== 90) return;
    if (next === days) return;
    setDays(next);
    try {
      localStorage.setItem(HIST_DAYS_KEY, String(next));
    } catch (e) {}
  }

  const events = data && Array.isArray(data.events) ? data.events : [];
  const summary = data ? data.summary : null;
  const trends = data && data.trends ? data.trends : {};

  const groups = [];
  const groupMap = {};
  events.forEach(function (ev, i) {
    const day = dayStart(ev.ts);
    if (!groupMap[day]) {
      groupMap[day] = { day: day, events: [] };
      groups.push(groupMap[day]);
    }
    groupMap[day].events.push({ ev: ev, key: String(ev.kind) + ':' + String(ev.ts) + ':' + i });
  });

  const chips = [];
  [7, 30, 90].forEach(function (d) {
    chips.push(
      E(
        'button',
        {
          key: d,
          type: 'button',
          class: 'chip' + (days === d ? ' is-active' : ''),
          'aria-pressed': days === d ? 'true' : 'false',
          onClick: function () {
            changeDays(d);
          },
        },
        c.t('hist_range_' + d)
      )
    );
  });
  if (mode === 'records') {
    chips.push(E('span', { key: 'sep', class: 'chip-sep', 'aria-hidden': 'true' }));
    [
      ['all', 'hist_filter_all'],
      ['charge', 'hist_filter_charge'],
      ['discharge', 'hist_filter_discharge'],
    ].forEach(function (it) {
      chips.push(
        E(
          'button',
          {
            key: it[0],
            type: 'button',
            class: 'chip' + (filter === it[0] ? ' is-active' : ''),
            'aria-pressed': filter === it[0] ? 'true' : 'false',
            onClick: function () {
              changeFilter(it[0]);
            },
          },
          c.t(it[1])
        )
      );
    });
  }

  const controls = E(
    'div',
    { class: 'hist-controls' },
    E(Seg, {
      id: 'hist_mode_seg',
      label: c.t('page_history'),
      value: mode,
      options: [
        { value: 'records', label: c.t('hist_mode_records') },
        { value: 'trends', label: c.t('hist_mode_trends') },
      ],
      onChange: changeMode,
    }),
    E('div', { class: 'hist-chips hist-chips--single', 'aria-label': c.t('hist_range_30') }, chips)
  );

  const summaryCards = E(
    'section',
    { class: 'hist-summary' },
    E(HistStat, {
      value: summary ? fmtInt(summary.valid_sessions) : '—',
      label: c.t('history_summary_valid'),
    }),
    E(HistStat, {
      value: summary ? fmtInt(summary.discharges) : '—',
      label: c.t('history_summary_discharges'),
    }),
    E(HistStat, {
      value: summary ? fmtInt(summary.samples) : '—',
      label: c.t('history_summary_samples'),
    })
  );

  let body;
  if (err && !data) {
    body = E(
      'div',
      { class: 'hist-empty' },
      E('div', { class: 't' }, c.t('err_run', { detail: err.detail })),
      E(
        'button',
        {
          type: 'button',
          class: 'load-more',
          onClick: function () {
            setData(null);
            load(true);
          },
        },
        c.t('hist_retry')
      )
    );
  } else if (!data && loading) {
    body = E(
      'div',
      null,
      E('div', { class: 'skl skl-summary' }),
      E('div', { class: 'skl skl-row' }),
      E('div', { class: 'skl skl-row' }),
      E('div', { class: 'skl skl-chart' })
    );
  } else if (mode === 'records') {
    if (!events.length) {
      body = E(
        'div',
        { class: 'hist-empty' },
        E(
          'div',
          { class: 't' },
          filter === 'all' ? c.t('hist_empty_records') : c.t('hist_empty_filter')
        ),
        filter !== 'all'
          ? E(
              'button',
              {
                type: 'button',
                class: 'load-more',
                onClick: function () {
                  changeFilter('all');
                },
              },
              c.t('hist_filter_all')
            )
          : null
      );
    } else {
      const timeline = E(
        'div',
        { class: 'timeline' },
        groups.map(function (g) {
          return E(
            'section',
            { class: 'tl-group', key: g.day },
            E(
              'div',
              { class: 'tl-date' },
              E('span', null, fmtDayLabel(lang, g.events[0].ev.ts)),
              E('span', { class: 'tl-count' }, c.t('hist_records_count', { n: g.events.length }))
            ),
            g.events.map(function (item) {
              return E(TimelineEvent, {
                key: item.key,
                event: item.ev,
                kind: item.ev.kind,
                open: open === item.key,
                onToggle: function () {
                  setOpen(open === item.key ? null : item.key);
                },
              });
            })
          );
        })
      );
      let foot;
      if (data && data.has_more) {
        foot = E(
          'div',
          { class: 'hist-foot' },
          E(
            'button',
            {
              type: 'button',
              class: 'load-more',
              disabled: loadingMore,
              onClick: function () {
                load(false);
              },
            },
            loadingMore ? c.t('loading') : c.t('hist_load_more')
          )
        );
      } else {
        foot = E(
          'div',
          { class: 'hist-foot' },
          E('div', { class: 'note' }, c.t('hist_all_loaded'))
        );
      }
      body = E('div', null, timeline, foot);
    }
  } else {
    body = trendsView(c, trends, openTile, setOpenTile);
  }

  return E(
    'section',
    { class: 'page' + (props.active ? ' is-active' : ''), id: 'page_history', role: 'tabpanel' },
    summaryCards,
    controls,
    body
  );
}
export { History };
