'use strict';

// ============================================================
// i18n
// ============================================================
let locale = {};

async function loadLocale(lang) {
  try {
    const res = await fetch(`/locales/${lang}.json`);
    if (res.ok) locale = await res.json();
  } catch (_) {}
  applyI18n();
}

function t(key, vars = {}) {
  let s = locale[key] || key;
  for (const [k, v] of Object.entries(vars)) s = s.replace(`{${k}}`, v);
  return s;
}

function applyI18n() {
  // Static text nodes
  document.querySelectorAll('[data-i18n]').forEach(el => {
    el.textContent = t(el.dataset.i18n);
  });
  // Placeholder attributes
  document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
    el.placeholder = t(el.dataset.i18nPlaceholder);
  });
  // Re-render any fq-chip containers that are already populated
  ['fq-chips', 'fq-chips-default', 'fq-chips-playlist'].forEach(id => {
    const el = document.getElementById(id);
    if (el && el.children.length > 0) renderFQChips(id, fqSelections[id]);
  });
}

// ============================================================
// Format / Quality options (combined chips)
// ============================================================
const FORMAT_OPTIONS = [
  { id: 'mp3-320k',  format: 'mp3',  quality: '320k',  i18n: 'fq.mp3_320',  group: 'audio' },
  { id: 'mp3-192k',  format: 'mp3',  quality: '192k',  i18n: 'fq.mp3_192',  group: 'audio' },
  { id: 'mp3-128k',  format: 'mp3',  quality: '128k',  i18n: 'fq.mp3_128',  group: 'audio' },
  { id: 'mp4-best',  format: 'mp4',  quality: 'best',  i18n: 'fq.mp4_best', group: 'video' },
  { id: 'mp4-1080p', format: 'mp4',  quality: '1080p', i18n: 'fq.mp4_1080', group: 'video' },
  { id: 'mp4-720p',  format: 'mp4',  quality: '720p',  i18n: 'fq.mp4_720',  group: 'video' },
  { id: 'webm-best', format: 'webm', quality: 'best',  i18n: 'fq.webm_best',group: 'video' },
  { id: 'webm-1080p',format: 'webm', quality: '1080p', i18n: 'fq.webm_1080',group: 'video' },
  { id: 'webm-720p', format: 'webm', quality: '720p',  i18n: 'fq.webm_720', group: 'video' },
];

// Active selections per chip container id → fqId
const fqSelections = {
  'fq-chips': 'mp3-320k',
  'fq-chips-default': 'mp3-320k',
  'fq-chips-playlist': 'mp3-320k',
};

function renderFQChips(containerId, selectedId) {
  const container = document.getElementById(containerId);
  if (!container) return;
  container.innerHTML = FORMAT_OPTIONS.map(opt => `
    <button class="fq-chip ${opt.group} ${opt.id === selectedId ? 'selected' : ''}"
            data-fq="${opt.id}" data-container="${containerId}">
      ${t(opt.i18n)}
    </button>
  `).join('');
  container.querySelectorAll('.fq-chip').forEach(btn => {
    btn.addEventListener('click', async () => {
      const cid = btn.dataset.container;
      fqSelections[cid] = btn.dataset.fq;
      renderFQChips(cid, btn.dataset.fq);
      if (cid === 'fq-chips-default') {
        const fq = getFQ('fq-chips-default');
        await saveSetting({ defaultFormat: fq.format, defaultQuality: fq.quality });
      }
    });
  });
}

function getFQ(containerId) {
  const id = fqSelections[containerId] || 'mp3-320k';
  return FORMAT_OPTIONS.find(o => o.id === id) || FORMAT_OPTIONS[0];
}

// ============================================================
// State
// ============================================================
let jobs = [];
let historyItems = [];
let pendingMeta = null;
let playlistItems = [];

