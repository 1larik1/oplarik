import { t, tx, lang, LANGS } from './i18n.js';
import {
  S, esc, icon, nf, fmtBytes, fmtGB, fmtDiskSize, fmtDate, fmtTime, fmtUptime, ago, cover, coverHTML, st, isOpt, recommended,
  CAT_ICON, toggle, check, impact, gaugeSVG, ring, sparkPath,
} from './core.js';
import { accountSection } from './auth.js';
import { DARK_THEMES, LIGHT_THEMES } from './core.js';
import { shaderTool, dnsSection, sessionsSection, inputModeCard } from './tools.js';
import { autoGameCard, nvidiaSection, compareSection, profilesCard, updatesSection, ioSection } from './v107.js';
import { quickLaunchSection, playButton, gameMeta } from './v109.js';

const OPT_CATS = ['fps', 'power', 'gpu', 'input', 'network', 'privacy', 'qol'];

// ================================================================= HOME
function memChannel(info) {
  const n = (info.ram.modules || []).length;
  if (!n) return '';
  return n >= 2 ? t('f.dual') : t('f.single');
}

function ramLine(info) {
  const m = (info.ram.modules || [])[0];
  const parts = [fmtGB(info.ram.total)];
  if (m && m.conf) parts.push(`${nf(m.conf)} ${t('u.mts')}`);
  const ch = memChannel(info);
  if (ch) parts.push(ch);
  return parts.join(', ');
}

function specRow(ic, k, v) {
  return `<div class="spec"><div class="ic">${icon(ic)}</div><div class="k">${esc(k)}</div><div class="v" title="${esc(v)}">${esc(v || '—')}</div></div>`;
}

function tipHTML(tp) {
  const p = Object.assign({}, tp.p || {});
  if (p.date) p.date = fmtDate(p.date);
  let action = '';
  if (tp.code === 'vbs') action = `<button class="btn sm" data-a="drawer" data-id="vbs_off">${t('tip.vbs.a')}</button>`;
  if (tp.code === 'hz60' || tp.code === 'hz_low') action = `<button class="btn sm" data-a="open" data-what="display">${t('tip.hz60.a')}</button>`;
  if (tp.code === 'driver_old') {
    const g = (tp.p && tp.p.gpu || '').toUpperCase();
    const what = g.includes('AMD') || g.includes('RADEON') ? 'amd' : g.includes('INTEL') ? 'intel' : 'drivers';
    action = `<button class="btn sm" data-a="open" data-what="${what}">${icon('external')}${t('tip.driver_old.a')}</button>`;
  }
  return `<div class="tip ${tp.level}"><div class="ic">${icon(tp.level === 'warn' ? 'alert' : 'info')}</div>
    <div><h4>${esc(t(`tip.${tp.code}.t`, p))}</h4><p>${esc(t(`tip.${tp.code}.d`, p))}</p>${p.legacy ? `<p class="tip-warn">${icon('alert')}${esc(t(`tip.${tp.code}.legacy`))}</p>` : ''}</div><div>${action}</div></div>`;
}

export function homePage() {
  const info = S.info;
  const sc = S.score || 0;
  const recIds = S.tweaks.filter((tw) => isOpt(tw) && recommended(tw) && tw.risk === 'safe' && !st(tw.id).a).map((tw) => tw.id);
  const title = sc < 40 ? t('score.low') : sc < 80 ? t('score.mid') : t('score.high');
  const tier = info && info.tier ? t('tier.' + info.tier) : '';
  const optOn = S.tweaks.filter((tw) => isOpt(tw) && st(tw.id).a).length;
  const gamesOn = S.games.list.filter((g) => g.prio || g.gpu || g.fso).length;
  const appsGone = new Set(S.backups.history.filter((h) => h.ok && h.action === 'apply' && (S.byId[h.id] || {}).cat in { apps: 1, sponsored: 1 }).map((h) => h.id)).size;

  const next = S.tweaks.filter((tw) => isOpt(tw) && recommended(tw) && !st(tw.id).a && !st(tw.id).u)
    .sort((a, b) => b.imp - a.imp).slice(0, 4);

  const pc = info && info.scanAt ? `
    ${specRow('cpu', t('k.cpu'), info.cpu.name)}
    ${specRow('gpu', t('k.gpu'), (info.gpus || []).map((g) => g.name + (g.vram ? ` (${fmtGB(g.vram)})` : '')).join(', '))}
    ${specRow('ram', t('k.ram'), ramLine(info))}
    ${specRow('disk', t('k.disk'), `${info.sysDrive.media || ''} ${info.sysDrive.letter}, ${t('f.free').toLowerCase()} ${fmtBytes(info.sysDrive.free, 0)}`)}
    ${specRow('windows', t('k.os'), `${(info.os.name || '').replace('Microsoft ', '')} ${info.os.display || ''}`)}` :
    `<div class="skeleton" style="height:230px"></div>`;

  const boostOn = S.boost.session && S.boost.session.active;
  return `
  <section class="hero">
    <div class="card hero-main">
      <div class="gauge" id="gauge"><div class="gauge-glow"></div>${gaugeSVG(S.gaugeShown ? sc : 0)}
        <div class="gauge-center"><div class="gauge-val"><b>${nf(S.gaugeShown ? sc : 0, 1)}</b><small>%</small></div><div class="gauge-lbl">${t('score.lbl')}</div></div></div>
      <div class="hero-copy">
        <h2>${esc(title)}</h2>
        <p>${recIds.length ? esc(t('home.avail', { n: recIds.length, tier })) : esc(t('home.allDone'))}</p>
        <div class="row">
          ${recIds.length ? `<button class="btn primary lg" data-a="preset" data-id="recommended">${icon('rocket')}${t('btn.optimizeNow')}</button>` : ''}
          <button class="btn lg" data-a="go" data-page="optimize">${t('btn.review')}</button>
        </div>
        <div class="hero-stats">
          <div><b>${optOn}</b><span>${t('stat.active')}</span></div>
          <div><b>${gamesOn}</b><span>${t('stat.games')}</span></div>
          <div><b>${appsGone}</b><span>${t('stat.apps')}</span></div>
        </div>
      </div>
    </div>
    <div class="card pc-card">
      <div class="head"><h3>${t('home.yourPc')}</h3><span class="spacer"></span>${info && info.tier ? `<span class="tier">${icon('chip')}${t('tierB.' + info.tier)}</span>` : ''}</div>
      ${pc}
      <div class="foot"><button class="link" data-a="go" data-page="system">${t('home.fullSpecs')}${icon('chevronRight')}</button></div>
    </div>
  </section>

  <section class="sec">
    <div class="sec-head"><div><h2>${t('home.live')}</h2><p>${t('home.liveSub')}</p></div></div>
    <div class="live">${['cpu', 'gpu', 'ram'].map(liveMeter).join('')}</div>
  </section>

  <section class="sec">
    <div class="sec-head"><div><h2>${t('home.quick')}</h2></div></div>
    <div class="quick">
      <button class="qa" data-a="rp">${'<span class="ic">' + icon('shield') + '</span>'}<span><b>${t('qa.rp')}</b><span>${t('qa.rpSub')}</span></span></button>
      <button class="qa" data-a="tool" data-tool="ram"><span class="ic">${icon('ram')}</span><span><b>${t('qa.ram')}</b><span>${S.boost.results.ram || t('qa.ramSub')}</span></span></button>
      <button class="qa" data-a="tool" data-tool="temp"><span class="ic">${icon('trash')}</span><span><b>${t('qa.temp')}</b><span>${S.boost.results.temp || t('qa.tempSub')}</span></span></button>
      <button class="qa ${boostOn ? 'active' : ''}" data-a="boost-toggle"><span class="ic">${icon('zap')}</span><span><b>${t('qa.boost')}</b><span>${boostOn ? t('qa.boostOn') : t('qa.boostOff')}</span></span></button>
    </div>
  </section>

  ${next.length ? `<section class="sec">
    <div class="sec-head"><div><h2>${t('home.next')}</h2><p>${t('home.nextSub')}</p></div></div>
    <div class="next-list">${next.map((tw) => `<div class="next-item"><div style="min-width:0;cursor:pointer" data-a="drawer" data-id="${tw.id}"><h4>${esc(tx(tw.name))} ${impact(tw.imp)}</h4><p>${esc(tx(tw.desc))}</p></div>
      ${toggle(false, `data-a="toggle" data-id="${tw.id}"`, { busy: S.pending.has(tw.id) })}</div>`).join('')}</div>
  </section>` : ''}

  <section class="sec">
    <div class="sec-head"><div><h2>${t('home.tips')}</h2><p>${t('home.tipsSub')}</p></div></div>
    <div class="tips">${info && info.scanAt ? ((info.tips || []).length ? info.tips.map(tipHTML).join('') : `<div class="empty">${icon('ok')}<div>${t('home.noTips')}</div></div>`) : `<div class="skeleton" style="height:76px"></div>`}</div>
  </section>`;
}

