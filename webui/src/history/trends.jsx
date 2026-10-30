import { useEffect, useState } from 'preact/hooks';
import { tapIndex, selectedText } from '../chartTap.js';
import { useTr } from '../ui/index.js';
import { buildTrendModel } from './trendsModel.js';

// 读数行：未选中显示点按提示，选中后显示该点的「时间 · 数值」。
function TrendDet({ text, hint }) {
  return <div class="trend-det">{text == null ? <span class="hint">{hint}</span> : text}</div>;
}

// 小多图卡片：标题 + 最新值 + 迷你图；只有 small 存在时才是可点开大图的入口。
// 展开后的大图内点按只更新读数/高亮，不冒泡到卡片（否则会顺手把卡片收起）。
function TrendTile(props) {
  const clickable = !!props.small;
  function onKey(e) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      if (clickable) props.onToggle();
    }
  }
  function onBigClick(e) {
    e.stopPropagation();
    props.onSelect(tapIndex(e, props.bigCount));
  }
  return (
    <div
      class={'trend-tile' + (props.open ? ' is-open' : '')}
      role={clickable ? 'button' : null}
      tabIndex={clickable ? 0 : null}
      aria-expanded={clickable ? (props.open ? 'true' : 'false') : null}
      onClick={clickable ? props.onToggle : null}
      onKeyDown={clickable ? onKey : null}
    >
      <div class="trend-head">
        <span class="trend-title">{props.title}</span>
        <span class="trend-value">{props.value}</span>
      </div>
      {clickable ? (
        <div class="trend-spark" dangerouslySetInnerHTML={{ __html: props.small }} />
      ) : (
        <div class="trend-empty">{props.empty || '—'}</div>
      )}
      {clickable && props.open && props.big ? (
        <div>
          <div
            class="trend-big"
            onClick={onBigClick}
            dangerouslySetInnerHTML={{ __html: props.big }}
          />
          <TrendDet text={props.det} hint={props.hint} />
        </div>
      ) : null}
      {props.cap ? <div class="trend-cap">{props.cap}</div> : null}
    </div>
  );
}

// 大图卡片：容量与充电会话共用；extra 是容量卡独有的估算变化行。
function TrendHero(props) {
  function onClick(e) {
    props.onSelect(tapIndex(e, props.points.length));
  }
  return (
    <section class="trend-hero">
      <div class="trend-head">
        <span class="trend-title">{props.title}</span>
        <span class="trend-value">{props.value}</span>
      </div>
      {props.svg ? (
        <div class="chart" onClick={onClick} dangerouslySetInnerHTML={{ __html: props.svg }} />
      ) : (
        <div class="trend-empty">{props.empty}</div>
      )}
      {props.svg ? <TrendDet text={props.det} hint={props.hint} /> : null}
      <div class="trend-cap">{props.caption}</div>
      {props.extra ? <div class="trend-cap">{props.extra}</div> : null}
    </section>
  );
}

// 四张小多图：点开同一时间只展开一张，再次点按收起。
function TrendGrid({ tiles, openTile, onToggleTile }) {
  return (
    <div class="trend-grid">
      {tiles.map((tile) => (
        <TrendTile
          key={tile.key}
          title={tile.title}
          value={tile.value}
          small={tile.small}
          big={tile.big}
          open={openTile === tile.key}
          onToggle={() => onToggleTile(openTile === tile.key ? null : tile.key)}
          cap={tile.cap}
          empty={tile.empty}
          bigCount={tile.rows.length}
          det={tile.det}
          hint={tile.hint}
          onSelect={tile.onSelect}
        />
      ))}
    </div>
  );
}

// 趋势空态：整页只有一个提示，独立成组件让 TrendsView 少一层 JSX 缩进。
function TrendEmpty({ message }) {
  return (
    <div class="hist-empty">
      <div class="t">{message}</div>
    </div>
  );
}

const NO_SEL = { cap: null, ses: null, tileKey: null, tileIndex: null };

// 趋势视图：派生数据全部在 buildTrendModel，这里只维护「哪张图的哪个点被选中」
// 并把模型排成两张 hero + 四张小图。
function TrendsView({ trends, openTile, onToggleTile }) {
  const { t, lang } = useTr();
  const [sel, setSel] = useState(NO_SEL);
  // 数据换了一份（切范围/筛选/刷新）就清空选中，避免旧下标指向新的点。
  useEffect(
    function () {
      setSel(NO_SEL);
    },
    [trends]
  );
  const selMap = { cap: sel.cap, ses: sel.ses, tiles: {} };
  if (openTile != null && sel.tileKey === openTile) selMap.tiles[openTile] = sel.tileIndex;
  const model = buildTrendModel(trends, t, lang, selMap);
  if (!model.hasAnyData) return <TrendEmpty message={t('hist_empty_trends')} />;

  const hint = t('tap_hint');
  function selectCap(index) {
    setSel((old) => ({ ...old, cap: index }));
  }
  function selectSes(index) {
    setSel((old) => ({ ...old, ses: index }));
  }
  function selectTile(index) {
    setSel((old) =>
      index == null
        ? { ...old, tileKey: null, tileIndex: null }
        : { ...old, tileKey: openTile, tileIndex: index }
    );
  }
  // 展开/收起/换一张小图都清空小图的选中，避免再打开时残留上一次的读数。
  function toggleTile(key) {
    setSel((old) => (old.tileKey == null ? old : { ...old, tileKey: null, tileIndex: null }));
    onToggleTile(key);
  }

  const capacityHero = {
    title: t('hist_trend_capacity'),
    value: model.capVal,
    svg: model.capSvg,
    empty: t('hist_empty_trends'),
    caption: model.capCaption,
    extra: model.estDelta,
    points: model.capRows,
    det: selectedText(model.capRows, model.capFormat, sel.cap),
    hint,
    onSelect: selectCap,
  };
  const sessionsHero = {
    title: t('hist_trend_sessions'),
    value: model.sesVal,
    svg: model.sesBig,
    empty: t('hist_empty_records'),
    caption: model.sesCap,
    extra: model.sesNote,
    points: model.sesRows,
    det: selectedText(model.sesRows, model.sesFormat, sel.ses),
    hint,
    onSelect: selectSes,
  };
  const tiles = model.tiles.map(function (tile) {
    const open = openTile === tile.key;
    return {
      ...tile,
      hint,
      det: open
        ? selectedText(tile.rows, tile.format, sel.tileKey === tile.key ? sel.tileIndex : null)
        : null,
      onSelect: selectTile,
    };
  });
  return (
    <div class="trend-wrap">
      <TrendHero {...capacityHero} />
      <TrendHero {...sessionsHero} />
      <TrendGrid tiles={tiles} openTile={openTile} onToggleTile={toggleTile} />
    </div>
  );
}

export { TrendsView };
