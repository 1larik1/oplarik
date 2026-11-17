import { t, tx, setLang, lang, LANGS } from './i18n.js';
import {
  S, TOKEN, api, saveSettings, runJob, onJob, handleJobEvent, setStates, esc, icon, toast, modal, closeModal, modalOpen, fmtBytes,
  animateGauge, st, isOpt, CAT_ICON,
} from './core.js';
import {
  homePage, optimizePage, gamesPage, boostPage, debloatPage, systemPage, backupPage, settingsPage, drawerHTML,
  updateLive, presetIds, specsText, OV_METRICS, hotkeyLabel,
} from './pages.js';
import { applyTheme } from './core.js';
import { initTools, TOOL_ACTIONS, startupPage, loadShader, loadDNSInfo, loadStartup, loadSessions, runShader, showChangelog } from './tools.js';
import { initAccount, authScreen, showLocked, ACCOUNT_ACTIONS, avatarHTML, authVisible } from './auth.js';
import { initV109, V109_ACTIONS, loadSessionsQuiet } from './v109.js';
import { renderSettings, hideSettings, openSettingsAt, SW_ACTIONS } from './settingswin.js';
import { initV107, V107_ACTIONS, healthPage, loadHealth, loadNV, loadOvStatus, loadDisks, diskAlert, autoUpdateCheck } from './v107.js';

const PAGES = {
  home: { ic: 'home', render: homePage },
  optimize: { ic: 'rocket', render: optimizePage },
  games: { ic: 'games', render: gamesPage },
  boost: { ic: 'zap', render: boostPage },
  debloat: { ic: 'debloat', render: debloatPage },
  startup: { ic: 'startup', render: startupPage },
  system: { ic: 'cpu', render: systemPage, sep: true },
  health: { ic: 'health', render: healthPage },
  backup: { ic: 'backup', render: backupPage },
  settings: { ic: 'settings', render: settingsPage },
};
const $ = (id) => document.getElementById(id);

// ================================================================= render
function renderRail() {
  let h = `<img class="rail-logo" src="img/logo.svg" alt="OpLarik">`;
  for (const [k, p] of Object.entries(PAGES)) {
    if (p.sep) h += '<div class="rail-sep"></div>';
    const al = k === 'health' && diskAlert() ? `alert-${diskAlert()}` : '';
    h += `<button class="rail-item ${S.page === k ? 'on' : ''} ${al}" data-a="go" data-page="${k}" ${S.page === k ? 'aria-current="page"' : ''}>${icon(p.ic)}<span>${t('nav.' + k)}</span></button>`;
  }
  const fresh = S.settings.seenVersion !== S.boot.version;
  h += `<div class="rail-foot"><span class="rail-dot ${S.boot.admin ? '' : 'limited'}" title="${S.boot.admin ? t('admin.on') : t('admin.off')}"></span><button class="rail-ver ${fresh ? 'fresh' : ''}" data-a="changelog" title="${t('cl.title')}">v${S.boot.version}</button></div>`;
  $('rail').innerHTML = h;
}

function pageTitle() {
  const p = S.page === 'settings' ? (S.bgPage || 'home') : S.page;
  if (p === 'home') {
    const u = (S.acct && S.acct.exists && S.acct.name) || (S.info && S.info.user);
    return [u ? t('home.hi', { user: u }) : t('home.hiAnon'), t('home.sub')];
  }
  if (p === 'optimize') return [t('opt.title'), t('opt.sub', { active: S.active, total: S.total })];
  if (p === 'games') return [t('g.title'), t('g.sub')];
  if (p === 'boost') return [t('bo.title'), t('bo.sub')];
  if (p === 'debloat') return [t('d.title'), t('d.sub')];
  if (p === 'startup') return [t('su.title'), t('su.subT')];
  if (p === 'system') return [t('s.title'), S.info && S.info.scanAt ? t('s.sub', { time: new Date(S.info.scanAt).toLocaleString(lang(), { hour: '2-digit', minute: '2-digit', day: 'numeric', month: 'short' }) }) : t('home.scanning')];
  if (p === 'backup') return [t('bk.title'), t('bk.sub')];
  if (p === 'health') return [t('hl.title'), t('hl.sub')];
  return [t('set.title'), ''];
}

function renderTop() {
  const [h, sub] = pageTitle();
  const j = S.job;
  let job = '';
  if (j) {
    const label = t('job.' + j.kind, { done: Math.min(j.done + 1, j.total), total: j.total });
    job = `<span class="pill job">${icon('loader')}<span>${esc(label)}</span>${j.total > 1 ? `<span class="bar"><i style="width:${(j.done / j.total) * 100}%"></i></span>` : ''}</span>`;
  }

  $('topbar').innerHTML = `<div class="tb-title"><h1>${esc(h)}</h1>${sub ? `<p>${esc(sub)}</p>` : ''}</div>
    <div class="tb-right">${job}
      ${S.page === 'optimize' ? `<label class="search">${icon('search')}<input id="q" type="search" placeholder="${t('search.ph')}" value="${esc(S.opt.q)}" autocomplete="off" spellcheck="false"></label>` : ''}
      ${S.boot.demo ? `<span class="pill demo">${icon('sparkles')}${t('demo')}</span>` : ''}
      <span class="pill ${S.boot.admin ? 'admin' : 'limited'}">${icon(S.boot.admin ? 'shield' : 'lock')}${S.boot.admin ? t('admin.on') : t('admin.off')}</span>
      ${S.acct && S.acct.exists ? `<button class="acct-chip" data-a="acct-menu" aria-haspopup="true" title="${esc(S.acct.name)}">${avatarHTML(S.acct, 34)}</button>` : ''}
    </div>`;
  const q = $('q');
  if (q) q.oninput = (e) => { S.opt.q = e.target.value; renderPage(); };
}