function liveMeter(k) {
  const L = S.live.last;
  let val = '—', sub = '', pct = 0;
  if (L) {
    if (k === 'cpu') { pct = L.cpu; val = nf(L.cpu, 0); sub = L.cpuTemp > 0 ? `${nf(L.cpuTemp, 0)} °C` : ''; }
    if (k === 'gpu') { pct = L.gpu < 0 ? 0 : L.gpu; val = L.gpu < 0 ? '—' : nf(L.gpu, 0); sub = [L.gpuTemp > 0 ? `${nf(L.gpuTemp, 0)} °C` : '', L.vram ? `${t('vram')} ${fmtBytes(L.vram)}` : ''].filter(Boolean).join(', '); }
    if (k === 'ram') { pct = L.ramTotal ? (L.ramUsed / L.ramTotal) * 100 : 0; val = nf(pct, 0); sub = `${fmtBytes(L.ramUsed)} / ${fmtBytes(L.ramTotal, 0)}`; }
  }
  const sp = sparkPath(S.live[k]);
  const ic = { cpu: 'cpu', gpu: 'gpu', ram: 'ram' }[k];
  return `<div class="card meter" id="m-${k}">
    <div class="lbl">${icon(ic)}${t(k)}</div>
    <div class="val"><span class="v">${val}</span><small>%</small></div>
    <div class="sub">${sub || '&nbsp;'}</div>
    ${ring(pct, 'r-' + k)}
    <svg class="spark" viewBox="0 0 300 44" preserveAspectRatio="none"><path class="a" d="${sp.area || ''}" fill="url(#sparkFill)"/><path class="l" d="${sp.line || ''}" fill="none" style="stroke:var(--violet-2)" stroke-width="1.6" vector-effect="non-scaling-stroke"/></svg>
  </div>`;
}

export function updateLive() {
  const L = S.live.last;
  if (!L) return;
  for (const k of ['cpu', 'gpu', 'ram']) {
    const el = document.getElementById('m-' + k);
    if (!el) continue;
    let pct = 0, val = '—', sub = '';
    if (k === 'cpu') { pct = L.cpu; val = nf(L.cpu, 0); sub = L.cpuTemp > 0 ? `${nf(L.cpuTemp, 0)} °C` : ''; }
    if (k === 'gpu') { pct = L.gpu < 0 ? 0 : L.gpu; val = L.gpu < 0 ? '—' : nf(L.gpu, 0); sub = [L.gpuTemp > 0 ? `${nf(L.gpuTemp, 0)} °C` : '', L.vram ? `${t('vram')} ${fmtBytes(L.vram)}` : ''].filter(Boolean).join(', '); }
    if (k === 'ram') { pct = L.ramTotal ? (L.ramUsed / L.ramTotal) * 100 : 0; val = nf(pct, 0); sub = `${fmtBytes(L.ramUsed)} / ${fmtBytes(L.ramTotal, 0)}`; }
    el.querySelector('.v').textContent = val;
    el.querySelector('.sub').innerHTML = sub || '&nbsp;';
    const fg = el.querySelector('.ring .fg');
    const c = 2 * Math.PI * 38;
    fg.setAttribute('stroke-dashoffset', (c * (1 - Math.max(0, Math.min(100, pct)) / 100)).toFixed(1));
    const sp = sparkPath(S.live[k]);
    el.querySelector('.spark .a').setAttribute('d', sp.area || '');
    el.querySelector('.spark .l').setAttribute('d', sp.line || '');
  }
}

// ================================================================= OPTIMIZE
export function tweakBadges(tw) {
  const s = st(tw.id);
  const b = [];
  if (s.f === false && !s.na) b.push(`<span class="tag warn" title="${esc((s.w || []).map((w) => t('why.' + w)).join(', '))}">${icon('alert')}${t('b.notFit')}</span>`);
  if (tw.restart) b.push(`<span class="tag gray">${icon(tw.restart === 'pc' ? 'restore' : tw.restart === 'signout' ? 'logout' : 'refresh')}${t('b.' + tw.restart)}</span>`);
  if (tw.tags.includes('power')) b.push(`<span class="tag info">${icon('bolt')}${t('b.power')}</span>`);
  if (tw.tags.includes('security')) b.push(`<span class="tag bad">${icon('shieldOff')}${t('b.security')}</span>`);
  if (tw.tags.includes('win11')) b.push(`<span class="tag gray">${icon('windows')}${t('b.win11')}</span>`);
  return b.join('');
}

function stateLabel(s, busy) {
  if (busy) return t('st.checking');
  if (s.u) return t('st.checking');
  if (s.na) return t('st.na');
  if (s.a) return t('st.on');
  if (s.p) return t('st.partial');
  return t('st.off');
}

export function tweakCard(tw) {
  const s = st(tw.id);
  const busy = S.pending.has(tw.id);
  const saved = S.saved.has(tw.id);
  return `<article class="card tw ${s.a ? 'on' : ''} ${s.na ? 'na' : ''}" data-card="${tw.id}">
    <div class="tw-top"><span class="tag">${icon(CAT_ICON[tw.cat])}${t('cat.' + tw.cat)}</span>
      ${recommended(tw) ? `<span class="tag good">${t('b.rec')}</span>` : ''}
      <span class="risk ${tw.risk}">${t('risk.' + tw.risk)}</span></div>
    <h3>${esc(tx(tw.name))}</h3>
    <p>${esc(tx(tw.desc))}</p>
    <div class="tw-badges">${s.na ? `<span class="tag gray">${(s.w || ['missing']).map((w) => t('why.' + w)).join(', ')}</span>` : tweakBadges(tw)}</div>
    <div class="tw-foot">
      <button class="iconbtn ${saved ? 'on' : ''}" data-a="save" data-id="${tw.id}" title="${t('bookmark')}">${icon(saved ? 'bookmarkOn' : 'bookmark')}</button>
      <button class="iconbtn" data-a="drawer" data-id="${tw.id}" title="${t('details')}">${icon('info')}</button>
      ${impact(tw.imp)}
      <span class="grow"></span>
      <span class="tg-wrap ${s.a ? 'on' : ''}">${stateLabel(s, busy)}
      ${toggle(!!s.a, `data-a="toggle" data-id="${tw.id}" aria-label="${esc(tx(tw.name))}"`, { busy, partial: !!s.p && !s.a, disabled: !!s.na || !!s.u })}</span>
    </div>
  </article>`;
}

