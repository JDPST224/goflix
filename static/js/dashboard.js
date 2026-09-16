// GoFlix admin console.
//
// Drives the dashboard shell: live stream monitoring, bandwidth charts,
// account management and instance health. The server gates this page
// (non-admins are redirected); the status check in init() is a client-side
// safety net only.

'use strict';

// ─── Element handles ────────────────────────────────────────────────────────

const $ = (id) => document.getElementById(id);

const el = {
    rail:            $('rail'),
    railToggle:      $('rail-toggle'),
    railUsername:    $('rail-username'),
    railAvatar:      $('rail-avatar'),
    logoutBtn:       $('logout-btn'),
    crumb:           $('crumb-current'),
    refreshBtn:      $('refresh-btn'),

    kpiStreams:      $('kpi-streams'),
    kpiStreamsNote:  $('kpi-streams-note'),
    kpiBandwidth:    $('kpi-bandwidth'),
    kpiBandwidthNote:$('kpi-bandwidth-note'),
    kpiAccounts:     $('kpi-accounts'),
    kpiAccountsNote: $('kpi-accounts-note'),
    kpiSessions:     $('kpi-sessions'),
    kpiSessionsNote: $('kpi-sessions-note'),

    streamsBody:     $('streams-body'),
    streamsPill:     $('streams-pill'),
    streamSearch:    $('stream-search'),
    streamFilter:    $('stream-filter'),
    blockedStrip:    $('blocked-strip'),
    navCountStreams: $('nav-count-streams'),

    chartOverall:    $('chart-overall'),
    chartPerIP:      $('chart-perip'),
    tipOverall:      $('tip-overall'),
    tipPerIP:        $('tip-perip'),
    overallNow:      $('overall-now'),
    overallFoot:     $('overall-foot'),
    ipSelect:        $('ip-select'),
    peripLegend:     $('perip-legend'),

    usersBody:       $('users-body'),
    usersRefresh:    $('users-refresh'),
    userSearch:      $('user-search'),
    accountsPill:    $('accounts-pill'),
    navCountAccounts:$('nav-count-accounts'),

    cacheGrid:       $('cache-grid'),
    cacheHitrate:    $('cache-hitrate'),
    instanceGrid:    $('instance-grid'),

    toastStack:      $('toast-stack'),
    confirmVeil:     $('confirm-veil'),
    confirmTitle:    $('confirm-title'),
    confirmText:     $('confirm-text'),
    confirmOk:       $('confirm-ok'),
    confirmCancel:   $('confirm-cancel'),
};

const DEFAULT_AVATAR = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='12' fill='%23e50914'/%3E%3Ctext x='32' y='45' font-family='Arial, sans-serif' font-size='38' font-weight='900' fill='%23ffffff' text-anchor='middle'%3EG%3C/text%3E%3C/svg%3E";

const SERIES_COLORS = ['#e50914', '#4f9cff', '#2fc57a', '#f0b429', '#a855f7', '#06b6d4', '#ec4899', '#84cc16'];

const POLL_MS = 2000;

// ─── State ──────────────────────────────────────────────────────────────────

const state = {
    autoRefresh: true,
    timer: null,
    inflight: false,

    streams: [],
    blocked: [],
    users: [],
    points: [],
    bucketSeconds: 5,

    selectedIP: '',
    selectedToken: '',
    expandedIPs: new Set(),
    statusFilter: 'all',
    streamQuery: '',
    userQuery: '',

    prevBytes: new Map(),   // token -> { bytes, t }, for throughput deltas
    speeds: new Map(),      // token -> bytes/sec
    healthAt: 0,
};

// ─── Small helpers ──────────────────────────────────────────────────────────

function escapeHtml(v) {
    return String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
}

function fmtBytes(n) {
    const v = Number(n) || 0;
    if (v < 1024) return v + ' B';
    const units = ['KB', 'MB', 'GB', 'TB'];
    let out = v;
    for (const u of units) {
        out /= 1024;
        if (out < 1024 || u === 'TB') return out.toFixed(out < 10 ? 2 : 1) + ' ' + u;
    }
    return v + ' B';
}

// Bytes per second → human bit rate.
function fmtSpeed(bytesPerSec) {
    return fmtMbps((Number(bytesPerSec) || 0) * 8 / 1e6);
}

// Already-Mbps → human bit rate. (The chart series are computed in Mbps, so
// they must not be run through fmtSpeed a second time.)
function fmtMbps(mbps) {
    const v = Number(mbps) || 0;
    if (v >= 1000) return (v / 1000).toFixed(2) + ' Gbps';
    if (v >= 1) return v.toFixed(1) + ' Mbps';
    return (v * 1000).toFixed(0) + ' Kbps';
}

function fmtAge(ms) {
    const s = Math.max(0, Math.floor(ms / 1000));
    if (s < 60) return s + 's';
    const m = Math.floor(s / 60);
    if (m < 60) return m + 'm ' + (s % 60) + 's';
    const h = Math.floor(m / 60);
    return h + 'h ' + (m % 60) + 'm';
}

function fmtUptime(seconds) {
    const s = Math.max(0, Math.floor(Number(seconds) || 0));
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    if (d) return `${d}d ${h}h`;
    if (h) return `${h}h ${m}m`;
    return `${m}m ${s % 60}s`;
}

function fmtCount(n) {
    return (Number(n) || 0).toLocaleString();
}

function plural(n, one, many) {
    return `${n} ${n === 1 ? one : many || one + 's'}`;
}

// ─── Toasts ─────────────────────────────────────────────────────────────────

const TOAST_ICONS = {
    ok: '<svg viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12"/></svg>',
    err: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><line x1="12" y1="8" x2="12" y2="13"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>',
    info: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><line x1="12" y1="16" x2="12" y2="11"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>',
};

