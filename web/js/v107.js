// OpLarik 1.0.7: PC health page (disks, GPU driver, network test, scheduled
// cleanup), NVIDIA driver settings and automatic game mode (Boost page),
// before / after comparison (Games page), per-game overlay profiles, updates
// and settings export / import (Settings page).
import { t, lang, tx } from './i18n.js';
import { S, api, esc, icon, toast, toggle, modal, closeModal, fmtBytes, fmtDiskSize, nf, cover, coverHTML, coverForExe, st, saveSettings, fmtDate } from './core.js';

let hooks = { render: () => {}, renderPage: () => {}, applyIds: async () => null, saveOverlay: async () => {}, ovMetrics: [] };
export function initV107(h) { hooks = { ...hooks, ...h }; }

S.v7 = {
  disks: null, disksErr: '', diskBusy: false,
  driver: null, drvBusy: false, drvErr: '',
  net: null, netBusy: false,
  clean: null, cleanBusy: false,
  nv: null, nvBusy: {},
  cmp: null, cmpOpen: false, cmpBusy: false,
  upd: null, updBusy: false, updErr: '',
  ov: null,
};
const V = S.v7;
const rerender = (...pages) => { if (!pages.length || pages.includes(S.page)) hooks.renderPage(); };
const errText = (e, prefix) => {
  const m = (e && e.message) || '';
  const k = prefix + m;
  const s = t(k);
  return s === k ? m || t('err.generic') : s;
};

// ================================================================= loaders
export async function loadDisks(force) {
  V.diskBusy = true; rerender('health');
  try {
    const r = await api('/api/health/disks' + (force ? '?force=1' : ''));
    V.disks = r.disks || []; V.disksErr = r.error || '';
  } catch (e) { V.disks = V.disks || []; V.disksErr = e.message; }
  V.diskBusy = false;
  hooks.render();
}
export async function loadDriver() {
  try { V.driver = await api('/api/health/driver'); } catch (e) { /* ignore */ }
  rerender('health');
}
export async function loadCleanup() {
  try { V.clean = await api('/api/cleanup'); } catch (e) { /* ignore */ }
  rerender('health');
}
export async function loadNV() {
  try { V.nv = await api('/api/nvidia'); } catch (e) { V.nv = { available: false }; }
  rerender('boost');
}
export async function loadOvStatus() {
  try { V.ov = await api('/api/overlay'); } catch (e) { /* ignore */ }
  rerender('boost');
}
export function loadHealth() {
  if (!V.disks && !V.diskBusy) loadDisks(false);
  loadDriver();
  loadCleanup();
  api('/api/nettest').then((r) => { if (r && r.at) { V.net = r; if (r.running) pollNet(); rerender('health'); } }).catch(() => {});
}

// Worst disk state for the rail badge.
export function diskAlert() {
  const l = (V.disks || []).map((d) => d.level);
  return l.includes('bad') ? 'bad' : l.includes('warn') ? 'warn' : '';
}

// ================================================================= HEALTH PAGE
const hours = (h) => (h >= 48 ? t('dk.days', { n: Math.round(h / 24) }) : t('dk.hours', { n: Math.round(h) }));

function diskCard(d) {
  const lvl = { good: ['good', 'ok', t('dk.good')], warn: ['warn', 'alert', t('dk.warn')], bad: ['bad', 'warn', t('dk.bad')] }[d.level] || ['gray', 'info', '—'];
  const stat = (label, v, cls = '') => `<div class="ses-stat ${cls}"><span>${label}</span><b class="num">${v}</b></div>`;
  const tempCls = d.issues.includes('hot') ? 'hot' : d.issues.includes('warm') ? 'warmish' : '';
  const life = d.wear >= 0 ? `${nf(Math.max(0, 100 - d.wear))}%` : '—';
  const errs = d.readErr >= 0 || d.writeErr >= 0 ? nf(Math.max(0, d.readErr) + Math.max(0, d.writeErr)) : '—';
  return `<div class="card disk-card lv-${d.level}">
    <div class="ses-head"><div class="dk-ic">${icon('disk')}</div>
      <div class="grow"><h4 title="${esc(d.name)}">${esc(d.name || t('dk.unknown'))}</h4><p>${[d.media, d.bus, d.size ? fmtDiskSize(d.size) : ''].filter(Boolean).map(esc).join(' · ')}</p></div>
      <span class="tag ${lvl[0]}">${icon(lvl[1])}${lvl[2]}</span></div>
    <div class="ses-stats dk-stats">${stat(t('dk.temp'), d.temp >= 0 ? `${nf(d.temp)}°C` : '—', tempCls)}${stat(t('dk.life'), life, d.issues.includes('wearBad') ? 'hot' : d.issues.includes('wear') ? 'warmish' : '')}
      ${stat(t('dk.on'), d.hours >= 0 ? hours(d.hours) : '—')}${stat(t('dk.errors'), errs, d.issues.includes('errors') ? 'warmish' : '')}</div>
    ${d.issues.length ? `<div class="dk-issues">${d.issues.map((k) => `<div class="dk-issue ${['unhealthy', 'predict', 'wearBad', 'hot'].includes(k) ? 'bad' : ''}">${icon('alert')}<span>${t('dk.i.' + k, { media: d.media || 'SSD' })}</span></div>`).join('')}</div>` : `<p class="muted dk-fine">${icon('ok')}${t('dk.fine')}</p>`}
  </div>`;
}

function disksSection() {
  let body;
  if (!V.disks) body = `<div class="card pad row">${icon('loader', 'spin')}<span class="muted">${t('dk.reading')}</span></div>`;
  else if (!V.disks.length) body = `<div class="card"><div class="empty" style="border:0">${icon('disk')}<div>${V.disksErr ? esc(V.disksErr) : t('dk.none')}</div></div></div>`;
  else body = `<div class="grid g2">${V.disks.map(diskCard).join('')}</div>`;
  const alert = diskAlert();
  return `<section><div class="sec-head"><div><h2>${t('dk.title')}</h2><p>${t('dk.sub')}</p></div>
      <div class="right"><button class="iconbtn" data-a="dk-refresh" title="${t('d.rescan')}" ${V.diskBusy ? 'disabled' : ''}>${icon(V.diskBusy ? 'loader' : 'refresh', V.diskBusy ? 'spin' : '')}</button></div></div>
    ${alert === 'bad' ? `<div class="banner warn" style="margin-bottom:12px">${icon('alert')}<div class="grow"><b>${t('dk.backupNow')}</b> ${t('dk.backupNowD')}</div></div>` : ''}
    ${body}${V.disks && V.disks.length ? `<p class="muted hl-foot">${icon('info')}${t('dk.note')}</p>` : ''}</section>`;
}