function renderBanners() {
  let h = '';
  if (!S.boot.admin) h += `<div class="banner warn">${icon('lock')}<div class="grow">${t('banner.limited')}</div><button class="btn sm primary" data-a="elevate">${icon('shield')}${t('btn.elevate')}</button></div>`;
  // One banner at a time: a restart also covers sign-out and Explorer changes.
  if (S.need.pc.size) h += `<div class="banner">${icon('restore')}<div class="grow">${t('banner.restart', { n: S.need.pc.size })}</div><button class="btn sm ghost" data-a="need-dismiss" data-k="all">${t('btn.later')}</button><button class="btn sm" data-a="power" data-v="restart">${t('btn.restartNow')}</button></div>`;
  else if (S.need.signout) h += `<div class="banner">${icon('logout')}<div class="grow">${t('banner.signout')}</div><button class="btn sm ghost" data-a="need-dismiss" data-k="all">${t('btn.later')}</button><button class="btn sm" data-a="power" data-v="signout">${t('btn.signout')}</button></div>`;
  else if (S.need.explorer) h += `<div class="banner">${icon('refresh')}<div class="grow">${t('banner.explorer')}</div><button class="btn sm ghost" data-a="need-dismiss" data-k="explorer">${t('btn.later')}</button><button class="btn sm" data-a="power" data-v="explorer">${t('btn.explorer')}</button></div>`;
  $('banners').innerHTML = h;
}

function renderPage() {
  if (S.page === 'settings') { renderSettings(); return; } // settings window over the current page
  const page = $('page');
  page.innerHTML = PAGES[S.page].render();
  if (S.page === 'home') {
    if (!S.gaugeShown && S.info && S.info.scanAt && S.states && Object.keys(S.states).length) {
      S.gaugeShown = true;
      animateGauge($('gauge'), S.score, 0);
    }
  }
  if (S.drawer) $('drawer').innerHTML = drawerHTML(S.drawer);
}

export function render() {
  renderRail();
  renderTop();
  renderBanners();
  renderPage();
}

function go(page, cat) {
  if (!PAGES[page]) return;
  if (page === 'settings') {
    // The settings window opens over whatever page is shown.
    if (S.page !== 'settings') S.bgPage = S.page;
    openSettingsAt(cat);
    S.page = 'settings';
    render();
    pollOverlay();
    return;
  }
  if (S.page === 'settings') hideSettings();
  S.page = page;
  $('main').scrollTop = 0;
  render();
  if (page === 'games' && !S.games.loaded) loadGames();
  if (page === 'games') loadSessions();
  if (page === 'boost') { loadBoost(); loadShader(); if (!S.tools.dnsInfo) loadDNSInfo(); loadNV(); loadOvStatus(); }
  if (page === 'health') loadHealth();
  if (page === 'startup') loadStartup();
  if (page === 'backup') loadBackups().then(render);
  if (page === 'settings') pollOverlay();
}

function closeSettings() {
  hideSettings();
  S.page = S.bgPage && S.bgPage !== 'settings' ? S.bgPage : 'home';
  render();
}

// ================================================================= data
async function loadStates(force) {
  setStates(await api('/api/states' + (force ? '?force=1' : '')));
}
async function loadBackups() {
  try {
    const b = await api('/api/backups');
    S.backups = { applied: b.applied || {}, history: b.history || [], restorePoints: b.restorePoints || [] };
  } catch (e) { /* ignore */ }
}
async function loadGames() {
  const d = await api('/api/games');
  S.games.list = d.games || []; S.games.popular = d.popular || []; S.games.loaded = true;
  if (S.page === 'games' || S.page === 'home') renderPage();
}
async function loadBoost() {
  try {
    const d = await api('/api/boost');
    S.boost.session = d.session; S.boost.bg = d.bg || [];
    if (!S.boost.selInit) { S.boost.bg.filter((b) => b.suggest).forEach((b) => S.boost.sel.add(b.exe)); S.boost.selInit = true; }
    if (!S.boost.temp) api('/api/boost/temp').then((t2) => { S.boost.temp = t2; if (S.page === 'boost') renderPage(); });
    if (S.page === 'boost') renderPage();
  } catch (e) { /* ignore */ }
}

// ================================================================= tweak actions
async function ensureRestorePoint() {
  if (S.rpAsked || !S.settings.autoRestore || !S.boot.admin) return true;
  S.rpAsked = true;
  const v = await modal({
    title: t('rp.ask.title'), body: `<p class="muted" style="margin:0">${t('rp.ask.body')}</p>`,
    actions: [{ label: t('rp.ask.no'), value: 'skip', kind: 'ghost' }, { label: `${icon('shield')}${t('rp.ask.yes')}`, value: 'yes', kind: 'primary' }],
  });
  if (v === null) { S.rpAsked = false; return false; }
  if (v === 'yes') await createRestorePoint();
  return true;
}

