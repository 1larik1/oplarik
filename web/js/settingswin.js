// Settings window (1.1.0): opens over the app like a separate window, with a
// sidebar of categories (and their sub-sections), search and Esc / × to close.
import { t, lang } from './i18n.js';
import { S, esc, icon } from './core.js';
import { settingsParts } from './pages.js';
import { avatarHTML } from './auth.js';

// Category → parts it shows; sub-items scroll to an anchor inside it.
const CATS = [
  { g: 'sw.g.user', items: [
    { id: 'account', ic: 'userRound', parts: ['account'] },
  ] },
  { g: 'sw.g.app', items: [
    { id: 'appearance', ic: 'brush', parts: ['theme'] },
    { id: 'lang', ic: 'globe', parts: ['lang'] },
    { id: 'safety', ic: 'shield', parts: ['safety'] },
    { id: 'tray', ic: 'layers', parts: ['tray'] },
  ] },
  { g: 'sw.g.games', items: [
    { id: 'overlay', ic: 'monitorPlay', parts: ['overlay'] },
  ] },
  { g: 'sw.g.larik', items: [
    { id: 'updates', ic: 'cloud', parts: ['updates'] },
    { id: 'backup', ic: 'fileDown', parts: ['backup'] },
    { id: 'install', ic: 'download', parts: ['install'] },
    { id: 'about', ic: 'info', parts: ['about'] },
  ] },
];
const ALL = CATS.flatMap((c) => c.items);
const SW = { cat: 'appearance', q: '', built: false };
const $ = (id) => document.getElementById(id);

export function settingsOpen() { return S.page === 'settings'; }

function layer() {
  let el = $('settings-layer');
  if (!el) {
    el = document.createElement('div');
    el.id = 'settings-layer';
    el.className = 'set-layer';
    el.setAttribute('role', 'dialog');
    el.setAttribute('aria-modal', 'true');
    $('app').appendChild(el);
  }
  return el;
}

function strip(html) { return html.replace(/<[^>]+>/g, ' ').replace(/&[a-z#0-9]+;/gi, ' ').replace(/\s+/g, ' ').toLowerCase(); }

function navHTML(parts) {
  const q = SW.q.trim().toLowerCase();
  const match = (it) => !q || t('sw.c.' + it.id).toLowerCase().includes(q) || it.parts.some((p) => strip(parts[p]).includes(q));
  let h = '';
  for (const c of CATS) {
    const items = c.items.filter(match);
    if (!items.length) continue;
    h += `<div class="sw-group">${t(c.g)}</div>`;
    for (const it of items) {
      const on = it.id === SW.cat;
      h += `<button class="sw-item ${on ? 'on' : ''}" data-a="sw-cat" data-v="${it.id}">${icon(it.ic)}<span>${t('sw.c.' + it.id)}</span></button>`;
      if (on && it.subs && !q) h += `<div class="sw-subs">${it.subs.map(([a, k]) => `<button class="sw-sub" data-a="sw-sub" data-v="${a}">${t(k)}</button>`).join('')}</div>`;
    }
  }
  return h || `<p class="sw-none">${t('sw.none')}</p>`;
}

function profileHTML() {
  const a = S.acct || {};
  if (a.exists) {
    return `<button class="sw-me" data-a="sw-cat" data-v="account">${avatarHTML(a, 44)}<span><b>${esc(a.name)}</b><span>${t('sw.editProfile')} ${icon('pencil')}</span></span></button>`;
  }
  return `<button class="sw-me" data-a="sw-cat" data-v="account"><img src="img/logo.svg" width="44" height="44" alt=""><span><b>OpLarik</b><span>${t('sw.makeProfile')}</span></span></button>`;
}

function build() {
  const el = layer();
  el.innerHTML = `<div class="set-win">
      <aside class="sw-nav"><div class="sw-nav-in">
        <div id="sw-me">${profileHTML()}</div>
        <label class="sw-search">${icon('search')}<input id="sw-q" type="search" placeholder="${esc(t('sw.search'))}" autocomplete="off" spellcheck="false" value="${esc(SW.q)}"></label>
        <nav id="sw-list"></nav>
        <div class="sw-ver">OpLarik v${esc(S.boot.version)}</div>
      </div></aside>
      <section class="sw-main">
        <header class="sw-head"><h1 id="sw-title"></h1><button class="sw-close" data-a="sw-close" aria-label="${esc(t('btn.close'))}" title="Esc">${icon('x')}</button></header>
        <div class="sw-body" id="sw-body"></div>
      </section>
    </div>`;
  $('sw-q').addEventListener('input', (e) => {
    SW.q = e.target.value;
    const parts = settingsParts();
    $('sw-list').innerHTML = navHTML(parts);
    // jump to the first match
    const first = $('sw-list').querySelector('.sw-item');
    if (SW.q.trim() && first && first.dataset.v !== SW.cat) { SW.cat = first.dataset.v; fill(parts); }
  });
  SW.built = true;
}

function fill(parts) {
  const it = ALL.find((x) => x.id === SW.cat) || ALL[0];
  $('sw-title').textContent = t('sw.c.' + it.id);
  $('sw-list').innerHTML = navHTML(parts);
  $('sw-me').innerHTML = profileHTML();
  const body = $('sw-body');
  const top = body.dataset.cat === it.id ? body.scrollTop : 0;
  body.innerHTML = `<div class="sw-content">${it.parts.map((p) => parts[p]).join('')}</div>`;
  body.dataset.cat = it.id;
  body.scrollTop = top;
}

// Called by the app's renderPage() while settings are open.
export function renderSettings() {
  const el = layer();
  if (!SW.built || !el.firstChild || SW.lang !== lang()) { build(); SW.lang = lang(); }
  el.classList.add('on');
  document.getElementById('app').classList.add('sw-open');
  const focused = document.activeElement && document.activeElement.dataset && document.activeElement.dataset.ovRange;
  if (focused) return; // don't replace a slider while it's being dragged
  fill(settingsParts());
}

export function hideSettings() {
  const el = $('settings-layer');
  if (el) el.classList.remove('on');
  document.getElementById('app').classList.remove('sw-open');
  SW.q = '';
  SW.built = false;
}

export function openSettingsAt(cat) {
  if (cat && ALL.some((x) => x.id === cat)) SW.cat = cat;
}

export const SW_ACTIONS = {
  'sw-cat': (d) => {
    SW.cat = d.v;
    fill(settingsParts());
    $('sw-body').scrollTop = 0;
  },
  'sw-sub': (d) => {
    const el = document.getElementById(d.v);
    if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
  },
};