// ============================================================
// DOM refs
// ============================================================
const urlInput       = document.getElementById('url-input');
const btnFetch       = document.getElementById('btn-fetch');
const preview        = document.getElementById('preview');
const previewThumb   = document.getElementById('preview-thumb');
const previewTitle   = document.getElementById('preview-title');
const btnDownload    = document.getElementById('btn-download');
const btnCancelPrev  = document.getElementById('btn-cancel-preview');
const jobList        = document.getElementById('job-list');
const emptyState     = document.getElementById('empty-state');
const queueCount     = document.getElementById('queue-count');
const historyList    = document.getElementById('history-list');
const btnClearHist   = document.getElementById('btn-clear-history');
const playlistModal  = document.getElementById('playlist-modal');
const playlistItems$ = document.getElementById('playlist-items');
const chkSelectAll   = document.getElementById('chk-select-all');
const playlistCount  = document.getElementById('playlist-count');
const playlistWarn   = document.getElementById('playlist-warning');
const btnClosePl     = document.getElementById('btn-close-playlist');
const btnDlPlaylist  = document.getElementById('btn-download-playlist');
const urlChoice      = document.getElementById('url-choice');
const btnChoiceSingle   = document.getElementById('btn-choice-single');
const btnChoicePlaylist = document.getElementById('btn-choice-playlist');
const btnChoiceCancel   = document.getElementById('btn-choice-cancel');

// ============================================================
// Tab switching
// ============================================================
function switchTab(name, pushHash = true) {
  document.querySelectorAll('.tab').forEach(t => t.classList.toggle('active', t.dataset.tab === name));
  document.querySelectorAll('.tab-content').forEach(c => c.classList.toggle('active', c.id === 'tab-' + name));
  if (pushHash) location.hash = name;
  if (name === 'history') loadHistory();
  if (name === 'settings') loadSettings();
}

window.addEventListener('hashchange', () => {
  const tab = location.hash.slice(1);
  if (['downloads', 'history', 'settings'].includes(tab)) switchTab(tab, false);
});

document.querySelectorAll('.tab').forEach(tab => {
  tab.addEventListener('click', () => switchTab(tab.dataset.tab));
});

// Logo → Downloads
document.getElementById('logo').addEventListener('click', () => switchTab('downloads'));

// ============================================================
// Mixed URL detection
// ============================================================
let _mixedURL = '';

btnChoiceSingle.addEventListener('click', async () => {
  urlChoice.hidden = true;
  await fetchSingleInfo(_mixedURL);
});
btnChoicePlaylist.addEventListener('click', async () => {
  urlChoice.hidden = true;
  setFetchLoading(true, true);
  await fetchPlaylist(_mixedURL);
  setFetchLoading(false, true);
});
btnChoiceCancel.addEventListener('click', () => {
  urlChoice.hidden = true;
  urlInput.value = '';
});

function isMixedURL(url) {
  try { const u = new URL(url); return u.searchParams.has('list') && u.searchParams.has('v'); }
  catch { return false; }
}
function isPlaylistOnly(url) {
  try { const u = new URL(url); return u.searchParams.has('list') && !u.searchParams.has('v'); }
  catch { return false; }
}

// ============================================================
// Fetch info / detect URL type
// ============================================================
btnFetch.addEventListener('click', fetchInfo);
urlInput.addEventListener('keydown', e => { if (e.key === 'Enter') fetchInfo(); });
urlInput.addEventListener('paste', e => {
  // Wait one tick for the pasted value to land in the input
  setTimeout(() => { if (urlInput.value.trim()) fetchInfo(); }, 0);
});
urlInput.addEventListener('input', () => {
  if (!urlInput.value.trim()) {
    preview.hidden = true;
    urlChoice.hidden = true;
    pendingMeta = null;
  }
});

async function fetchInfo() {
  const url = urlInput.value.trim();
  if (!url) return;
  const isPlaylist = isPlaylistOnly(url);
  setFetchLoading(true, isPlaylist);
  try {
    if (isMixedURL(url)) {
      _mixedURL = url;
      urlChoice.hidden = false;
    } else if (isPlaylist) {
      await fetchPlaylist(url);
    } else {
      await fetchSingleInfo(url);
    }
  } finally {
    setFetchLoading(false, isPlaylist);
  }
}

async function fetchSingleInfo(url) {
  const res = await api('POST', '/api/info', { url });
  if (res.error) { alert(res.error); return; }
  pendingMeta = res;
  showPreview(res);
}

async function fetchPlaylist(url) {
  const res = await api('POST', '/api/playlist', { url });
  if (res.error) { alert(res.error); return; }
  playlistItems = res;
  openPlaylistModal(res);
}

const pageLoading = document.getElementById('page-loading');