async function createRestorePoint() {
  if (!S.boot.admin) { toast(t('err.admin'), 'bad'); return; }
  const j = await runJob('rp');
  if (j.error) toast(t('rp.fail'), 'bad', j.error); else toast(t('rp.ok'), 'good');
  await loadBackups();
  render();
}

function noteRestart(ids) {
  for (const id of ids) {
    const tw = S.byId[id];
    if (!tw || !tw.restart) continue;
    if (tw.restart === 'pc') S.need.pc.add(id);
    if (tw.restart === 'signout') S.need.signout = true;
    if (tw.restart === 'explorer') S.need.explorer = true;
  }
}

async function applyIds(ids, on, { quiet = false } = {}) {
  if (!S.boot.admin) { toast(t('err.admin'), 'bad'); return null; }
  if (!ids.length) return null;
  if (on && !(await ensureRestorePoint())) return null;
  ids.forEach((id) => S.pending.add(id));
  render();
  let j;
  try {
    j = await runJob(on ? 'apply' : 'revert', { ids });
  } catch (e) {
    ids.forEach((id) => S.pending.delete(id));
    toast(t('err.generic'), 'bad', e.message);
    render();
    return null;
  }
  await loadStates(false).catch(() => {});
  ids.forEach((id) => S.pending.delete(id));
  const res = j.results || [];
  const ok = res.filter((r) => r.ok).map((r) => r.id);
  const bad = res.filter((r) => !r.ok);
  if (on) noteRestart(ok); else ok.forEach((id) => S.need.pc.delete(id));
  await loadBackups();
  if (!quiet) {
    const isApps = ok.length && ok.every((id) => ['apps', 'sponsored'].includes((S.byId[id] || {}).cat));
    if (ok.length && ids.length > 1) toast(t(isApps ? 'done.remove' : on ? 'done.apply' : 'done.revert', { n: ok.length }), 'good');
    if (bad.length) toast(t('done.failed', { n: bad.length }), 'bad', bad.map((b) => `${tx((S.byId[b.id] || {}).name) || b.id}: ${b.error}`).slice(0, 3).join('\n'));
  }
  render();
  return j;
}

async function toggleTweak(id, on) {
  const tw = S.byId[id];
  if (!tw) return;
  if (on && tw.risk === 'advanced') {
    const v = await modal({
      title: t('adv.title'),
      body: `<p class="muted" style="margin:0 0 12px">${t('adv.body')}</p><div class="card pad" style="background:var(--panel-2)"><b>${esc(tx(tw.name))}</b><p class="muted" style="margin:6px 0 0;font-size:13px">${esc(tx(tw.desc))}</p>
        ${tw.warn ? `<div class="note warn" style="margin-top:12px">${icon('alert')}<div>${esc(tx(tw.warn))}</div></div>` : ''}</div>`,
      actions: [{ label: t('btn.cancel'), value: null, kind: 'ghost' }, { label: t('adv.ok'), value: 'ok', kind: 'primary' }],
    });
    if (v !== 'ok') return;
  }
  await applyIds([id], on);
}

async function presetFlow(pid) {
  const p = S.boot.presets.find((x) => x.id === pid);
  const ids = presetIds(pid).filter((id) => !st(id).a && !st(id).u);
  if (!ids.length) { toast(t('pm.none'), 'info'); return; }
  const list = ids.map((id) => `<div>${icon(CAT_ICON[S.byId[id].cat])}<span>${esc(tx(S.byId[id].name))}</span></div>`).join('');
  const needRp = S.settings.autoRestore && !S.rpAsked && S.boot.admin;
  const v = await modal({
    title: t('pm.title', { name: tx(p.name) }),
    body: `<p class="muted" style="margin:0 0 12px">${t('pm.body', { n: ids.length })}</p><div class="mini-list">${list}</div>
      ${needRp ? `<label class="row" style="margin-top:14px;cursor:pointer" data-rp-toggle><button class="check on" data-rp="1">${icon('check')}</button><span>${t('pm.rp')}</span></label>` : ''}`,
    actions: [{ label: t('btn.cancel'), value: null, kind: 'ghost' }, { label: `${icon('rocket')}${t('btn.applyN', { n: ids.length })}`, value: 'go', kind: 'primary', collect: (l) => ({ rp: !!l.querySelector('[data-rp].on') }) }],
    onMount: (l) => { const lab = l.querySelector('[data-rp-toggle]'); if (lab) lab.onclick = (e) => { e.preventDefault(); lab.querySelector('.check').classList.toggle('on'); }; },
  });
  if (!v || v.value !== 'go') return;
  if (needRp) { S.rpAsked = true; if (v.extra && v.extra.rp) await createRestorePoint(); }
  await applyIds(ids, true);
  if (S.page === 'home') { animateGauge($('gauge'), S.score, 0); }
}

