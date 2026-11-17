// OpLarik 1.0.5 screens: shader cache + fastest DNS (Boost page), startup apps
// page and game sessions (Games page).
import { t, lang, tx } from './i18n.js';
import { S, api, esc, icon, toast, toggle, modal, fmtBytes, nf, cover, coverHTML, coverForExe, st, saveSettings } from './core.js';
import { CHANGELOG } from './changelog.js';

let hooks = { render: () => {}, renderPage: () => {}, applyIds: async () => null };
export function initTools(h) { hooks = { ...hooks, ...h }; }

S.tools = { shader: null, dns: null, dnsInfo: null, dnsBusy: false, startup: null, suBusy: {}, sessions: null, recording: true };

const errMsg = (e) => {
  const k = 'tl.e.' + ((e && e.message) || '');
  const s = t(k);
  return s === k ? (e && e.message) || t('err.generic') : s;
};

// ================================================================= loaders
export async function loadShader() {
  try { S.tools.shader = await api('/api/tools/shader'); } catch (e) { /* ignore */ }
  if (S.page === 'boost') hooks.renderPage();
}
export async function loadDNSInfo() {
  try { S.tools.dnsInfo = await api('/api/tools/dns'); } catch (e) { /* ignore */ }
  if (S.page === 'boost') hooks.renderPage();
}
export async function loadStartup() {
  try { S.tools.startup = (await api('/api/startup')).items; } catch (e) { S.tools.startup = []; toast(t('err.generic'), 'bad', e.message); }
  if (S.page === 'startup') hooks.renderPage();
}
export async function loadSessions() {
  try { const r = await api('/api/sessions'); S.tools.sessions = r.sessions; S.tools.recording = r.recording; } catch (e) { S.tools.sessions = []; }
  if (S.page === 'games') hooks.renderPage();
}

// ================================================================= Boost page parts
export function shaderTool() {
  const sh = S.tools.shader;
  return { k: 'shader', ic: 'layers', t: t('tool.shader'), d: t('tool.shaderD'),
    extra: sh ? (sh.bytes > 0 ? t('tool.shaderS', { size: fmtBytes(sh.bytes) }) : t('tool.shaderEmpty')) : '' };
}

export async function runShader() {
  const sh = S.tools.shader;
  const v = await modal({
    title: t('tool.shaderQ'), body: `<p class="muted" style="margin:0 0 10px">${t('tool.shaderQD')}</p>
      ${sh && sh.dirs && sh.dirs.length ? `<div class="mini-list">${sh.dirs.filter((d) => d.bytes > 0).map((d) => `<div>${icon('layers')}${esc(d.name)}<span class="spacer"></span><span class="muted num">${fmtBytes(d.bytes)}</span></div>`).join('')}</div>` : ''}`,
    actions: [{ label: t('btn.cancel'), value: null, kind: 'ghost' }, { label: `${icon('trash')}${t('tool.shaderGo')}`, value: 'go', kind: 'primary' }],
  });
  if (v !== 'go') return null;
  const r = await api('/api/tools/shader', {});
  S.tools.shader = { dirs: r.dirs, bytes: r.left, files: 0 };
  return r.left > 0 ? t('tool.shaderRLeft', { size: fmtBytes(r.bytes), left: fmtBytes(r.left) }) : t('tool.shaderR', { size: fmtBytes(r.bytes) });
}