function driverSection() {
  const D = V.driver;
  const gpus = (D && D.gpus) || [];
  const latest = D && D.latest;
  const hasNV = gpus.some((g) => g.vendor === 'NVIDIA');
  let rows;
  if (!D) rows = `<div class="setting"><div class="row">${icon('loader', 'spin')}<span class="muted">${t('home.scanning')}</span></div></div>`;
  else if (!gpus.length) rows = `<div class="setting"><div><h4>${t('drv.noGpu')}</h4></div></div>`;
  else rows = gpus.map((g) => {
    const nv = g.vendor === 'NVIDIA';
    let status = '';
    if (nv && g.legacy) status = `<span class="tag info">${t('drv.legacy')}</span>`;
    else if (nv && latest) status = g.outdated ? `<span class="tag warn">${icon('cloud')}${t('drv.newer', { v: latest.version })}</span>` : `<span class="tag good">${icon('check')}${t('drv.latestOk')}</span>`;
    const vendorBtn = !nv ? `<button class="btn sm" data-a="open" data-what="${/amd|radeon/i.test(g.vendor + g.name) ? 'amd' : 'intel'}">${icon('external')}${t('drv.site')}</button>` : '';
    const dl = nv && latest && g.outdated ? `<button class="btn sm primary" data-a="drv-open">${icon('download')}${t('drv.get', { v: latest.version })}</button>` : '';
    return `<div class="setting"><div><h4>${esc(g.name)}</h4><p>${t('drv.installed')}: <b class="num">${esc(nv && g.version ? g.version : g.driver || '—')}</b>${g.date ? ` · ${esc(fmtDate(g.date))}` : ''}${nv && g.legacy ? `<br>${t('drv.legacyD')}` : ''}</p></div>
      <div class="row" style="gap:8px;flex-wrap:wrap;justify-content:flex-end">${status}${dl}${vendorBtn}</div></div>`;
  }).join('');
  return `<section class="sec"><div class="sec-head"><div><h2>${t('drv.title')}</h2><p>${t('drv.sub')}</p></div>
      <div class="right">${hasNV ? `<button class="btn sm" data-a="drv-check" ${V.drvBusy ? 'disabled' : ''}>${V.drvBusy ? icon('loader', 'spin') : icon('cloud')}${t('drv.check')}</button>` : ''}</div></div>
    <div class="card">${rows}
      ${latest ? `<div class="setting drv-latest"><div><p>${t('drv.latestIs', { v: latest.version })}${latest.date ? ` · ${esc(latest.date)}` : ''}</p></div></div>` : ''}
      ${V.drvErr ? `<div class="setting"><div><p style="color:var(--warn)">${icon('alert')} ${esc(V.drvErr)}</p></div></div>` : ''}</div>
    <p class="muted hl-foot">${icon('info')}${t('drv.note')}</p></section>`;
}

const gradeCls = (g) => (!g ? 'gray' : g.startsWith('A') ? 'good' : g === 'B' ? 'info' : g === 'C' ? 'warn' : 'bad');

function netAdvice(n) {
  const out = [];
  const r = (n.pings || []).find((p) => p.id === 'router');
  const pub = (n.pings || []).filter((p) => p.id !== 'router');
  const loss = (p) => (p.sent ? (p.lost / p.sent) * 100 : 0);
  if (r && (r.jitter > 5 || loss(r) >= 5)) out.push(['wifi', 'net.tip.wifi']);
  else if (pub.some((p) => loss(p) >= 5)) out.push(['alert', 'net.tip.loss']);
  if (n.grade && 'CDF'.includes(n.grade[0])) out.push(['router', 'net.tip.bloat']);
  if (pub.some((p) => p.avg > 60)) out.push(['globe', 'net.tip.far']);
  if (!out.length && n.stage === 'done' && !n.error) out.push(['ok', 'net.tip.ok']);
  return out;
}

function netSection() {
  const n = V.net;
  const running = n && n.running;
  const stageLabel = n ? t('net.stage.' + (n.stage || 'ping')) : '';
  let body = '';
  if (!n) body = `<div class="net-empty">${icon('radar')}<div><b>${t('net.idle')}</b><p class="muted">${t('net.idleD')}</p></div></div>`;
  else {
    const loss = (p) => (p.sent ? (p.lost / p.sent) * 100 : 0);
    const rows = (n.pings || []).map((p) => `<div class="net-row"><div class="net-name"><b>${t('net.t.' + p.id)}</b><span class="mono">${esc(p.target)}</span></div>
        <div class="num">${p.avg >= 0 ? `${nf(p.avg, 0)} ${t('u.ms')}` : '—'}</div>
        <div class="num muted">${p.jitter >= 0 ? `± ${nf(p.jitter, 1)} ${t('u.ms')}` : '—'}</div>
        <div><span class="tag ${loss(p) === 0 ? 'good' : loss(p) < 5 ? 'warn' : 'bad'}">${t('net.loss', { n: nf(loss(p), loss(p) % 1 ? 1 : 0) })}</span></div></div>`).join('');
    const speed = (v) => (v >= 0 ? `${nf(v, v < 10 ? 1 : 0)}<small>${t('net.mbps')}</small>` : '—');
    const grade = n.grade || '';
    body = `${running ? `<div class="net-prog"><div class="row"><span>${icon('loader', 'spin')}${stageLabel}</span><span class="spacer"></span><span class="num muted">${Math.round((n.progress || 0) * 100)}%</span></div><div class="bar2"><i style="width:${(n.progress || 0) * 100}%"></i></div></div>` : ''}
      ${rows ? `<div class="net-head"><span>${t('net.col.target')}</span><span>${t('net.col.ping')}</span><span>${t('net.col.jitter')}</span><span>${t('net.col.loss')}</span></div><div class="net-list">${rows}</div>` : ''}
      ${n.stage === 'download' || n.stage === 'upload' || n.stage === 'done' ? `<div class="net-speed">
        <div class="ses-stat"><span>${icon('download')}${t('net.down')}</span><b class="num">${speed(n.down)}</b></div>
        <div class="ses-stat"><span>${icon('upload')}${t('net.up')}</span><b class="num">${speed(n.up)}</b></div>
        <div class="ses-stat"><span>${t('net.loaded')}</span><b class="num">${n.loadDown >= 0 || n.loadUp >= 0 ? `${nf(Math.max(n.loadDown, n.loadUp), 0)}<small>${t('u.ms')}</small>` : '—'}</b></div>
        <div class="ses-stat grade g-${gradeCls(grade)}"><span>${t('net.bloat')}</span><b>${grade || '—'}${n.bloat >= 0 ? `<small>+${nf(n.bloat, 0)} ${t('u.ms')}</small>` : ''}</b></div></div>` : ''}
      ${n.error ? `<div class="note warn" style="margin-top:12px">${icon('alert')}<div>${t('net.e.' + n.error)}</div></div>` : ''}
      ${!running ? `<div class="net-tips">${netAdvice(n).map(([ic, k]) => `<div class="net-tip">${icon(ic)}<span>${t(k)}</span></div>`).join('')}</div>` : ''}`;
  }
  return `<section class="sec"><div class="sec-head"><div><h2>${t('net.title')}</h2><p>${t('net.sub')}</p></div>
      <div class="right">${n && n.at && !running ? `<span class="muted" style="font-size:12px">${esc(fmtDate(n.at, true))}</span>` : ''}
      <button class="btn sm ${n ? '' : 'primary'}" data-a="net-run" ${running || V.netBusy ? 'disabled' : ''}>${running ? icon('loader', 'spin') : icon('play')}${n ? t('net.again') : t('net.run')}</button></div></div>
    <div class="card pad net-card">${body}</div>
    <p class="muted hl-foot">${icon('info')}${t('net.note')}</p></section>`;
}

