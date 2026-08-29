// localStorage persistence + server sync: My List, playback progress,
// continue-watching. Keys: goflix_mylist, goflix_progress, goflix_cw,
// goflix_removed (migrated from gsflix_*).
//
// Everything is written to localStorage first (offline-safe, instant), then
// mirrored to the server via /api/userdata/sync so every device sees the
// same list and progress. The server merges per item: progress and
// continue-watching entries keep the newest timestamp, My List is a union,
// and removals are tombstoned (goflix_removed) so one device deleting a
// title cannot be resurrected by another device's older copy.
import { mediaKey } from './utils.js';

// One-time migration: data saved before the GoFlix rename lived under
// `gsflix_*` keys. Copy anything found across once, then drop the legacy
// entries â€” devices that already used the app keep their list and progress.
(function migrateLegacyStorageKeys() {
    const keys = ['goflix_mylist', 'goflix_progress', 'goflix_cw', 'goflix_volume', 'goflix_removed'];
    keys.forEach((k) => {
        try {
            const legacy = 'gsflix_' + k.slice('goflix_'.length);
            if (localStorage.getItem(k) === null && localStorage.getItem(legacy) !== null) {
                localStorage.setItem(k, localStorage.getItem(legacy));
            }
            localStorage.removeItem(legacy);
        } catch (_) {
            // Storage blocked (private mode etc.) â€” reads/writes below no-op too.
        }
    });
})();

// â”€â”€â”€ Local read/write helpers â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
function readJSON(key, fallback) {
    try {
        const v = JSON.parse(localStorage.getItem(key) || 'null');
        return v === null ? fallback : v;
    } catch {
        return fallback;
    }
}
function writeJSON(key, value) {
    try { localStorage.setItem(key, JSON.stringify(value)); } catch (_) {}
}

// â”€â”€â”€ My List â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
export function getMyList() {
    const list = readJSON('goflix_mylist', []);
    return Array.isArray(list) ? list : [];
}
export function saveMyList(list) {
    writeJSON('goflix_mylist', list);
    queueSync();
}
export function isInMyList(movie) {
    const key = mediaKey(movie);
    return getMyList().some(m => mediaKey(m) === key);
}
export function toggleMyList(movie) {
    let list = getMyList();
    const key = mediaKey(movie);
    const idx = list.findIndex(m => mediaKey(m) === key);
    let added;
    if (idx === -1) {
        // listAt timestamps the add so a tombstone from another device can
        // be outranked by this newer re-add.
        list.unshift(Object.assign({}, movie, { listAt: Date.now() }));
        added = true;
    } else {
        list.splice(idx, 1);
        added = false;
    }
    saveMyList(list);
    tombstone(key, Date.now(), !added);
    return added;
}

// â”€â”€â”€ Progress tracking â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
export function getProgress(movie) {
    const prog = readJSON('goflix_progress', {});
    const key = mediaKey(movie);
    return prog[key] || prog[movie.id] || { season: 1, episode: 1 };
}
export function saveProgress(movie, season, episode, position, duration) {
    const prog = readJSON('goflix_progress', {});
    const key = mediaKey(movie);
    const previous = prog[key] || {};
    const sameEpisode = previous.season === season && previous.episode === episode;
    delete prog[movie.id];
    prog[key] = {
        season,
        episode,
        position: Number.isFinite(position) ? Math.max(0, position) : (sameEpisode ? previous.position || 0 : 0),
        // Track the real duration so the Continue Watching progress bar
        // can show an accurate percentage.
        duration: Number.isFinite(duration) && duration > 0
            ? Math.floor(duration)
            : (sameEpisode ? previous.duration || 0 : 0),
        // Client clock of this save â€” the server merges concurrent edits
        // from multiple devices by this timestamp.
        at: Date.now()
    };
    writeJSON('goflix_progress', prog);
    queueSync();
}

// â”€â”€â”€ Continue watching â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
export function addToContinueWatching(movie) {
    let cw = readJSON('goflix_cw', []);
    const key = mediaKey(movie);
    cw = cw.filter(m => mediaKey(m) !== key);
    const clone = Object.assign({}, movie, { at: Date.now() });
    delete clone.description;
    delete clone.banner;
    cw.unshift(clone);
    if (cw.length > 20) cw.pop();
    writeJSON('goflix_cw', cw);
    queueSync();
}

export function getContinueWatching() {
    const list = readJSON('goflix_cw', []);
    return Array.isArray(list) ? list : [];
}

export function removeFromContinueWatching(movie) {
    let cw = readJSON('goflix_cw', []);
    const key = mediaKey(movie);
    cw = cw.filter(m => mediaKey(m) !== key);
    writeJSON('goflix_cw', cw);
    tombstone(key, Date.now(), true);
    queueSync();
}

// clearLocalUserData wipes every synced dataset (and the account binding)
// from this browser. Called on sign-out: the server already holds the
// account's state, so nothing is lost â€” but a shared computer's next
// visitor starts clean instead of inheriting the previous user's list
// and progress in localStorage.
export function clearLocalUserData() {
    ['goflix_mylist', 'goflix_progress', 'goflix_cw', 'goflix_removed', USER_KEY].forEach(k => {
        try { localStorage.removeItem(k); } catch (_) {}
    });
    clearTimeout(syncTimer);
    syncState = 'off'; // anonymous until the next sign-in
}