export function dnsSection() {
  const D = S.tools.dns;
  const info = S.tools.dnsInfo || {};
  const busy = S.tools.dnsBusy;
  const cur = (D ? D.current : info.current) || [];
  const backup = D ? D.backup : info.backup;
  let body;
  if (!D) {
    body = `<div class="dns-empty">${icon('globe')}<div><b>${t('dns.cur')}</b> <span class="mono">${cur.length ? esc(cur.join(', ')) : '—'}</span></div></div>`;
  } else {
    const max = Math.max(1, ...D.results.filter((r) => r.ms > 0).map((r) => r.ms));
    const rows = [...D.results].sort((a, b) => (a.ms < 0) - (b.ms < 0) || a.ms - b.ms);
    body = `<div class="dns-list">${rows.map((r) => {
      const name = r.id === 'current' ? t('dns.yours') : r.name;
      const best = r.id === D.best;
      const isCur = r.current || r.id === 'current';
      const pct = r.ms > 0 ? Math.max(4, (r.ms / max) * 100) : 0;
      return `<div class="dns-row ${best ? 'best' : ''}">
        <div class="dns-name"><b>${esc(name)}</b><span class="mono">${esc([r.primary, r.secondary].filter(Boolean).join(' · '))}</span></div>
        <div class="dns-bar"><i style="width:${pct}%"></i></div>
        <div class="dns-ms num">${r.ms > 0 ? `${nf(r.ms, r.ms < 10 ? 1 : 0)} ${t('u.ms')}` : t('dns.noAnswer')}${r.fails && r.ms > 0 ? `<span class="tag warn">${t('dns.lost', { n: r.fails })}</span>` : ''}</div>
        <div class="dns-act">${best ? `<span class="tag good">${icon('zap')}${t('dns.best')}</span>` : ''}${isCur ? `<span class="tag gray">${t('dns.now')}</span>` : ''}
          ${r.id !== 'current' && !isCur && r.ms > 0 ? `<button class="btn sm ${best ? 'primary' : ''}" data-a="dns-apply" data-id="${r.id}" ${busy || !S.boot.admin ? 'disabled' : ''}>${t('dns.apply')}</button>` : ''}</div>
      </div>`;
    }).join('')}</div>`;
  }
  return `<section class="sec"><div class="sec-head"><div><h2>${t('dns.title')}</h2><p>${t('dns.sub')}</p></div>
      <div class="right">${backup ? `<button class="btn sm ghost" data-a="dns-restore" ${busy || !S.boot.admin ? 'disabled' : ''}>${icon('restore')}${t('dns.restore')}</button>` : ''}
      <button class="btn sm" data-a="dns-bench" ${busy ? 'disabled' : ''}>${busy ? icon('loader', 'spin') : icon('gauge')}${D ? t('dns.again') : t('dns.test')}</button></div></div>
    <div class="card pad dns-card">${body}<p class="muted dns-note">${icon('info')}${t('dns.note')}</p></div></section>`;
}

// ================================================================= Startup page
const KEEP = [/^securityhealth/i, /^windowsdefender/i, /^rtk/i, /realtek/i, /^nv/i, /nvidia/i, /^amd/i, /radeon/i, /^igfx/i, /^intel/i, /synaptics/i, /^elan/i, /bluetooth/i];

export function startupPage() {
  const list = S.tools.startup;
  if (!list) return `<div class="empty">${icon('loader', 'spin')}<div>${t('home.scanning')}</div></div>`;
  const on = list.filter((x) => x.on).length;
  const src = (s) => t('su.src.' + s);
  return `<section><div class="sec-head"><div><h2>${t('su.count', { on, n: list.length })}</h2><p>${t('su.sub')}</p></div>
      <div class="right"><button class="iconbtn" data-a="su-refresh" title="${t('d.rescan')}">${icon('refresh')}</button></div></div>
    <div class="card">${list.length ? `<div class="list">${list.map((it) => {
      const keep = KEEP.some((re) => re.test(it.name));
      return `<div class="li su-row ${it.on ? '' : 'off'}">${coverHTML(it.name, it.cover, 'sm')}
        <div><h4>${esc(it.name)}<span class="tag gray">${src(it.source)}</span>${keep ? `<span class="tag info">${t('su.keep')}</span>` : ''}</h4>
          <p class="su-cmd" title="${esc(it.command)}">${esc(it.command)}</p></div>
        <div class="actions"><span class="tg-wrap ${it.on ? 'on' : ''}">${it.on ? t('su.on') : t('su.off')}${toggle(it.on, `data-a="su-toggle" data-id="${esc(it.id)}" aria-label="${esc(it.name)}"`, { busy: !!S.tools.suBusy[it.id], disabled: !S.boot.admin })}</span></div></div>`;
    }).join('')}</div>` : `<div class="empty" style="border:0">${icon('ok')}<div>${t('su.empty')}</div></div>`}</div></section>
    <section class="sec"><div class="note">${icon('info')}<div>${t('su.note')}</div></div></section>`;
}

// ================================================================= Sessions (Games page)
function dur(sec) {
  const h = Math.floor(sec / 3600), m = Math.max(1, Math.round((sec % 3600) / 60));
  return h ? t('ses.durH', { h, m }) : t('ses.durM', { m });
}