function setFetchLoading(on, overlay = false) {
  btnFetch.disabled = on;
  if (on) {
    btnFetch.innerHTML = '';
    btnFetch.appendChild(spinnerEl());
    btnFetch.appendChild(document.createTextNode(t('add.fetching')));
  } else {
    btnFetch.textContent = t('add.btn');
  }
  if (overlay) pageLoading.hidden = !on;
}

function showPreview(meta) {
  previewThumb.src = meta.thumbnailUrl || '';
  previewThumb.style.visibility = '';
  previewTitle.textContent = meta.title || meta.webpageUrl || '';
  renderFQChips('fq-chips', fqSelections['fq-chips']);
  preview.hidden = false;
}

btnCancelPrev.addEventListener('click', () => {
  preview.hidden = true;
  urlChoice.hidden = true;
  pendingMeta = null;
  urlInput.value = '';
});

// ============================================================
// Download single
// ============================================================
btnDownload.addEventListener('click', async () => {
  if (!pendingMeta) return;
  btnDownload.disabled = true;
  const fq = getFQ('fq-chips');
  const res = await api('POST', '/api/download', {
    url:          pendingMeta.webpageUrl || urlInput.value.trim(),
    format:       fq.format,
    quality:      fq.quality,
    title:        pendingMeta.title,
    thumbnailUrl: pendingMeta.thumbnailUrl,
  });
  btnDownload.disabled = false;
  if (res.error) { alert(res.error); return; }
  preview.hidden = true;
  urlInput.value = '';
  pendingMeta = null;
  jobs.unshift(res);
  renderJobs();
  connectWS(res.id);
});

// ============================================================
// Playlist modal
// ============================================================
function openPlaylistModal(items) {
  renderFQChips('fq-chips-playlist', fqSelections['fq-chips-playlist']);
  playlistItems$.innerHTML = items.map((item, i) => `
    <label class="playlist-item">
      <input type="checkbox" class="pl-chk" data-index="${i}" checked/>
      <img class="playlist-item-thumb" src="${escHtml(item.thumbnail || '')}" alt="" loading="lazy"
           onerror="this.style.display='none'"/>
      <span class="playlist-item-title">${escHtml(item.title || item.id)}</span>
    </label>
  `).join('');
  playlistCount.textContent = items.length + ' videos';
  const big = items.length > 50;
  playlistWarn.hidden = !big;
  if (big) playlistWarn.textContent = t('playlist.warning', { n: items.length });
  chkSelectAll.checked = true;
  playlistModal.hidden = false;
}

btnClosePl.addEventListener('click', () => { playlistModal.hidden = true; });
playlistModal.addEventListener('click', e => { if (e.target === playlistModal) playlistModal.hidden = true; });
chkSelectAll.addEventListener('change', () => {
  document.querySelectorAll('.pl-chk').forEach(c => { c.checked = chkSelectAll.checked; });
});

btnDlPlaylist.addEventListener('click', async () => {
  const checked = [...document.querySelectorAll('.pl-chk:checked')].map(c => parseInt(c.dataset.index));
  if (!checked.length) { alert(t('playlist.select_all')); return; }
  btnDlPlaylist.disabled = true;
  const fq = getFQ('fq-chips-playlist');
  for (const idx of checked) {
    const item = playlistItems[idx];
    const res = await api('POST', '/api/download', {
      url: item.url, format: fq.format, quality: fq.quality,
      title: item.title, thumbnailUrl: item.thumbnail,
    });
    if (!res.error) { jobs.unshift(res); connectWS(res.id); }
  }
  btnDlPlaylist.disabled = false;
  playlistModal.hidden = true;
  urlInput.value = '';
  renderJobs();
});

// ============================================================
// WebSocket per job
// ============================================================
const wsMap = {};

function connectWS(jobId) {
  if (wsMap[jobId]) return;
  const ws = new WebSocket(`ws://${location.host}/ws/${jobId}`);
  wsMap[jobId] = ws;
  ws.onmessage = (e) => {
    const msg = JSON.parse(e.data);
    const job = jobs.find(j => j.id === jobId);
    if (!job) return;
    if (msg.type === 'progress') {
      job.progress = msg.percent; job.speed = msg.speed;
      job.eta = msg.eta; job.status = 'running';
    } else if (msg.type === 'done') {
      job.status = 'done'; job.progress = 100;
      job.outputPath = msg.outputPath; job.fileSize = msg.fileSize || 0;
      ws.close();
    } else if (msg.type === 'error') {
      job.status = 'error'; job.error = msg.message; ws.close();
    } else if (msg.type === 'status') {
      job.status = msg.status;
    }
    updateJobCard(job);
  };
  ws.onclose = () => { delete wsMap[jobId]; };
}