// â”€â”€â”€ A/V preferences (audio + subtitle language memory) â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
// {"audio": "eng", "sub": "eng"|"off", "at": ...}. Remembered across
// episodes and devices (synced with the userdata blob, newest clock wins).
export function getAVPrefs() {
    const p = readJSON('goflix_avprefs', {});
    return (p && typeof p === 'object') ? p : {};
}
export function saveAVPrefs(patch) {
    const p = getAVPrefs();
    Object.assign(p, patch, { at: Date.now() });
    writeJSON('goflix_avprefs', p);
    queueSync();
}

// â”€â”€â”€ Watch history (derived from progress) â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
// A title counts as watched when the last saved position is â‰¥ 92% of its
// duration. No extra storage: progress already tracks position+duration+at
// for everything played.
const WATCHED_RATIO = 0.92;
export function watchedRatio(movie) {
    const entry = getProgress(movie);
    if (!entry || !entry.duration) return 0;
    return (entry.position || 0) / entry.duration;
}
export function isWatched(movie) {
    return watchedRatio(movie) >= WATCHED_RATIO;
}

// â”€â”€â”€ Removal tombstones â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
// mediaKey â†’ timestamp of the removal. An entry (list item or CW item) whose
// own timestamp is older than the tombstone stays deleted everywhere.
function tombstone(key, at, removed) {
    const t = readJSON('goflix_removed', {});
    if (removed) {
        if ((t[key] || 0) < at) t[key] = at;
    } else {
        // Re-add: clear the tombstone so the item syncs again.
        delete t[key];
    }
    writeJSON('goflix_removed', t);
}
function tombstones() {
    const t = readJSON('goflix_removed', {});
    return t && typeof t === 'object' ? t : {};
}
function applyTombstones(list, atField) {
    const t = tombstones();
    return list.filter(item => {
        const k = mediaKey(item);
        const at = item[atField] || 0;
        return !(t[k] || 0) || at > t[k];
    });
}

// â”€â”€â”€ Server sync â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
let syncTimer = null;
let syncState = 'pending'; // pending | on | off

// The local data belongs to ONE account (localStorage is per-browser, not
// per-account). goflix_user records which one; on an account switch the
// local state is wiped â€” it stays safe on the server under the previous
// account â€” and the new account's state is pulled down.
const USER_KEY = 'goflix_user';
function localUser() {
    try { return localStorage.getItem(USER_KEY) || ''; } catch { return ''; }
}
function setLocalUser(u) {
    try { localStorage.setItem(USER_KEY, u); } catch (_) {}
}
function wipeLocalUserData() {
    ['goflix_mylist', 'goflix_progress', 'goflix_cw', 'goflix_removed', 'goflix_avprefs'].forEach(k => {
        try { localStorage.removeItem(k); } catch (_) {}
    });
    clearTimeout(syncTimer);
}

function snapshotLocal() {
    const prefs = getAVPrefs();
    return {
        mylist: applyTombstones(getMyList(), 'listAt'),
        progress: readJSON('goflix_progress', {}),
        cw: applyTombstones(getContinueWatching(), 'at'),
        removed: tombstones(),
        avprefs: prefs,
        avprefs_at: Number(prefs.at) || 0
    };
}

function applyMerged(m) {
    if (!m || typeof m !== 'object') return;
    if (Array.isArray(m.mylist)) writeJSON('goflix_mylist', m.mylist);
    if (m.progress && typeof m.progress === 'object') writeJSON('goflix_progress', m.progress);
    if (Array.isArray(m.cw)) writeJSON('goflix_cw', m.cw);
    if (m.removed && typeof m.removed === 'object') writeJSON('goflix_removed', m.removed);
    if (m.avprefs && typeof m.avprefs === 'object' &&
        (Number(m.avprefs_at) || 0) >= (Number(getAVPrefs().at) || 0)) {
        writeJSON('goflix_avprefs', m.avprefs);
    }
}

async function syncNow() {
    if (syncState === 'off') return;
    try {
        const res = await fetch('/api/userdata/sync', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(snapshotLocal())
        });
        if (res.status === 401) {
            // Anonymous visitor: the site works fully, this browser just
            // stays localStorage-only. No redirect, no error.
            syncState = 'off';
            return;
        }
        if (!res.ok) return;
        const merged = await res.json();
        if (merged && merged.success !== false) {
            applyMerged(merged);
            if (syncState !== 'on') {
                syncState = 'on';
                // First successful sync: the Continue Watching row may not
                // have existed locally before the merge â€” let the app
                // re-render the home rows with the merged data.
                window.dispatchEvent(new Event('goflix:userdata-synced'));
            }
        }
    } catch (_) { /* offline: local state remains authoritative */ }
}

function queueSync() {
    if (syncState === 'off') return;
    clearTimeout(syncTimer);
    syncTimer = setTimeout(syncNow, 1500);
}

// Initial sync: signed-in users get cross-device persistence; anonymous
// visitors browse with localStorage only (their data never leaves this
// browser). Signing in adopts the anonymous local data into the account
// when the browser was never bound to one; switching between known
// accounts starts clean, since local data belongs to the previous account.
(async function initialSync() {
    let user = '';
    try {
        const res = await fetch('/api/auth/status');
        const s = await res.json();
        if (!s.authed || !s.user) {
            syncState = 'off'; // anonymous: localStorage only
            return;
        }
        user = s.user;
    } catch (_) {
        // Status unreachable â€” attempt the sync under whatever binding exists.
    }
    const bound = localUser();
    if (user && bound && user !== bound) {
        // Different known account on this browser: local data belongs to
        // the previous account (still safe on the server) â€” start clean.
        wipeLocalUserData();
    }
    setLocalUser(user);
    syncNow();
})();
