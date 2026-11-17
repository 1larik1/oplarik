import { ICONS } from './icons.js';
import { t, tx, lang } from './i18n.js';

// ---------------------------------------------------------------- state
export const S = {
  boot: null, tweaks: [], byId: {}, states: {}, score: 0, active: 0, total: 0,
  info: null, saved: new Set(), settings: {}, page: 'home',
  opt: { tab: 'all', cat: 'all', q: '' },
  live: { cpu: [], gpu: [], ram: [], last: null },
  deb: { scanned: false, tab: 'apps', sel: new Set(), scanning: false, showGone: false },
  games: { list: [], popular: [], found: null, scanning: false },
  boost: { session: null, bg: [], sel: new Set(), temp: null, results: {} , busy: {} },
  backups: { applied: {}, history: [], restorePoints: [] },
  pending: new Set(), need: { pc: new Set(), signout: false, explorer: false },
  job: null, rpAsked: false, drawer: null, gaugeShown: false,
};

// ---------------------------------------------------------------- API
const hashToken = new URLSearchParams(location.hash.slice(1)).get('t');
if (hashToken) {
  try { sessionStorage.setItem('larik.t', hashToken); } catch (e) { /* ignore */ }
  history.replaceState(null, '', location.pathname);
}
let token = hashToken;
if (!token) { try { token = sessionStorage.getItem('larik.t'); } catch (e) { token = ''; } }
export const TOKEN = token || '';

export async function api(path, body) {
  const opt = { method: body === undefined ? 'GET' : 'POST', headers: { 'X-Larik-Token': TOKEN } };
  if (body !== undefined) { opt.headers['Content-Type'] = 'application/json'; opt.body = JSON.stringify(body); }
  const r = await fetch(path, opt);
  let data = null;
  try { data = await r.json(); } catch (e) { /* empty */ }
  if (r.status === 423) window.dispatchEvent(new Event('larik-locked'));
  if (!r.ok) throw new Error((data && data.error) || r.statusText);
  return data;
}

const jobWaiters = new Map();
const jobsDone = new Map(); // finished before anyone waited (fast jobs)
const jobListeners = new Set();
export function onJob(fn) { jobListeners.add(fn); }

export function handleJobEvent(j) {
  S.job = j.finished ? null : j;
  jobListeners.forEach((fn) => fn(j));
  if (!j.finished) return;
  if (jobWaiters.has(j.id)) { jobWaiters.get(j.id)(j); jobWaiters.delete(j.id); }
  else { jobsDone.set(j.id, j); if (jobsDone.size > 50) jobsDone.delete(jobsDone.keys().next().value); }
}

export async function runJob(kind, body) {
  const path = { apply: '/api/apply', revert: '/api/revert', scan: '/api/debloat/scan', rp: '/api/restore-point' }[kind];
  const { job } = await api(path, body || {});
  if (jobsDone.has(job)) { const j = jobsDone.get(job); jobsDone.delete(job); return j; }
  return new Promise((res) => jobWaiters.set(job, res));
}

export async function saveSettings() {
  const r = await api('/api/settings', S.settings);
  if (r && r.settings) S.settings = r.settings;
  return r || {};
}

export function setStates(d) {
  S.states = d.states || S.states;
  S.score = d.score ?? S.score; S.active = d.active ?? S.active; S.total = d.total ?? S.total;
}

// ---------------------------------------------------------------- helpers
export const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
export const icon = (name, cls = '') => `<svg class="i ${cls}" viewBox="0 0 24 24" aria-hidden="true">${ICONS[name] || ''}</svg>`;

export function nf(n, d = 0) { return new Intl.NumberFormat(lang(), { maximumFractionDigits: d, minimumFractionDigits: d }).format(n); }

export function fmtBytes(b, d) {
  if (!b && b !== 0) return '—';
  const gb = b / 1073741824;
  if (gb >= 1) return `${nf(gb, d ?? (gb >= 100 ? 0 : 1))} ${t('u.gb')}`;
  const mb = b / 1048576;
  if (mb >= 1) return `${nf(mb, 0)} ${t('u.mb')}`;
  return `${nf(b / 1024, 0)} ${t('u.kb')}`;
}