function toast(message, kind = 'ok', ttl = 4000) {
    if (!el.toastStack) return;
    const node = document.createElement('div');
    node.className = `toast ${kind}`;
    node.innerHTML = `${TOAST_ICONS[kind] || TOAST_ICONS.info}<span></span>`;
    node.lastElementChild.textContent = message;
    el.toastStack.appendChild(node);
    setTimeout(() => {
        node.classList.add('leaving');
        setTimeout(() => node.remove(), 220);
    }, ttl);
}

// ─── Confirm dialog (replaces window.confirm) ───────────────────────────────

let confirmResolver = null;

function askConfirm(title, text, okLabel = 'Confirm') {
    return new Promise((resolve) => {
        confirmResolver = resolve;
        el.confirmTitle.textContent = title;
        el.confirmText.textContent = text;
        el.confirmOk.textContent = okLabel;
        el.confirmVeil.classList.add('show');
        el.confirmOk.focus();
    });
}

function settleConfirm(value) {
    el.confirmVeil.classList.remove('show');
    if (confirmResolver) {
        const r = confirmResolver;
        confirmResolver = null;
        r(value);
    }
}

el.confirmOk.addEventListener('click', () => settleConfirm(true));
el.confirmCancel.addEventListener('click', () => settleConfirm(false));
el.confirmVeil.addEventListener('mousedown', (e) => { if (e.target === el.confirmVeil) settleConfirm(false); });
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && el.confirmVeil.classList.contains('show')) settleConfirm(false);
});

// ─── Shell: sidebar, scroll-spy, refresh controls ───────────────────────────

const navItems = Array.from(document.querySelectorAll('.nav-item[data-section]'));
const sections = navItems
    .map((item) => document.getElementById(item.dataset.section))
    .filter(Boolean);

function closeRail() {
    document.body.classList.remove('rail-open');
    el.railToggle.setAttribute('aria-expanded', 'false');
}

el.railToggle.addEventListener('click', () => {
    const open = document.body.classList.toggle('rail-open');
    el.railToggle.setAttribute('aria-expanded', String(open));
});
document.addEventListener('click', (e) => {
    if (!document.body.classList.contains('rail-open')) return;
    if (el.rail.contains(e.target) || el.railToggle.contains(e.target)) return;
    closeRail();
});
navItems.forEach((item) => item.addEventListener('click', closeRail));

// Highlight the section nearest the top of the viewport.
function syncActiveSection() {
    const line = window.scrollY + 140;
    let active = sections[0];
    for (const section of sections) {
        if (section.offsetTop <= line) active = section;
    }
    if (!active) return;
    navItems.forEach((item) => item.classList.toggle('active', item.dataset.section === active.id));
    const label = active.querySelector('.page-title, .section-head h2');
    if (label && el.crumb) el.crumb.textContent = label.textContent.trim();
}
window.addEventListener('scroll', syncActiveSection, { passive: true });

function setAutoRefresh(on) {
    state.autoRefresh = on;
    clearInterval(state.timer);
    if (on) {
        state.timer = setInterval(pollStreams, POLL_MS);
        pollStreams();
    }
}


el.refreshBtn.addEventListener('click', async () => {
    el.refreshBtn.classList.add('is-spinning');
    await Promise.all([pollStreams(), loadUsers(), loadHealth()]);
    setTimeout(() => el.refreshBtn.classList.remove('is-spinning'), 350);
});

// Stop hammering the API while the tab is in the background.
document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
        clearInterval(state.timer);
    } else if (state.autoRefresh) {
        state.timer = setInterval(pollStreams, POLL_MS);
        pollStreams();
    }
});

el.logoutBtn.addEventListener('click', async () => {
    try { await fetch('/api/auth/logout', { method: 'POST' }); } catch (_) {}
    try {
        ['goflix_mylist', 'goflix_progress', 'goflix_cw', 'goflix_removed', 'goflix_avprefs', 'goflix_user']
            .forEach((k) => localStorage.removeItem(k));
    } catch (_) {}
    location.href = '/';
});

// ─── Boot ───────────────────────────────────────────────────────────────────

(async function init() {
    let status;
    try {
        status = await fetch('/api/auth/status').then((r) => r.json());
    } catch (_) {
        location.href = '/login';
        return;
    }
    if (!status.authed || !status.user) { location.href = '/login'; return; }
    if (!status.isAdmin) { location.href = '/'; return; }

    el.railUsername.textContent = status.user.username || status.user.email || 'Admin';
    if (status.user.has_avatar || status.hasAvatar) el.railAvatar.src = '/api/auth/avatar';

    renderStreamSkeleton();
    renderUserSkeleton();
    loadUsers();
    loadHealth();
    setAutoRefresh(true);
    syncActiveSection();
})();

// ─── Accounts ───────────────────────────────────────────────────────────────

function renderUserSkeleton(rows = 3) {
    el.usersBody.innerHTML = Array.from({ length: rows }, () => `
        <tr>
            <td><div class="user-cell"><span class="sk circle"></span><span class="sk w-md"></span></div></td>
            <td><span class="sk w-lg"></span></td>
            <td><span class="sk w-sm"></span></td>
            <td class="num"><span class="sk w-sm"></span></td>
            <td class="act"><span class="sk w-md"></span></td>
        </tr>`).join('');
}

async function loadUsers() {
    try {
        const res = await fetch('/api/admin/users');
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || `Request failed (${res.status})`);
        state.users = data.users || [];
        renderUsers();
    } catch (_) {
        el.usersBody.innerHTML = emptyRow(5, 'Could not load accounts', 'The server refused the request or is unreachable.');
        el.kpiAccounts.textContent = '–';
        el.kpiSessions.textContent = '–';
    }
}