function dayNames() {
  const f = new Intl.DateTimeFormat(lang(), { weekday: 'short' });
  return [0, 1, 2, 3, 4, 5, 6].map((d) => f.format(new Date(2026, 0, 4 + d))); // 4 Jan 2026 is a Sunday
}

function cleanSection() {
  const c = S.settings.autoClean || { enabled: false, day: 0, hour: 12 };
  const C = V.clean || {};
  const runs = C.runs || [];
  const days = dayNames();
  const order = [1, 2, 3, 4, 5, 6, 0];
  return `<section class="sec"><div class="sec-head"><div><h2>${t('cln.title')}</h2><p>${t('cln.sub')}</p></div>
      <div class="right"><button class="btn sm" data-a="cln-run" ${V.cleanBusy ? 'disabled' : ''}>${V.cleanBusy ? icon('loader', 'spin') : icon('trash')}${t('cln.now')}</button></div></div>
    <div class="card">
      <div class="setting"><div><h4>${t('cln.auto')}</h4><p>${c.enabled && C.next ? t('cln.next', { when: fmtDate(C.next, true) }) : t('cln.autoD')}</p></div>${toggle(!!c.enabled, 'data-a="cln-toggle" aria-label="' + esc(t('cln.auto')) + '"', { disabled: !S.boot.admin })}</div>
      <div class="setting ${c.enabled ? '' : 'dim'}"><div><h4>${t('cln.when')}</h4></div>
        <div class="row" style="gap:10px;flex-wrap:wrap;justify-content:flex-end"><div class="seg" role="radiogroup">${order.map((d) => `<button class="${c.day === d ? 'on' : ''}" role="radio" aria-checked="${c.day === d}" data-a="cln-day" data-v="${d}" ${c.enabled ? '' : 'disabled'}>${esc(days[d])}</button>`).join('')}</div>
        <select class="select" data-cln-hour aria-label="${t('cln.hour')}" ${c.enabled ? '' : 'disabled'}>${Array.from({ length: 24 }, (_, h) => `<option value="${h}" ${c.hour === h ? 'selected' : ''}>${String(h).padStart(2, '0')}:00</option>`).join('')}</select></div></div>
      ${runs.length ? `<div class="setting"><div style="width:100%"><h4>${t('cln.history')}</h4><div class="cln-runs">${runs.slice(0, 5).map((r) => `<div class="cln-run"><span>${esc(fmtDate(r.at, true))}</span><span class="tag ${r.auto ? 'info' : 'gray'}">${r.auto ? t('cln.byAuto') : t('cln.byHand')}</span><span class="spacer"></span><b class="num">${fmtBytes(r.bytes)}</b><span class="muted num">${t('cln.files', { n: r.files })}</span></div>`).join('')}</div></div></div>` : ''}
    </div>
    <p class="muted hl-foot">${icon('info')}${t('cln.note')}</p></section>`;
}

export function healthPage() {
  return `${disksSection()}${driverSection()}${netSection()}${cleanSection()}`;
}

// ================================================================= BOOST PAGE: automatic game mode + NVIDIA
export function autoGameCard() {
  const a = S.settings.autoGame || { enabled: false, boost: true, overlay: true };
  const ov = V.ov || {};
  const game = ov.running && ov.game;
  const overlayOn = !!(S.settings.overlay || {}).enabled;
  return `<section class="sec"><div class="card input-mode auto-game ${a.enabled ? 'on' : ''}">
    <div class="im-head"><div class="im-ic">${icon('bot')}</div>
      <div class="grow"><h3>${t('ag.title')}${a.enabled ? `<span class="tag good">${icon('check')}${t('im.on')}</span>` : ''}${game ? `<span class="tag info">${icon('games')}${esc(t('ag.now', { game }))}</span>` : ''}</h3><p>${t('ag.sub')}</p></div>
      ${toggle(!!a.enabled, 'data-a="ag-toggle" aria-label="' + esc(t('ag.title')) + '"', { disabled: !S.boot.admin })}</div>
    <div class="ag-opts">
      <div class="ag-opt ${a.boost ? 'on' : ''}"><div><b>${icon('zap')}${t('ag.boost')}</b><p>${t('ag.boostD')}</p></div>${toggle(!!a.boost, 'data-a="ag-opt" data-k="boost" aria-label="' + esc(t('ag.boost')) + '"', { disabled: !a.enabled })}</div>
      <div class="ag-opt ${a.overlay ? 'on' : ''}"><div><b>${icon('monitorPlay')}${t('ag.overlay')}</b><p>${overlayOn ? t('ag.overlayD') : t('ag.overlayOff')}</p></div>${toggle(!!a.overlay && overlayOn, 'data-a="ag-opt" data-k="overlay" aria-label="' + esc(t('ag.overlay')) + '"', { disabled: !a.enabled || !overlayOn })}</div>
    </div>
    <p class="muted im-note">${icon('info')}${t('ag.note')}</p></div></section>`;
}

const NV_ROWS = ['power', 'latency', 'fpslimit', 'shadercache', 'texfilter'];
const NV_TARGET = { power: 1, latency: 1, shadercache: 0xFFFFFFFF, texfilter: 0x14 };

function nvOn(s) { return !!s && s.set && (s.key === 'fpslimit' ? s.value > 0 : s.value === NV_TARGET[s.key]); }