// ================================================================= boost & tools
async function runTool(k) {
  if (S.boost.busy[k]) return;
  S.boost.busy[k] = true; renderPage();
  try {
    if (k === 'ram') {
      const r = await api('/api/boost/ram', {});
      S.boost.results.ram = t('tool.ramR', { mb: fmtBytes(Math.max(0, r.after - r.before)) });
      toast(S.boost.results.ram, 'good');
    } else if (k === 'temp') {
      const r = await api('/api/boost/temp', {});
      S.boost.results.temp = t('tool.tempR', { size: fmtBytes(r.bytes) });
      S.boost.temp = null;
      api('/api/boost/temp').then((t2) => { S.boost.temp = t2; });
      toast(S.boost.results.temp, 'good');
    } else if (k === 'dns') {
      await api('/api/boost/dns', {});
      S.boost.results.dns = t('tool.dnsR'); toast(t('tool.dnsR'), 'good');
    } else if (k === 'shader') {
      const msg = await runShader();
      if (msg) { S.boost.results.shader = msg; toast(msg, 'good'); }
    } else if (k === 'explorer') {
      await api('/api/power', { action: 'explorer' });
      S.need.explorer = false;
      S.boost.results.explorer = t('tool.expR'); toast(t('tool.expR'), 'good');
    }
  } catch (e) { toast(t('err.generic'), 'bad', e.message); }
  S.boost.busy[k] = false;
  render();
}

async function boostToggle() {
  if (!S.boot.admin) { toast(t('err.admin'), 'bad'); return; }
  const on = !(S.boost.session && S.boost.session.active);
  S.boost.busy.session = true; renderPage();
  try {
    const d = await api('/api/boost/session', { on });
    S.boost.session = d.session;
    toast(on ? t('bo.since', { time: new Date().toLocaleTimeString(lang(), { hour: '2-digit', minute: '2-digit' }) }) : t('bo.stop'), on ? 'good' : 'info');
  } catch (e) { toast(t('err.generic'), 'bad', e.message); }
  S.boost.busy.session = false;
  render();
}

// ================================================================= games
async function gameUpdate(id, key, val) {
  if (!S.boot.admin) { toast(t('err.admin'), 'bad'); return; }
  S.pending.add(id + key); renderPage();
  try {
    const d = await api('/api/games/update', { id, [key]: val });
    S.games.list = d.games;
  } catch (e) { toast(t('err.generic'), 'bad', e.message); }
  S.pending.delete(id + key);
  renderPage();
}

async function gameAdd(g, prio = false) {
  try {
    const d = await api('/api/games/add', g);
    S.games.list = d.games;
    if (prio) {
      const added = d.games.find((x) => x.exe.toLowerCase() === g.exe.toLowerCase());
      if (added && !added.prio) await gameUpdate(added.id, 'prio', true);
    }
  } catch (e) { toast(t('err.generic'), 'bad', e.message); }
  renderPage();
}

// ================================================================= drawer
function openDrawer(id) {
  S.drawer = id;
  $('drawer').innerHTML = drawerHTML(id);
  $('drawer').classList.add('on'); $('drawer').setAttribute('aria-hidden', 'false');
  $('scrim').classList.add('on');
}
function closeDrawer() {
  S.drawer = null;
  $('drawer').classList.remove('on'); $('drawer').setAttribute('aria-hidden', 'true');
  $('scrim').classList.remove('on');
}

// ================================================================= language
async function changeLang(code) {
  setLang(code);
  S.settings.lang = code;
  render();
  try { await saveSettings(); } catch (e) { /* ignore */ }
}

// ================================================================= onboarding
function onboarding() {
  let step = 2; // language comes from Windows; it can be changed in Settings › Language
  let chosen = lang();
  let rp = !!S.boot.admin;
  const layer = $('modal');
  const draw = () => {
    const body = step === 1
      ? `<div class="modal-head"><img class="ob-logo" src="img/logo.svg" alt=""><div class="steps"><i class="on"></i><i></i></div>
          <p class="muted" style="margin:0 0 4px;font-size:12.5px">${t('ob.step', { n: 1 })}</p><h2>${t('ob.welcome')}</h2><p>${t('ob.lang')}</p></div>
        <div class="modal-body"><div class="langs" style="grid-template-columns:repeat(2,minmax(0,1fr))">${LANGS.map((l) => `<button class="lang ${chosen === l.code ? 'on' : ''}" data-ob-lang="${l.code}"><span class="code">${l.code.toUpperCase()}</span><span><b>${l.native}</b><span>${l.name}</span></span></button>`).join('')}</div></div>
        <div class="modal-foot"><button class="btn primary" data-ob-next>${t('ob.next')}</button></div>`
      : `<div class="modal-head"><img class="ob-logo" src="img/logo.svg" alt=""><h2>${t('ob.welcome')}</h2></div>
        <div class="modal-body"><p style="margin:0 0 8px;font-weight:600">${t('ob.safeT')}</p><p style="margin:0 0 10px;color:var(--text-2)">${t('ob.safe1')}</p><p class="muted" style="margin:0 0 16px">${t('ob.safe2')}</p>
          ${S.boot.admin ? `<button class="row" style="cursor:pointer;background:none;border:0;padding:0" data-ob-rp><span class="check ${rp ? 'on' : ''}">${icon('check')}</span><span>${t('ob.rp')}</span></button>` : ''}</div>
        <div class="modal-foot"><button class="btn primary" data-ob-start>${icon('rocket')}${t('ob.start')}</button></div>`;
    layer.innerHTML = `<div class="modal" role="dialog" aria-modal="true">${body}</div>`;
    layer.classList.add('on');
    const back = layer.querySelector('[data-ob-back] svg');
    if (back) back.style.transform = 'rotate(180deg)';
    (layer.querySelector('.btn.primary') || {}).focus?.();
  };
  layer.onclick = async (e) => {
    const l = e.target.closest('[data-ob-lang]');
    if (l) { chosen = l.dataset.obLang; setLang(chosen); render(); draw(); return; }
    if (e.target.closest('[data-ob-next]')) { step = 2; draw(); return; }
    if (e.target.closest('[data-ob-back]')) { step = 1; draw(); return; }
    if (e.target.closest('[data-ob-rp]')) { rp = !rp; draw(); return; }
    if (e.target.closest('[data-ob-start]')) {
      layer.classList.remove('on'); layer.innerHTML = ''; layer.onclick = null;
      S.settings.onboarded = true;
      S.settings.seenVersion = S.boot.version; // fresh install: nothing "new" to show
      S.settings.lang = chosen;
      try { await saveSettings(); } catch (err) { /* ignore */ }
      if (rp && S.boot.admin) { S.rpAsked = true; createRestorePoint(); }
      render();
    }
  };
  draw();
}

