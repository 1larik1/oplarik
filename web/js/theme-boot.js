// Applies the saved theme before the page paints (no flash of the default colors).
try {
  var t = localStorage.getItem('larik.theme');
  if (t) {
    document.documentElement.setAttribute('data-theme', t);
    // keep in sync with LIGHT_THEMES in core.js
    var light = ['light', 'sky', 'lagoon', 'mint', 'sand', 'rose', 'paper'].indexOf(t) >= 0;
    document.documentElement.setAttribute('data-tone', light ? 'light' : 'dark');
  }
} catch (e) { /* storage unavailable: default theme */ }