function emptyRow(cols, title, detail) {
    return `<tr><td colspan="${cols}">
        <div class="empty-state">
            <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path d="M12 16v-4M12 8h.01"/></svg>
            <strong>${escapeHtml(title)}</strong>
            <span>${escapeHtml(detail)}</span>
        </div>
    </td></tr>`;
}

function renderUsers() {
    const users = state.users;
    const sessions = users.reduce((n, u) => n + (u.sessions || 0), 0);
    const admins = users.filter((u) => u.is_admin).length;

    el.kpiAccounts.textContent = fmtCount(users.length);
    el.kpiAccountsNote.textContent = admins
        ? `${plural(admins, 'administrator')} · ${plural(users.length - admins, 'standard user')}`
        : 'No administrators registered';
    el.kpiSessions.textContent = fmtCount(sessions);
    el.kpiSessionsNote.textContent = sessions
        ? `Across ${plural(users.filter((u) => (u.sessions || 0) > 0).length, 'account')}`
        : 'Nobody signed in right now';
    el.accountsPill.textContent = plural(users.length, 'account');
    el.navCountAccounts.textContent = users.length;

    const q = state.userQuery.trim().toLowerCase();
    const shown = q
        ? users.filter((u) => `${u.username || ''} ${u.email || ''}`.toLowerCase().includes(q))
        : users;

    el.usersBody.innerHTML = '';
    if (!shown.length) {
        el.usersBody.innerHTML = users.length
            ? emptyRow(5, 'No matches', 'No account matches that search.')
            : emptyRow(5, 'No accounts yet', 'The first person to register becomes the administrator.');
        return;
    }

    for (const u of shown) {
        const tr = document.createElement('tr');
        tr.className = 'row-hover';
        const created = u.created_at ? new Date(u.created_at).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : '—';
        const avatar = u.has_avatar ? `/api/admin/users/${encodeURIComponent(u.id)}/avatar` : DEFAULT_AVATAR;
        tr.innerHTML = `
            <td>
                <div class="user-cell">
                    <img class="user-avatar" src="${escapeHtml(avatar)}" alt="" width="30" height="30" loading="lazy">
                    <span class="user-name">${escapeHtml(u.username)}${u.is_admin ? '<span class="badge admin">Admin</span>' : ''}</span>
                </div>
            </td>
            <td class="muted">${u.email ? escapeHtml(u.email) : '—'}</td>
            <td class="muted">${escapeHtml(created)}</td>
            <td class="num">${u.sessions ? `<span class="pill">${u.sessions}</span>` : '<span class="faint">0</span>'}</td>
            <td class="act"><div class="row-actions"></div></td>`;

        const actions = tr.querySelector('.row-actions');

        const signOut = mkButton('Force sign-out', 'btn btn-sm', async () => {
            const ok = await askConfirm('Force sign-out',
                `Sign ${u.username} out of every device? They will need to log in again.`, 'Sign out');
            if (ok) adminAction(`/api/admin/users/${encodeURIComponent(u.id)}/logout`, `Signed ${u.username} out everywhere.`);
        });
        signOut.disabled = !u.sessions;

        const del = mkButton('Delete', 'btn btn-sm btn-danger', async () => {
            const ok = await askConfirm('Delete account',
                `Permanently delete "${u.username}"? Their synced My List, progress and Continue Watching are removed too.`, 'Delete');
            if (ok) adminAction(`/api/admin/users/${encodeURIComponent(u.id)}`, 'Account deleted.', 'DELETE');
        });

        actions.append(signOut, del);
        el.usersBody.appendChild(tr);
    }
}

function mkButton(label, className, onClick) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = className;
    b.textContent = label;
    b.addEventListener('click', (e) => { e.stopPropagation(); onClick(); });
    return b;
}

async function adminAction(url, okText, method = 'POST') {
    try {
        const res = await fetch(url, { method });
        const data = await res.json().catch(() => ({}));
        if (res.ok && data.success) {
            toast(okText, 'ok');
            loadUsers();
        } else {
            toast(data.error || 'Action failed.', 'err');
        }
    } catch (_) {
        toast('Could not reach the server.', 'err');
    }
}

el.usersRefresh.addEventListener('click', () => {
    el.usersRefresh.classList.add('is-spinning');
    loadUsers().finally(() => setTimeout(() => el.usersRefresh.classList.remove('is-spinning'), 350));
});

el.userSearch.addEventListener('input', () => {
    state.userQuery = el.userSearch.value;
    renderUsers();
});

// ─── Instance health ────────────────────────────────────────────────────────

async function loadHealth() {
    let health;
    try {
        health = await fetch('/api/health').then((r) => r.json());
    } catch (_) {
        return;
    }
    state.healthAt = Date.now();

    const cache = health.resolutionCache || {};
    const hits = Number(cache.hits) || 0;
    const misses = Number(cache.misses) || 0;
    const lookups = hits + misses;
    el.cacheHitrate.textContent = lookups ? `${Math.round((hits / lookups) * 100)}% hit rate` : 'No lookups yet';
    el.cacheHitrate.className = lookups && hits / lookups >= 0.5 ? 'pill ok' : 'pill';

    el.cacheGrid.innerHTML = statCells([
        ['Records', fmtCount(cache.records)],
        ['Hits', fmtCount(hits)],
        ['Misses', fmtCount(misses)],
        ['Heals', fmtCount(cache.heals)],
        ['Heal failures', fmtCount(cache.healFailures)],
        ['Prewarms', fmtCount(cache.prewarms)],
    ]);

    const cap = Number(health.maxStreamHeight) || 0;
    el.instanceGrid.innerHTML = statCells([
        ['Status', health.status === 'ok' ? 'Healthy' : String(health.status || 'unknown'), 'sm'],
        ['Uptime', fmtUptime(health.uptimeSeconds), 'sm'],
        ['Movies', fmtCount(health.movies)],
        ['TV shows', fmtCount(health.tvshows)],
        ['Popular', fmtCount(health.popular)],
        ['Quality cap', cap ? `${cap}p` : 'Uncapped', 'sm'],
    ]);
}

