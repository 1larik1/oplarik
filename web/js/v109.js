// OpLarik 1.0.9: start games from OpLarik (Games page and Home).
import { t, lang } from './i18n.js';
import { S, api, esc, icon, toast, modal, coverHTML } from './core.js';

let hooks = { render: () => {}, renderPage: () => {} };
export function initV109(h) { hooks = { ...hooks, ...h }; }

const busy = new Set();
let hinted = false;

// Last time a game ran: started from OpLarik or recorded as a session.
export function lastPlayed(g) {
  let last = g.lastLaunch ? Date.parse(g.lastLaunch) : 0;
  const exe = (g.exe || '').toLowerCase();
  for (const s of S.tools.sessions || []) {
    if ((s.exe || '').toLowerCase() === exe || (!s.exe && s.game === g.name)) last = Math.max(last, Date.parse(s.end) || 0);
  }
  return last;
}

function playtime(g) {
  const exe = (g.exe || '').toLowerCase();
  return (S.tools.sessions || []).filter((s) => (s.exe || '').toLowerCase() === exe || (!s.exe && s.game === g.name)).reduce((a, s) => a + (s.duration || 0), 0);
}

export function agoText(ms) {
  const m = Math.round((Date.now() - ms) / 60000);
  const rtf = new Intl.RelativeTimeFormat(lang(), { numeric: 'auto' });
  if (m < 60) return rtf.format(-Math.max(1, m), 'minute');
  if (m < 48 * 60) return rtf.format(-Math.round(m / 60), 'hour');
  return rtf.format(-Math.round(m / 1440), 'day');
}

// Second line of a game row: launcher + when it was last played.
export function gameMeta(g) {
  const parts = [t('g.src.' + (g.source || 'manual'))];
  const last = lastPlayed(g);
  if (last) parts.push(t('play.last', { when: agoText(last) }));
  const pt = playtime(g);
  if (pt >= 60) {
    const h = Math.floor(pt / 3600), m = Math.round((pt % 3600) / 60);
    parts.push(h ? t('play.total', { h, m }) : t('play.totalM', { m }));
  }
  return parts.join(' · ');
}

export function canPlay(g) { return !!(g.path || g.launch || g.steamId); }

export function playButton(g, cls = 'sm primary') {
  const b = busy.has(g.id);
  return `<button class="btn ${cls} play-btn" data-a="game-play" data-id="${esc(g.id)}" ${b || !canPlay(g) ? 'disabled' : ''} title="${esc(canPlay(g) ? t('play.btn') : t('g.needPath'))}">${b ? icon('loader', 'spin') : icon('play')}${t('play.btn')}</button>`;
}

// Games page: your games as posters, most recently played first, ready to start.
export function quickLaunchSection() {
  const list = (S.games.list || []).filter(canPlay);
  if (!list.length) return '';
  const sorted = [...list].sort((a, b) => lastPlayed(b) - lastPlayed(a) || String(b.added || '').localeCompare(String(a.added || '')));
  return `<section class="play-lib"><div class="sec-head"><div><h2>${t('play.title')}</h2><p>${t('play.libSub')}</p></div></div>
    <div class="ql">${sorted.map((g) => {
      const last = lastPlayed(g);
      return `<div class="ql-item">${coverHTML(g.name, g.cover, 'big')}
        <div class="ql-meta"><b title="${esc(g.name)}">${esc(g.name)}</b><span>${last ? esc(t('play.last', { when: agoText(last) })) : esc(t('play.never'))}</span></div>
        ${playButton(g, 'sm primary')}</div>`;
    }).join('')}</div></section>`;
}

async function play(id, admin = false) {
  const g = (S.games.list || []).find((x) => x.id === id);
  if (!g || (busy.has(id) && !admin)) return;
  busy.add(id);
  hooks.renderPage();
  try {
    const r = await api('/api/games/launch', { id, admin });
    if (r.games) { S.games.list = r.games; hooks.renderPage(); }
    const via = { steam: 'Steam', 'com.epicgames.launcher': 'Epic Games Launcher', riot: 'Riot Client' }[r.via];
    const auto = S.settings.autoGame && S.settings.autoGame.enabled;
    let sub = via ? t('play.via', { via }) : '';
    if (!auto && !hinted) { hinted = true; sub = [sub, t('play.autoHint')].filter(Boolean).join('. '); }
    toast(t('play.started', { name: esc(g.name) }), 'good', sub);
  } catch (e) {
    if (e.message === 'user_start_failed' && !admin) {
      busy.delete(id);
      hooks.renderPage();
      const v = await modal({
        title: `${icon('alert')}${t('play.adminQ')}`, body: `<p class="muted" style="margin:0">${t('play.adminD', { name: esc(g.name) })}</p>`,
        actions: [{ label: t('btn.cancel'), value: null, kind: 'ghost' }, { label: `${icon('play')}${t('play.adminGo')}`, value: 'go', kind: 'primary' }],
      });
      if (v === 'go') play(id, true);
      return;
    }
    const k = 'play.e.' + e.message;
    toast(t(k) === k ? t('err.generic') : t(k), 'bad', t(k) === k ? e.message : '');
  }
  // keep the button busy a moment: launchers take a few seconds to react
  setTimeout(() => { busy.delete(id); hooks.renderPage(); }, 4000);
}

export async function loadSessionsQuiet() {
  try { const r = await api('/api/sessions'); S.tools.sessions = r.sessions; S.tools.recording = r.recording; } catch (e) { /* ignore */ }
}

export const V109_ACTIONS = {
  'game-play': (d) => play(d.id),
};