export function nvidiaSection() {
  const N = V.nv;
  if (!N) return '';
  if (!N.available) {
    const info = S.info;
    const hasNV = info && (info.gpus || []).some((g) => /nvidia/i.test(g.vendor + g.name));
    if (!hasNV) return '';
    return `<section class="sec"><div class="sec-head"><div><h2>${t('nv.title')}</h2><p>${t('nv.sub')}</p></div></div>
      <div class="card"><div class="empty" style="border:0">${icon('gpu')}<div>${t('nv.na')}</div></div></div></section>`;
  }
  const by = Object.fromEntries((N.settings || []).map((s) => [s.key, s]));
  const hz = ((S.info && S.info.displays) || []).reduce((m, d) => Math.max(m, d.hz || 0), 0) || 60;
  const row = (k) => {
    const s = by[k] || { key: k, value: 0, set: false };
    const on = nvOn(s);
    const busy = !!V.nvBusy[k];
    if (k === 'fpslimit') {
      const cur = on ? s.value : '';
      const sugg = [...new Set([hz - 3, hz, 60, 144].filter((v) => v >= 30))].slice(0, 4);
      return `<div class="setting nv-row"><div><h4>${t('nv.k.fpslimit')}${on ? `<span class="tag good">${nf(s.value)} FPS</span>` : ''}</h4><p>${t('nv.d.fpslimit', { hz })}</p>
          <div class="nv-chips">${sugg.map((v) => `<button class="chip sm" data-a="nv-fps" data-v="${v}" ${busy || !S.boot.admin ? 'disabled' : ''}>${v}${v === hz - 3 ? ` <i>${t('nv.vrr')}</i>` : ''}</button>`).join('')}</div></div>
        <div class="row" style="gap:8px"><input class="input num" id="nv-fps" type="number" min="20" max="1023" step="1" value="${cur}" placeholder="${hz - 3}" aria-label="${t('nv.k.fpslimit')}" ${!S.boot.admin ? 'disabled' : ''}>
          <button class="btn sm" data-a="nv-fps" ${busy || !S.boot.admin ? 'disabled' : ''}>${busy ? icon('loader', 'spin') : ''}${t('nv.set')}</button>
          ${on ? `<button class="btn sm ghost" data-a="nv-set" data-k="fpslimit" data-on="0" ${busy ? 'disabled' : ''}>${t('nv.off')}</button>` : ''}</div></div>`;
    }
    return `<div class="setting nv-row"><div><h4>${t('nv.k.' + k)}${s.ours ? `<span class="tag gray">${t('nv.ours')}</span>` : ''}</h4><p>${t('nv.d.' + k)}</p></div>
      ${toggle(on, `data-a="nv-set" data-k="${k}" data-on="${on ? 0 : 1}" aria-label="${esc(t('nv.k.' + k))}"`, { busy, disabled: !S.boot.admin })}</div>`;
  };
  const ours = (N.settings || []).some((s) => s.ours);
  return `<section class="sec"><div class="sec-head"><div><h2>${icon('gpu')}${t('nv.title')}</h2><p>${t('nv.sub')}</p></div>
      <div class="right">${ours ? `<button class="btn sm ghost" data-a="nv-restore" ${!S.boot.admin ? 'disabled' : ''}>${icon('restore')}${t('nv.restore')}</button>` : ''}</div></div>
    <div class="card">${NV_ROWS.map(row).join('')}</div>
    <p class="muted hl-foot">${icon('info')}${t('nv.note')}</p></section>`;
}

// ================================================================= GAMES PAGE: before / after
const histName = (h) => {
  if (h.id === 'dns_tool') return h.action === 'apply' ? t('cmp.dns', { name: h.note || '' }) : t('cmp.dnsBack');
  if (h.id.startsWith('nv_')) return `NVIDIA: ${t('nv.k.' + h.id.slice(3))}${h.note ? ` (${h.note})` : ''}`;
  const tw = S.byId[h.id];
  return tw ? tx(tw.name) : h.id;
};

// Lines are drawn in a stretched SVG; labels are HTML so text isn't distorted.
function chartSVG(a, b) {
  const W = 1000, H = 200;
  const all = [...(a ? a.series : []), ...(b ? b.series : [])];
  if (!all.length) return '';
  let max = Math.max(...all), min = Math.min(...all);
  const pad = (max - min) * 0.1 || 10;
  max = Math.ceil((max + pad) / 10) * 10; min = Math.max(0, Math.floor((min - pad) / 10) * 10);
  const x = (i, n) => (n <= 1 ? 0 : (i / (n - 1)) * W);
  const y = (v) => (1 - (v - min) / (max - min || 1)) * H;
  const line = (s) => s.map((v, i) => `${i ? 'L' : 'M'}${x(i, s.length).toFixed(1)},${y(v).toFixed(1)}`).join('');
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => Math.round(min + (max - min) * f));
  return `<div class="cmp-plot"><div class="cmp-y">${ticks.map((v) => `<span style="top:${((y(v) / H) * 100).toFixed(2)}%">${v}</span>`).join('')}</div>
    <svg class="cmp-chart" viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" role="img" aria-label="${esc(t('cmp.chart'))}">
    ${ticks.map((v) => `<line x1="0" x2="${W}" y1="${y(v)}" y2="${y(v)}" class="grid"/>`).join('')}
    ${a && a.series.length ? `<path d="${line(a.series)}" class="ln a"/>` : ''}${b && b.series.length ? `<path d="${line(b.series)}" class="ln b"/>` : ''}</svg>
    <div class="cmp-x"><span>${t('cmp.start')}</span><span>${t('cmp.end')}</span></div></div>`;
}

function sesLabel(s) {
  const d = new Date(s.start);
  return `${s.game} · ${d.toLocaleString(lang(), { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}`;
}