function statCells(pairs) {
    return pairs.map(([label, value, mod]) => `
        <div class="stat-cell">
            <div class="stat-label">${escapeHtml(label)}</div>
            <div class="stat-value${mod ? ' ' + mod : ''}">${escapeHtml(value)}</div>
        </div>`).join('');
}

// ─── Live streams ───────────────────────────────────────────────────────────

function renderStreamSkeleton(rows = 3) {
    el.streamsBody.innerHTML = Array.from({ length: rows }, () => `
        <tr>
            <td><span class="sk w-md"></span></td>
            <td><span class="sk w-lg"></span></td>
            <td><span class="sk w-sm"></span></td>
            <td><span class="sk w-sm"></span></td>
            <td class="num"><span class="sk w-sm"></span></td>
            <td class="num"><span class="sk w-sm"></span></td>
            <td><span class="sk w-sm"></span></td>
            <td class="act"><span class="sk w-md"></span></td>
        </tr>`).join('');
}

async function pollStreams() {
    if (state.inflight) return;
    state.inflight = true;
    let streamsData, statsData;
    try {
        [streamsData, statsData] = await Promise.all([
            fetch('/api/admin/streams').then((r) => r.json()),
            fetch('/api/admin/streams/stats').then((r) => r.json()),
        ]);
        if (!streamsData.success || !statsData.success) throw new Error('bad payload');
    } catch (_) {
        state.inflight = false;
        return;
    } finally {
        state.inflight = false;
    }

    state.streams = streamsData.streams || [];
    state.blocked = streamsData.blocked || [];
    state.points = statsData.points || [];
    state.bucketSeconds = statsData.bucket_seconds || 5;

    computeSpeeds();

    try {
        renderStreams();
        renderCharts();
        renderKPIs();
    } catch (err) {
        // A rendering fault must never kill the monitor loop.
        console.error('dashboard render failed:', err);
    }

    // Refresh the slower health panel roughly every 15 seconds.
    if (Date.now() - state.healthAt > 15000) loadHealth();
}

// Throughput comes from the byte delta between polls.
function computeSpeeds() {
    const now = Date.now();
    state.speeds = new Map();
    for (const s of state.streams) {
        const prev = state.prevBytes.get(s.token);
        if (prev && s.bytes >= prev.bytes) {
            const dt = (now - prev.t) / 1000;
            if (dt > 0.4) state.speeds.set(s.token, (s.bytes - prev.bytes) / dt);
        }
        state.prevBytes.set(s.token, { bytes: s.bytes, t: now });
    }
    for (const token of Array.from(state.prevBytes.keys())) {
        if (!state.streams.some((s) => s.token === token)) state.prevBytes.delete(token);
    }
}

function visibleStreams() {
    return state.streams.filter((s) => !s.expired);
}

function activeStreams() {
    return visibleStreams().filter((s) => s.active && !s.paused);
}

function matchesFilters(s) {
    if (state.statusFilter === 'live' && !(s.active && !s.paused)) return false;
    if (state.statusFilter === 'paused' && !s.paused) return false;
    const q = state.streamQuery.trim().toLowerCase();
    if (!q) return true;
    const haystack = `${s.ip || ''} ${s.provider || ''} ${s.media_type || ''} ${s.media_id || ''}`.toLowerCase();
    return haystack.includes(q);
}

function resLabel(s) {
    const w = s.width || 0, h = s.height || 0;
    if (!w && !h) return null;
    // Tier by width first so cinemascope encodes (1920x800) still read as
    // their marketing tier; fall back to the raw height.
    if (w >= 7680 || h >= 4320) return '8K';
    if (w >= 3800 || h >= 2160) return '4K';
    if (w >= 2500 || h >= 1440) return '1440p';
    if (w >= 1800 || h >= 1080) return '1080p';
    if (w >= 1200 || h >= 720) return '720p';
    if (h >= 480) return '480p';
    return h + 'p';
}

function statusBadge(s) {
    if (s.paused) return '<span class="badge paused"><span class="badge-dot"></span>Paused</span>';
    if (s.active) return '<span class="badge live"><span class="badge-dot"></span>Streaming</span>';
    return '<span class="badge idle"><span class="badge-dot"></span>Idle</span>';
}

function titleCell(s) {
    const kind = s.media_type === 'tv' ? 'TV' : 'Movie';
    const ep = s.media_type === 'tv' ? ` · S${s.season || '?'}E${s.episode || '?'}` : '';
    return `<div class="title-cell">
        <span class="title-main">${escapeHtml(kind)}${escapeHtml(ep)}</span>
        <span class="title-sub">${escapeHtml(s.provider || 'unknown')} · TMDB ${escapeHtml(s.media_id || '—')}</span>
    </div>`;
}

const CHEV_SVG = '<svg viewBox="0 0 24 24"><polyline points="9 18 15 12 9 6"/></svg>';