function optFilter() {
  const q = S.opt.q.trim().toLowerCase();
  return S.tweaks.filter((tw) => {
    if (!isOpt(tw)) return false;
    if (S.opt.tab === 'recommended' && !recommended(tw)) return false;
    if (S.opt.tab === 'advanced' && tw.risk === 'safe') return false;
    if (S.opt.tab === 'saved' && !S.saved.has(tw.id)) return false;
    if (S.opt.cat !== 'all' && tw.cat !== S.opt.cat) return false;
    if (q) {
      const hay = `${tx(tw.name)} ${tx(tw.desc)} ${tw.name.en} ${tw.desc.en} ${t('cat.' + tw.cat)}`.toLowerCase();
      if (!hay.includes(q)) return false;
    }
    return true;
  });
}

export function presetIds(pid) {
  const opt = S.tweaks.filter(isOpt);
  const ok = (tw) => { const s = st(tw.id); return !s.na && s.f !== false; };
  const p = (S.boot.presets || []).find((x) => x.id === pid);
  const extra = new Set((p && p.extra) || []);
  if (pid === 'recommended') return opt.filter((tw) => tw.rec && tw.risk === 'safe' && ok(tw)).map((tw) => tw.id);
  if (pid === 'competitive') return opt.filter((tw) => ((tw.rec && tw.risk === 'safe') || extra.has(tw.id)) && ok(tw)).map((tw) => tw.id);
  if (pid === 'privacy') return opt.filter((tw) => tw.cat === 'privacy' && tw.risk !== 'advanced' && ok(tw)).map((tw) => tw.id);
  return [];
}

export function optimizePage() {
  const list = optFilter();
  const pIcons = { recommended: 'sparkles', competitive: 'swords', privacy: 'eyeOff' };
  const presets = (S.boot.presets || []).map((p) => {
    const ids = presetIds(p.id);
    const todo = ids.filter((id) => !st(id).a).length;
    return `<div class="card preset ${p.id === 'recommended' ? 'featured' : ''}">
      <div class="ic">${icon(pIcons[p.id] || 'sparkles')}</div>
      <h3>${esc(tx(p.name))}</h3>
      <p>${esc(tx(p.desc))}</p>
      <div class="row"><span class="count">${t('preset.count', { n: ids.length })}</span><span class="spacer"></span>
        ${todo ? `<button class="btn sm ${p.id === 'recommended' ? 'primary' : ''}" data-a="preset" data-id="${p.id}">${t('preset.apply')}</button>` : `<span class="tag good">${icon('check')}${t('preset.allOn')}</span>`}</div>
    </div>`;
  }).join('');

  const optAll = S.tweaks.filter(isOpt);
  const counts = {
    all: optAll.length, recommended: optAll.filter(recommended).length,
    advanced: optAll.filter((tw) => tw.risk !== 'safe').length, saved: optAll.filter((tw) => S.saved.has(tw.id)).length,
  };
  const tabs = ['all', 'recommended', 'advanced', 'saved'].map((k) =>
    `<button class="tab ${S.opt.tab === k ? 'on' : ''}" data-a="opt-tab" data-v="${k}">${t('tab.' + k)}<span class="n">${counts[k]}</span></button>`).join('');
  const chips = ['all', ...OPT_CATS].map((c) =>
    `<button class="chip ${S.opt.cat === c ? 'on' : ''}" data-a="opt-cat" data-v="${c}">${c !== 'all' ? icon(CAT_ICON[c]) : ''}${t('cat.' + c)}</button>`).join('');

  let body;
  if (!list.length) {
    const msg = S.opt.q ? t('empty.search', { q: esc(S.opt.q) }) : S.opt.tab === 'saved' ? t('empty.saved') : t('empty.cat');
    body = `<div class="empty">${icon(S.opt.tab === 'saved' ? 'bookmark' : 'search')}<div>${msg}</div></div>`;
  } else {
    body = `<div class="tweaks">${list.map(tweakCard).join('')}</div>`;
  }
  return `<section class="presets">${presets}</section>
    <div class="toolbar"><div class="tabs" role="tablist">${tabs}</div></div>
    <div class="chips">${chips}</div>
    ${body}`;
}

// ================================================================= DRAWER
export function drawerHTML(id) {
  const tw = S.byId[id];
  if (!tw) return '';
  const s = st(id);
  const busy = S.pending.has(id);
  const appliedAt = S.backups.applied[id];
  const ops = tw.ops.map((o) => o.code
    ? `<details class="op"><summary>${esc(o.text)}</summary><pre>${esc(o.code)}</pre></details>`
    : `<div class="op">${esc(o.text)}</div>`).join('');
  return `<div class="drawer-head">
      <div class="row"><span class="tag">${icon(CAT_ICON[tw.cat])}${t('cat.' + tw.cat)}</span>${recommended(tw) ? `<span class="tag good">${t('b.rec')}</span>` : ''}
        <span class="spacer"></span><button class="iconbtn" data-a="drawer-close" aria-label="${t('btn.close')}">${icon('x')}</button></div>
      <h2>${esc(tx(tw.name))}</h2>
    </div>
    <div class="drawer-body">
      <p>${esc(tx(tw.desc))}</p>
      <div class="tw-badges">${tweakBadges(tw)}</div>
      ${tw.warn ? `<h5>${t('dr.warn')}</h5><div class="note warn">${icon('alert')}<div>${esc(tx(tw.warn))}</div></div>` : ''}
      ${s.f === false && !s.na ? `<h5>${t('dr.unfit')}</h5><div class="note warn">${icon('alert')}<div>${(s.w || []).map((w) => esc(t('why.' + w))).join('. ')}.</div></div>` : ''}
      <h5>${t('dr.risk')}</h5>
      <div class="row" style="align-items:flex-start"><span class="risk ${tw.risk}" style="margin:2px 0 0">${t('risk.' + tw.risk)}</span><span class="muted" style="font-size:13px">${t('riskD.' + tw.risk)}</span></div>
      <h5>${t('imp')}</h5><div class="row">${impact(tw.imp)}<span class="muted" style="font-size:13px">${t('imp.' + tw.imp)}</span></div>
      <h5>${t('dr.changes')}</h5>
      <div class="ops">${ops}</div>
      <h5>${t('bk.title')}</h5>
      <p class="muted" style="font-size:13px">${appliedAt ? esc(t('dr.backup', { date: fmtDate(appliedAt, true) })) : esc(t('dr.nobackup'))}</p>
    </div>
    <div class="drawer-foot">
      <button class="btn ghost" data-a="save" data-id="${id}">${icon(S.saved.has(id) ? 'bookmarkOn' : 'bookmark')}${t('bookmark')}</button>
      <span class="spacer"></span>
      ${s.na ? `<span class="tag gray">${t('st.na')}</span>` : s.a
        ? `<button class="btn" data-a="set" data-id="${id}" data-on="0" ${busy ? 'disabled' : ''}>${icon('undo')}${t('btn.turnOff')}</button>`
        : `<button class="btn primary" data-a="set" data-id="${id}" data-on="1" ${busy || s.u ? 'disabled' : ''}>${icon('zap')}${t('btn.turnOn')}</button>`}
    </div>`;
}

// ================================================================= GAMES
function gameFlag(g, key, title, desc) {
  const needsPath = key !== 'prio' && !g.path;
  return `<div class="gflag ${needsPath ? 'dis' : ''}" title="${esc(needsPath ? t('g.needPath') : desc)}">${toggle(!!g[key], `data-a="game-flag" data-id="${g.id}" data-k="${key}" aria-label="${esc(title)}"`, { disabled: needsPath || S.pending.has(g.id + key), busy: S.pending.has(g.id + key) })}${needsPath ? `<span style="font-size:11.5px">${t('g.needPath')}</span>` : ''}</div>`;
}