// ============================================================
// Render Jobs
// ============================================================
function renderJobs() {
  const active = jobs.filter(j => ['pending','running'].includes(j.status));
  queueCount.textContent = active.length;

  if (!jobs.length) {
    jobList.innerHTML = '';
    jobList.appendChild(emptyState);
    return;
  }
  emptyState.remove();
  jobList.innerHTML = jobs.map(jobCardHTML).join('');
  attachCancelBtns();
}

function updateJobCard(job) {
  const card = document.getElementById('card-' + job.id);
  if (!card) { renderJobs(); return; }
  card.outerHTML = jobCardHTML(job);
  attachCancelBtns();
  queueCount.textContent = jobs.filter(j => ['pending','running'].includes(j.status)).length;
}

function attachCancelBtns() {
  document.querySelectorAll('.btn-cancel-job').forEach(btn => {
    btn.addEventListener('click', async () => {
      const id = btn.dataset.jobid;
      await api('DELETE', `/api/jobs/${id}`);
      const job = jobs.find(j => j.id === id);
      if (job) { job.status = 'cancelled'; updateJobCard(job); }
    });
  });
}

function jobCardHTML(job) {
  const canCancel = job.status === 'pending' || job.status === 'running';
  const pct = Math.round(job.progress || 0);
  const fqOpt = FORMAT_OPTIONS.find(o => o.format === job.format && o.quality === job.quality);
  const fqLabel = fqOpt ? t(fqOpt.i18n) : `${(job.format||'').toUpperCase()} · ${job.quality||''}`;

  const statusLabel = {
    pending:   `<span class="status-dot pending"></span>${t('status.pending')}`,
    running:   `<span class="status-dot running"></span>${pct}%`,
    done:      `<span class="status-dot done"></span>${t('status.done')}`,
    error:     `<span class="status-dot error"></span>${t('status.error')}`,
    cancelled: `<span class="status-dot cancelled"></span>${t('status.cancelled')}`,
  }[job.status] || job.status;

  let detailsRow = '';
  if (job.status === 'running') {
    detailsRow = `
      <div class="job-detail-row">
        <span class="detail-item">${escHtml(job.speed||'…')} · ETA ${escHtml(job.eta||'…')}</span>
      </div>
      <div class="progress-bar-wrap"><div class="progress-bar" style="width:${pct}%"></div></div>`;
  } else if (job.status === 'pending') {
    detailsRow = `<div class="progress-bar-wrap"><div class="progress-bar pending-bar" style="width:100%"></div></div>`;
  } else if (job.status === 'done') {
    const fname = job.outputPath ? job.outputPath.split('/').pop() : '';
    const sz = job.fileSize ? formatBytes(job.fileSize) : '';
    detailsRow = `
      <div class="job-detail-row">
        ${sz ? `<span class="detail-item detail-size">${sz}</span>` : ''}
        ${fname ? `<span class="detail-item detail-path" title="${escHtml(job.outputPath)}">${escHtml(fname)}</span>` : ''}
      </div>`;
  } else if (job.status === 'error') {
    detailsRow = `<div class="job-detail-row"><span class="detail-item detail-error" title="${escHtml(job.error||'')}">${escHtml((job.error||'').slice(0,90))}</span></div>`;
  }

  return `
  <div class="job-card ${job.status}" id="card-${job.id}">
    <div class="job-thumb-wrap">
      <img class="job-thumb" src="${escHtml(job.thumbnailUrl||'')}" alt="" loading="lazy"
           onerror="this.style.display='none'"/>
    </div>
    <div class="job-body">
      <div class="job-title" title="${escHtml(job.title||job.url)}">${escHtml(job.title||job.url)}</div>
      <div class="job-meta">
        <span class="fq-badge ${job.format}">${fqLabel}</span>
        <span class="status-text">${statusLabel}</span>
      </div>
      ${detailsRow}
    </div>
    ${canCancel ? `<div class="job-actions"><button class="btn-cancel-job" data-jobid="${job.id}" data-i18n="add.cancel">Cancel</button></div>` : ''}
  </div>`;
}