function streamRow(s, opts = {}) {
    const now = Date.now();
    const tr = document.createElement('tr');
    tr.className = 'row-hover';
    if (s.token === state.selectedToken) tr.classList.add('selected');
    if (!s.active && !s.paused) tr.classList.add('is-idle');
    if (opts.child) tr.classList.add('stream-child');

    const ip = s.ip || 'unknown';
    const started = new Date(s.started_at);
    const res = resLabel(s);
    const speed = state.speeds.get(s.token);

    const viewer = opts.child
        ? `<span class="child-label">${CHEV_SVG} Device ${opts.n}</span>`
        : `<span class="mono">${escapeHtml(ip)}</span>`;

    tr.innerHTML = `
        <td><div class="ip-cell">${viewer}</div></td>
        <td>${titleCell(s)}</td>
        <td>${res ? `<span class="res-tag${/4K|8K|1440/.test(res) ? ' hi' : ''}">${escapeHtml(res)}</span>` : '<span class="faint">—</span>'}</td>
        <td><div class="title-cell">
            <span class="title-main" style="font-weight:500">${started.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
            <span class="title-sub">${escapeHtml(fmtAge(now - started.getTime()))} ago</span>
        </div></td>
        <td class="num">${fmtBytes(s.bytes)}</td>
        <td class="num">${speed != null ? fmtSpeed(speed) : '<span class="faint">—</span>'}</td>
        <td>${statusBadge(s)}</td>
        <td class="act"><div class="row-actions"></div></td>`;

    tr.addEventListener('click', (e) => {
        if (e.target.closest('button')) return;
        state.selectedIP = ip;
        state.selectedToken = s.token;
        syncIPSelect();
        el.streamsBody.querySelectorAll('tr.selected').forEach((x) => x.classList.remove('selected'));
        tr.classList.add('selected');
        renderCharts();
    });

    const actions = tr.querySelector('.row-actions');
    actions.appendChild(mkButton(s.paused ? 'Resume' : 'Pause', 'btn btn-sm', () => {
        streamAction(`/api/admin/streams/${s.token}/${s.paused ? 'resume' : 'pause'}`,
            s.paused ? 'Stream resumed.' : 'Stream paused.');
    }));
    actions.appendChild(mkButton('Stop', 'btn btn-sm btn-danger', async () => {
        const ok = await askConfirm('Stop stream', `Stop this playback session on ${ip}? The viewer's player will halt.`, 'Stop');
        if (ok) streamAction(`/api/admin/streams/${s.token}/stop`, 'Stream stopped.');
    }));
    if (opts.block) actions.appendChild(blockButton(ip, s.token));

    return tr;
}

function blockButton(ip, token) {
    return mkButton('Block IP', 'btn btn-sm btn-danger', async () => {
        const ok = await askConfirm('Block address',
            `Block ${ip} from streaming? Every current session stops and new ones are refused until you unblock it.`, 'Block');
        if (ok) streamAction(`/api/admin/streams/${token}/block`, `${ip} blocked.`);
    });
}

function renderStreams() {
    const visible = visibleStreams();
    const filtered = visible.filter(matchesFilters);

    el.streamsBody.innerHTML = '';
    if (!filtered.length) {
        el.streamsBody.innerHTML = visible.length
            ? emptyRow(8, 'No matching sessions', 'Try a different search term or status filter.')
            : emptyRow(8, 'No active streams', 'Sessions appear here the moment someone starts playback.');
    }

    // Group by address: a lone viewer is one row; a household with several
    // devices collapses into a summary row that expands on demand.
    const groups = new Map();
    for (const s of filtered) {
        const ip = s.ip || 'unknown';
        if (!groups.has(ip)) groups.set(ip, []);
        groups.get(ip).push(s);
    }

    for (const [ip, list] of groups) {
        if (list.length === 1) {
            el.streamsBody.appendChild(streamRow(list[0], { block: true }));
            continue;
        }
        const expanded = state.expandedIPs.has(ip);
        const allPaused = list.every((s) => s.paused);
        const anyPaused = list.some((s) => s.paused);
        const status = allPaused
            ? '<span class="badge paused"><span class="badge-dot"></span>Paused</span>'
            : anyPaused
                ? '<span class="badge mixed"><span class="badge-dot"></span>Mixed</span>'
                : '<span class="badge live"><span class="badge-dot"></span>Streaming</span>';
        const totalBytes = list.reduce((a, s) => a + (s.bytes || 0), 0);
        const totalSpeed = list.reduce((a, s) => a + (state.speeds.get(s.token) || 0), 0);
        const oldest = list.reduce((a, s) => (s.started_at < a ? s.started_at : a), list[0].started_at);

        const tr = document.createElement('tr');
        tr.className = 'stream-group row-hover';
        tr.innerHTML = `
            <td><div class="ip-cell">
                <button type="button" class="chev${expanded ? ' open' : ''}" aria-label="Toggle devices" aria-expanded="${expanded}">${CHEV_SVG}</button>
                <span class="mono">${escapeHtml(ip)}</span>
            </div></td>
            <td><span class="pill">${plural(list.length, 'device')}</span></td>
            <td><span class="faint">—</span></td>
            <td class="muted">${new Date(oldest).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</td>
            <td class="num">${fmtBytes(totalBytes)}</td>
            <td class="num">${totalSpeed > 0 ? fmtSpeed(totalSpeed) : '<span class="faint">—</span>'}</td>
            <td>${status}</td>
            <td class="act"><div class="row-actions"></div></td>`;

        tr.addEventListener('click', (e) => {
            if (e.target.closest('.row-actions')) return;
            if (state.expandedIPs.has(ip)) state.expandedIPs.delete(ip);
            else state.expandedIPs.add(ip);
            renderStreams();
        });

        tr.querySelector('.row-actions').appendChild(blockButton(ip, list[0].token));
        el.streamsBody.appendChild(tr);

        if (expanded) {
            list.forEach((s, i) => el.streamsBody.appendChild(streamRow(s, { child: true, n: i + 1 })));
        }
    }

    renderBlocked();
    syncIPSelect();

    const active = activeStreams().length;
    el.streamsPill.textContent = filtered.length === visible.length
        ? `${plural(active, 'streaming session')}`
        : `${filtered.length} of ${visible.length} shown`;
    el.navCountStreams.textContent = active;
}