export function compareSection() {
  if (!V.cmpOpen) return '';
  const C = V.cmp;
  const list = S.tools.sessions || [];
  const close = `<button class="iconbtn" data-a="cmp-close" title="${t('btn.close')}">${icon('x')}</button>`;
  if (!C) return `<section class="sec"><div class="card pad row">${icon('loader', 'spin')}<span class="muted">${t('cmp.loading')}</span><span class="spacer"></span>${close}</div></section>`;
  const a = C.a, b = C.b;
  const same = b ? list.filter((s) => s.id !== b.id && ((s.exe && b.exe) ? s.exe === b.exe : s.game === b.game)) : [];
  const sel = (side, cur, opts) => `<select class="select cmp-sel" data-cmp="${side}" aria-label="${side === 'a' ? t('cmp.before') : t('cmp.after')}">
      ${side === 'a' && !cur ? `<option value="">—</option>` : ''}${opts.map((s) => `<option value="${esc(s.id)}" ${cur && cur.id === s.id ? 'selected' : ''}>${esc(sesLabel(s))}</option>`).join('')}</select>`;
  const head = `<div class="cmp-head"><div class="cmp-pick"><span class="cmp-badge a">${t('cmp.before')}</span>${sel('a', a, same)}</div>
      <div class="cmp-arrow">${icon('arrowRight')}</div>
      <div class="cmp-pick"><span class="cmp-badge b">${t('cmp.after')}</span>${sel('b', b, list)}</div><span class="spacer"></span>${close}</div>`;
  if (!b) return `<section class="sec"><div class="card pad">${head}<div class="empty" style="border:0">${icon('activity')}<div>${t('ses.empty')}</div></div></div></section>`;
  if (!a) return `<section class="sec"><div class="card pad cmp">${head}<div class="empty" style="border:0">${icon('compare')}<div>${t('cmp.needTwo', { game: esc(b.game) })}</div></div></div></section>`;
  const A = a.stats, B = b.stats;
  const delta = (x, y, unit = '', better = 'up', dec = 0) => {
    if (x < 0 || y < 0 || x === undefined || y === undefined) return `<span class="muted">—</span>`;
    const d = y - x;
    const pct = x ? (d / x) * 100 : 0;
    const good = better === 'up' ? d > 0 : d < 0;
    const flat = Math.abs(pct) < 1 && Math.abs(d) < 1;
    const cls = flat ? 'flat' : good ? 'good' : 'bad';
    const txt = unit === '%' ? `${d > 0 ? '+' : ''}${nf(d, dec)} ${t('cmp.pp')}` : `${d > 0 ? '+' : ''}${nf(pct, Math.abs(pct) < 10 ? 1 : 0)}%`;
    return `<span class="dl ${cls}">${icon(flat ? 'arrowRight' : d > 0 ? 'up' : 'down')}${txt}</span>`;
  };
  const tile = (label, x, y, unit, better, hint) => `<div class="cmp-tile"><span>${label}${hint ? ` <i class="muted" title="${esc(hint)}">${icon('info')}</i>` : ''}</span>
      <div class="cmp-vals"><b class="num a">${x >= 0 ? nf(x, 0) : '—'}</b>${icon('arrowRight')}<b class="num b">${y >= 0 ? nf(y, 0) : '—'}</b><small>${unit}</small></div>${delta(x, y, unit, better)}</div>`;
  const row = (label, x, y, unit, better, fmt = (v) => nf(v, 0)) => `<div class="cmp-row"><span>${label}</span><b class="num">${x >= 0 ? fmt(x) + unit : '—'}</b><b class="num">${y >= 0 ? fmt(y) + unit : '—'}</b>${delta(x, y, unit === '%' ? '%' : '', better)}</div>`;
  const dur = (s) => `${Math.round(s / 60)} ${t('cmp.min')}`;
  const verdict = (() => {
    if (A.avgFps <= 0 || B.avgFps <= 0) return '';
    const p = ((B.avgFps - A.avgFps) / A.avgFps) * 100;
    const l = A.low1 > 0 ? ((B.low1 - A.low1) / A.low1) * 100 : 0;
    const k = p >= 3 || l >= 5 ? 'better' : p <= -3 || l <= -5 ? 'worse' : 'same';
    return `<div class="cmp-verdict ${k}">${icon(k === 'better' ? 'up' : k === 'worse' ? 'down' : 'arrowRight')}<div><b>${t('cmp.v.' + k, { p: nf(Math.abs(p), 0), l: nf(Math.abs(l), 0) })}</b><p>${t('cmp.v.hint')}</p></div></div>`;
  })();
  return `<section class="sec" id="cmp"><div class="card pad cmp">${head}
    ${verdict}
    <div class="cmp-tiles">${tile(t('ses.avg'), A.avgFps, B.avgFps, 'FPS', 'up')}${tile('1% low', A.low1, B.low1, 'FPS', 'up', t('cmp.lowD'))}${tile('0.1% low', A.low01, B.low01, 'FPS', 'up')}${tile(t('cmp.stab'), A.stability, B.stability, '%', 'up', t('cmp.stabD'))}</div>
    <div class="cmp-chart-wrap"><div class="cmp-legend"><span class="a">${t('cmp.before')}</span><span class="b">${t('cmp.after')}</span><span class="spacer"></span><span class="muted">FPS</span></div>${chartSVG(a, b)}</div>
    <div class="cmp-table"><div class="cmp-row th"><span></span><span>${t('cmp.before')}</span><span>${t('cmp.after')}</span><span></span></div>
      ${row(t('cmp.median'), A.median, B.median, '', 'up')}${row(t('ses.min'), A.minFps, B.minFps, '', 'up')}${row(t('cmp.dips'), A.dips, B.dips, '', 'down')}
      ${row(t('ses.gpu'), a.maxGpuTemp, b.maxGpuTemp, '°C', 'down')}${row(t('ses.cpu'), a.maxCpuTemp, b.maxCpuTemp, '°C', 'down')}${row(t('ses.ping'), a.avgPing, b.avgPing, ` ${t('u.ms')}`, 'down')}
      <div class="cmp-row"><span>${t('cmp.length')}</span><b class="num">${dur(a.duration)}</b><b class="num">${dur(b.duration)}</b><span></span></div></div>
    <div class="cmp-changes"><h4>${t('cmp.changes')}</h4>${C.changes && C.changes.length ? `<div class="pop">${C.changes.slice(0, 24).map((h) => `<span class="chip static ${h.action === 'revert' ? 'off' : ''}">${icon(h.action === 'revert' ? 'undo' : 'check')}${esc(histName(h))}</span>`).join('')}</div>` : `<p class="muted">${t('cmp.noChanges')}</p>`}</div>
  </div></section>`;
}

export async function openCompare(a, b) {
  V.cmpOpen = true;
  if (!a && !b) V.cmp = null;
  rerender('games');
  try {
    const q = a !== undefined || b !== undefined ? `?a=${encodeURIComponent(a || '')}&b=${encodeURIComponent(b || '')}` : '';
    V.cmp = await api('/api/sessions/compare' + q);
  } catch (e) { toast(t('err.generic'), 'bad', e.message); V.cmpOpen = false; }
  rerender('games');
  setTimeout(() => { const el = document.getElementById('cmp'); if (el && !a && !b) el.scrollIntoView({ behavior: 'smooth', block: 'start' }); }, 50);
}

document.addEventListener('change', (e) => {
  const side = e.target.dataset && e.target.dataset.cmp;
  if (side) {
    const C = V.cmp || {};
    let a = C.a ? C.a.id : '', b = C.b ? C.b.id : '';
    if (side === 'a') a = e.target.value;
    else {
      b = e.target.value;
      // pick an older session of the same game for "before"
      const list = S.tools.sessions || [];
      const nb = list.find((s) => s.id === b);
      const i = list.findIndex((s) => s.id === b);
      const prev = nb && list.slice(i + 1).find((s) => ((s.exe && nb.exe) ? s.exe === nb.exe : s.game === nb.game));
      const any = nb && list.find((s) => s.id !== b && ((s.exe && nb.exe) ? s.exe === nb.exe : s.game === nb.game));
      a = (prev || any || {}).id || '';
    }
    openCompare(a, b);
  }
  if (e.target.dataset && e.target.dataset.clnHour !== undefined) {
    S.settings.autoClean = { ...(S.settings.autoClean || {}), hour: +e.target.value };
    saveAndReport().then(loadCleanup);
  }
});

// ================================================================= SETTINGS: overlay profiles
const POS = ['tl', 'tc', 'tr', 'bl', 'bc', 'br'];