// ================================================================= overlay settings
function vkFromCode(code) {
  let m;
  if ((m = /^Key([A-Z])$/.exec(code))) return [m[1].charCodeAt(0), m[1]];
  if ((m = /^Digit([0-9])$/.exec(code))) return [0x30 + +m[1], m[1]];
  if ((m = /^F([0-9]{1,2})$/.exec(code)) && +m[1] >= 1 && +m[1] <= 24) return [0x6f + +m[1], 'F' + m[1]];
  if ((m = /^Numpad([0-9])$/.exec(code))) return [0x60 + +m[1], 'Num ' + m[1]];
  const T = {
    NumpadMultiply: [0x6a, 'Num *'], NumpadAdd: [0x6b, 'Num +'], NumpadSubtract: [0x6d, 'Num -'], NumpadDecimal: [0x6e, 'Num .'], NumpadDivide: [0x6f, 'Num /'],
    Insert: [0x2d, 'Insert'], Delete: [0x2e, 'Delete'], Home: [0x24, 'Home'], End: [0x23, 'End'], PageUp: [0x21, 'PgUp'], PageDown: [0x22, 'PgDn'],
    Backquote: [0xc0, '`'], Minus: [0xbd, '-'], Equal: [0xbb, '='], BracketLeft: [0xdb, '['], BracketRight: [0xdd, ']'], Backslash: [0xdc, '\\'],
    Semicolon: [0xba, ';'], Quote: [0xde, "'"], Comma: [0xbc, ','], Period: [0xbe, '.'], Slash: [0xbf, '/'], Space: [0x20, 'Space'],
    Pause: [0x13, 'Pause'], ScrollLock: [0x91, 'ScrLk'], ArrowUp: [0x26, '↑'], ArrowDown: [0x28, '↓'], ArrowLeft: [0x25, '←'], ArrowRight: [0x27, '→'],
  };
  return T[code] || null;
}
const SOLO_OK = new Set([0x13, 0x91, 0x2d, 0x24, 0x23, 0x21, 0x22]);

async function saveOverlay(patch, quiet) {
  const before = S.settings.overlay || {};
  S.settings.overlay = { ...before, ...patch };
  renderPage();
  try {
    const r = await saveSettings();
    if (r.warn) toast(t('err.generic'), 'bad', r.warn);
  } catch (e) { toast(t('err.generic'), 'bad', e.message); }
  S.ovV = (S.ovV || 0) + 1;
  refreshOvPreview();
  if (!quiet && patch.enabled === true) toast(t('ov.onToast', { keys: hotkeyLabel(S.settings.overlay).join(' + ') }), 'good');
  setTimeout(pollOverlay, 1500);
}

function refreshOvPreview() {
  const img = $('ov-img');
  if (!img) return;
  const next = new Image();
  next.onload = () => { const cur = $('ov-img'); if (cur) cur.src = next.src; };
  next.src = `/api/overlay/preview.png?t=${encodeURIComponent(TOKEN)}&v=${S.ovV || 0}&r=${Date.now()}`;
}

async function pollOverlay() {
  try {
    const prev = JSON.stringify(S.ovStatus || {});
    S.ovStatus = await api('/api/overlay');
    if (S.page !== 'settings') return;
    if (JSON.stringify(S.ovStatus) !== prev) {
      const focused = document.activeElement && document.activeElement.dataset && document.activeElement.dataset.ovRange;
      if (!focused) renderPage();
    }
  } catch (e) { /* ignore */ }
}
setInterval(() => { if (S.page === 'settings') { pollOverlay(); refreshOvPreview(); } }, 2500);