function renderBlocked() {
    el.blockedStrip.innerHTML = '';
    if (!state.blocked.length) return;

    const label = document.createElement('span');
    label.className = 'blocked-label';
    label.innerHTML = '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><line x1="5" y1="5" x2="19" y2="19"/></svg>Blocked addresses';
    el.blockedStrip.appendChild(label);

    for (const ip of state.blocked) {
        const chip = document.createElement('span');
        chip.className = 'blocked-chip';
        chip.append(ip);
        chip.appendChild(mkButton('Unblock', 'btn btn-sm', async () => {
            try {
                await fetch(`/api/admin/streams/unblock?ip=${encodeURIComponent(ip)}`, { method: 'POST' });
                toast(`${ip} unblocked.`, 'ok');
            } catch (_) {
                toast('Could not reach the server.', 'err');
            }
            pollStreams();
        }));
        el.blockedStrip.appendChild(chip);
    }
}

async function streamAction(url, okText) {
    try {
        const res = await fetch(url, { method: 'POST' });
        const data = await res.json().catch(() => ({}));
        if (res.ok && data.success) toast(okText, 'ok');
        else toast(data.error || 'Action failed.', 'err');
    } catch (_) {
        toast('Could not reach the server.', 'err');
    }
    pollStreams();
}

el.streamSearch.addEventListener('input', () => {
    state.streamQuery = el.streamSearch.value;
    renderStreams();
});

el.streamFilter.addEventListener('click', (e) => {
    const btn = e.target.closest('button[data-filter]');
    if (!btn) return;
    state.statusFilter = btn.dataset.filter;
    el.streamFilter.querySelectorAll('button').forEach((b) => {
        b.setAttribute('aria-pressed', String(b === btn));
    });
    renderStreams();
});

// Keep the address dropdown in sync without stealing focus mid-selection.
function syncIPSelect() {
    if (document.activeElement === el.ipSelect) return;
    const ips = Array.from(new Set(visibleStreams().map((s) => s.ip || 'unknown'))).sort();
    if (state.selectedIP && !ips.includes(state.selectedIP)) {
        state.selectedIP = '';
        state.selectedToken = '';
    }
    const markup = '<option value="">All addresses</option>' +
        ips.map((ip) => `<option value="${escapeHtml(ip)}">${escapeHtml(ip)}</option>`).join('');
    if (el.ipSelect.innerHTML !== markup) el.ipSelect.innerHTML = markup;
    el.ipSelect.value = state.selectedIP;
}

el.ipSelect.addEventListener('change', () => {
    state.selectedIP = el.ipSelect.value;
    if (!state.selectedIP) state.selectedToken = '';
    renderStreams();
    renderCharts();
});

// ─── KPIs ───────────────────────────────────────────────────────────────────

function renderKPIs() {
    const active = activeStreams();
    const visible = visibleStreams();
    const paused = visible.filter((s) => s.paused).length;

    el.kpiStreams.textContent = fmtCount(active.length);
    const households = new Set(active.map((s) => s.ip || 'unknown')).size;
    el.kpiStreamsNote.textContent = active.length
        ? `${plural(households, 'address', 'addresses')}${paused ? ` · ${paused} paused` : ''}`
        : (paused ? `${plural(paused, 'paused session')}` : 'Nothing playing right now');

    // Prefer the server's own rolling history: it is accurate from the very
    // first poll, whereas client-side byte deltas need two samples to exist.
    const recent = state.points.slice(-6); // last ~30 seconds
    const rate = recent.length
        ? recent.reduce((a, p) => a + p.total, 0) / recent.length / state.bucketSeconds
        : Array.from(state.speeds.values()).reduce((a, v) => a + v, 0);
    el.kpiBandwidth.textContent = fmtSpeed(rate);
    el.kpiBandwidthNote.textContent = active.length
        ? `${fmtSpeed(rate / active.length)} average per stream`
        : 'Idle — no traffic being proxied';

}

// ─── Canvas charts ──────────────────────────────────────────────────────────
// Hand-rolled so the dashboard stays dependency-free: one filled area chart
// for the whole server, one multi-line chart per viewer address, plus tiny
// KPI sparklines. All of them share the DPR setup below.

function setupCanvas(canvas, fallbackHeight) {
    const dpr = window.devicePixelRatio || 1;
    const w = canvas.clientWidth || canvas.parentElement.clientWidth || 300;
    const h = canvas.clientHeight || fallbackHeight || 180;
    if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
        canvas.width = Math.round(w * dpr);
        canvas.height = Math.round(h * dpr);
    }
    const ctx = canvas.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    return { ctx, w, h };
}

function niceMax(v) {
    if (v <= 0.5) return 0.5;
    const pow = Math.pow(10, Math.floor(Math.log10(v)));
    const n = v / pow;
    const step = n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10;
    return step * pow;
}

const PAD = { l: 50, r: 12, t: 14, b: 24 };