export function gamesPage() {
  const G = S.games;
  const added = (f) => G.list.some((g) => (f.path && g.path && g.path.toLowerCase() === f.path.toLowerCase()) || (!f.path && g.exe.toLowerCase() === f.exe.toLowerCase()));
  let found = '';
  if (G.scanning) found = `<div class="card pad row">${icon('loader', 'spin')}<span class="muted">${t('g.scanning')}</span></div>`;
  else if (G.found) {
    found = G.found.length ? `<div class="found">${G.found.map((f, i) => `<div class="found-item ${added(f) ? 'is-added' : ''}">${coverHTML(f.name, f.cover, 'big')}
      <div class="fi-meta"><b title="${esc(f.name)}">${esc(f.name)}</b><span>${t('g.src.' + f.source)}</span></div>
      ${added(f) ? `<span class="tag good">${icon('check')}${t('g.added')}</span>` : `<button class="btn sm" data-a="game-add" data-i="${i}">${icon('plus')}${t('g.add')}</button>`}</div>`).join('')}</div>`
      : `<div class="empty">${icon('search')}<div>${t('g.noneFound')}</div></div>`;
  }
  const rows = G.list.length ? `<div class="card"><div class="games-head"><span></span><span>${t('g.col.game')}</span>
      <span class="gh" title="${t('g.prioD')}">${t('g.prio')}</span><span class="gh" title="${t('g.gpuD')}">${t('g.gpu')}</span><span class="gh" title="${t('g.fsoD')}">${t('g.fso')}</span><span></span></div>
      ${G.list.map((g) => `<div class="game">${coverHTML(g.name, g.cover)}
        <div style="min-width:0"><h4 title="${esc(g.name)}">${esc(g.name)}</h4><p class="g-meta" title="${esc(g.path || g.exe)}">${esc(gameMeta(g))}</p></div>
        ${gameFlag(g, 'prio', t('g.prio'), t('g.prioD'))}${gameFlag(g, 'gpu', t('g.gpu'), t('g.gpuD'))}${gameFlag(g, 'fso', t('g.fso'), t('g.fsoD'))}
        <button class="iconbtn" data-a="game-remove" data-id="${g.id}" title="${t('g.remove')}">${icon('trash')}</button></div>`).join('')}</div>`
    : `<div class="empty">${icon('games')}<div>${t('g.empty')}</div></div>`;
  const popular = G.popular.filter((p) => !G.list.some((g) => g.exe.toLowerCase() === p.exe.toLowerCase()));
  return `${quickLaunchSection()}
    <div class="row ${G.list.length ? 'sec' : ''}" style="flex-wrap:wrap">
      <button class="btn primary" data-a="game-scan" ${G.scanning ? 'disabled' : ''}>${icon('scan')}${t('g.scan')}</button>
      <button class="btn" data-a="game-browse">${icon('plus')}${t('g.browse')}</button>
    </div>
    ${G.found || G.scanning ? `<section class="sec"><div class="sec-head"><div><h2>${t('g.found')}</h2></div></div>${found}</section>` : ''}
    <section class="sec"><div class="sec-head"><div><h2>${t('g.yours')}</h2></div></div>${rows}</section>
    ${popular.length ? `<section class="sec"><div class="sec-head"><div><h2>${t('g.popular')}</h2><p>${t('g.popularSub')}</p></div></div>
      <div class="pop">${popular.map((p) => `<button class="chip" data-a="game-pop" data-exe="${esc(p.exe)}" data-name="${esc(p.name)}">${icon('plus')}${esc(p.name)}</button>`).join('')}</div></section>` : ''}
    ${compareSection()}
    ${sessionsSection()}
    <section class="sec"><div class="note">${icon('info')}<div>${t('g.anticheat')}</div></div></section>`;
}

// ================================================================= BOOST
export function boostPage() {
  const B = S.boost;
  const on = B.session && B.session.active;
  const tools = [
    { k: 'ram', ic: 'ram', t: t('tool.ram'), d: t('tool.ramD') },
    { k: 'temp', ic: 'trash', t: t('tool.temp'), d: t('tool.tempD'), extra: B.temp ? t('tool.tempS', { size: fmtBytes(B.temp.bytes) }) : '' },
    { k: 'dns', ic: 'signal', t: t('tool.dns'), d: t('tool.dnsD') },
    { k: 'explorer', ic: 'folder', t: t('tool.exp'), d: t('tool.expD') },
    shaderTool(),
  ];
  const sel = [...B.sel].filter((e) => B.bg.some((b) => b.exe === e));
  return `<section class="card boost-hero ${on ? 'on' : ''}">
      <div>
        <h2>${icon('zap')}${t('bo.session')}${on ? `<span class="tag good">${t('bo.since', { time: fmtTime(B.session.since) })}</span>` : ''}</h2>
        <p>${t('bo.sessionD')}</p>
        ${on && B.session.stopped ? `<p class="muted" style="margin-top:8px;font-size:13px">${t('bo.paused', { n: B.session.stopped.length })}</p>` : ''}
        <div class="row" style="margin-top:18px"><button class="btn ${on ? '' : 'primary'} lg" data-a="boost-toggle" ${B.busy.session ? 'disabled' : ''}>${icon(on ? 'power' : 'play')}${on ? t('bo.stop') : t('bo.start')}</button></div>
      </div>
      <button class="boost-orb ${on ? 'on' : ''}" data-a="boost-toggle" aria-label="${t('bo.session')}">${icon('zap')}</button>
    </section>
    ${autoGameCard()}
    ${inputModeCard()}
    ${nvidiaSection()}
    <section class="sec"><div class="grid gtools">${tools.map((x) => `<div class="card tool"><div class="ic">${icon(x.ic)}</div><h3>${x.t}</h3><p>${x.d}</p>
      <div class="res">${esc(B.results[x.k] || x.extra || '')}</div>
      <div class="row"><button class="btn sm" data-a="tool" data-tool="${x.k}" ${B.busy[x.k] ? 'disabled' : ''}>${B.busy[x.k] ? icon('loader') : icon('play')}${t('btn.run')}</button></div></div>`).join('')}</div></section>
    <section class="sec"><div class="sec-head"><div><h2>${t('bg.title')}</h2><p>${t('bg.sub')}</p></div>
      <div class="right"><button class="iconbtn" data-a="bg-refresh" title="${t('d.rescan')}">${icon('refresh')}</button>
      ${B.bg.length ? `<button class="btn sm primary" data-a="bg-close" ${sel.length ? '' : 'disabled'}>${t('bg.close', { n: sel.length })}</button>` : ''}</div></div>
      <div class="card">${B.bg.length ? `<div class="list">${B.bg.map((b) => `<div class="li">${check(B.sel.has(b.exe), `data-a="bg-sel" data-exe="${esc(b.exe)}"`)}
        <div><h4>${esc(b.name)}</h4><p>${esc(b.exe)} · ${t('bg.inst', { n: b.count })}</p></div><div class="actions"><span class="muted num">${fmtBytes(b.mem)}</span></div></div>`).join('')}</div>`
        : `<div class="empty" style="border:0">${icon('ok')}<div>${t('bg.empty')}</div></div>`}</div></section>
    ${dnsSection()}`;
}