// ============================================================
// History
// ============================================================
async function loadHistory() {
  const res = await api('GET', '/api/history');
  if (Array.isArray(res)) { historyItems = res; renderHistory(); }
}

function renderHistory() {
  if (!historyItems.length) {
    historyList.innerHTML = `<div class="empty-state"><p>${t('history.empty')}</p></div>`;
    return;
  }
  historyList.innerHTML = historyItems.map(e => {
    const fname = e.outputPath ? e.outputPath.split('/').pop() : '';
    const sz = e.fileSize ? formatBytes(e.fileSize) : '';
    const fqOpt = FORMAT_OPTIONS.find(o => o.format === e.format && o.quality === e.quality);
    const fqLabel = fqOpt ? t(fqOpt.i18n) : `${(e.format||'').toUpperCase()} · ${e.quality||''}`;
    return `
    <div class="history-card">
      <div class="history-thumb-wrap">
        <img class="history-thumb" src="${escHtml(e.thumbnailUrl||'')}" alt="" loading="lazy"
             onerror="this.style.display='none'"/>
      </div>
      <div class="history-info">
        <div class="history-title" title="${escHtml(e.title||e.url)}">${escHtml(e.title||e.url)}</div>
        <div class="history-meta">
          <span class="fq-badge ${e.format}">${fqLabel}</span>
          ${sz ? `<span class="detail-size">${sz}</span>` : ''}
          <span class="history-date">${formatDate(e.completedAt)}</span>
        </div>
        ${fname ? `<div class="history-path" title="${escHtml(e.outputPath)}">${escHtml(fname)}</div>` : ''}
      </div>
    </div>`;
  }).join('');
}

btnClearHist.addEventListener('click', async () => {
  if (!confirm(t('history.confirm_clear'))) return;
  await api('DELETE', '/api/history');
  historyItems = [];
  renderHistory();
});

// ============================================================
// Settings
// ============================================================
const sOutputDir     = document.getElementById('s-output-dir');
const sConcurrency   = document.getElementById('s-concurrency');
const sEmbedThumb    = document.getElementById('s-embed-thumbnail');
const sLanguage      = document.getElementById('s-language');
const siPort         = document.getElementById('si-port');
const siYtdlp        = document.getElementById('si-ytdlp');
const siFfmpeg       = document.getElementById('si-ffmpeg');
const sFfmpegStatus  = document.getElementById('s-ffmpeg-status');
const sActiveInfo    = document.getElementById('s-active-info');
// Output dir change controls
const dirDisplay     = document.getElementById('dir-display');
const dirPathText    = document.getElementById('dir-path-text');
const btnChangeDir   = document.getElementById('btn-change-dir');
const dirEdit        = document.getElementById('dir-edit');
const btnApplyDir    = document.getElementById('btn-apply-dir');
const btnCancelDir   = document.getElementById('btn-cancel-dir');
// Log dir change controls
const logdirDisplay    = document.getElementById('logdir-display');
const logdirPathText   = document.getElementById('logdir-path-text');
const btnChangeLogdir  = document.getElementById('btn-change-logdir');
const logdirEdit       = document.getElementById('logdir-edit');
const sLogDir          = document.getElementById('s-log-dir');
const btnApplyLogdir   = document.getElementById('btn-apply-logdir');
const btnCancelLogdir  = document.getElementById('btn-cancel-logdir');
const sLogRetention    = document.getElementById('s-log-retention');

// ---- Output folder change button wiring ----
btnChangeDir.addEventListener('click', async () => {
  const picked = await pickDir();
  if (picked === null) {
    // Picker not supported or cancelled — fall back to text input
    sOutputDir.value = dirPathText.textContent === '—' ? '' : dirPathText.textContent;
    dirDisplay.hidden = true;
    dirEdit.hidden = false;
    sOutputDir.focus();
    sOutputDir.select();
    return;
  }
  if (!picked) return; // cancelled, do nothing
  const res = await api('POST', '/api/settings', { outputDir: picked });
  if (res.error) { showToast(res.error, 'error'); return; }
  dirPathText.textContent = res.outputDir || picked;
  showToast(t('settings.dir_applied'), 'success');
});

btnCancelDir.addEventListener('click', () => {
  dirEdit.hidden = true;
  dirDisplay.hidden = false;
});