export function profilesCard() {
  const o = S.settings.overlay || {};
  const list = o.profiles || [];
  return `<div class="card ov-profiles" id="set-ov-profiles"><div class="row" style="margin-bottom:6px"><div><b style="font:600 14px var(--display)">${t('pf.title')}</b><p class="muted" style="margin:2px 0 0;font-size:12.5px">${t('pf.sub')}</p></div><span class="spacer"></span>
      <button class="btn sm" data-a="pf-add">${icon('plus')}${t('pf.add')}</button></div>
    ${list.length ? `<div class="pf-list">${list.map((p) => `<div class="pf-row">${coverHTML(p.name, coverForExe(p.exe), 'sm')}
        <div class="grow"><b>${esc(p.name)}</b><span class="muted">${esc(p.exe)} · ${t('pf.summary', { n: p.metrics.length, pos: t('ov.pos.' + p.position) })}</span></div>
        <button class="iconbtn" data-a="pf-edit" data-exe="${esc(p.exe)}" title="${t('pf.edit')}">${icon('pencil')}</button>
        <button class="iconbtn" data-a="pf-del" data-exe="${esc(p.exe)}" title="${t('pf.del')}">${icon('trash')}</button></div>`).join('')}</div>`
      : `<p class="muted pf-empty">${t('pf.empty')}</p>`}</div>`;
}

function pfCandidates() {
  const have = new Set(((S.settings.overlay || {}).profiles || []).map((p) => p.exe));
  const m = new Map();
  const add = (exe, name) => { const k = (exe || '').toLowerCase(); if (k.endsWith('.exe') && !have.has(k) && !m.has(k)) m.set(k, name || k.replace(/\.exe$/, '')); };
  (S.tools.sessions || []).forEach((s) => add(s.exe, s.game));
  (S.games.list || []).forEach((g) => add(g.exe, g.name));
  (S.games.popular || []).forEach((g) => add(g.exe, g.name));
  return [...m.entries()].slice(0, 40);
}