// ================================================================= DEBLOAT
function debRow(tw) {
  const s = st(tw.id);
  const app = tw.cat === 'apps' || tw.cat === 'sponsored';
  if (app) {
    return `<div class="li" style="padding-top:11px;padding-bottom:11px">${check(S.deb.sel.has(tw.id), `data-a="deb-sel" data-id="${tw.id}"`)}
      <div><h4>${esc(tx(tw.name))}${tw.rec ? `<span class="tag good">${t('b.rec')}</span>` : ''}${tw.risk !== 'safe' ? `<span class="risk ${tw.risk}" style="margin:0">${t('risk.' + tw.risk)}</span>` : ''}</h4>
      ${tw.warn ? `<p style="color:var(--warn)">${esc(tx(tw.warn))}</p>` : ''}</div>
      <div class="actions">${S.pending.has(tw.id) ? icon('loader') : ''}</div></div>`;
  }
  return `<div class="li"><div class="li-ic">${icon(CAT_ICON[tw.cat])}</div>
    <div><h4>${esc(tx(tw.name))}${recommended(tw) && !s.a ? `<span class="tag good">${t('b.recOff')}</span>` : ''}${tw.risk !== 'safe' ? `<span class="risk ${tw.risk}" style="margin:0">${t('risk.' + tw.risk)}</span>` : ''}
      ${s.f === false && !s.na ? `<span class="tag warn">${(s.w || []).map((w) => t('why.' + w)).join(', ')}</span>` : ''}</h4>
      <p>${esc(tx(tw.desc))}${tw.warn ? ` <span style="color:var(--warn)">${esc(tx(tw.warn))}</span>` : ''}</p></div>
    <div class="actions">${s.na ? `<span class="tag gray">${t('why.missing')}</span>` : `<span class="tg-wrap ${!s.a ? 'on' : ''}">${S.pending.has(tw.id) || s.u ? t('st.checking') : s.a ? t('d.disabled') : t('d.enabled')}${toggle(!s.a, `data-a="toggle-inv" data-id="${tw.id}" aria-label="${esc(tx(tw.name))}"`, { busy: S.pending.has(tw.id), disabled: !!s.u })}</span>`}</div></div>`;
}

export function debloatPage() {
  const D = S.deb;
  if (!D.scanned) {
    return `<section class="card scan-hero"><div class="big-ic">${icon(D.scanning ? 'loader' : 'scan')}</div>
      <h3>${t('d.scanT')}</h3><p>${t('d.scanD')}</p>
      <button class="btn primary lg" data-a="deb-scan" ${D.scanning ? 'disabled' : ''}>${D.scanning ? t('loading') : t('d.scanBtn')}</button></section>
      <section class="sec"><div class="note">${icon('shield')}<div>${t('d.keep')}</div></div></section>`;
  }
  const tabs = ['apps', 'sponsored', 'components', 'services'];
  const items = (c) => S.tweaks.filter((tw) => tw.cat === c);
  const present = (tw) => !st(tw.id).a;
  const counts = {
    apps: items('apps').filter(present).length, sponsored: items('sponsored').filter(present).length,
    components: items('components').filter((tw) => !st(tw.id).na).length, services: items('services').filter((tw) => !st(tw.id).na).length,
  };
  const tabBar = tabs.map((k) => `<button class="tab ${D.tab === k ? 'on' : ''}" data-a="deb-tab" data-v="${k}">${icon(CAT_ICON[k])}${t('cat.' + k)}<span class="n">${counts[k]}</span></button>`).join('');
  let body = '';
  const list = items(D.tab);
  if (D.tab === 'apps' || D.tab === 'sponsored') {
    const inst = list.filter(present);
    const gone = list.filter((tw) => !present(tw));
    const selN = [...D.sel].filter((id) => inst.some((tw) => tw.id === id)).length;
    body = `<p class="muted" style="margin:0 0 12px;font-size:13px">${esc(tx((list[0] || {}).desc))}</p><div class="card"><div class="list">
        ${inst.length ? inst.map(debRow).join('') : `<div class="empty" style="border:0">${icon('ok')}<div>${t('d.clean')}</div></div>`}
        ${gone.length ? `<button class="group-head" style="width:100%;border-left:0;border-right:0;border-bottom:0;text-align:left;cursor:pointer;color:var(--muted)" data-a="deb-gone">${icon(D.showGone ? 'chevronDown' : 'chevronRight')}${t('d.gone', { n: gone.length })}</button>
          ${D.showGone ? gone.map((tw) => `<div class="li dim"><div class="li-ic" style="opacity:.6">${icon('check')}</div><div><h4>${esc(tx(tw.name))}</h4><p>${t('d.notInst')}</p></div>
            <div class="actions"><button class="link" data-a="deb-reinstall" data-id="${tw.id}">${icon('store')}${t('d.reinstall')}</button></div></div>`).join('') : ''}` : ''}
      </div></div>
      ${inst.length ? `<div class="sel-bar"><span class="muted">${t('d.installed')}: <b class="num" style="color:var(--text)">${inst.length}</b></span><span class="spacer"></span>
        <button class="btn sm ghost" data-a="deb-selrec">${t('d.selRec')}</button>
        ${selN ? `<button class="btn sm ghost" data-a="deb-selnone">${t('d.selNone')}</button>` : ''}
        <button class="btn primary" data-a="deb-remove" ${selN ? '' : 'disabled'}>${icon('trash')}${t('d.remove', { n: selN })}</button></div>` : ''}
      <section class="sec"><div class="note">${icon('shield')}<div>${t('d.keep')}</div></div></section>`;
  } else {
    const recTodo = list.filter((tw) => recommended(tw) && !st(tw.id).a && !st(tw.id).u).map((tw) => tw.id);
    body = `<div class="note" style="margin-bottom:14px">${icon('info')}<div style="flex:1">${D.tab === 'components' ? t('d.compNote') : t('d.svcNote')}</div>
        ${recTodo.length ? `<button class="btn sm primary" data-a="deb-offrec" data-ids="${recTodo.join(',')}">${t('d.offRec', { n: recTodo.length })}</button>` : ''}</div>
      <div class="card"><div class="list">${list.map(debRow).join('')}</div></div>`;
  }
  return `<div class="toolbar" style="margin-top:6px"><div class="tabs">${tabBar}</div><span class="spacer"></span>
      <button class="btn sm ghost" data-a="deb-scan" ${D.scanning ? 'disabled' : ''}>${icon('refresh')}${t('d.rescan')}</button></div>${body}`;
}

// ================================================================= SYSTEM
function kv(rows) { return `<div class="kv">${rows.filter((r) => r && r[1] !== undefined && r[1] !== '').map(([k, v]) => `<div>${esc(k)}</div><div>${esc(v)}</div>`).join('')}</div>`; }
function spCard(ic, title, head, rows, extra = '') {
  return `<div class="card sp"><div class="sp-head"><div class="ic">${icon(ic)}</div><div style="min-width:0"><div class="sub">${esc(title)}</div><div class="title">${esc(head || '—')}</div></div></div>${kv(rows)}${extra}</div>`;
}
const ramType = (c) => ({ 20: 'DDR', 21: 'DDR2', 24: 'DDR3', 26: 'DDR4', 34: 'DDR5', 35: 'LPDDR5' }[c] || '');