export function fmtGB(b) { return `${nf(Math.round(b / 1073741824))} ${t('u.gb')}`; }
export function fmtDiskSize(b) { const gb = b / 1e9; return gb >= 1000 ? `${nf(gb / 1000, 1)} TB` : `${nf(gb, 0)} ${t('u.gb')}`; }

export function fmtDate(iso, withTime = false) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  const o = { day: 'numeric', month: 'short', year: 'numeric' };
  if (withTime) { o.hour = '2-digit'; o.minute = '2-digit'; }
  return d.toLocaleString(lang(), o);
}

export function fmtTime(iso) {
  const d = new Date(iso);
  return isNaN(d) ? '' : d.toLocaleTimeString(lang(), { hour: '2-digit', minute: '2-digit' });
}

export function fmtUptime(sec) {
  const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  if (d) return `${d} ${t('u.d')} ${h} ${t('u.h')}`;
  if (h) return `${h} ${t('u.h')} ${m} ${t('u.m')}`;
  return `${m} ${t('u.m')}`;
}

export function ago(iso) {
  const s = (Date.now() - new Date(iso)) / 1000;
  if (s < 60) return t('ago.now');
  if (s < 3600) return t('ago.min', { n: Math.floor(s / 60) });
  if (s < 86400) return t('ago.h', { n: Math.floor(s / 3600) });
  return fmtDate(iso);
}

export function hue(str) {
  let h = 0;
  for (const c of String(str)) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return 250 + (h % 70);
}
export function cover(name) {
  const hh = hue(name);
  return `style="background:linear-gradient(135deg,hsl(${hh} 70% 52%),hsl(${hh + 35} 75% 42%))"`;
}

// Game / program picture from this PC (Steam artwork or the .exe icon), with
// the letter tile as fallback when there is none or it fails to load.
export function coverHTML(name, url, cls = '') {
  const letter = esc((String(name || '?').trim()[0] || '?').toUpperCase());
  if (!url) return `<div class="cover ${cls}" ${cover(name)}>${letter}</div>`;
  const kind = url.includes('/cover/s') ? 'poster' : 'icon';
  return `<div class="cover ${cls} pic ${kind}" ${cover(name)} data-l="${letter}"><img src="${esc(url)}?t=${encodeURIComponent(TOKEN)}" alt="" loading="lazy" decoding="async" data-cover></div>`;
}
export function coverForExe(exe) {
  const k = String(exe || '').toLowerCase();
  if (!k) return '';
  const hit = (S.games.list || []).find((g) => g.cover && g.exe.toLowerCase() === k) || (S.games.found || []).find((g) => g.cover && g.exe.toLowerCase() === k);
  return hit ? hit.cover : '';
}
document.addEventListener('error', (e) => {
  const im = e.target;
  if (!im || !im.dataset || im.dataset.cover === undefined) return;
  const p = im.parentElement;
  p.classList.remove('pic', 'poster', 'icon');
  p.textContent = p.dataset.l || '?';
}, true);

// ---------------------------------------------------------------- tweak helpers
export function st(id) { return S.states[id] || {}; }
export function isOpt(tw) { return ['fps', 'power', 'gpu', 'input', 'network', 'privacy', 'qol'].includes(tw.cat); }
export function recommended(tw) { const s = st(tw.id); return tw.rec && s.f !== false && !s.na; }

export const CAT_ICON = { fps: 'gauge', power: 'power', gpu: 'gpu', input: 'mouse', network: 'network', privacy: 'shield', qol: 'wand',
  services: 'server', components: 'puzzle', apps: 'package', sponsored: 'store' };

