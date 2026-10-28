import { useTr } from '../ui/index.js';
import { buildTrendModel } from './trendsModel.js';

// 小多图卡片：标题 + 最新值 + 迷你图；只有 small 存在时才是可点开大图的入口。
function TrendTile({ title, value, small, big, open, onToggle, cap, empty }) {
  const clickable = !!small;
  function onKey(e) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      if (clickable) onToggle();
    }
  }
  return (
    <div
      class={'trend-tile' + (open ? ' is-open' : '')}
      role={clickable ? 'button' : null}
      tabIndex={clickable ? 0 : null}
      aria-expanded={clickable ? (open ? 'true' : 'false') : null}
      onClick={clickable ? onToggle : null}
      onKeyDown={clickable ? onKey : null}
    >
      <div class="trend-head">
        <span class="trend-title">{title}</span>
        <span class="trend-value">{value}</span>
      </div>
      {clickable ? (
        <div class="trend-spark" dangerouslySetInnerHTML={{ __html: small }} />
      ) : (
        <div class="trend-empty">{empty || '—'}</div>
      )}
      {clickable && open && big ? (
        <div class="trend-big" dangerouslySetInnerHTML={{ __html: big }} />
      ) : null}
      {cap ? <div class="trend-cap">{cap}</div> : null}
    </div>
  );
}

// 大图卡片：容量与充电会话共用；extra 是容量卡独有的估算变化行。
function TrendHero({ title, value, svg, empty, caption, extra }) {
  return (
    <section class="trend-hero">
      <div class="trend-head">
        <span class="trend-title">{title}</span>
        <span class="trend-value">{value}</span>
      </div>
      {svg ? (
        <div class="chart" dangerouslySetInnerHTML={{ __html: svg }} />
      ) : (
        <div class="trend-empty">{empty}</div>
      )}
      <div class="trend-cap">{caption}</div>
      {extra ? <div class="trend-cap">{extra}</div> : null}
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

// 趋势视图：派生数据全部在 buildTrendModel，这里只把模型排成两张 hero + 四张小图。
function TrendsView({ trends, openTile, onToggleTile }) {
  const { t, lang } = useTr();
  const model = buildTrendModel(trends, t, lang);
  if (!model.hasAnyData) return <TrendEmpty message={t('hist_empty_trends')} />;

  const capacityHero = {
    title: t('hist_trend_capacity'),
    value: model.capVal,
    svg: model.capSvg,
    empty: t('hist_empty_trends'),
    caption: model.capCaption,
    extra: model.estDelta,
  };
  const sessionsHero = {
    title: t('hist_trend_sessions'),
    value: model.sesVal,
    svg: model.sesBig,
    empty: t('hist_empty_records'),
    caption: model.sesCap,
    extra: model.sesNote,
  };
  return (
    <div class="trend-wrap">
      <TrendHero {...capacityHero} />
      <TrendHero {...sessionsHero} />
      <TrendGrid tiles={model.tiles} openTile={openTile} onToggleTile={onToggleTile} />
    </div>
  );
}

export { TrendsView };