export function systemPage() {
  const i = S.info;
  if (!i || !i.scanAt) return `<div class="grid g3">${[1, 2, 3, 4, 5, 6].map(() => '<div class="skeleton" style="height:220px"></div>').join('')}</div>`;
  const yes = (b) => (b ? t('f.on') : t('f.off'));
  const m0 = (i.ram.modules || [])[0] || {};
  const used = i.sysDrive.total ? (1 - i.sysDrive.free / i.sysDrive.total) * 100 : 0;
  const cards = [
    spCard('cpu', t('s.cpu'), i.cpu.name, [
      [t('f.cores'), `${i.cpu.cores} / ${i.cpu.threads}`], [t('f.clock'), i.cpu.maxMHz ? `${nf(i.cpu.maxMHz)} ${t('u.mhz')}` : ''],
      [t('f.cache'), `${nf(i.cpu.l2 / 1024, 1)} / ${nf(i.cpu.l3 / 1024, 0)} MB`], [t('f.socket'), i.cpu.socket], [t('f.virt'), yes(i.cpu.virt)]]),
    ...(i.gpus || []).map((g) => spCard('gpu', t('s.gpu'), g.name, [
      [t('f.vram'), g.vram ? fmtGB(g.vram) : ''], [t('f.driver'), g.driver], [t('f.driverDate'), fmtDate(g.driverDate)],
      [t('f.res'), g.res], [t('f.hz'), g.hz ? `${g.hz} ${t('u.hz')}` : ''],
      [t('f.rebar'), g.rebar === 'on' ? t('f.on') : g.rebar === 'off' ? t('f.off') : '']])),
    spCard('ram', t('s.ram'), `${fmtGB(i.ram.total)} ${ramType(m0.typeCode)}`, [
      [t('f.modules'), (i.ram.modules || []).map((m) => fmtGB(m.size)).join(' + ')], [t('f.slots'), i.ram.slots ? `${(i.ram.modules || []).length} / ${i.ram.slots}` : ''],
      [t('f.speed'), m0.speed ? `${nf(m0.speed)} ${t('u.mts')}` : ''], [t('f.conf'), m0.conf ? `${nf(m0.conf)} ${t('u.mts')}` : ''],
      [t('f.channel'), memChannel(i)], ['', [m0.maker, m0.part].filter(Boolean).join(' ')]]),
    spCard('disk', t('s.storage'), `${i.sysDrive.letter} ${i.sysDrive.media || ''}`, [
      [t('f.free'), `${fmtBytes(i.sysDrive.free, 0)} / ${fmtBytes(i.sysDrive.total, 0)}`],
      ...(i.disks || []).map((d) => [`${d.media || ''} ${d.bus || ''}`.trim(), `${d.model} (${fmtDiskSize(d.size)})${d.system ? ', ' + t('f.system').toLowerCase() : ''}`])],
      `<div class="bar2"><i style="width:${used.toFixed(0)}%"></i></div>`),
    spCard('board', t('s.board'), `${i.board.maker} ${i.board.product}`, [[t('f.bios'), i.board.bios], ['', fmtDate(i.board.biosDate)]]),
    spCard('monitor', t('s.display'), (i.displays || [])[0] ? (i.displays[0].name || i.displays[0].res) : t('f.none'),
      (i.displays || []).map((d) => [d.name || t('s.display'), `${d.res} @ ${d.hz} ${t('u.hz')}${d.maxHz > d.hz ? ` (${t('s.upTo', { n: d.maxHz })})` : ''}`])),
    spCard('network', t('s.net'), (i.net || [])[0] ? i.net[0].desc : t('f.none'),
      (i.net || []).map((n) => [n.wifi ? t('f.wifi') : t('f.eth'), `${n.name}, ${n.speed}`])),
    spCard('windows', t('s.os'), `${(i.os.name || '').replace('Microsoft ', '')}`, [
      [t('f.version'), i.os.display], [t('f.build'), `${i.os.build}.${i.os.ubr}`], [t('f.installed'), fmtDate(i.os.install)],
      [t('f.uptime'), fmtUptime(i.os.uptime)], [t('f.computer'), i.computer]]),
    spCard('shield', t('s.security'), i.security.av || '—', [
      [t('f.vbs'), yes(i.security.hvci)], [t('f.secureBoot'), i.security.secureBoot === 'on' ? t('f.on') : i.security.secureBoot === 'off' ? t('f.off') : t('f.unsupported')],
      [t('f.av'), i.security.av]]),
    spCard('power', t('s.power'), i.power.plan, [
      [t('f.form'), i.power.laptop ? t('f.laptop') : t('f.desktop')], [t('f.procs'), nf(i.counts.processes)],
      [t('f.svcs'), nf(i.counts.services)], [t('f.startup'), nf(i.counts.startup)], [t('f.printers'), nf(i.printers)]]),
  ];
  return `<div class="row" style="margin-bottom:16px;flex-wrap:wrap">${i.tier ? `<span class="tier">${icon('chip')}${t('tierB.' + i.tier)}</span>` : ''}<span class="spacer"></span>
      <button class="btn sm" data-a="sys-copy">${icon('copy')}${t('s.copy')}</button>
      <button class="btn sm" data-a="sys-rescan">${icon('refresh')}${t('s.rescan')}</button></div>
    <div class="specs">${cards.join('')}</div>
    ${(i.tips || []).length ? `<section class="sec"><div class="sec-head"><div><h2>${t('home.tips')}</h2><p>${t('home.tipsSub')}</p></div></div><div class="tips">${i.tips.map(tipHTML).join('')}</div></section>` : ''}`;
}

export function specsText() {
  const i = S.info;
  if (!i) return '';
  const lines = [
    `${t('s.cpu')}: ${i.cpu.name} (${i.cpu.cores}C/${i.cpu.threads}T)`,
    ...(i.gpus || []).map((g) => `${t('s.gpu')}: ${g.name} ${g.vram ? fmtGB(g.vram) : ''} (${t('f.driver')} ${g.driver})`),
    `${t('s.ram')}: ${ramLine(i)}`,
    ...(i.disks || []).map((d) => `${t('s.storage')}: ${d.model} ${d.media} ${fmtDiskSize(d.size)}`),
    `${t('s.board')}: ${i.board.maker} ${i.board.product}, BIOS ${i.board.bios}`,
    ...(i.displays || []).map((d) => `${t('s.display')}: ${d.name} ${d.res} @ ${d.hz} ${t('u.hz')}`),
    `${t('s.os')}: ${i.os.name} ${i.os.display} (${i.os.build}.${i.os.ubr})`,
  ];
  return lines.join('\n');
}

// ================================================================= BACKUP
export function backupPage() {
  const B = S.backups;
  const ids = Object.keys(B.applied).filter((id) => S.byId[id]).sort((a, b) => B.applied[b].localeCompare(B.applied[a]));
  const last = B.restorePoints.length ? B.restorePoints[B.restorePoints.length - 1] : null;
  const hist = B.history.slice(-60).reverse();
  return `<section class="card pad"><div class="row" style="align-items:flex-start;flex-wrap:wrap">
      <div class="li-ic" style="width:46px;height:46px">${icon('shield')}</div>
      <div style="flex:1;min-width:260px"><h2 style="margin:0;font:600 18px var(--display)">${t('bk.rp')}</h2>
        <p class="muted" style="margin:4px 0 0;max-width:70ch">${t('bk.rpD')}</p>
        <p style="margin:8px 0 0;font-size:13px;color:var(--text-2)">${last ? t('bk.rpLast', { date: fmtDate(last, true) }) : t('bk.rpNone')}</p></div>
      <div class="row"><button class="btn" data-a="open" data-what="restore">${icon('external')}${t('bk.rpOpen')}</button>
        <button class="btn primary" data-a="rp">${icon('plus')}${t('bk.rpCreate')}</button></div></div></section>
    <section class="sec"><div class="sec-head"><div><h2>${t('bk.applied')}</h2><p>${t('bk.appliedD')}</p></div>
      <div class="right">${ids.length ? `<button class="btn sm danger" data-a="undo-all">${icon('restore')}${t('bk.undoAll')}</button>` : ''}</div></div>
      <div class="card">${ids.length ? `<div class="list">${ids.map((id) => { const tw = S.byId[id]; return `<div class="li"><div class="li-ic">${icon(CAT_ICON[tw.cat])}</div>
        <div><h4>${esc(tx(tw.name))}</h4><p>${t('cat.' + tw.cat)} · ${fmtDate(B.applied[id], true)}</p></div>
        <div class="actions"><button class="btn sm" data-a="set" data-id="${id}" data-on="0" ${S.pending.has(id) ? 'disabled' : ''}>${icon('undo')}${t('bk.undo')}</button></div></div>`; }).join('')}</div>`
        : `<div class="empty" style="border:0">${icon('backup')}<div>${t('bk.empty')}</div></div>`}</div></section>
    <section class="sec"><div class="sec-head"><div><h2>${t('bk.history')}</h2></div></div>
      <div class="card">${hist.length ? `<div class="list list-scroll" style="max-height:420px;overflow:auto">${hist.map((h) => { const tw = S.byId[h.id]; return `<div class="li" style="grid-template-columns:auto 1fr auto">
        <span class="tag ${h.ok ? (h.action === 'apply' ? 'good' : 'gray') : 'bad'}">${h.ok ? (h.action === 'apply' ? t('bk.h.apply') : t('bk.h.revert')) : icon('warn')}</span>
        <div><h4 style="font-weight:500">${esc(tw ? tx(tw.name) : h.id)}</h4>${h.err ? `<p style="color:var(--bad)">${esc(h.err)}</p>` : ''}</div>
        <span class="muted num" style="font-size:12px">${fmtDate(h.at, true)}</span></div>`; }).join('')}</div>`
        : `<div class="empty" style="border:0">${t('bk.h.empty')}</div>`}</div></section>`;
}