export function sessionsSection() {
  const list = S.tools.sessions;
  if (!list) return '';
  const stat = (label, v, unit = '', cls = '') => `<div class="ses-stat ${cls}"><span>${label}</span><b class="num">${v}${unit ? `<small>${unit}</small>` : ''}</b></div>`;
  const tmp = (v) => (v > 0 ? nf(v, 0) : '—');
  const card = (s) => {
    const d = new Date(s.start);
    const when = d.toLocaleString(lang(), { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
    return `<div class="card ses">
      <div class="ses-head">${coverHTML(s.game, coverForExe(s.exe))}
        <div class="grow"><h4>${esc(s.game)}</h4><p>${esc(when)} · ${dur(s.duration)}</p></div>
        ${s.warnings > 0 ? `<span class="tag bad">${icon('thermo')}${t('ses.warn', { n: s.warnings })}</span>` : ''}</div>
      <div class="ses-stats">${stat(t('ses.avg'), nf(s.avgFps, 0), ' FPS', 'main')}${stat('1% low', nf(s.low1, 0))}${stat(t('ses.min'), nf(s.minFps, 0))}
        ${stat(t('ses.gpu'), tmp(s.maxGpuTemp), s.maxGpuTemp > 0 ? '°C' : '', s.maxGpuTemp >= 85 ? 'hot' : '')}${stat(t('ses.cpu'), tmp(s.maxCpuTemp), s.maxCpuTemp > 0 ? '°C' : '', s.maxCpuTemp >= 90 ? 'hot' : '')}
        ${stat(t('ses.ping'), s.avgPing >= 0 ? nf(s.avgPing, 0) : '—', s.avgPing >= 0 ? ` ${t('u.ms')}` : '')}</div>
      <div class="ses-foot"><button class="btn sm" data-a="ses-open" data-id="${esc(s.id)}">${icon('file')}${t('ses.csv')}</button>
        <button class="btn sm ghost" data-a="ses-del" data-id="${esc(s.id)}">${icon('trash')}${t('ses.del')}</button></div></div>`;
  };
  return `<section class="sec"><div class="sec-head"><div><h2>${t('ses.title')}</h2><p>${t('ses.sub')}</p></div>
      <div class="right">${list.length >= 2 && !S.v7.cmpOpen ? `<button class="btn sm primary" data-a="cmp-open">${icon('compare')}${t('cmp.open')}</button>` : ''}<button class="btn sm ghost" data-a="ses-folder">${icon('folder')}${t('ses.folder')}</button></div></div>
    ${!S.tools.recording ? `<div class="banner" style="margin-bottom:12px">${icon('info')}<div class="grow">${t('ses.off')}</div><button class="btn sm" data-a="go" data-page="settings" data-cat="overlay">${icon('settings')}${t('nav.settings')}</button></div>` : ''}
    ${list.length ? `<div class="grid g2">${list.slice(0, 8).map(card).join('')}</div>` : `<div class="card"><div class="empty" style="border:0">${icon('activity')}<div>${t('ses.empty')}</div></div></div>`}</section>`;
}

// ================================================================= low input lag mode
// One switch for the input-related optimizations. It remembers which ones it
// turned on, so switching it off never undoes something you enabled yourself.
export const INPUT_MODE = ['input_latency', 'mouse_accel_off', 'keyboard_fast', 'sticky_keys_off', 'usb_suspend_off'];

export function inputModeOn() { return !!st('input_latency').a; }

export function inputModeCard() {
  const on = inputModeOn();
  const busy = INPUT_MODE.some((id) => S.pending.has(id));
  const members = INPUT_MODE.filter((id) => S.byId[id] && !st(id).na);
  return `<section class="sec"><div class="card input-mode ${on ? 'on' : ''}">
    <div class="im-head"><div class="im-ic">${icon('mouse')}</div>
      <div class="grow"><h3>${t('im.title')}${on ? `<span class="tag good">${icon('check')}${t('im.on')}</span>` : ''}</h3><p>${t('im.sub')}</p></div>
      ${toggle(on, 'data-a="input-mode" aria-label="' + esc(t('im.title')) + '"', { busy, disabled: !S.boot.admin })}</div>
    <div class="im-list">${members.map((id) => `<div class="im-item ${st(id).a ? 'done' : ''}"><span class="check ${st(id).a ? 'on' : ''}">${icon('check')}</span>${esc(tx(S.byId[id].name))}</div>`).join('')}</div>
    <p class="muted im-note">${icon('info')}${t('im.note')}</p></div></section>`;
}

async function toggleInputMode() {
  if (inputModeOn()) {
    const mine = new Set([...(S.settings.inputMode || []), 'input_latency']);
    const ids = [...mine].filter((id) => st(id).a);
    const j = await hooks.applyIds(ids, false, { quiet: true });
    if (!j) return;
    S.settings.inputMode = [];
    try { await saveSettings(); } catch (e) { /* ignore */ }
    toast(t('im.offToast'), 'good');
  } else {
    const ids = INPUT_MODE.filter((id) => S.byId[id] && !st(id).a && !st(id).na);
    const j = await hooks.applyIds(ids, true, { quiet: true });
    if (!j) return;
    const ok = (j.results || []).filter((r) => r.ok).map((r) => r.id);
    S.settings.inputMode = ok;
    try { await saveSettings(); } catch (e) { /* ignore */ }
    if (ok.length) toast(t('im.onToast'), 'good', t('im.restart'));
  }
  hooks.render();
}

// ================================================================= what's new
export function showChangelog() {
  const L = lang();
  const body = `<div class="cl">${CHANGELOG.map((r, i) => `<div class="cl-ver ${i === 0 ? 'cur' : ''}">
      <div class="cl-head"><b>v${r.v}</b>${r.v === S.boot.version ? `<span class="tag good">${t('cl.current')}</span>` : ''}<span class="spacer"></span><span class="muted">${new Date(r.date).toLocaleDateString(L, { day: 'numeric', month: 'long', year: 'numeric' })}</span></div>
      <ul>${(r[L] || r.en).map((x) => `<li>${esc(x)}</li>`).join('')}</ul></div>`).join('')}</div>`;
  if (S.settings.seenVersion !== S.boot.version) {
    S.settings.seenVersion = S.boot.version;
    saveSettings().catch(() => {});
    hooks.render();
  }
  return modal({ title: `${icon('sparkles')}${t('cl.title')}`, body, wide: true, actions: [{ label: t('cl.ok'), value: 'ok', kind: 'primary' }] });
}

// ================================================================= actions
export const TOOL_ACTIONS = {
  'input-mode': () => toggleInputMode(),
  changelog: () => showChangelog(),
  'dns-bench': async () => {
    S.tools.dnsBusy = true; hooks.renderPage();
    try { S.tools.dns = await api('/api/tools/dns/bench', {}); } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    S.tools.dnsBusy = false; hooks.renderPage();
  },
  'dns-apply': async (d) => {
    S.tools.dnsBusy = true; hooks.renderPage();
    try {
      const r = await api('/api/tools/dns/apply', { id: d.id });
      const name = (S.tools.dns && S.tools.dns.results.find((x) => x.id === d.id) || {}).name || d.id;
      if (S.tools.dns) {
        S.tools.dns.current = r.current; S.tools.dns.backup = r.backup;
        S.tools.dns.results.forEach((x) => { x.current = x.id !== 'current' && r.current.includes(x.primary); });
      }
      S.tools.dnsInfo = r;
      toast(t('dns.applied', { name }), 'good');
    } catch (e) { toast(errMsg(e), 'bad'); }
    S.tools.dnsBusy = false; hooks.renderPage();
  },
  'dns-restore': async () => {
    S.tools.dnsBusy = true; hooks.renderPage();
    try {
      const r = await api('/api/tools/dns/restore', {});
      S.tools.dnsInfo = r;
      if (S.tools.dns) { S.tools.dns.current = r.current; S.tools.dns.backup = r.backup; S.tools.dns.results.forEach((x) => { x.current = x.id === 'current' || r.current.includes(x.primary); }); }
      toast(t('dns.restored'), 'good');
    } catch (e) { toast(errMsg(e), 'bad'); }
    S.tools.dnsBusy = false; hooks.renderPage();
  },
  'su-refresh': () => { S.tools.startup = null; hooks.renderPage(); loadStartup(); },
  'su-toggle': async (d, el) => {
    const on = !el.classList.contains('on');
    S.tools.suBusy[d.id] = true; hooks.renderPage();
    try { S.tools.startup = (await api('/api/startup', { id: d.id, on })).items; } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    delete S.tools.suBusy[d.id]; hooks.renderPage();
  },
  'ses-open': (d) => api('/api/open', { what: 'session', id: d.id }).catch(() => {}),
  'ses-folder': () => api('/api/open', { what: 'sessions' }).catch(() => {}),
  'ses-del': async (d) => {
    try { S.tools.sessions = (await api('/api/sessions/delete', { id: d.id })).sessions; } catch (e) { toast(t('err.generic'), 'bad', e.message); }
    hooks.renderPage();
  },
};
