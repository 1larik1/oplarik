// OpLarik account: sign-in / setup screens, two-step verification, profile photo
// cropper and the Account section in Settings.
import { t, setLang, lang, LANGS } from './i18n.js';
import { S, TOKEN, api, esc, icon, toast, toggle, fmtDate } from './core.js';

let hooks = { render: () => {}, renderPage: () => {} };
export function initAccount(h) { hooks = { ...hooks, ...h }; }

// ================================================================= helpers
export function avatarURL(st) {
  return st && st.avatar ? `/api/auth/avatar.png?v=${st.avatar}&t=${encodeURIComponent(TOKEN)}` : '';
}

function initials(name) {
  const p = String(name || '').trim().split(/\s+/).filter(Boolean);
  if (!p.length) return '';
  return (p[0][0] + (p.length > 1 ? p[p.length - 1][0] : '')).toUpperCase();
}

export function avatarHTML(st, size = 40, extra = '', dataURL = '') {
  const src = dataURL || avatarURL(st);
  if (src) return `<img class="av ${extra}" src="${src}" width="${size}" height="${size}" alt="" style="width:${size}px;height:${size}px">`;
  const ini = initials(st && st.name);
  return `<span class="av av-ini ${extra}" style="width:${size}px;height:${size}px;font-size:${Math.round(size * 0.38)}px;background:var(--grad)">${ini ? esc(ini) : icon('userRound')}</span>`;
}

function errText(e) {
  const code = (e && e.message) || '';
  const k = 'au.e.' + code;
  const s = t(k);
  return s === k ? `${t('err.generic')}${code ? ` (${esc(code)})` : ''}` : s;
}

function strength(p) {
  if (!p || p.length < 6) return 0;
  let s = 1;
  if (p.length >= 10) s++;
  if (/[a-zа-яіїєґ]/.test(p) && /[A-ZА-ЯІЇЄҐ]/.test(p)) s++;
  if (/\d/.test(p) && /[^\p{L}\d]/u.test(p)) s++;
  return Math.min(3, s);
}