// ================================================================= SETTINGS
// Settings parts, arranged into categories by the settings window (settingswin.js).
// Language name in the current UI language ("Japanese" / "Японский" / "Japonés").
function langName(l) {
  try { const n = new Intl.DisplayNames([lang()], { type: 'language' }).of(l.code); if (n && n !== l.code) return n.charAt(0).toLocaleUpperCase(lang()) + n.slice(1); } catch (e) { /* old engine */ }
  return l.name;
}

export function settingsParts() {
  const st0 = S.settings;
  const ins = S.boot.install || {};
  return {
    account: accountSection(),
    lang: `<section class="sec" id="set-lang"><div class="sec-head"><div><h2>${t('set.lang')}</h2><p>${t('set.langD')}</p></div></div>
      <div class="langs">${LANGS.map((l) => `<button class="lang ${lang() === l.code ? 'on' : ''}" data-a="lang" data-v="${l.code}"><span class="code">${l.code.toUpperCase()}</span><span><b>${l.native}</b><span>${langName(l)}</span></span></button>`).join('')}</div></section>`,
    theme: themeSection(),
    safety: `<section class="sec" id="set-safety"><div class="sec-head"><div><h2>${t('set.safety')}</h2></div></div>
      <div class="card">
        <div class="setting"><div><h4>${t('set.autoRp')}</h4><p>${t('set.autoRpD')}</p></div>${toggle(!!st0.autoRestore, 'data-a="setting" data-k="autoRestore"')}</div>
      </div></section>`,
    overlay: overlaySection(),
    tray: `<section class="sec" id="set-tray"><div class="sec-head"><div><h2>${t('tray.title')}</h2><p>${t('tray.sub')}</p></div></div>
      <div class="card">
        <div class="setting"><div><h4>${t('tray.on')}</h4><p>${t('tray.onD')}</p></div>${toggle(!!st0.tray, 'data-a="setting" data-k="tray"')}</div>
        <div class="setting"><div><h4>${t('tray.auto')}</h4><p>${t('tray.autoD')}</p></div>${toggle(!!st0.trayAutostart, 'data-a="setting" data-k="trayAutostart"', { disabled: !st0.tray })}</div>
      </div></section>`,
    updates: updatesSection(),
    backup: ioSection(),
    install: `<section class="sec" id="set-install"><div class="sec-head"><div><h2>${t('set.install')}</h2></div></div>
      <div class="card"><div class="setting"><div><h4>${ins.installed ? esc(t('set.installed', { dir: ins.dir })) : t('set.portable')}</h4>
        <p>${ins.installed ? t('set.uninstallHint') : t('set.installD')}</p></div>
        ${ins.installed ? `<span class="tag good">${icon('check')}${t('f.yes')}</span>` : `<button class="btn primary" data-a="install" ${S.boot.admin ? '' : 'disabled'}>${icon('download')}${t('set.installBtn')}</button>`}</div>
      <div class="setting"><div><h4>${t('set.data')}</h4><p>${esc(t('set.dataD', { dir: S.boot.dataDir }))}</p></div><button class="btn" data-a="open" data-what="data">${icon('folder')}${t('set.open')}</button></div></div></section>`,
    about: `<section class="sec" id="set-about"><div class="sec-head"><div><h2>${t('set.about')}</h2></div></div>
      <div class="card pad"><div class="row"><img src="img/logo.svg" width="48" height="48" alt=""><div><b style="font:700 18px var(--display)">${t('set.version', { v: S.boot.version })}</b>
        <p class="muted" style="margin:2px 0 0;font-size:13px">${t('set.credits')}</p></div><span class="spacer"></span><button class="btn sm" data-a="changelog">${icon('sparkles')}${t('cl.title')}</button></div>
        <p class="muted" style="margin:14px 0 0;font-size:13px;max-width:80ch">${t('set.aboutD')}</p></div></section>`,
  };
}

export function settingsPage() {
  const p = settingsParts();
  return Object.values(p).join('');
}

// ================================================================= THEMES
// Mini previews use each theme's own colors (rail, background, panel, accent).
const THEME_PV = {
  violet: ['#0d0916', '#110c1c', '#1e162f', '#7c3aed', '#d16bff', '#f3effb'],
  ocean: ['#070d18', '#0a1120', '#13203a', '#2563eb', '#22d3ee', '#eef4ff'],
  emerald: ['#060e0b', '#08130f', '#11231c', '#059669', '#a3e635', '#ecfbf4'],
  ruby: ['#0f0709', '#150a0e', '#25121a', '#be123c', '#ff80a0', '#fdf0f3'],
  amber: ['#0e0b07', '#14100a', '#241c11', '#dc500a', '#fabe32', '#fdf7ec'],
  sakura: ['#0e080e', '#140c14', '#241625', '#db2777', '#c084fc', '#fdf0fa'],
  graphite: ['#0b0c0d', '#0f1012', '#1b1d20', '#3e4450', '#969eac', '#f2f3f5'],
  light: ['#ffffff',  '#f6f4fb',  '#ffffff',  '#7c3aed',  '#c026d3',  '#1c1530'],
  sky: ['#ffffff',  '#f1f6fe',  '#ffffff',  '#1d4ed8',  '#0891b2',  '#0f1b33'],
  lagoon: ['#ffffff',  '#effaf9',  '#ffffff',  '#0f766e',  '#06b6d4',  '#0b2a2c'],
  mint: ['#ffffff',  '#eefaf6',  '#ffffff',  '#047857',  '#65a30d',  '#0f2a1f'],
  sand: ['#ffffff',  '#fcf9f2',  '#ffffff',  '#c2410c',  '#ca8a04',  '#2a1f0e'],
  rose: ['#ffffff',  '#fef2f4',  '#ffffff',  '#be123c',  '#db2777',  '#2e0f18'],
  paper: ['#ffffff',  '#f5f5f6',  '#ffffff',  '#334155',  '#94a3b8',  '#18181b'],
};

function themeSection() {
  const cur = S.settings.theme || 'violet';
  const card = (k) => {
    const [rail, bg, panel, a1, a2, text] = THEME_PV[k];
    return `<button class="theme ${cur === k ? 'on' : ''}" data-a="theme" data-v="${k}" role="radio" aria-checked="${cur === k}">
      <div class="theme-pv" style="background:${bg}"><div class="r" style="background:${rail}"><i style="background:${a1}"></i><i style="background:${text}"></i><i style="background:${text}"></i></div>
        <div class="m"><b style="background:${text};opacity:.85"></b><div class="c" style="background:${panel}"><span class="bar" style="background:linear-gradient(90deg,${a1},${a2})"></span><span class="dot" style="background:${a2}"></span></div></div></div>
      <div class="theme-name">${t('th.' + k)}<span class="check ${cur === k ? 'on' : ''}">${icon('check')}</span></div></button>`;
  };
  return `<section class="sec" id="set-theme"><div class="sec-head"><div><h2>${t('th.title')}</h2><p>${t('th.sub')}</p></div></div>
    <div role="radiogroup" aria-label="${esc(t('th.title'))}">
      <div class="th-group">${t('th.dark')}</div><div class="themes">${DARK_THEMES.map(card).join('')}</div>
      <div class="th-group">${t('th.lightGroup')}</div><div class="themes">${LIGHT_THEMES.map(card).join('')}</div></div></section>`;
}