// ---------------------------------------------------------------- toasts
export function toast(msg, kind = 'info', sub = '') {
  const el = document.createElement('div');
  el.className = `toast ${kind}`;
  const ic = kind === 'good' ? 'ok' : kind === 'bad' ? 'warn' : 'info';
  el.innerHTML = `${icon(ic)}<div><div>${msg}</div>${sub ? `<div class="t-sub">${esc(sub)}</div>` : ''}</div><button class="iconbtn" aria-label="${t('btn.close')}">${icon('x')}</button>`;
  const close = () => { el.classList.add('out'); setTimeout(() => el.remove(), 220); };
  el.querySelector('button').onclick = close;
  document.getElementById('toasts').appendChild(el);
  setTimeout(close, kind === 'bad' ? 9000 : 4500);
}

// ---------------------------------------------------------------- modal
let modalResolve = null;
export function modal({ title, body = '', actions = [], wide = false, onMount }) {
  const layer = document.getElementById('modal');
  if (modalResolve) modalResolve(null);
  layer.innerHTML = `<div class="modal ${wide ? 'wide' : ''}" role="dialog" aria-modal="true">
    <div class="modal-head"><h2>${title}</h2></div>
    <div class="modal-body">${body}</div>
    <div class="modal-foot">${actions.map((a, i) => `<button class="btn ${a.kind || ''} ${a.left ? 'left' : ''}" data-mi="${i}">${a.label}</button>`).join('')}</div>
  </div>`;
  layer.classList.add('on');
  return new Promise((res) => {
    modalResolve = res;
    layer.onclick = (e) => {
      const b = e.target.closest('[data-mi]');
      if (b) { const a = actions[+b.dataset.mi]; closeModal(a.value ?? true, a.collect ? a.collect(layer) : undefined); }
      else if (e.target === layer) closeModal(null);
    };
    if (onMount) onMount(layer);
    const primary = layer.querySelector('.btn.primary');
    (primary || layer.querySelector('.btn'))?.focus();
  });
}

export function closeModal(v = null, extra) {
  const layer = document.getElementById('modal');
  layer.classList.remove('on');
  layer.innerHTML = '';
  if (modalResolve) { const r = modalResolve; modalResolve = null; r(extra !== undefined ? { value: v, extra } : v); }
}
export function modalOpen() { return document.getElementById('modal').classList.contains('on'); }

// ---------------------------------------------------------------- components
export function toggle(on, attrs = '', { busy = false, partial = false, disabled = false } = {}) {
  return `<button class="tg ${on ? 'on' : ''} ${busy ? 'busy' : ''} ${partial ? 'partial' : ''}" role="switch" aria-checked="${on}" ${disabled ? 'disabled' : ''} ${attrs}></button>`;
}

export function check(on, attrs = '') {
  return `<button class="check ${on ? 'on' : ''}" role="checkbox" aria-checked="${on}" ${attrs}>${icon('check')}</button>`;
}

export function impact(n) {
  return `<span class="imp" title="${t('imp')}: ${t('imp.' + n)}">${[1, 2, 3].map((i) => `<i class="${i <= n ? 'f' : ''}"></i>`).join('')}</span>`;
}

// Segmented performance dial: the signature element of OpLarik's home screen.
const SEG = 46;
export function gaugeSVG(value) {
  const cx = 118, cy = 118, r1 = 92, r2 = 112, start = 135, sweep = 270;
  const lit = Math.round((value / 100) * SEG);
  const off = segOff();
  let segs = '';
  for (let i = 0; i < SEG; i++) {
    const a = ((start + (sweep / (SEG - 1)) * i) * Math.PI) / 180;
    const w = 1.6 * Math.PI / 180;
    const p = (r, ang) => `${(cx + r * Math.cos(ang)).toFixed(2)},${(cy + r * Math.sin(ang)).toFixed(2)}`;
    const col = i < lit ? segColor(i / (SEG - 1)) : off;
    segs += `<polygon class="seg" data-i="${i}" points="${p(r1, a - w)} ${p(r2, a - w * 1.25)} ${p(r2, a + w * 1.25)} ${p(r1, a + w)}" fill="${col}"/>`;
  }
  return `<svg viewBox="0 0 236 236">${segs}<circle cx="118" cy="118" r="80" fill="none" style="stroke:var(--seg-off)" stroke-width="1" stroke-dasharray="2 5"/></svg>`;
}

