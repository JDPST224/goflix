// Pure rendering/formatting helpers shared across the app.
    // ─── Inline SVG placeholders (no external image dependencies) ────────────
export const svgPlaceholder = (w, h, bg, fg, label) => 'data:image/svg+xml,' + encodeURIComponent(
        `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">` +
        `<rect width="${w}" height="${h}" fill="${bg}"/>` +
        `<text x="50%" y="52%" font-family="Arial, sans-serif" font-size="${Math.round(h / 9)}" ` +
        `fill="${fg}" text-anchor="middle" dominant-baseline="middle">${label}</text></svg>`);
export const PLACEHOLDER_POSTER = svgPlaceholder(200, 300, '#1a1a2e', '#8b8ba7', 'No Image');

    // Quick-list button icons, shared so the click handler can re-render them
    // after a toggle instead of leaving a stale glyph.
export const LIST_CHECK_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
export const LIST_PLUS_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>`;
export const PLACEHOLDER_AVATAR = svgPlaceholder(72, 72, '#2a2a3a', '#8b8ba7', '?');
export const PLACEHOLDER_STILL  = svgPlaceholder(300, 169, '#1a1a2e', '#8b8ba7', 'No Image');

export function escapeHtml(value) {
        return String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
    }
export function mediaKey(movie) {
        if (!movie || typeof movie !== 'object') return '';
        return `${movie.type || 'movie'}-${movie.id}`;
    }

export function formatPlayerTime(seconds) {
        if (!Number.isFinite(seconds)) return '0:00';
        seconds = Math.max(0, Math.floor(seconds));
        const h = Math.floor(seconds / 3600), m = Math.floor((seconds % 3600) / 60), s = seconds % 60;
        return h > 0 ? `${h}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}` : `${m}:${String(s).padStart(2,'0')}`;
    }

export function isHlsServer(server) {
        // All resolvers (cinesrc, vixsrc, vidking, vidlove, vidsrcme) stream HLS. Kept for
        // clarity and forward-compatibility if non-HLS server types are added.
        return server === 'cinesrc' || server === 'vixsrc' || server === 'vidking' || server === 'vidlove' || server === 'vidsrcme';
    }
