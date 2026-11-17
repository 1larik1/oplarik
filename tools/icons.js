// Extracts Lucide icons (ISC license) from react-icons into web/js/icons.js
const fs = require('fs');
const path = require('path');
const src = fs.readFileSync(path.join(process.argv[2], 'lu/index.js'), 'utf8');
const want = {
  home:'LuHouse', rocket:'LuRocket', games:'LuGamepad2', zap:'LuZap', debloat:'LuPackageMinus', cpu:'LuCpu',
  backup:'LuHistory', settings:'LuSettings', search:'LuSearch', bookmark:'LuBookmark', bookmarkOn:'LuBookmarkCheck',
  info:'LuInfo', plus:'LuPlus', check:'LuCheck', x:'LuX', alert:'LuTriangleAlert', refresh:'LuRefreshCw',
  globe:'LuLanguages', chevronDown:'LuChevronDown', chevronRight:'LuChevronRight', download:'LuDownload',
  monitor:'LuMonitor', disk:'LuHardDrive', ram:'LuMemoryStick', wifi:'LuWifi', lock:'LuLock', shield:'LuShieldCheck',
  shieldOff:'LuShieldOff', power:'LuPower', clock:'LuClock', flame:'LuFlame', gauge:'LuGauge', activity:'LuActivity',
  undo:'LuUndo2', restore:'LuRotateCcw', folder:'LuFolderOpen', copy:'LuCopy', external:'LuExternalLink',
  ok:'LuCircleCheck', warn:'LuCircleAlert', chip:'LuMicrochip', board:'LuCircuitBoard', network:'LuNetwork',
  mouse:'LuMousePointer2', keyboard:'LuKeyboard', battery:'LuBatteryCharging', store:'LuStore', wand:'LuWandSparkles',
  layers:'LuLayers', terminal:'LuTerminal', sliders:'LuSlidersHorizontal', sparkles:'LuSparkles', loader:'LuLoaderCircle',
  swords:'LuSwords', eyeOff:'LuEyeOff', package:'LuPackage', puzzle:'LuPuzzle', server:'LuServer', wrench:'LuWrench',
  trash:'LuTrash2', brush:'LuPaintbrush', timer:'LuTimer', signal:'LuSignal', logout:'LuLogOut', play:'LuPlay',
  stop:'LuSquare', eye:'LuEye', windows:'LuAppWindow', thermo:'LuThermometer', dna:'LuFingerprint', scan:'LuScanLine',
  ethernet:'LuEthernetPort', leaf:'LuLeaf', gpu:'LuMonitorCog', user:'LuUser', bolt:'LuBolt', target:'LuCrosshair',
  arrowUp:'LuArrowUpRight', list:'LuListChecks', file:'LuFileCog', badge:'LuBadgeCheck', steam:'LuLibrary', star:'LuStar',
  camera:'LuCamera', key:'LuKeyRound', phone:'LuSmartphone', image:'LuImage', userRound:'LuUserRound', lockKey:'LuLockKeyhole',
  unlock:'LuLockOpen', pencil:'LuPencil', userX:'LuUserRoundX', qr:'LuQrCode', startup:'LuLogIn',
  health:'LuHeartPulse', chart:'LuChartLine', compare:'LuGitCompareArrows', upload:'LuUpload', calendar:'LuCalendarClock',
  cloud:'LuCloudDownload', radar:'LuRadar', arrowRight:'LuArrowRight', fileDown:'LuFileDown', fileUp:'LuFileUp', bot:'LuBot',
  up:'LuTrendingUp', down:'LuTrendingDown', router:'LuRouter', monitorPlay:'LuMonitorPlay'
};
const out = {};
for (const [k, name] of Object.entries(want)) {
  const re = new RegExp('function ' + name + ' \\(props\\) \\{\\n\\s+return GenIcon\\((\\{.*?\\})\\)\\(props\\);');
  const m = src.match(re);
  if (!m) { console.error('missing', name); continue; }
  const node = JSON.parse(m[1]);
  const render = (n) => n.child.map(c => '<' + c.tag + ' ' + Object.entries(c.attr).map(([a, v]) => a.replace(/[A-Z]/g, x => '-' + x.toLowerCase()) + '="' + v + '"').join(' ') + (c.child && c.child.length ? '>' + render(c) + '</' + c.tag + '>' : '/>')).join('');
  out[k] = render(node);
}
fs.writeFileSync(process.argv[3], '// Icons: Lucide (https://lucide.dev), ISC License.\nexport const ICONS = ' + JSON.stringify(out, null, 0).replace(/","/g, '",\n"') + ';\n');
console.log(Object.keys(out).length, 'icons');