function drawGrid(ctx, w, h, max, labels) {
    const steps = 4;
    ctx.font = '10px Inter, sans-serif';
    for (let i = 0; i <= steps; i++) {
        const y = PAD.t + ((h - PAD.t - PAD.b) * i) / steps;
        ctx.beginPath();
        ctx.strokeStyle = i === steps ? 'rgba(255,255,255,0.11)' : 'rgba(255,255,255,0.055)';
        ctx.lineWidth = 1;
        ctx.setLineDash(i === steps ? [] : [3, 5]);
        ctx.moveTo(PAD.l, y + 0.5);
        ctx.lineTo(w - PAD.r, y + 0.5);
        ctx.stroke();

        const v = max * (1 - i / steps);
        ctx.setLineDash([]);
        ctx.fillStyle = 'rgba(255,255,255,0.4)';
        ctx.textAlign = 'right';
        ctx.fillText(v >= 10 ? v.toFixed(0) : v.toFixed(1), PAD.l - 8, y + 3);
    }
    ctx.setLineDash([]);

    // Unit caption on the value axis.
    ctx.fillStyle = 'rgba(255,255,255,0.28)';
    ctx.textAlign = 'left';
    ctx.fillText('Mbps', PAD.l - 44, PAD.t - 3);

    // Time labels along the bottom.
    if (labels && labels.length) {
        ctx.textAlign = 'center';
        ctx.fillStyle = 'rgba(255,255,255,0.38)';
        for (const { x, text } of labels) {
            ctx.fillText(text, Math.min(Math.max(x, PAD.l + 16), w - PAD.r - 16), h - 7);
        }
    }
}

function seriesGeometry(w, h, count, max) {
    const x0 = PAD.l, x1 = w - PAD.r, yBase = h - PAD.b;
    return {
        x: (i) => (count <= 1 ? x1 : x0 + ((x1 - x0) * i) / (count - 1)),
        y: (v) => PAD.t + (yBase - PAD.t) * (1 - Math.min(v, max) / (max || 1)),
        x0, x1, yBase,
    };
}

function drawSeries(ctx, geom, values, color, { fill = false, width = 1.6, dim = false } = {}) {
    if (!values.length) return;
    ctx.save();
    if (dim) ctx.globalAlpha = 0.32;

    const trace = () => {
        ctx.beginPath();
        values.forEach((v, i) => (i === 0 ? ctx.moveTo(geom.x(i), geom.y(v)) : ctx.lineTo(geom.x(i), geom.y(v))));
    };

    if (fill) {
        trace();
        ctx.lineTo(geom.x1, geom.yBase);
        ctx.lineTo(geom.x0, geom.yBase);
        ctx.closePath();
        const grad = ctx.createLinearGradient(0, PAD.t, 0, geom.yBase);
        grad.addColorStop(0, color + '4d');
        grad.addColorStop(1, color + '00');
        ctx.fillStyle = grad;
        ctx.fill();
    }

    trace();
    ctx.strokeStyle = color;
    ctx.lineWidth = width;
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';
    ctx.stroke();
    ctx.restore();
}

function drawCrosshair(ctx, geom, h, index, points) {
    const x = geom.x(index);
    ctx.save();
    ctx.strokeStyle = 'rgba(255,255,255,0.22)';
    ctx.lineWidth = 1;
    ctx.setLineDash([3, 4]);
    ctx.beginPath();
    ctx.moveTo(x, PAD.t);
    ctx.lineTo(x, geom.yBase);
    ctx.stroke();
    ctx.setLineDash([]);
    for (const { value, color } of points) {
        ctx.beginPath();
        ctx.arc(x, geom.y(value), 3.2, 0, Math.PI * 2);
        ctx.fillStyle = color;
        ctx.strokeStyle = '#0c0e13';
        ctx.lineWidth = 2;
        ctx.fill();
        ctx.stroke();
    }
    ctx.restore();
}

// Hover tracking, one entry per chart.
const hover = { overall: null, perip: null };

function bindHover(canvas, key) {
    canvas.addEventListener('mousemove', (e) => {
        const rect = canvas.getBoundingClientRect();
        hover[key] = { x: e.clientX - rect.left, y: e.clientY - rect.top };
        renderCharts();
    });
    canvas.addEventListener('mouseleave', () => {
        hover[key] = null;
        renderCharts();
    });
}
bindHover(el.chartOverall, 'overall');
bindHover(el.chartPerIP, 'perip');

function hoverIndex(key, geom, count, w) {
    const h = hover[key];
    if (!h || count < 2) return -1;
    if (h.x < PAD.l - 6 || h.x > w - PAD.r + 6) return -1;
    const ratio = (h.x - geom.x0) / Math.max(1, geom.x1 - geom.x0);
    return Math.max(0, Math.min(count - 1, Math.round(ratio * (count - 1))));
}

function placeTip(tipEl, canvas, index, geom, rows, timeLabel) {
    if (index < 0) { tipEl.classList.remove('show'); return; }
    tipEl.innerHTML = `<div class="tip-time">${escapeHtml(timeLabel)}</div>` + rows.map((r) => `
        <div class="tip-row">
            <span class="dot" style="background:${escapeHtml(r.color)}"></span>
            <span class="tip-label">${escapeHtml(r.label)}</span>
            <span class="tip-val">${escapeHtml(r.value)}</span>
        </div>`).join('');
    tipEl.classList.add('show');

    const x = geom.x(index);
    const stageW = canvas.clientWidth;
    const tipW = tipEl.offsetWidth;
    const left = x + 14 + tipW > stageW ? x - 14 - tipW : x + 14;
    tipEl.style.left = `${Math.max(4, left)}px`;
    tipEl.style.top = `${Math.max(4, (hover[canvas === el.chartOverall ? 'overall' : 'perip']?.y || 40) - 12)}px`;
}