async function pfAdd() {
  const c = pfCandidates();
  const body = `<p class="muted" style="margin:0 0 12px">${t('pf.pickD')}</p>
    ${c.length ? `<div class="pf-pick">${c.map(([exe, name]) => `<button class="chip" data-pick="${esc(exe)}" data-name="${esc(name)}">${esc(name)}</button>`).join('')}</div>` : ''}
    <label class="field"><span>${t('pf.exe')}</span><input class="input" id="pf-exe" placeholder="game.exe" autocomplete="off" spellcheck="false" maxlength="120"></label>
    <p class="muted" id="pf-err" style="color:var(--bad);min-height:18px;margin:6px 0 0"></p>`;
  let picked = null;
  const v = await modal({
    title: t('pf.add'), body,
    actions: [{ label: t('btn.cancel'), value: null, kind: 'ghost' }, { label: t('pf.next'), value: 'next', kind: 'primary', collect: (l) => l.querySelector('#pf-exe').value }],
    onMount: (layer) => {
      layer.querySelectorAll('[data-pick]').forEach((b) => b.addEventListener('click', () => {
        picked = { exe: b.dataset.pick, name: b.dataset.name };
        closeModal('pick');
      }));
    },
  });
  let exe, name;
  if (v === 'pick' && picked) ({ exe, name } = picked);
  else if (v && v.value === 'next') {
    exe = String(v.extra || '').trim().toLowerCase();
    if (exe && !exe.endsWith('.exe')) exe += '.exe';
    if (!/^[^\\/:*?"<>|]{1,120}$/.test(exe) || exe === '.exe') { toast(t('pf.badExe'), 'bad'); return; }
    name = exe.replace(/\.exe$/, '');
  } else return;
  const o = S.settings.overlay || {};
  await pfEdit({ exe, name, metrics: [...(o.metrics || [])], position: o.position || 'tr', layout: o.layout || 'row', size: o.size || 'm' }, true);
}

async function pfEdit(p, isNew) {
  const M = hooks.ovMetrics;
  const cur = { ...p, metrics: [...p.metrics] };
  const seg = (k, opts) => `<div class="seg" data-pf-seg="${k}">${opts.map(([v, l]) => `<button type="button" class="${cur[k] === v ? 'on' : ''}" data-v="${v}">${l}</button>`).join('')}</div>`;
  const body = `<label class="field"><span>${t('pf.name')}</span><input class="input" id="pf-name" value="${esc(cur.name)}" maxlength="60"></label>
    <div class="pf-grid">
      <div><h4 class="pf-h">${t('ov.metrics')}</h4><div class="metrics-grid pf-metrics">${M.map((m) => `<button type="button" class="metric ${cur.metrics.includes(m) ? 'on' : ''}" data-m="${m}"><span class="check ${cur.metrics.includes(m) ? 'on' : ''}">${icon('check')}</span><span><b>${t('ov.m.' + m)}</b></span></button>`).join('')}</div></div>
      <div><h4 class="pf-h">${t('pf.where')}</h4><div class="ov-screen pf-screen">${POS.map((q) => `<button type="button" class="ov-slot s-${q} ${cur.position === q ? 'on' : ''}" data-pos="${q}" aria-label="${t('ov.pos.' + q)}" title="${t('ov.pos.' + q)}"></button>`).join('')}</div>
        <h4 class="pf-h">${t('ov.layout')}</h4>${seg('layout', [['row', t('ov.l.row')], ['column', t('ov.l.column')]])}
        <h4 class="pf-h">${t('ov.size')}</h4>${seg('size', [['s', t('ov.s.s')], ['m', t('ov.s.m')], ['l', t('ov.s.l')]])}</div>
    </div><p class="muted" style="margin:12px 0 0;font-size:12.5px">${t('pf.editD', { exe: esc(cur.exe) })}</p>`;
  const v = await modal({
    title: `${icon('monitorPlay')}${esc(isNew ? t('pf.new') : t('pf.edit'))}`, body, wide: true,
    actions: [{ label: t('btn.cancel'), value: null, kind: 'ghost' }, { label: t('pf.save'), value: 'save', kind: 'primary', collect: (l) => l.querySelector('#pf-name').value }],
    onMount: (layer) => {
      layer.querySelectorAll('[data-m]').forEach((b) => b.addEventListener('click', () => {
        const m = b.dataset.m;
        cur.metrics = cur.metrics.includes(m) ? cur.metrics.filter((x) => x !== m) : M.filter((x) => x === m || cur.metrics.includes(x));
        b.classList.toggle('on'); b.querySelector('.check').classList.toggle('on');
      }));
      layer.querySelectorAll('[data-pos]').forEach((b) => b.addEventListener('click', () => {
        cur.position = b.dataset.pos;
        layer.querySelectorAll('[data-pos]').forEach((x) => x.classList.toggle('on', x === b));
      }));
      layer.querySelectorAll('[data-pf-seg] button').forEach((b) => b.addEventListener('click', () => {
        const k = b.parentElement.dataset.pfSeg;
        cur[k] = b.dataset.v;
        b.parentElement.querySelectorAll('button').forEach((x) => x.classList.toggle('on', x === b));
      }));
    },
  });
  if (!v || v.value !== 'save') return;
  cur.name = String(v.extra || '').trim() || cur.name;
  const o = S.settings.overlay || {};
  const list = (o.profiles || []).filter((x) => x.exe !== cur.exe);
  list.push(cur);
  await hooks.saveOverlay({ profiles: list }, true);
  toast(t('pf.saved', { name: cur.name }), 'good');
}

// ================================================================= SETTINGS: updates + export / import
export function updatesSection() {
  const s = S.settings;
  const U = V.upd;
  let status = '';
  if (V.updErr) status = `<p style="color:var(--warn);margin:6px 0 0;font-size:12.5px">${icon('alert')} ${esc(V.updErr)}</p>`;
  else if (U) status = U.available
    ? `<div class="upd-new"><div class="row" style="gap:10px;flex-wrap:wrap"><span class="tag good">${icon('sparkles')}${t('upd.avail', { v: U.latest })}</span>
        ${U.canApply ? `<button class="btn sm primary" data-a="upd-install" ${V.updBusy || !S.boot.admin ? 'disabled' : ''}>${V.updBusy ? icon('loader', 'spin') : icon('download')}${t('upd.install')}</button>` : `<span class="muted" style="font-size:12.5px">${t('upd.noAssets')}</span>`}</div>
        ${U.notes ? `<pre class="upd-notes">${esc(U.notes)}</pre>` : ''}</div>`
    : `<p class="muted" style="margin:6px 0 0;font-size:12.5px">${icon('ok')} ${t('upd.latest', { v: S.boot.version })}</p>`;
  return `<section class="sec" id="set-updates"><div class="sec-head"><div><h2>${t('upd.title')}</h2><p>${t('upd.sub')}</p></div></div>
    <div class="card">
      <div class="setting"><div><h4>${t('upd.repo')}</h4><p>${t('upd.repoD')}</p></div>
        <div class="row" style="gap:8px"><input class="input" id="upd-repo" value="${esc(s.updateRepo || '')}" placeholder="owner/repo" spellcheck="false" autocomplete="off" style="width:220px">
          <button class="btn sm" data-a="upd-repo">${t('upd.save')}</button></div></div>
      <div class="setting"><div><h4>${t('upd.auto')}</h4><p>${t('upd.autoD')}</p></div>${toggle(!!s.updateAuto, 'data-a="upd-auto" aria-label="' + esc(t('upd.auto')) + '"', { disabled: !s.updateRepo })}</div>
      <div class="setting"><div><h4>${t('set.version', { v: S.boot.version })}</h4>${status}</div>
        <button class="btn sm" data-a="upd-check" ${V.updBusy || !s.updateRepo ? 'disabled' : ''}>${V.updBusy && !U ? icon('loader', 'spin') : icon('cloud')}${t('upd.check')}</button></div>
    </div></section>`;
}

export function ioSection() {
  return `<section class="sec" id="set-backup"><div class="sec-head"><div><h2>${t('io.title')}</h2><p>${t('io.sub')}</p></div></div>
    <div class="card">
      <div class="setting"><div><h4>${t('io.export')}</h4><p>${t('io.exportD')}</p></div><button class="btn" data-a="io-export">${icon('fileDown')}${t('io.exportBtn')}</button></div>
      <div class="setting"><div><h4>${t('io.import')}</h4><p>${t('io.importD')}</p></div><button class="btn" data-a="io-import">${icon('fileUp')}${t('io.importBtn')}</button>
        <input type="file" id="io-file" accept=".json,application/json" hidden></div>
    </div></section>`;
}

async function saveAndReport() {
  try {
    const r = await saveSettings();
    if (r.warn) toast(t('err.generic'), 'bad', r.warn);
    return r;
  } catch (e) { toast(t('err.generic'), 'bad', e.message); return null; }
}

async function doExport() {
  try {
    const data = await api('/api/export');
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `larik-settings-${new Date().toISOString().slice(0, 10)}.json`;
    document.body.appendChild(a);
    a.click();
    setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 1000);
    toast(t('io.exported'), 'good', t('io.exportedD', { n: data.tweaks.length }));
  } catch (e) { toast(t('err.generic'), 'bad', e.message); }
}

function doImport() {
  const inp = document.getElementById('io-file');
  if (!inp) return;
  inp.value = '';
  inp.click();
}

async function importFile(f) {
  if (!f) return;
  if (f.size > 1024 * 1024) { toast(t('io.bad'), 'bad'); return; }
  let data;
  try { data = JSON.parse(await f.text()); } catch (e) { toast(t('io.bad'), 'bad'); return; }
  let r;
  try { r = await api('/api/import', data); } catch (e) { toast(errText(e, 'io.e.'), 'bad'); return; }
  S.settings = r.settings;
  hooks.render();
  const missing = (r.tweaks || []).filter((id) => S.byId[id] && !st(id).a && !st(id).na);
  if (Object.keys(r.nvidia || {}).length) await loadNV();
  const nvOn = new Map(((V.nv && V.nv.settings) || []).map((x) => [x.key, x]));
  const nv = Object.entries(r.nvidia || {}).filter(([k, v]) => { const c = nvOn.get(k); return !c || !c.set || c.value !== v; });
  if (r.warn) toast(t('err.generic'), 'bad', r.warn);
  if (!missing.length && !nv.length) { toast(t('io.done'), 'good'); return; }
  const v = await modal({
    title: `${icon('fileUp')}${t('io.done')}`,
    body: `<p class="muted" style="margin:0 0 10px">${t('io.more')}</p><div class="mini-list">
      ${missing.length ? `<div>${icon('rocket')}${t('io.tweaks', { n: missing.length })}</div>` : ''}
      ${nv.length ? `<div>${icon('gpu')}${t('io.nv', { n: nv.length })}</div>` : ''}</div>`,
    actions: [{ label: t('btn.later'), value: null, kind: 'ghost' }, { label: `${icon('zap')}${t('io.apply')}`, value: 'go', kind: 'primary' }],
  });
  if (v !== 'go') return;
  for (const [k, val] of nv) { try { await api('/api/nvidia', { key: k, on: true, value: val }); } catch (e) { /* skip */ } }
  if (nv.length) loadNV();
  if (missing.length) await hooks.applyIds(missing, true);
}

document.addEventListener('change', (e) => {
  if (e.target && e.target.id === 'io-file') importFile(e.target.files && e.target.files[0]);
});

// Automatic update check at start (at most once a day, only with a repo set).
export async function autoUpdateCheck() {
  const s = S.settings;
  const today = new Date().toISOString().slice(0, 10);
  if (!s.updateAuto || !s.updateRepo || s.updateSeen === today) return;
  s.updateSeen = today;
  saveSettings().catch(() => {});
  try {
    const u = await api('/api/update/check', {});
    V.upd = u;
    if (u.available) toast(t('upd.toast', { v: u.latest }), 'good', t('upd.toastD'));
  } catch (e) { /* offline: try tomorrow */ }
}

// ================================================================= network test polling
let netTimer = null;
function pollNet() {
  clearTimeout(netTimer);
  netTimer = setTimeout(async () => {
    try { V.net = await api('/api/nettest'); } catch (e) { /* ignore */ }
    rerender('health');
    if (V.net && V.net.running) pollNet();
  }, 500);
}

// ================================================================= actions
export const V107_ACTIONS = {
  'dk-refresh': () => loadDisks(true),
  'drv-check': async () => {
    V.drvBusy = true; V.drvErr = ''; rerender('health');
    try { V.driver = await api('/api/health/driver/check', {}); } catch (e) { V.drvErr = t('drv.offline'); }
    V.drvBusy = false; rerender('health');
  },
  'drv-open': () => api('/api/health/driver/open', {}).catch(() => {}),
  'net-run': async () => {
    V.netBusy = true; rerender('health');
    try { V.net = await api('/api/nettest', {}); pollNet(); } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    V.netBusy = false; rerender('health');
  },
  'cln-run': async () => {
    V.cleanBusy = true; rerender('health');
    try {
      const r = await api('/api/cleanup/run', {});
      V.clean = { ...(V.clean || {}), runs: r.runs };
      toast(t('cln.done', { size: fmtBytes(r.run.bytes) }), 'good');
    } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    V.cleanBusy = false; rerender('health');
  },
  'cln-toggle': async (d, el) => {
    S.settings.autoClean = { day: 0, hour: 12, ...(S.settings.autoClean || {}), enabled: !el.classList.contains('on') };
    rerender('health');
    await saveAndReport();
    await loadCleanup();
  },
  'cln-day': async (d) => {
    S.settings.autoClean = { ...(S.settings.autoClean || {}), day: +d.v };
    rerender('health');
    await saveAndReport();
    await loadCleanup();
  },
  'ag-toggle': async (d, el) => {
    const on = !el.classList.contains('on');
    S.settings.autoGame = { boost: true, overlay: true, ...(S.settings.autoGame || {}), enabled: on };
    rerender('boost');
    const r = await saveAndReport();
    if (r && on) toast(t('ag.onToast'), 'good', t('ag.onToastD'));
    setTimeout(loadOvStatus, 1500);
  },
  'ag-opt': async (d, el) => {
    S.settings.autoGame = { ...(S.settings.autoGame || {}), [d.k]: !el.classList.contains('on') };
    rerender('boost');
    await saveAndReport();
  },
  'nv-set': async (d) => {
    const on = d.on === '1';
    V.nvBusy[d.k] = true; rerender('boost');
    try { V.nv = await api('/api/nvidia', { key: d.k, on }); toast(on ? t('nv.applied') : t('nv.reverted'), 'good'); } catch (e) { toast(errText(e, 'nv.e.'), 'bad'); }
    delete V.nvBusy[d.k]; rerender('boost');
  },
  'nv-fps': async (d) => {
    const inp = document.getElementById('nv-fps');
    const v = d.v ? +d.v : +(inp && inp.value);
    if (!(v >= 20 && v <= 1023)) { toast(t('nv.badFps'), 'bad'); return; }
    V.nvBusy.fpslimit = true; rerender('boost');
    try { V.nv = await api('/api/nvidia', { key: 'fpslimit', on: true, value: Math.round(v) }); toast(t('nv.fpsSet', { n: Math.round(v) }), 'good'); } catch (e) { toast(errText(e, 'nv.e.'), 'bad'); }
    delete V.nvBusy.fpslimit; rerender('boost');
  },
  'nv-restore': async () => {
    for (const s of (V.nv && V.nv.settings) || []) {
      if (!s.ours) continue;
      try { V.nv = await api('/api/nvidia', { key: s.key, on: false }); } catch (e) { /* continue */ }
    }
    toast(t('nv.reverted'), 'good');
    rerender('boost');
  },
  'cmp-open': () => openCompare(),
  'cmp-close': () => { V.cmpOpen = false; V.cmp = null; rerender('games'); },
  'pf-add': () => pfAdd(),
  'pf-edit': (d) => {
    const p = ((S.settings.overlay || {}).profiles || []).find((x) => x.exe === d.exe);
    if (p) pfEdit(p, false);
  },
  'pf-del': async (d) => {
    const o = S.settings.overlay || {};
    await hooks.saveOverlay({ profiles: (o.profiles || []).filter((x) => x.exe !== d.exe) }, true);
  },
  'upd-repo': async () => {
    const v = (document.getElementById('upd-repo') || {}).value || '';
    S.settings.updateRepo = v.trim();
    const r = await saveAndReport();
    V.upd = null; V.updErr = '';
    if (r && v.trim() && !S.settings.updateRepo) toast(t('upd.badRepo'), 'bad');
    else if (r) toast(t('upd.saved'), 'good');
    hooks.renderPage();
  },
  'upd-auto': async (d, el) => {
    S.settings.updateAuto = !el.classList.contains('on');
    hooks.renderPage();
    await saveAndReport();
  },
  'upd-check': async () => {
    V.updBusy = true; V.updErr = ''; V.upd = null; hooks.renderPage();
    try { V.upd = await api('/api/update/check', {}); } catch (e) { V.updErr = errText(e, 'upd.e.'); }
    V.updBusy = false; hooks.renderPage();
  },
  'upd-install': async () => {
    const U = V.upd;
    if (!U) return;
    const ok = await modal({
      title: `${icon('cloud')}${t('upd.q', { v: U.latest })}`, body: `<p class="muted" style="margin:0">${t('upd.qD')}</p>`,
      actions: [{ label: t('btn.cancel'), value: null, kind: 'ghost' }, { label: `${icon('download')}${t('upd.install')}`, value: 'go', kind: 'primary' }],
    });
    if (ok !== 'go') return;
    V.updBusy = true; hooks.renderPage();
    try {
      await api('/api/update/install', {});
      modal({ title: `${icon('loader', 'spin')}${t('upd.restarting')}`, body: `<p class="muted" style="margin:0">${t('upd.restartingD')}</p>`, actions: [] });
      setTimeout(() => { try { window.close(); } catch (e) { /* ignore */ } }, 4000);
    } catch (e) { V.updErr = errText(e, 'upd.e.'); toast(V.updErr, 'bad'); }
    V.updBusy = false; hooks.renderPage();
  },
  'io-export': () => doExport(),
  'io-import': () => doImport(),
};