document.addEventListener('keydown', (e) => {
  if (!S.ovRec) return;
  e.preventDefault();
  e.stopPropagation();
  if (e.key === 'Escape') { S.ovRec = false; S.ovRecErr = ''; renderPage(); return; }
  if (['Control', 'Alt', 'Shift', 'Meta', 'OS'].includes(e.key)) return;
  const k = vkFromCode(e.code);
  if (!k) { S.ovRecErr = t('ov.badKey'); renderPage(); return; }
  const mods = [];
  if (e.ctrlKey) mods.push('ctrl');
  if (e.altKey) mods.push('alt');
  if (e.shiftKey) mods.push('shift');
  if (e.metaKey) mods.push('win');
  const solo = (k[0] >= 0x70 && k[0] <= 0x87) || (k[0] >= 0x60 && k[0] <= 0x6f) || SOLO_OK.has(k[0]);
  if (!mods.length && !solo) { S.ovRecErr = t('ov.needMod'); renderPage(); return; }
  S.ovRec = false; S.ovRecErr = '';
  saveOverlay({ mods, key: k[1], vk: k[0] }, true);
}, true);

document.addEventListener('input', (e) => {
  const k = e.target.dataset && e.target.dataset.ovRange;
  if (k) { const v = $('ov-opv'); if (v) v.textContent = e.target.value + '%'; }
});
document.addEventListener('change', (e) => {
  const k = e.target.dataset && e.target.dataset.ovRange;
  if (k) saveOverlay({ [k]: +e.target.value }, true);
});