function timeLabelAt(index) {
    const p = state.points[index];
    if (!p) return '';
    return new Date(p.t * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function renderCharts() {
    const points = state.points;
    if (!points.length) return;
    const bucket = state.bucketSeconds;

    // ── Server throughput ──
    const total = points.map((p) => (p.total * 8) / (bucket * 1e6));
    const peak = Math.max(...total, 0.001);
    const maxOverall = niceMax(peak);

    {
        const { ctx, w, h } = setupCanvas(el.chartOverall, 218);
        const geom = seriesGeometry(w, h, total.length, maxOverall);
        const labels = [0, Math.floor((points.length - 1) / 2), points.length - 1]
            .filter((i, n, arr) => arr.indexOf(i) === n && points[i])
            .map((i) => ({
                x: geom.x(i),
                text: new Date(points[i].t * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
            }));
        drawGrid(ctx, w, h, maxOverall, labels);
        drawSeries(ctx, geom, total, '#e50914', { fill: true, width: 2 });

        const idx = hoverIndex('overall', geom, total.length, w);
        if (idx >= 0) {
            drawCrosshair(ctx, geom, h, idx, [{ value: total[idx], color: '#e50914' }]);
            placeTip(el.tipOverall, el.chartOverall, idx, geom,
                [{ label: 'All viewers', value: fmtMbps(total[idx]), color: '#e50914' }], timeLabelAt(idx));
        } else {
            el.tipOverall.classList.remove('show');
        }

        const recent = points.slice(-6); // last ~30 seconds
        const avgBytesPerSec = recent.reduce((a, p) => a + p.total, 0) / Math.max(1, recent.length) / bucket;
        el.overallNow.textContent = recent.length ? fmtSpeed(avgBytesPerSec) : '–';

        const windowBytes = points.reduce((a, p) => a + p.total, 0);
        el.overallFoot.textContent =
            `Peak ${fmtMbps(peak)} · ${fmtBytes(windowBytes)} served in the last ${Math.round(points.length * bucket / 60)} min`;
    }

    // ── Per-viewer lines ──
    {
        const perIP = new Map();
        points.forEach((p) => {
            for (const [ip, n] of Object.entries(p.per_ip || {})) {
                if (!perIP.has(ip)) perIP.set(ip, []);
                perIP.get(ip).push((n * 8) / (bucket * 1e6));
            }
        });
        // Zero-fill so every line spans the whole window.
        perIP.forEach((vals) => { while (vals.length < points.length) vals.unshift(0); });

        const ranked = Array.from(perIP.entries())
            .sort((a, b) => b[1].reduce((x, y) => x + y, 0) - a[1].reduce((x, y) => x + y, 0));
        const shown = ranked.slice(0, 6);

        const { ctx, w, h } = setupCanvas(el.chartPerIP, 218);

        if (!shown.length) {
            ctx.font = '12px Inter, sans-serif';
            ctx.fillStyle = 'rgba(255,255,255,0.32)';
            ctx.textAlign = 'center';
            ctx.fillText('No per-viewer traffic in this window', w / 2, h / 2);
            el.peripLegend.innerHTML = '';
            el.tipPerIP.classList.remove('show');
            return;
        }

        const maxIP = niceMax(Math.max(...shown.flatMap(([, v]) => v), 0.001));
        const geom = seriesGeometry(w, h, points.length, maxIP);
        const labels = [0, Math.floor((points.length - 1) / 2), points.length - 1]
            .filter((i, n, arr) => arr.indexOf(i) === n && points[i])
            .map((i) => ({
                x: geom.x(i),
                text: new Date(points[i].t * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
            }));
        drawGrid(ctx, w, h, maxIP, labels);

        const colorOf = (ip, i) => (state.selectedIP && ip === state.selectedIP ? '#ffffff' : SERIES_COLORS[i % SERIES_COLORS.length]);

        shown.forEach(([ip, vals], i) => {
            const selected = state.selectedIP && ip === state.selectedIP;
            drawSeries(ctx, geom, vals, colorOf(ip, i), {
                width: selected ? 2.2 : 1.5,
                dim: Boolean(state.selectedIP) && !selected,
            });
        });

        const idx = hoverIndex('perip', geom, points.length, w);
        if (idx >= 0) {
            const rows = shown
                .map(([ip, vals], i) => ({ ip, value: vals[idx] || 0, color: colorOf(ip, i) }))
                .filter((r) => r.value > 0)
                .sort((a, b) => b.value - a.value)
                .slice(0, 5);
            if (rows.length) {
                drawCrosshair(ctx, geom, h, idx, rows);
                placeTip(el.tipPerIP, el.chartPerIP, idx, geom,
                    rows.map((r) => ({ label: r.ip, value: fmtMbps(r.value), color: r.color })), timeLabelAt(idx));
            } else {
                el.tipPerIP.classList.remove('show');
            }
        } else {
            el.tipPerIP.classList.remove('show');
        }

        // Legend doubles as a selector.
        el.peripLegend.innerHTML = '';
        shown.forEach(([ip, vals], i) => {
            const selected = state.selectedIP && ip === state.selectedIP;
            const devices = visibleStreams().filter((s) => (s.ip || 'unknown') === ip).length;
            const chip = document.createElement('button');
            chip.type = 'button';
            chip.className = 'legend-item' + (selected ? ' selected' : '');
            chip.innerHTML = `
                <span class="dot" style="background:${colorOf(ip, i)}"></span>
                <span class="legend-ip">${escapeHtml(ip)}</span>
                <span class="legend-val">${escapeHtml(fmtMbps(vals[vals.length - 1] || 0))}${devices > 1 ? ` · ${devices} devices` : ''}</span>`;
            chip.addEventListener('click', () => {
                state.selectedIP = selected ? '' : ip;
                if (!state.selectedIP) state.selectedToken = '';
                syncIPSelect();
                renderStreams();
                renderCharts();
            });
            el.peripLegend.appendChild(chip);
        });
    }
}

// Charts are sized from their container, so a resize needs a redraw.
let resizeTimer = null;
window.addEventListener('resize', () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => {
        try { renderCharts(); renderKPIs(); } catch (_) {}
    }, 120);
});