function fmtWait(ms) {
  const s = Math.max(0, Math.ceil(ms / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

function pwField(id, label, { ph = '', ac = 'current-password', value = '' } = {}) {
  return `<label class="fld"><span>${label}</span><span class="fld-pw">
    <input id="${id}" type="password" autocomplete="${ac}" placeholder="${esc(ph)}" value="${esc(value)}" spellcheck="false">
    <button type="button" class="iconbtn" data-pw-eye="${id}" aria-label="${t('au.show')}" title="${t('au.show')}">${icon('eye')}</button></span></label>`;
}

function bindEyes(root) {
  root.querySelectorAll('[data-pw-eye]').forEach((b) => {
    b.onclick = () => {
      const inp = root.querySelector('#' + b.dataset.pwEye);
      const show = inp.type === 'password';
      inp.type = show ? 'text' : 'password';
      b.innerHTML = icon(show ? 'eyeOff' : 'eye');
      b.title = b.ariaLabel = t(show ? 'au.hide' : 'au.show');
    };
  });
}

function strengthHTML(p) {
  const s = strength(p);
  return `<div class="pw-str s${s}"><i></i><i></i><i></i><span>${p ? t('au.str.' + s) : ''}</span></div>`;
}

function codeBoxes(id) {
  return `<div class="code6" id="${id}">${[0, 1, 2, 3, 4, 5].map((i) => `<input inputmode="numeric" maxlength="1" autocomplete="${i ? 'off' : 'one-time-code'}" aria-label="${i + 1}" data-ci="${i}">`).join('')}</div>`;
}

// Six single-digit boxes: typing advances, Backspace goes back, paste fills all.
function bindCode(root, id, onFull) {
  const box = root.querySelector('#' + id);
  if (!box) return () => '';
  const ins = [...box.querySelectorAll('input')];
  const val = () => ins.map((i) => i.value).join('');
  const fill = (digits, from = 0) => {
    digits.split('').forEach((d, k) => { if (ins[from + k]) ins[from + k].value = d; });
    const next = Math.min(from + digits.length, 5);
    ins[next].focus();
    if (val().length === 6) onFull(val());
  };
  ins.forEach((inp, i) => {
    inp.addEventListener('input', () => {
      const d = inp.value.replace(/\D/g, '');
      inp.value = '';
      if (d) fill(d.slice(0, 6 - i), i);
    });
    inp.addEventListener('keydown', (e) => {
      if (e.key === 'Backspace' && !inp.value && i > 0) { ins[i - 1].value = ''; ins[i - 1].focus(); e.preventDefault(); }
      if (e.key === 'ArrowLeft' && i > 0) ins[i - 1].focus();
      if (e.key === 'ArrowRight' && i < 5) ins[i + 1].focus();
    });
    inp.addEventListener('paste', (e) => {
      const d = (e.clipboardData.getData('text') || '').replace(/\D/g, '').slice(0, 6);
      if (d) { e.preventDefault(); ins.forEach((x) => { x.value = ''; }); fill(d, 0); }
    });
  });
  return { value: val, clear: () => { ins.forEach((x) => { x.value = ''; }); ins[0].focus(); }, focus: () => ins[0].focus() };
}

function keyBlock(key) {
  return `<div class="rkey" id="rkey">${key.split('-').map((g) => `<span>${esc(g)}</span>`).join('<i>–</i>')}</div>
    <div class="rkey-act"><button type="button" class="btn sm" data-key-copy>${icon('copy')}${t('au.copy')}</button>
    <button type="button" class="btn sm" data-key-file>${icon('download')}${t('au.saveFile')}</button></div>`;
}

function bindKey(root, key) {
  const c = root.querySelector('[data-key-copy]');
  if (c) c.onclick = async () => {
    try { await navigator.clipboard.writeText(key); c.innerHTML = `${icon('check')}${t('au.copied')}`; } catch (e) { /* ignore */ }
  };
  const f = root.querySelector('[data-key-file]');
  if (f) f.onclick = () => {
    const txt = `OpLarik — ${t('au.key')}\r\n\r\n${key}\r\n\r\n${t('au.keySub')}\r\n`;
    const a = document.createElement('a');
    a.href = URL.createObjectURL(new Blob(['﻿' + txt], { type: 'text/plain;charset=utf-8' }));
    a.download = 'OpLarik-recovery-key.txt';
    document.body.appendChild(a); a.click(); a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 5000);
  };
}

// ================================================================= sign-in screen
let authOpen = false;
export function authVisible() { return authOpen; }

/**
 * Full-screen account gate. Modes: login | code | recover | setup | key | noaccess.
 * Resolves with 'unlocked' | 'created' | 'skipped' | 'cancel'.
 */
export function authScreen(start, { canSkip = false, cancelable = false } = {}) {
  authOpen = true;
  return new Promise(async (resolve) => {
    const el = document.createElement('div');
    el.className = 'auth';
    el.id = 'auth';
    document.body.appendChild(el);
    let info = {};
    try { info = await api('/api/auth'); } catch (e) { /* engine gone */ }
    const st = () => info.auth || {};
    let mode = start;
    let err = '';
    let busy = false;
    let ticket = '';
    let key = '';
    let keyResult = 'created';
    let photo = '';
    let savedKey = false;
    let timer = null;
    const v = { name: '', pw: '', pw2: '', rkey: '' };

    const finish = (r) => {
      clearInterval(timer);
      authOpen = false;
      el.classList.add('out');
      setTimeout(() => el.remove(), 260);
      resolve(r);
    };

    const langPill = () => `<div class="auth-lang">${LANGS.map((l) => `<button type="button" class="${lang() === l.code ? 'on' : ''}" data-au-lang="${l.code}">${l.code.toUpperCase()}</button>`).join('')}</div>`;

    const waitLeft = () => (st().waitUntil || 0) - Date.now();

    function card() {
      const s = st();
      const errLine = `<p class="au-err" role="alert">${err ? `${icon('warn')}${err}` : ''}</p>`;
      if (mode === 'login') {
        const w = waitLeft();
        return `<div class="au-av">${avatarHTML(s, 92)}</div>
          <h1>${esc(t('au.welcome', { name: s.name || '' }))}</h1><p class="au-sub">${t('au.loginSub')}</p>
          <form data-au-form="login" autocomplete="on">
            <input type="text" name="username" autocomplete="username" value="${esc(s.name || '')}" hidden>
            ${pwField('au-pw', t('au.password'), { value: v.pw })}
            ${w > 0 ? `<p class="au-wait">${icon('timer')}<span id="au-wait">${t('au.wait', { time: fmtWait(w) })}</span></p>` : errLine}
            <button class="btn primary au-go" ${busy || w > 0 ? 'disabled' : ''}>${busy ? icon('loader', 'spin') : icon('unlock')}${t('au.signin')}</button>
          </form>
          <button type="button" class="au-link" data-au-mode="recover">${t('au.forgot')}</button>`;
      }
      if (mode === 'code') {
        return `<div class="au-ic">${icon('phone')}</div>
          <h1>${t('au.codeT')}</h1><p class="au-sub">${t('au.codeSub')}</p>
          <form data-au-form="code">${codeBoxes('au-code')}${errLine}
            <button class="btn primary au-go" ${busy ? 'disabled' : ''}>${busy ? icon('loader', 'spin') : icon('shield')}${t('au.verify')}</button></form>
          <div class="au-links"><button type="button" class="au-link" data-au-mode="login">${t('au.back')}</button>
          <button type="button" class="au-link" data-au-mode="recover">${t('au.useKey')}</button></div>`;
      }
      if (mode === 'recover') {
        return `<div class="au-ic">${icon('key')}</div>
          <h1>${t('au.recT')}</h1><p class="au-sub">${t('au.recSub')}</p>
          <form data-au-form="recover">
            <label class="fld"><span>${t('au.key')}</span><input id="au-rkey" class="mono" placeholder="XXXX-XXXX-XXXX-XXXX" maxlength="24" autocomplete="off" spellcheck="false" value="${esc(v.rkey)}"></label>
            ${pwField('au-pw', t('au.newPw'), { ac: 'new-password', value: v.pw })}
            <div id="au-str">${strengthHTML(v.pw)}</div>
            ${pwField('au-pw2', t('au.repeat'), { ac: 'new-password', value: v.pw2 })}
            ${s.totp ? `<p class="au-note">${icon('info')}${t('au.recNote')}</p>` : ''}
            ${waitLeft() > 0 ? `<p class="au-wait">${icon('timer')}<span id="au-wait">${t('au.wait', { time: fmtWait(waitLeft()) })}</span></p>` : errLine}
            <button class="btn primary au-go" ${busy || waitLeft() > 0 ? 'disabled' : ''}>${busy ? icon('loader', 'spin') : icon('restore')}${t('au.reset')}</button>
          </form>
          <button type="button" class="au-link" data-au-mode="login">${t('au.back')}</button>`;
      }
      if (mode === 'setup') {
        return `<button type="button" class="au-av pick" data-au-photo title="${t('au.photo')}">${avatarHTML({ name: v.name }, 92, '', photo)}<span class="au-cam">${icon('camera')}</span></button>
          <h1>${t('au.setupT')}</h1><p class="au-sub">${t('au.setupSub')}</p>
          <form data-au-form="setup">
            <label class="fld"><span>${t('au.name')}</span><input id="au-name" maxlength="32" autocomplete="username" placeholder="${esc(t('au.namePh'))}" value="${esc(v.name)}" spellcheck="false"></label>
            ${pwField('au-pw', t('au.password'), { ac: 'new-password', value: v.pw })}
            <div id="au-str">${strengthHTML(v.pw)}</div>
            ${pwField('au-pw2', t('au.repeat'), { ac: 'new-password', value: v.pw2 })}
            ${errLine}
            <button class="btn primary au-go" ${busy ? 'disabled' : ''}>${busy ? icon('loader', 'spin') : icon('userRound')}${t('au.create')}</button>
          </form>
          <p class="au-note center">${icon('lockKey')}${t('au.local')}</p>
          ${canSkip || cancelable ? `<button type="button" class="au-link" data-au-skip>${canSkip ? t('au.skip') : t('btn.cancel')}</button>` : ''}`;
      }
      if (mode === 'key') {
        return `<div class="au-ic good">${icon('key')}</div>
          <h1>${t('au.keyT')}</h1><p class="au-sub">${t('au.keySub')}</p>
          ${keyBlock(key)}
          <button type="button" class="au-check" data-au-saved><span class="check ${savedKey ? 'on' : ''}">${icon('check')}</span>${t('au.saved')}</button>
          <button type="button" class="btn primary au-go" data-au-done ${savedKey ? '' : 'disabled'}>${t('au.continue')}${icon('chevronRight')}</button>`;
      }
      if (mode === 'damaged') {
        return `<div class="au-ic warn">${icon('alert')}</div>
          <h1>${t('au.dmgT')}</h1><p class="au-sub">${t('au.dmgSub')}</p>
          <code class="au-path">C:\\ProgramData\\Larik\\account</code>`;
      }
      // noaccess
      return `<div class="au-ic warn">${icon('lock')}</div>
        <h1>${t('au.noAccT')}</h1><p class="au-sub">${t('au.noAccSub')}</p>
        ${errLine}
        <button type="button" class="btn primary au-go" data-au-elevate>${icon('shield')}${t('btn.elevate')}</button>`;
    }

    function draw() {
      el.innerHTML = `<div class="auth-bg"></div>${langPill()}
        <div class="auth-card" data-mode="${mode}"><div class="auth-brand"><img src="img/logo.svg" alt="" width="28" height="28"><b>OpLarik</b></div>${card()}</div>`;
      bind();
    }

    async function refreshInfo() {
      try { info = await api('/api/auth'); } catch (e) { /* ignore */ }
    }

    function tick() {
      clearInterval(timer);
      if (waitLeft() <= 0) return;
      timer = setInterval(() => {
        const w = waitLeft();
        const s = el.querySelector('#au-wait');
        if (w <= 0) { clearInterval(timer); err = ''; draw(); return; }
        if (s) s.textContent = t('au.wait', { time: fmtWait(w) });
      }, 500);
    }

    async function run(fn) {
      if (busy) return;
      busy = true; err = ''; draw();
      try { await fn(); } catch (e) {
        err = errText(e);
        if (e.message === 'too_many' || e.message === 'bad_password' || e.message === 'bad_code' || e.message === 'bad_key') await refreshInfo();
        if (e.message === 'expired') { mode = 'login'; v.pw = ''; }
      }
      busy = false;
      if (el.isConnected) { draw(); tick(); }
    }

    let codeCtl = null;
    function bind() {
      bindEyes(el);
      el.querySelectorAll('[data-au-lang]').forEach((b) => {
        b.onclick = () => {
          setLang(b.dataset.auLang);
          api('/api/auth/lang', { lang: b.dataset.auLang }).catch(() => {});
          if (S.settings) S.settings.lang = b.dataset.auLang;
          draw();
        };
      });
      el.querySelectorAll('[data-au-mode]').forEach((b) => {
        b.onclick = () => { mode = b.dataset.auMode; err = ''; v.pw = ''; v.pw2 = ''; draw(); tick(); };
      });
      const keep = (id, k) => { const i = el.querySelector('#' + id); if (i) i.addEventListener('input', () => { v[k] = i.value; if (k === 'pw') { const s = el.querySelector('#au-str'); if (s) s.innerHTML = strengthHTML(v.pw); } }); };
      keep('au-pw', 'pw'); keep('au-pw2', 'pw2'); keep('au-name', 'name'); keep('au-rkey', 'rkey');
      const rk = el.querySelector('#au-rkey');
      if (rk) rk.addEventListener('input', () => {
        const raw = rk.value.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 16);
        rk.value = raw.replace(/(.{4})(?=.)/g, '$1-');
        v.rkey = rk.value;
      });
      const nm = el.querySelector('#au-name');
      if (nm && !photo) nm.addEventListener('input', () => { const a = el.querySelector('.au-av'); if (a) a.innerHTML = `${avatarHTML({ name: v.name }, 92)}<span class="au-cam">${icon('camera')}</span>`; });

      const form = el.querySelector('[data-au-form]');
      if (form) form.onsubmit = (e) => { e.preventDefault(); submit(form.dataset.auForm); };
      if (mode === 'code') {
        codeCtl = bindCode(el, 'au-code', () => submit('code'));
        codeCtl.focus();
      }
      const ph = el.querySelector('[data-au-photo]');
      if (ph) ph.onclick = async () => { const d = await pickAvatar(); if (d) { photo = d; draw(); } };
      const sk = el.querySelector('[data-au-skip]');
      if (sk) sk.onclick = async () => {
        if (canSkip) await api('/api/auth/skip', {}).catch(() => {});
        finish(canSkip ? 'skipped' : 'cancel');
      };
      const sv = el.querySelector('[data-au-saved]');
      if (sv) sv.onclick = () => { savedKey = !savedKey; draw(); };
      const dn = el.querySelector('[data-au-done]');
      if (dn) dn.onclick = () => finish(keyResult);
      const elv = el.querySelector('[data-au-elevate]');
      if (elv) elv.onclick = () => run(async () => { const r = await api('/api/elevate', {}); if (r.url) location.href = r.url; });
      if (mode === 'key') bindKey(el, key);
      const first = el.querySelector(mode === 'setup' ? '#au-name' : mode === 'recover' ? '#au-rkey' : '#au-pw');
      if (first && !busy) setTimeout(() => first.focus(), 30);
    }

    function submit(kind) {
      if (kind === 'login') {
        if (!v.pw) return;
        run(async () => {
          const r = await api('/api/auth/login', { password: v.pw });
          v.pw = '';
          if (r.ticket) { ticket = r.ticket; mode = 'code'; return; }
          finish('unlocked');
        });
      } else if (kind === 'code') {
        const code = codeCtl ? codeCtl.value() : '';
        if (code.length !== 6) return;
        run(async () => {
          try { await api('/api/auth/code', { ticket, code }); } catch (e) { codeCtl && codeCtl.clear(); throw e; }
          finish('unlocked');
        });
      } else if (kind === 'recover') {
        if (v.rkey.replace(/-/g, '').length < 16) { err = errText({ message: 'bad_key' }); draw(); return; }
        if (strength(v.pw) === 0) { err = errText({ message: 'weak_password' }); draw(); return; }
        if (v.pw !== v.pw2) { err = t('au.mismatch'); draw(); return; }
        run(async () => {
          const r = await api('/api/auth/recover', { key: v.rkey, password: v.pw });
          key = r.key; keyResult = 'unlocked'; savedKey = false; mode = 'key'; v.pw = v.pw2 = '';
        });
      } else if (kind === 'setup') {
        if (!v.name.trim()) { err = errText({ message: 'bad_name' }); draw(); return; }
        if (strength(v.pw) === 0) { err = errText({ message: 'weak_password' }); draw(); return; }
        if (v.pw !== v.pw2) { err = t('au.mismatch'); draw(); return; }
        run(async () => {
          const r = await api('/api/auth/setup', { name: v.name, password: v.pw });
          if (photo) await api('/api/auth/avatar', { data: photo }).catch(() => {});
          key = r.key; keyResult = 'created'; savedKey = false; mode = 'key'; v.pw = v.pw2 = '';
        });
      }
    }

    if (mode === 'login' && !st().exists) mode = 'setup';
    if (st().noAccess) mode = 'noaccess';
    if (st().damaged) mode = 'damaged';
    draw();
    tick();
  });
}

// Shown when the engine reports 423 (locked from another window, by
// auto-lock or by "Lock now"). After signing in the app reloads its data.
export async function showLocked() {
  if (authOpen) return;
  document.querySelectorAll('.crop-layer').forEach((c) => c.remove());
  const ml = document.getElementById('modal');
  if (ml) { ml.classList.remove('on'); ml.innerHTML = ''; }
  const r = await authScreen('login');
  if (r === 'unlocked') location.reload();
}

// ================================================================= photo picker + cropper
export function pickAvatar() {
  return new Promise((resolve) => {
    const inp = document.createElement('input');
    inp.type = 'file';
    inp.accept = 'image/png,image/jpeg,image/webp,image/gif,image/bmp';
    inp.onchange = () => {
      const f = inp.files && inp.files[0];
      if (!f) { resolve(null); return; }
      if (f.size > 25 * 1024 * 1024) { toast(t('au.e.bad_image'), 'bad'); resolve(null); return; }
      // Read as a data: URL — the page's CSP allows data: images but not blob: ones.
      const rd = new FileReader();
      rd.onerror = () => { toast(t('au.e.bad_image'), 'bad'); resolve(null); };
      rd.onload = () => {
        const img = new Image();
        img.onload = () => cropper(img).then(resolve);
        img.onerror = () => { toast(t('au.e.bad_image'), 'bad'); resolve(null); };
        img.src = rd.result;
      };
      rd.readAsDataURL(f);
    };
    inp.click();
  });
}

function cropper(img) {
  return new Promise((resolve) => {
    const V = 300;
    const layer = document.createElement('div');
    layer.className = 'crop-layer';
    layer.innerHTML = `<div class="modal" role="dialog" aria-modal="true">
      <div class="modal-head"><h2>${t('ac.photoT')}</h2><p>${t('ac.photoHint')}</p></div>
      <div class="modal-body"><canvas width="${V * 2}" height="${V * 2}" style="width:${V}px;height:${V}px" class="crop-cv"></canvas>
        <label class="crop-zoom">${icon('image')}<input type="range" min="100" max="400" value="100" aria-label="${t('ac.photoZoom')}">${icon('image', 'big')}</label></div>
      <div class="modal-foot"><button class="btn ghost" data-c="0">${t('btn.cancel')}</button><button class="btn primary" data-c="1">${icon('check')}${t('ac.save')}</button></div></div>`;
    document.body.appendChild(layer);
    const cv = layer.querySelector('canvas');
    const ctx = cv.getContext('2d');
    const base = Math.max(V / img.naturalWidth, V / img.naturalHeight);
    let zoom = 1;
    let cx = img.naturalWidth / 2;
    let cy = img.naturalHeight / 2;
    const clamp = () => {
      const s = base * zoom;
      const hw = V / 2 / s;
      cx = Math.min(Math.max(cx, hw), img.naturalWidth - hw);
      cy = Math.min(Math.max(cy, hw), img.naturalHeight - hw);
    };
    const paint = (c, size, mask) => {
      const k = size / V;
      const s = base * zoom * k;
      c.clearRect(0, 0, size, size);
      c.imageSmoothingQuality = 'high';
      c.drawImage(img, size / 2 - cx * s, size / 2 - cy * s, img.naturalWidth * s, img.naturalHeight * s);
      if (mask) {
        c.save();
        c.fillStyle = 'rgba(10,6,18,.62)'; // dims outside the circle in every theme
        c.beginPath(); c.rect(0, 0, size, size); c.arc(size / 2, size / 2, size / 2 - 2 * k, 0, Math.PI * 2, true); c.fill('evenodd');
        c.strokeStyle = `rgba(${getComputedStyle(document.documentElement).getPropertyValue('--a5-rgb')},.95)`; c.lineWidth = 2 * k;
        c.beginPath(); c.arc(size / 2, size / 2, size / 2 - 2 * k, 0, Math.PI * 2); c.stroke();
        c.restore();
      }
    };
    const redraw = () => { clamp(); paint(ctx, V * 2, true); };
    redraw();
    let drag = null;
    cv.addEventListener('pointerdown', (e) => { drag = { x: e.clientX, y: e.clientY, cx, cy }; cv.setPointerCapture(e.pointerId); });
    cv.addEventListener('pointermove', (e) => {
      if (!drag) return;
      const s = base * zoom;
      cx = drag.cx - (e.clientX - drag.x) / s; cy = drag.cy - (e.clientY - drag.y) / s; redraw();
    });
    cv.addEventListener('pointerup', () => { drag = null; });
    cv.addEventListener('wheel', (e) => {
      e.preventDefault();
      zoom = Math.min(4, Math.max(1, zoom * (e.deltaY < 0 ? 1.08 : 1 / 1.08)));
      layer.querySelector('input').value = Math.round(zoom * 100); redraw();
    }, { passive: false });
    layer.querySelector('input').oninput = (e) => { zoom = +e.target.value / 100; redraw(); };
    const close = (ok) => {
      let out = null;
      if (ok) {
        const o = document.createElement('canvas');
        o.width = o.height = 256;
        paint(o.getContext('2d'), 256, false);
        out = o.toDataURL('image/png');
      }
      layer.remove();
      document.removeEventListener('keydown', onKey, true);
      resolve(out);
    };
    const onKey = (e) => { if (e.key === 'Escape') { e.stopPropagation(); close(false); } };
    document.addEventListener('keydown', onKey, true);
    layer.querySelectorAll('[data-c]').forEach((b) => { b.onclick = () => close(b.dataset.c === '1'); });
    layer.querySelector('[data-c="1"]').focus();
  });
}

// ================================================================= form modal
// Like core.modal but keeps the dialog open to show an error inline.
function formModal({ title, sub = '', body = '', submit, kind = 'primary', onSubmit, onMount, wide = false }) {
  return new Promise((resolve) => {
    const layer = document.getElementById('modal');
    layer.innerHTML = `<form class="modal ${wide ? 'wide' : ''}" role="dialog" aria-modal="true">
      <div class="modal-head"><h2>${title}</h2>${sub ? `<p>${sub}</p>` : ''}</div>
      <div class="modal-body">${body}<p class="au-err" role="alert"></p></div>
      <div class="modal-foot"><button type="button" class="btn ghost" data-f="cancel">${t('btn.cancel')}</button>
        <button class="btn ${kind}" data-f="ok">${submit}</button></div></form>`;
    layer.classList.add('on');
    const form = layer.querySelector('form');
    const errEl = layer.querySelector('.au-err');
    const done = (v) => { layer.classList.remove('on'); layer.innerHTML = ''; layer.onclick = null; resolve(v); };
    layer.onclick = (e) => { if (e.target === layer || e.target.closest('[data-f="cancel"]')) done(null); };
    let busy = false;
    const ctl = {
      error: (e) => { errEl.innerHTML = e ? `${icon('warn')}${typeof e === 'string' ? e : errText(e)}` : ''; },
      submit: () => form.requestSubmit(),
    };
    form.onsubmit = async (e) => {
      e.preventDefault();
      if (busy) return;
      busy = true;
      const btn = layer.querySelector('[data-f="ok"]');
      btn.disabled = true;
      ctl.error('');
      try {
        const r = await onSubmit(form, ctl);
        if (r !== false) { done(r === undefined ? true : r); return; }
      } catch (err) { ctl.error(err); }
      busy = false;
      btn.disabled = false;
    };
    bindEyes(layer);
    if (onMount) onMount(layer, ctl);
    setTimeout(() => (layer.querySelector('input') || layer.querySelector('[data-f="ok"]')).focus(), 30);
  });
}

function keyModal(key) {
  return new Promise((resolve) => {
    const layer = document.getElementById('modal');
    let saved = false;
    const draw = () => {
      layer.innerHTML = `<div class="modal" role="dialog" aria-modal="true"><div class="modal-head"><h2>${t('au.keyT')}</h2><p>${t('au.keySub')}</p></div>
        <div class="modal-body">${keyBlock(key)}<button type="button" class="au-check" data-saved><span class="check ${saved ? 'on' : ''}">${icon('check')}</span>${t('au.saved')}</button></div>
        <div class="modal-foot"><button class="btn primary" data-done ${saved ? '' : 'disabled'}>${t('au.continue')}</button></div></div>`;
      bindKey(layer, key);
      layer.querySelector('[data-saved]').onclick = () => { saved = !saved; draw(); };
      layer.querySelector('[data-done]').onclick = () => { layer.classList.remove('on'); layer.innerHTML = ''; resolve(); };
    };
    layer.onclick = null;
    layer.classList.add('on');
    draw();
  });
}

// ================================================================= account settings
export function accountSection() {
  const a = S.acct || {};
  const admin = S.boot && S.boot.admin;
  if (!a.exists) {
    return `<section class="sec" id="acct-sec"><div class="sec-head"><div><h2>${t('ac.title')}</h2><p>${t('ac.sub')}</p></div></div>
      <div class="card acct-empty"><div class="acct-empty-ic">${icon('lockKey')}</div><div class="grow"><h4>${t('ac.none')}</h4><p>${t('ac.noneD')}</p></div>
        <button class="btn primary" data-a="acct-create" ${admin ? '' : 'disabled'}>${icon('userRound')}${t('au.create')}</button></div></section>`;
  }
  const al = (S.settings && S.settings.autoLock) || 0;
  const seg = [0, 5, 15, 30].map((m) => `<button class="${al === m ? 'on' : ''}" role="radio" aria-checked="${al === m}" data-a="acct-autolock" data-v="${m}">${m ? t('ac.min', { n: m }) : t('ac.never')}</button>`).join('');
  const dis = admin ? '' : 'disabled';
  return `<section class="sec" id="acct-sec"><div class="sec-head"><div><h2>${t('ac.title')}</h2><p>${t('ac.sub')}</p></div>
      <button class="btn" data-a="acct-lock">${icon('lock')}${t('ac.lockNow')}</button></div>
    <div class="acct-grid">
      <div class="card acct-profile">
        <div class="acct-av">${avatarHTML(a, 112)}<button class="acct-cam" data-a="acct-photo" title="${a.avatar ? t('ac.photo') : t('ac.photoAdd')}" ${dis}>${icon('camera')}</button></div>
        <div class="acct-name"><b>${esc(a.name)}</b><span>${t('ac.since', { date: fmtDate(a.created) })}</span></div>
        <div class="acct-photo-btns"><button class="btn sm" data-a="acct-photo" ${dis}>${icon('image')}${a.avatar ? t('ac.photo') : t('ac.photoAdd')}</button>
          ${a.avatar ? `<button class="btn sm ghost" data-a="acct-photo-clear" ${dis}>${icon('trash')}${t('ac.photoRm')}</button>` : ''}</div>
        <form class="acct-rename" data-acct-rename><label class="fld"><span>${t('ac.name')}</span><input id="acct-name" maxlength="32" value="${esc(a.name)}" spellcheck="false" ${dis}></label>
          <button class="btn" ${dis}>${icon('check')}${t('ac.save')}</button></form>
      </div>
      <div class="card">
        <div class="setting"><div><h4>${icon('key')}${t('ac.pass')}</h4><p>${t('ac.passD')}</p></div><button class="btn" data-a="acct-pass" ${dis}>${t('ac.passBtn')}</button></div>
        <div class="setting"><div><h4>${icon('phone')}${t('ac.2fa')} ${a.totp ? `<span class="tag good">${icon('check')}${t('ac.on')}</span>` : ''}</h4><p>${t('ac.2faD')}</p></div>
          ${toggle(!!a.totp, `data-a="acct-2fa" ${dis ? 'disabled' : ''}`)}</div>
        <div class="setting"><div><h4>${icon('lockKey')}${t('ac.key')}</h4><p>${t('ac.keyD')}</p></div><button class="btn" data-a="acct-key" ${dis}>${t('ac.keyBtn')}</button></div>
        <div class="setting"><div><h4>${icon('timer')}${t('ac.lock')}</h4><p>${t('ac.lockD')}</p></div><div class="seg" role="radiogroup">${seg}</div></div>
        <div class="setting"><div><h4>${icon('userX')}${t('ac.remove')}</h4><p>${t('ac.removeD')}</p></div><button class="btn danger" data-a="acct-remove" ${dis}>${t('ac.removeBtn')}</button></div>
        ${admin ? '' : `<p class="muted" style="margin:12px 4px 0;font-size:12.5px">${t('ac.limited')}</p>`}
      </div>
    </div></section>`;
}

async function refreshAcct() {
  try { const r = await api('/api/auth'); S.acct = r.auth; } catch (e) { /* ignore */ }
  hooks.render();
}

function passwordForm(label = t('au.password')) {
  return pwField('fm-pw', label);
}

export const ACCOUNT_ACTIONS = {
  'acct-create': async () => {
    const r = await authScreen('setup', { cancelable: true });
    if (r === 'created') { await refreshAcct(); toast(t('ac.created'), 'good'); }
  },
  'acct-photo': async () => {
    const d = await pickAvatar();
    if (!d) return;
    try { S.acct = await api('/api/auth/avatar', { data: d }); toast(t('ac.photoOk'), 'good'); } catch (e) { toast(errText(e), 'bad'); }
    hooks.render();
  },
  'acct-photo-clear': async () => {
    try { S.acct = await api('/api/auth/avatar/clear', {}); } catch (e) { toast(errText(e), 'bad'); }
    hooks.render();
  },
  'acct-pass': async () => {
    const ok = await formModal({
      title: t('ac.passT'), submit: `${icon('check')}${t('ac.passBtn')}`,
      body: `${pwField('fm-old', t('ac.cur'))}${pwField('fm-pw', t('au.newPw'), { ac: 'new-password' })}<div id="fm-str">${strengthHTML('')}</div>${pwField('fm-pw2', t('au.repeat'), { ac: 'new-password' })}`,
      onMount: (l) => { l.querySelector('#fm-pw').addEventListener('input', (e) => { l.querySelector('#fm-str').innerHTML = strengthHTML(e.target.value); }); },
      onSubmit: async (f, ctl) => {
        const old = f.querySelector('#fm-old').value, pw = f.querySelector('#fm-pw').value, pw2 = f.querySelector('#fm-pw2').value;
        if (strength(pw) === 0) { ctl.error({ message: 'weak_password' }); return false; }
        if (pw !== pw2) { ctl.error(t('au.mismatch')); return false; }
        await api('/api/auth/password', { old, password: pw });
      },
    });
    if (ok) toast(t('ac.passOk'), 'good');
  },
  'acct-2fa': async () => {
    const a = S.acct || {};
    if (a.totp) {
      const ok = await formModal({
        title: t('ac.2faOffT'), sub: t('ac.2faOffD'), kind: 'danger', submit: t('ac.turnOff'), body: passwordForm(),
        onSubmit: async (f) => { S.acct = await api('/api/auth/totp/disable', { password: f.querySelector('#fm-pw').value }); },
      });
      if (ok) toast(t('ac.2faOffOk'), 'good');
      hooks.render();
      return;
    }
    let setup;
    try { setup = await api('/api/auth/totp/begin', {}); } catch (e) { toast(errText(e), 'bad'); return; }
    const ok = await formModal({
      title: t('ac.2faT'), wide: true, submit: `${icon('shield')}${t('ac.enable')}`,
      body: `<div class="tfa"><div class="tfa-qr"><img src="${setup.qr}" alt="QR" width="200" height="200"></div>
        <ol class="tfa-steps"><li>${t('ac.2fa1')}</li><li>${t('ac.2fa2')}</li>
          <li>${t('ac.2fa3')}${codeBoxes('fm-code')}</li></ol></div>
        <details class="tfa-manual"><summary>${t('ac.2faManual')}</summary><code>${esc(setup.secret)}</code></details>
        <div class="tfa-pw">${pwField('fm-pw', t('ac.2faPw'))}</div>`,
      onMount: (l, ctl) => {
        const c = bindCode(l, 'fm-code', () => { const pw = l.querySelector('#fm-pw'); if (pw.value) ctl.submit(); else pw.focus(); });
        l._code = c;
        setTimeout(() => c.focus(), 40);
      },
      onSubmit: async (f, ctl) => {
        const layer = document.getElementById('modal');
        const code = layer._code ? layer._code.value() : '';
        const password = f.querySelector('#fm-pw').value;
        if (code.length !== 6) { ctl.error({ message: 'bad_code' }); return false; }
        if (!password) { ctl.error({ message: 'bad_password' }); f.querySelector('#fm-pw').focus(); return false; }
        try { S.acct = await api('/api/auth/totp/confirm', { code, password }); } catch (e) { if (e.message === 'bad_code') layer._code && layer._code.clear(); throw e; }
      },
    });
    if (ok) toast(t('ac.2faOk'), 'good');
    hooks.render();
  },
  'acct-key': async () => {
    let key = '';
    const ok = await formModal({
      title: t('ac.keyT'), sub: t('ac.keyD'), submit: `${icon('key')}${t('ac.keyBtn')}`, body: passwordForm(),
      onSubmit: async (f) => { key = (await api('/api/auth/recovery-key', { password: f.querySelector('#fm-pw').value })).key; },
    });
    if (ok && key) await keyModal(key);
  },
  'acct-autolock': async (d) => {
    S.settings.autoLock = +d.v;
    hooks.renderPage();
    try { await api('/api/settings', S.settings); } catch (e) { /* ignore */ }
  },
  'acct-lock': async () => {
    try { await api('/api/auth/lock', {}); } catch (e) { /* ignore */ }
    showLocked();
  },
  'acct-remove': async () => {
    const ok = await formModal({
      title: t('ac.removeT'), sub: t('ac.removeD'), kind: 'danger', submit: `${icon('trash')}${t('ac.removeBtn')}`, body: passwordForm(),
      onSubmit: async (f) => { S.acct = await api('/api/auth/remove', { password: f.querySelector('#fm-pw').value }); },
    });
    if (ok) toast(t('ac.removeOk'), 'good');
    hooks.render();
  },
  'acct-menu': (d, btn) => {
    document.querySelectorAll('.menu-pop').forEach((m) => m.remove());
    const a = S.acct || {};
    const r = btn.getBoundingClientRect();
    const m = document.createElement('div');
    m.className = 'menu-pop acct-pop';
    m.style.cssText = `position:fixed;top:${r.bottom + 6}px;right:${window.innerWidth - r.right}px;z-index:50`;
    m.innerHTML = `<div class="acct-pop-head">${avatarHTML(a, 44)}<div><b>${esc(a.name)}</b><span>${a.totp ? t('ac.2faOn') : t('ac.local')}</span></div></div>
      <button class="btn ghost" data-a="acct-open">${icon('settings')}${t('ac.menuSettings')}</button>
      <button class="btn ghost" data-a="acct-lock">${icon('lock')}${t('ac.lockShort')}</button>`;
    document.body.appendChild(m);
    m.addEventListener('click', () => setTimeout(() => m.remove(), 0));
    setTimeout(() => document.addEventListener('click', function off(e) { if (!m.contains(e.target)) { m.remove(); document.removeEventListener('click', off); } }), 0);
  },
};

// Rename is a form submit (Enter works), handled here.
document.addEventListener('submit', async (e) => {
  const f = e.target.closest('[data-acct-rename]');
  if (!f) return;
  e.preventDefault();
  const name = f.querySelector('#acct-name').value;
  try { S.acct = await api('/api/auth/name', { name }); toast(t('ac.nameOk'), 'good'); hooks.render(); } catch (err) { toast(errText(err), 'bad'); }
});

// ================================================================= auto-lock
let lastActive = Date.now();
['pointerdown', 'pointermove', 'keydown', 'wheel'].forEach((ev) => document.addEventListener(ev, () => { lastActive = Date.now(); }, { passive: true, capture: true }));
setInterval(() => {
  const mins = (S.settings && S.settings.autoLock) || 0;
  if (!mins || authOpen || !S.acct || !S.acct.exists || S.acct.locked) return;
  if (Date.now() - lastActive > mins * 60000) {
    lastActive = Date.now();
    api('/api/auth/lock', {}).catch(() => {}).finally(showLocked);
  }
}, 10000);
