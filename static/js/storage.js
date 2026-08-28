// localStorage persistence: My List, playback progress, continue-watching.
// Keys: goflix_mylist, goflix_progress, goflix_cw (migrated from gsflix_*).
import { mediaKey } from './utils.js';

// One-time migration: data saved before the GoFlix rename lived under
// `gsflix_*` keys. Copy anything found across once, then drop the legacy
// entries — devices that already used the app keep their list and progress.
(function migrateLegacyStorageKeys() {
    const keys = ['goflix_mylist', 'goflix_progress', 'goflix_cw', 'goflix_volume'];
    keys.forEach((k) => {
        try {
            const legacy = 'gsflix_' + k.slice('goflix_'.length);
            if (localStorage.getItem(k) === null && localStorage.getItem(legacy) !== null) {
                localStorage.setItem(k, localStorage.getItem(legacy));
            }
            localStorage.removeItem(legacy);
        } catch (_) {
            // Storage blocked (private mode etc.) — reads/writes below no-op too.
        }
    });
})();

    // ─── My List (localStorage) ──────────────────────────────────────────────
export function getMyList() {
        try {
            const list = JSON.parse(localStorage.getItem('goflix_mylist') || '[]');
            return Array.isArray(list) ? list : [];
        } catch {
            return [];
        }
    }
export function saveMyList(list) {
        try {
            localStorage.setItem('goflix_mylist', JSON.stringify(list));
        } catch (e) { console.warn('Could not save My List:', e); }
    }
export function isInMyList(movie) {
        const key = mediaKey(movie);
        return getMyList().some(m => mediaKey(m) === key);
    }
export function toggleMyList(movie) {
        let list = getMyList();
        const key = mediaKey(movie);
        const idx = list.findIndex(m => mediaKey(m) === key);
        if (idx === -1) { list.unshift(movie); }
        else { list.splice(idx, 1); }
        saveMyList(list);
        return idx === -1;
    }

    // ─── Progress Tracking (localStorage) ────────────────────────────────────
export function getProgress(movie) {
        try {
            const prog = JSON.parse(localStorage.getItem('goflix_progress') || '{}');
            const key = mediaKey(movie);
            return prog[key] || prog[movie.id] || { season: 1, episode: 1 };
        }
        catch { return { season: 1, episode: 1 }; }
    }
export function saveProgress(movie, season, episode, position, duration) {
        try {
            const prog = JSON.parse(localStorage.getItem('goflix_progress') || '{}');
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
                    : (sameEpisode ? previous.duration || 0 : 0)
            };
            localStorage.setItem('goflix_progress', JSON.stringify(prog));
        } catch (e) { console.warn('Could not save playback progress:', e); }
    }

export function addToContinueWatching(movie) {
        try {
            let cw = JSON.parse(localStorage.getItem('goflix_cw') || '[]');
            const key = mediaKey(movie);
            cw = cw.filter(m => mediaKey(m) !== key);
            const clone = Object.assign({}, movie);
            delete clone.description;
            delete clone.banner;
            cw.unshift(clone);
            if (cw.length > 20) cw.pop();
            localStorage.setItem('goflix_cw', JSON.stringify(cw));
        } catch (e) { console.warn('Could not save continue-watching state:', e); }
    }

export function getContinueWatching() {
        try {
            const list = JSON.parse(localStorage.getItem('goflix_cw') || '[]');
            return Array.isArray(list) ? list : [];
        } catch (e) {
            console.warn('Could not read continue-watching state:', e);
            return [];
        }
    }

export function removeFromContinueWatching(movie) {
        try {
            let cw = JSON.parse(localStorage.getItem('goflix_cw') || '[]');
            const key = mediaKey(movie);
            cw = cw.filter(m => mediaKey(m) !== key);
            localStorage.setItem('goflix_cw', JSON.stringify(cw));
        } catch (e) { console.warn('Could not update continue-watching state:', e); }
    }