btnApplyDir.addEventListener('click', async () => {
  const newDir = sOutputDir.value.trim();
  if (!newDir) return;
  btnApplyDir.disabled = true;
  const res = await api('POST', '/api/settings', { outputDir: newDir });
  btnApplyDir.disabled = false;
  if (res.error) { showToast(res.error, 'error'); return; }
  dirPathText.textContent = res.outputDir || newDir;
  dirEdit.hidden = true;
  dirDisplay.hidden = false;
  showToast(t('settings.dir_applied'), 'success');
});

// ---- /end output dir ----

// ---- Log dir change button wiring ----
btnChangeLogdir.addEventListener('click', async () => {
  const picked = await pickDir();
  if (picked === null) {
    // Fallback to text input
    sLogDir.value = logdirPathText.textContent === '—' ? '' : logdirPathText.textContent;
    logdirDisplay.hidden = true;
    logdirEdit.hidden = false;
    sLogDir.focus();
    sLogDir.select();
    return;
  }
  if (!picked) return;
  const res = await api('POST', '/api/settings', { logDir: picked });
  if (res.error) { showToast(res.error, 'error'); return; }
  logdirPathText.textContent = res.logDir || picked;
  showToast(t('settings.log_dir_applied'), 'success');
});

btnCancelLogdir.addEventListener('click', () => {
  logdirEdit.hidden = true;
  logdirDisplay.hidden = false;
});

btnApplyLogdir.addEventListener('click', async () => {
  const newDir = sLogDir.value.trim();
  if (!newDir) return;
  btnApplyLogdir.disabled = true;
  const res = await api('POST', '/api/settings', { logDir: newDir });
  btnApplyLogdir.disabled = false;
  if (res.error) { showToast(res.error, 'error'); return; }
  logdirPathText.textContent = res.logDir || newDir;
  logdirEdit.hidden = true;
  logdirDisplay.hidden = false;
  showToast(t('settings.log_dir_applied'), 'success');
});
// ---- /end log dir ----

async function loadSettings() {
  const res = await api('GET', '/api/settings');
  if (res.error) return;

  dirPathText.textContent    = res.outputDir || '—';
  sOutputDir.value           = res.outputDir || '';
  sConcurrency.value         = res.maxConcurrent || 3;
  sConcurrency.dataset.saved = res.maxConcurrent || 3;
  sEmbedThumb.checked = !!res.embedThumbnail;
  sLanguage.value    = res.language || 'en';
  logdirPathText.textContent  = res.logDir || '—';
  sLogDir.value               = res.logDir || '';
  sLogRetention.value         = res.logRetentionDays || 15;
  sLogRetention.dataset.saved = res.logRetentionDays || 15;

  // Default FQ chip in settings
  const defId = FORMAT_OPTIONS.find(o => o.format === res.defaultFormat && o.quality === res.defaultQuality)?.id || 'mp3-320k';
  fqSelections['fq-chips-default'] = defId;
  renderFQChips('fq-chips-default', defId);

  // Active thread info
  if (sActiveInfo) sActiveInfo.textContent = t('settings.active_info', { active: res.activeDL ?? 0, max: res.maxDL ?? res.maxConcurrent ?? '?' });

  // System info
  siPort.textContent = res.port ? `:${res.port}` : '—';
  siYtdlp.textContent = t(res.hasYtdlp ? 'sys.found' : 'sys.not_found');
  siYtdlp.className  = 'sysinfo-value ' + (res.hasYtdlp ? 'ok' : 'warn');
  siFfmpeg.textContent = t(res.hasFFmpeg ? 'sys.found' : 'sys.not_found');
  siFfmpeg.className = 'sysinfo-value ' + (res.hasFFmpeg ? 'ok' : 'warn');

  if (res.hasFFmpeg) {
    sFfmpegStatus.textContent = t('settings.ffmpeg_found');
    sEmbedThumb.disabled = false;
  } else {
    sFfmpegStatus.textContent = t('settings.ffmpeg_missing');
    sEmbedThumb.disabled = true;
    sEmbedThumb.checked = false;
  }

  // Apply language to UI if loaded from server
  if (res.language && res.language !== (locale._lang || 'en')) {
    await loadLocale(res.language);
    locale._lang = res.language;
  }
}