// Theme colors come from CSS variables so every theme recolors the gauge too.
function cssVar(name) { return getComputedStyle(document.documentElement).getPropertyValue(name).trim(); }
function rgbVar(name, dflt) {
  const v = cssVar(name).split(',').map((x) => +x.trim());
  return v.length === 3 && v.every((x) => !isNaN(x)) ? v : dflt;
}
export const DARK_THEMES = ['violet', 'ocean', 'emerald', 'ruby', 'amber', 'sakura', 'graphite'];
export const LIGHT_THEMES = ['light', 'sky', 'lagoon', 'mint', 'sand', 'rose', 'paper'];
export const THEMES = [...DARK_THEMES, ...LIGHT_THEMES];
export function applyTheme(name) {
  const th = THEMES.includes(name) ? name : 'violet';
  document.documentElement.setAttribute('data-theme', th);
  document.documentElement.setAttribute('data-tone', LIGHT_THEMES.includes(th) ? 'light' : 'dark');
  try { localStorage.setItem('larik.theme', th); } catch (e) { /* ignore */ }
  const m = document.querySelector('meta[name="theme-color"]');
  if (m) m.content = cssVar('--bg') || '#110c1c';
  const cs = document.querySelector('meta[name="color-scheme"]');
  if (cs) cs.content = LIGHT_THEMES.includes(th) ? 'light' : 'dark';
}

export function segOff() { return cssVar('--seg-off') || '#2b2142'; }

export function segColor(f) {
  // accent deep → accent → accent light (violet → purple → orchid in the default theme)
  const stops = [rgbVar('--a1-rgb', [124, 58, 237]), rgbVar('--a3-rgb', [168, 85, 247]), rgbVar('--a4-rgb', [209, 107, 255])];
  const x = f * 2, i = Math.min(1, Math.floor(x)), k = x - i;
  const c = stops[i].map((v, j) => Math.round(v + (stops[i + 1][j] - v) * k));
  return `rgb(${c.join(',')})`;
}

export function animateGauge(root, to, from = 0) {
  if (!root) return;
  const val = root.querySelector('.gauge-val b');
  const segs = root.querySelectorAll('.seg');
  const reduced = document.getElementById('app').classList.contains('reduced');
  const dur = reduced ? 0 : 1100;
  const t0 = performance.now();
  const step = (now) => {
    const k = dur ? Math.min(1, (now - t0) / dur) : 1;
    const e = 1 - Math.pow(1 - k, 3);
    const v = from + (to - from) * e;
    val.textContent = nf(v, 1);
    const lit = Math.round((v / 100) * SEG);
    const off = segOff();
    segs.forEach((s, i) => { s.setAttribute('fill', i < lit ? segColor(i / (SEG - 1)) : off); });
    if (k < 1) requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
}

export function ring(pct, id) {
  const c = 2 * Math.PI * 38;
  const off = c * (1 - Math.max(0, Math.min(100, pct || 0)) / 100);
  return `<svg class="ring" viewBox="0 0 92 92" id="${id}"><circle class="bg" cx="46" cy="46" r="38"/><circle class="fg" cx="46" cy="46" r="38" stroke-dasharray="${c.toFixed(1)}" stroke-dashoffset="${off.toFixed(1)}" transform="rotate(-90 46 46)"/></svg>`;
}

export function sparkPath(arr, w = 300, h = 44) {
  if (!arr.length) return {};
  const n = 40;
  const pts = arr.slice(-n);
  if (pts.length === 1) pts.unshift(pts[0]);
  const dx = w / (pts.length - 1);
  let d = '';
  pts.forEach((v, i) => { const x = i * dx, y = h - 3 - (Math.max(0, Math.min(100, v)) / 100) * (h - 6); d += `${i ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`; });
  return { line: d, area: `${d}L${w},${h}L0,${h}Z` };
}

export function tx2(o) { return tx(o); }