// ================================================================= events
const ACTIONS = {
  ...ACCOUNT_ACTIONS,
  ...TOOL_ACTIONS,
  ...V107_ACTIONS,
  ...V109_ACTIONS,
  ...SW_ACTIONS,
  'ov-flag-inv': (d, el) => saveOverlay({ [d.k]: el.classList.contains('on') }),
  theme: async (d) => {
    S.settings.theme = d.v;
    applyTheme(d.v);
    S.gaugeShown = false;
    render();
    try { await saveSettings(); } catch (e) { /* ignore */ }
    S.ovV = (S.ovV || 0) + 1; // the overlay preview follows the theme's colors
    refreshOvPreview();
  },
  'acct-open': () => { go('settings'); setTimeout(() => { const el = $('acct-sec'); if (el) el.scrollIntoView({ block: 'start' }); }, 0); },
  go: (d) => go(d.page, d.cat),
  'sw-close': () => closeSettings(),
  toggle: (d, el) => toggleTweak(d.id, !el.classList.contains('on')),
  // Components & services: the switch shows whether the Windows part is enabled.
  'toggle-inv': (d, el) => toggleTweak(d.id, el.classList.contains('on')),
  'ov-flag': (d, el) => saveOverlay({ [d.k]: !el.classList.contains('on') }),
  'ov-set': (d) => saveOverlay({ [d.k]: d.k === 'interval' ? +d.v : d.v }, true),
  'ov-metric': (d) => {
    const cur = new Set((S.settings.overlay || {}).metrics || []);
    cur.has(d.v) ? cur.delete(d.v) : cur.add(d.v);
    saveOverlay({ metrics: OV_METRICS.filter((m) => cur.has(m)) }, true);
  },
  'ov-rec': () => { S.ovRec = !S.ovRec; S.ovRecErr = ''; renderPage(); },
  'ov-vis': async (d) => {
    try { await api('/api/overlay/visible', { visible: d.v === '1' }); } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    S.ovStatus = { ...(S.ovStatus || {}), visible: d.v === '1' };
    renderPage();
    setTimeout(pollOverlay, 1200);
  },
  set: (d) => applyIds([d.id], d.on === '1'),
  drawer: (d) => openDrawer(d.id),
  'drawer-close': () => closeDrawer(),
  save: async (d) => {
    const on = !S.saved.has(d.id);
    on ? S.saved.add(d.id) : S.saved.delete(d.id);
    renderPage();
    try { await api('/api/saved', { id: d.id, on }); } catch (e) { /* ignore */ }
  },
  'opt-tab': (d) => { S.opt.tab = d.v; renderPage(); },
  'opt-cat': (d) => { S.opt.cat = d.v; renderPage(); },
  preset: (d) => presetFlow(d.id),
  rp: () => createRestorePoint(),
  tool: (d) => runTool(d.tool),
  'boost-toggle': () => boostToggle(),
  'bg-sel': (d) => { S.boost.sel.has(d.exe) ? S.boost.sel.delete(d.exe) : S.boost.sel.add(d.exe); renderPage(); },
  'bg-refresh': () => loadBoost(),
  'bg-close': async () => {
    const exes = [...S.boost.sel].filter((e) => S.boost.bg.some((b) => b.exe === e));
    try { const r = await api('/api/boost/kill', { exes }); S.boost.bg = r.bg || []; toast(t('bg.closed', { n: r.closed }), 'good'); } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    renderPage();
  },
  'deb-scan': async () => {
    S.deb.scanning = true; renderPage();
    const j = await runJob('scan');
    S.deb.scanning = false;
    if (j.error) toast(t('err.generic'), 'bad', j.error); else S.deb.scanned = true;
    await loadStates(false);
    render();
  },
  'deb-tab': (d) => { S.deb.tab = d.v; renderPage(); },
  'deb-sel': (d) => { S.deb.sel.has(d.id) ? S.deb.sel.delete(d.id) : S.deb.sel.add(d.id); renderPage(); },
  'deb-selrec': () => { S.tweaks.filter((tw) => tw.cat === S.deb.tab && tw.rec && !st(tw.id).a).forEach((tw) => S.deb.sel.add(tw.id)); renderPage(); },
  'deb-selnone': () => { S.deb.sel.clear(); renderPage(); },
  'deb-offrec': (d) => applyIds(d.ids.split(',').filter(Boolean), true),
  'deb-gone': () => { S.deb.showGone = !S.deb.showGone; renderPage(); },
  'deb-reinstall': (d) => api('/api/revert', { ids: [d.id] }).catch(() => {}),
  'deb-remove': async () => {
    const ids = [...S.deb.sel].filter((id) => S.byId[id] && ['apps', 'sponsored'].includes(S.byId[id].cat) && !st(id).a);
    if (!ids.length) return;
    const v = await modal({
      title: t('d.rmT', { n: ids.length }),
      body: `<p class="muted" style="margin:0 0 12px">${t('d.rmB')}</p><div class="mini-list">${ids.map((id) => `<div>${icon('package')}${esc(tx(S.byId[id].name))}</div>`).join('')}</div>`,
      actions: [{ label: t('btn.cancel'), value: null, kind: 'ghost' }, { label: `${icon('trash')}${t('d.remove', { n: ids.length })}`, value: 'go', kind: 'primary' }],
    });
    if (v !== 'go') return;
    const j = await applyIds(ids, true);
    if (j) ids.forEach((id) => S.deb.sel.delete(id));
    renderPage();
  },
  'game-scan': async () => {
    S.games.scanning = true; renderPage();
    try { const d = await api('/api/games/detect', {}); S.games.found = d.found || []; } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    S.games.scanning = false; renderPage();
  },
  'game-add': (d) => gameAdd(S.games.found[+d.i]),
  'game-pop': (d) => gameAdd({ name: d.name, exe: d.exe, source: 'preset' }, true),
  'game-browse': async () => {
    try { const d = await api('/api/games/browse', {}); if (d.path) await gameAdd({ name: d.name, exe: d.exe, path: d.path, source: 'manual' }); }
    catch (e) { if (!/cancel/i.test(e.message)) toast(t('err.generic'), 'bad', e.message); }
  },
  'game-flag': (d, el) => gameUpdate(d.id, d.k, !el.classList.contains('on')),
  'game-remove': async (d) => {
    try { const r = await api('/api/games/remove', { id: d.id }); S.games.list = r.games; } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    renderPage();
  },
  'sys-rescan': async () => {
    S.info = null; renderPage(); renderTop();
    S.info = await api('/api/system?rescan=1');
    await loadStates(true); render();
  },
  'sys-copy': async () => {
    try { await navigator.clipboard.writeText(specsText()); toast(t('s.copied'), 'good'); } catch (e) { toast(t('err.generic'), 'bad', e.message); }
  },
  'undo-all': async () => {
    const n = Object.keys(S.backups.applied).length;
    const v = await modal({ title: t('bk.uaT', { n }), body: `<p class="muted" style="margin:0">${t('bk.uaB')}</p>`,
      actions: [{ label: t('btn.cancel'), value: null, kind: 'ghost' }, { label: `${icon('restore')}${t('bk.undoAll')}`, value: 'go', kind: 'primary' }] });
    if (v === 'go') await applyIds(Object.keys(S.backups.applied), false);
  },
  lang: (d) => { document.querySelectorAll('.menu-pop').forEach((m) => m.remove()); changeLang(d.v); },
  setting: async (d, el) => {
    const on = !el.classList.contains('on');
    S.settings[d.k] = on;
    if (d.k === 'motion') $('app').classList.toggle('reduced', !on);
    renderPage();
    try { await saveSettings(); } catch (e) { /* ignore */ }
  },
  install: async () => {
    try { S.boot.install = await api('/api/install', {}); toast(t('set.installOk'), 'good'); } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    renderPage();
  },
  open: (d) => api('/api/open', { what: d.what }).catch(() => {}),
  power: async (d) => {
    if (d.v === 'explorer') { await runTool('explorer'); return; }
    try { await api('/api/power', { action: d.v }); } catch (e) { toast(t('err.generic'), 'bad', e.message); }
  },
  'need-dismiss': (d) => { if (d.k === 'all') { S.need.pc.clear(); S.need.signout = false; S.need.explorer = false; } else S.need[d.k] = false; renderBanners(); },
  elevate: async () => {
    try { const r = await api('/api/elevate', {}); if (r.url) location.href = r.url; } catch (e) { toast(t('err.generic'), 'bad', e.message); }
  },
};

document.addEventListener('click', (e) => {
  const el = e.target.closest('[data-a]');
  if (!el || el.disabled) return;
  const fn = ACTIONS[el.dataset.a];
  if (fn) { e.preventDefault(); fn(el.dataset, el); }
});
$('scrim').addEventListener('click', closeDrawer);
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') { if (modalOpen()) closeModal(null); else if (S.drawer) closeDrawer(); else if (S.page === 'settings' && !S.ovRec) closeSettings(); }
  if (e.key === '/' && !S.ovRec && document.activeElement.tagName !== 'INPUT') { e.preventDefault(); if (S.page !== 'optimize') go('optimize'); setTimeout(() => $('q') && $('q').focus(), 0); }
});