// ---- Auto-save helpers ----
async function saveSetting(payload, toastMsg) {
  const res = await api('POST', '/api/settings', payload);
  if (res.error) { showToast(res.error, 'error'); return null; }
  if (toastMsg) showToast(toastMsg, 'success', 2500);
  return res;
}

// Concurrent downloads → save on change
sConcurrency.addEventListener('change', async () => {
  const n = Math.max(1, parseInt(sConcurrency.value) || 1);
  sConcurrency.value = n;
  const res = await saveSetting({ maxConcurrent: n }, t('settings.threads_updated', { n }));
  if (res && sActiveInfo) {
    sActiveInfo.textContent = t('settings.active_info', { active: res.activeDL ?? 0, max: n });
  }
});

// Embed thumbnail → save on toggle
sEmbedThumb.addEventListener('change', async () => {
  await saveSetting({ embedThumbnail: sEmbedThumb.checked });
});

// Language → save + apply immediately
sLanguage.addEventListener('change', async () => {
  locale._lang = sLanguage.value;
  await loadLocale(sLanguage.value);
  await saveSetting({ language: sLanguage.value });
});

// Log retention → save on change
sLogRetention.addEventListener('change', async () => {
  const n = Math.max(1, parseInt(sLogRetention.value) || 15);
  sLogRetention.value = n;
  await saveSetting({ logRetentionDays: n }, t('settings.retention_updated', { n }));
});

// ============================================================
// Init
// ============================================================
async function init() {
  // Load locale first (must happen before any renderFQChips call)
  const settingsRes = await api('GET', '/api/settings');
  const lang = settingsRes.language || 'en';
  locale._lang = lang;
  await loadLocale(lang);

  // Restore active tab AFTER locale loaded so chips render with correct labels
  const hash = location.hash.slice(1);
  if (['downloads', 'history', 'settings'].includes(hash)) switchTab(hash, false);

  // Dependency warnings
  document.getElementById('dep-warn-ytdlp').hidden  = !!settingsRes.hasYtdlp;
  document.getElementById('dep-warn-ffmpeg').hidden = !!settingsRes.hasFFmpeg;

  // Set default FQ chip from settings
  const defId = FORMAT_OPTIONS.find(o => o.format === settingsRes.defaultFormat && o.quality === settingsRes.defaultQuality)?.id || 'mp3-320k';
  fqSelections['fq-chips'] = defId;
  fqSelections['fq-chips-default'] = defId;
  fqSelections['fq-chips-playlist'] = defId;

  // Load jobs
  const res = await api('GET', '/api/jobs');
  if (Array.isArray(res)) {
    jobs = res;
    renderJobs();
    jobs.forEach(j => { if (j.status === 'running' || j.status === 'pending') connectWS(j.id); });
  }
}

// ============================================================
// Helpers
// ============================================================
async function api(method, path, body) {
  const opts = { method, headers: { 'Content-Type': 'application/json' } };
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch(path, opts);
  if (res.status === 204) return {};
  return res.json().catch(() => ({}));
}

function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function formatDate(iso) {
  if (!iso) return '';
  try { return new Date(iso).toLocaleString(); } catch { return iso; }
}

function formatBytes(b) {
  if (!b) return '';
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b/1024).toFixed(1) + ' KB';
  if (b < 1073741824) return (b/1048576).toFixed(1) + ' MB';
  return (b/1073741824).toFixed(2) + ' GB';
}

function spinnerEl() {
  const d = document.createElement('span');
  d.className = 'spinner';
  return d;
}

// ---- Native folder picker ----
// Returns selected path string, empty string if cancelled, null if not supported.
async function pickDir() {
  const res = await api('GET', '/api/pick-dir');
  if (res.error === 'cancelled') return '';   // user cancelled picker
  if (res.error) return null;                 // not supported → fall back to text input
  return res.path || null;
}

// ---- Toast notification ----
function showToast(msg, type = 'success', duration = 3000) {
  const el = document.createElement('div');
  el.className = `toast toast-${type}`;
  el.textContent = msg;
  document.body.appendChild(el);
  // Trigger animation
  requestAnimationFrame(() => el.classList.add('toast-show'));
  setTimeout(() => {
    el.classList.remove('toast-show');
    el.addEventListener('transitionend', () => el.remove());
  }, duration);
}

init();