// ================================================================= OVERLAY SETTINGS
export const OV_METRICS = ['fps', 'low1', 'frametime', 'ftgraph', 'gpu', 'gputemp', 'gpuclock', 'gpupower', 'vram', 'cpu', 'cputemp', 'ram', 'ping', 'clock'];
const OV_HINT = { fps: 'ov.h.dx', low1: 'ov.h.dx', frametime: 'ov.h.dx', ftgraph: 'ov.h.ftgraph', gputemp: 'ov.h.nv', gpuclock: 'ov.h.nv', gpupower: 'ov.h.nv', cputemp: 'ov.h.cpu', ping: 'ov.h.ping' };

export function hotkeyLabel(o) {
  const names = { ctrl: 'Ctrl', alt: 'Alt', shift: 'Shift', win: 'Win' };
  return [...(o.mods || []).map((m) => names[m] || m), o.key].filter(Boolean);
}

function seg(key, cur, opts) {
  return `<div class="seg" role="radiogroup">${opts.map(([v, label]) => `<button class="${String(cur) === String(v) ? 'on' : ''}" role="radio" aria-checked="${String(cur) === String(v)}" data-a="ov-set" data-k="${key}" data-v="${v}">${label}</button>`).join('')}</div>`;
}

function ovStatusPill() {
  const o = S.settings.overlay || {};
  const st = S.ovStatus || {};
  if (!o.enabled) return `<span class="pill">${icon('eyeOff')}${t('ov.st.off')}</span>`;
  if (!st.running) return `<span class="pill">${icon('loader')}${t('ov.st.starting')}</span>`;
  if (st.hotkey === 'busy') return `<span class="pill limited">${icon('alert')}${t('ov.st.busy')}</span>`;
  if (st.admin === false || st.fps === 'unavailable') return `<span class="pill limited">${icon('alert')}${t('ov.st.nofps')}</span>`;
  return `<span class="pill admin">${icon('ok')}${t('ov.st.running')}</span>`;
}

export function overlaySection() {
  const o = S.settings.overlay || {};
  const st = S.ovStatus || {};
  const on = new Set(o.metrics || []);
  const rec = S.ovRec;
  const keys = rec ? `<span class="keycap rec">${t('ov.press')}</span>` : hotkeyLabel(o).map((k) => `<span class="keycap">${esc(k)}</span>`).join('<span class="plus">+</span>');
  const slots = ['tl', 'tc', 'tr', 'bl', 'bc', 'br'].map((p) => `<button class="ov-slot s-${p} ${o.position === p ? 'on' : ''}" data-a="ov-set" data-k="position" data-v="${p}" title="${t('ov.pos.' + p)}" aria-label="${t('ov.pos.' + p)}"></button>`).join('');
  const visible = st.visible !== false;
  return `<section class="sec" id="ov-sec"><div class="sec-head"><div><h2>${t('ov.title')}</h2><p>${t('ov.sub')}</p></div><div class="right" id="ov-pill">${ovStatusPill()}</div></div>
    <div class="ov-grid">
      <div class="card ov-settings" id="set-ov-main">
        <div class="setting"><div><h4>${t('ov.enable')}</h4><p>${t('ov.enableD')}</p></div>${toggle(!!o.enabled, 'data-a="ov-flag" data-k="enabled"')}</div>
        <div class="setting"><div><h4>${t('ov.hotkey')}</h4><p>${S.ovRecErr ? `<span style="color:var(--warn)">${S.ovRecErr}</span>` : t('ov.hotkeyD')}</p></div>
          <div class="row" style="gap:10px"><span class="keys">${keys}</span>
            <button class="btn sm ${rec ? 'primary' : ''}" data-a="ov-rec">${rec ? t('btn.cancel') : t('ov.change')}</button></div></div>
        ${o.enabled && st.running ? `<div class="setting"><div><h4>${visible ? t('ov.shown') : t('ov.hidden')}</h4><p>${t('ov.toggleD', { keys: hotkeyLabel(o).join(' + ') })}</p></div>
          <button class="btn sm" data-a="ov-vis" data-v="${visible ? '0' : '1'}">${icon(visible ? 'eyeOff' : 'eye')}${visible ? t('ov.hide') : t('ov.show')}</button></div>` : ''}
        <div class="setting"><div><h4>${t('ov.layout')}</h4></div>${seg('layout', o.layout, [['row', t('ov.l.row')], ['column', t('ov.l.column')]])}</div>
        <div class="setting"><div><h4>${t('ov.size')}</h4></div>${seg('size', o.size, [['s', t('ov.s.s')], ['m', t('ov.s.m')], ['l', t('ov.s.l')]])}</div>
        <div class="setting"><div><h4>${t('ov.opacity')}</h4></div><div class="row" style="gap:12px"><input type="range" class="range" min="30" max="100" step="5" value="${o.opacity || 85}" data-ov-range="opacity" aria-label="${t('ov.opacity')}"><span class="num muted" style="width:40px;text-align:right" id="ov-opv">${o.opacity || 85}%</span></div></div>
        <div class="setting"><div><h4>${t('ov.interval')}</h4></div>${seg('interval', o.interval, [[500, `0,5 ${t('u.s')}`], [1000, `1 ${t('u.s')}`], [2000, `2 ${t('u.s')}`]])}</div>
        <div class="setting"><div><h4>${t('ov.autostart')}</h4><p>${t('ov.autostartD')}</p></div>${toggle(!!o.autostart, 'data-a="ov-flag" data-k="autostart"', { disabled: !o.enabled })}</div>
        <div class="setting"><div><h4>${t('ov.warn')}</h4><p>${t('ov.warnD')}</p></div>${toggle(!o.warnOff, 'data-a="ov-flag-inv" data-k="warnOff"')}</div>
        <div class="setting"><div><h4>${t('ov.record')}</h4><p>${t('ov.recordD')}</p></div>${toggle(!o.recordOff, 'data-a="ov-flag-inv" data-k="recordOff"')}</div>
      </div>
      <div class="ov-side">
        <div class="card ov-preview-card"><div class="row" style="margin-bottom:10px"><b style="font:600 14px var(--display)">${t('ov.preview')}</b><span class="spacer"></span><span class="muted" style="font-size:12px">${t('ov.posHint')}</span></div>
          <div class="ov-screen pos-${o.position || 'tr'}">${slots}<img id="ov-img" alt="" src="/api/overlay/preview.png?t=${encodeURIComponent(S.tok)}&v=${S.ovV || 0}"></div>
          <p class="muted" style="font-size:12px;margin:10px 0 0">${t('ov.previewNote')}</p></div>
        <div class="card ov-metrics" id="set-ov-metrics"><b style="font:600 14px var(--display)">${t('ov.metrics')}</b>
          <div class="metrics-grid">${OV_METRICS.map((m) => `<button class="metric ${on.has(m) ? 'on' : ''}" data-a="ov-metric" data-v="${m}"><span class="check ${on.has(m) ? 'on' : ''}">${icon('check')}</span><span><b>${t('ov.m.' + m)}</b>${OV_HINT[m] ? `<i>${t(OV_HINT[m])}</i>` : ''}</span></button>`).join('')}</div></div>
        ${profilesCard()}
      </div>
    </div>
    <div class="note" style="margin-top:14px">${icon('info')}<div>${t('ov.note')}</div></div>
  </section>`;
}