// ================================================================= live events
function connectEvents() {
  const es = new EventSource('/api/events?t=' + encodeURIComponent(TOKEN));
  es.addEventListener('live', (e) => {
    const d = JSON.parse(e.data);
    S.live.last = d;
    S.live.cpu.push(d.cpu); S.live.gpu.push(Math.max(0, d.gpu)); S.live.ram.push(d.ramTotal ? (d.ramUsed / d.ramTotal) * 100 : 0);
    for (const k of ['cpu', 'gpu', 'ram']) if (S.live[k].length > 60) S.live[k].shift();
    if (S.boot && S.page === 'home') updateLive();
  });
  es.addEventListener('job', (e) => { handleJobEvent(JSON.parse(e.data)); if (S.boot) renderTop(); });
  es.addEventListener('auth', (e) => {
    const a = JSON.parse(e.data);
    if (!S.boot || authVisible()) return;
    S.acct = a;
    if (a.exists && a.locked) showLocked(); else render();
  });
  es.addEventListener('states', (e) => {
    if (!S.boot) return;
    setStates(JSON.parse(e.data));
    if (S.page !== 'home' || S.gaugeShown) { renderPage(); renderTop(); }
  });
  window.addEventListener('pagehide', () => { try { navigator.sendBeacon('/api/bye?t=' + encodeURIComponent(TOKEN)); } catch (e) { /* ignore */ } });
}

// ================================================================= boot
function hideSplash() {
  const sp = $('splash');
  if (!sp || sp.classList.contains('gone')) return;
  sp.classList.add('gone');
  setTimeout(() => sp.remove(), 400);
}

window.addEventListener('larik-locked', () => { if (S.boot) showLocked(); });

async function boot() {
  const svg = `<svg width="0" height="0" style="position:absolute"><defs>
    <linearGradient id="ringGrad" x1="0" y1="0" x2="1" y2="1"><stop offset="0" style="stop-color:rgb(var(--a1-rgb))"/><stop offset=".6" style="stop-color:rgb(var(--a3-rgb))"/><stop offset="1" style="stop-color:rgb(var(--a4-rgb))"/></linearGradient>
    <linearGradient id="sparkFill" x1="0" y1="0" x2="0" y2="1"><stop offset="0" style="stop-color:var(--violet-2);stop-opacity:.35"/><stop offset="1" style="stop-color:var(--violet-2);stop-opacity:0"/></linearGradient></defs></svg>`;
  document.body.insertAdjacentHTML('afterbegin', svg);
  const fail = (e) => {
    const sp = $('splash');
    if (sp) sp.innerHTML = `<img src="img/logo.svg" width="72" height="72" alt=""><div class="splash-name">OpLarik</div><p class="muted">${esc(e.message)}</p>`;
  };
  // Account gate: nothing about the PC loads until the user has signed in.
  let ai;
  try { ai = await api('/api/auth'); } catch (e) { fail(e); return; }
  setLang(ai.lang || ai.sysLang || 'en');
  applyTheme(ai.theme);
  connectEvents(); // keeps the engine alive while the sign-in screen is open
  const a0 = ai.auth || {};
  const needLogin = a0.exists && a0.locked;
  const offerSetup = !a0.exists && !ai.asked && ai.admin;
  if (needLogin || offerSetup) {
    hideSplash();
    const r = await authScreen(needLogin ? 'login' : 'setup', { canSkip: offerSetup });
    if (needLogin && r !== 'unlocked') return;
  }
  try {
    S.boot = await api('/api/bootstrap');
    S.acct = (await api('/api/auth')).auth;
  } catch (e) {
    fail(e);
    return;
  }
  S.tok = TOKEN;
  S.tweaks = S.boot.tweaks;
  S.tweaks.forEach((tw) => { S.byId[tw.id] = tw; });
  S.saved = new Set(S.boot.saved || []);
  S.settings = S.boot.settings || {};
  setLang(S.settings.lang || S.boot.sysLang || 'en');
  // animations are always on (1.1.1)
  initAccount({ render, renderPage });
  initTools({ render, renderPage, applyIds });
  initV107({ render, renderPage, applyIds, saveOverlay, ovMetrics: OV_METRICS });
  initV109({ render, renderPage });
  await Promise.all([loadStates(false), loadBackups(), loadGames().catch(() => {}), loadSessionsQuiet()]);
  render();
  $('app').classList.remove('booting');
  hideSplash();
  api('/api/boost').then((d) => { S.boost.session = d.session; S.boost.bg = d.bg || []; if (S.page === 'home') renderPage(); }).catch(() => {});
  api('/api/system').then(async (info) => {
    S.info = info;
    await loadStates(false);
    render();
  });
  api('/api/debloat').then((d) => { if (d.scanned) { S.deb.scanned = true; renderPage(); } }).catch(() => {});
  loadDisks(false); // disk warnings show as a dot on the Health page
  autoUpdateCheck();
  if (!S.settings.onboarded) onboarding();
  else if (S.settings.seenVersion !== S.boot.version) setTimeout(showChangelog, 900); // first start after an update
}

onJob(() => {});
boot();
