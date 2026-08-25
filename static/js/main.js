import { svgPlaceholder, PLACEHOLDER_POSTER, LIST_CHECK_SVG, LIST_PLUS_SVG,
         PLACEHOLDER_AVATAR, PLACEHOLDER_STILL, escapeHtml, mediaKey,
         formatPlayerTime, isHlsServer } from './utils.js';
import { getMyList, saveMyList, isInMyList, toggleMyList, getProgress,
         saveProgress, addToContinueWatching, getContinueWatching,
         removeFromContinueWatching } from './storage.js';

document.addEventListener('DOMContentLoaded', () => {
    // ─── DOM References ──────────────────────────────────────────────────────
    const navbar              = document.getElementById('navbar');
    const hero                = document.getElementById('hero');
    const heroBgLayer         = document.getElementById('hero-bg-layer');
    const heroClickArea       = document.getElementById('hero-click-area');
    const heroTitle           = document.getElementById('hero-title');
    const heroDesc            = document.getElementById('hero-desc');
    const heroMetaRow         = document.getElementById('hero-meta-row');
    const heroPlay            = document.getElementById('hero-play');
    const heroInfo            = document.getElementById('hero-info');
    const heroAddList         = document.getElementById('hero-add-list');
    const heroDots            = document.getElementById('hero-dots');
    const heroTypeBadge       = document.getElementById('hero-type-badge');
    const heroButtonsEl       = document.querySelector('.hero-buttons');
    const heroBadgeRowEl      = document.querySelector('.hero-badge-row');
    const profileWrap         = document.getElementById('profile-wrap');
    const profileBtn          = document.getElementById('profile-btn');
    const profileMenuMyList   = document.getElementById('profile-menu-mylist');
    const profileMenuAccount  = document.getElementById('profile-menu-account');
    const profileMenuSignout  = document.getElementById('profile-menu-signout');
    const carouselsContainer  = document.getElementById('carousels-container');
    const mylistEmpty         = document.getElementById('mylist-empty');
    const mylistBrowseBtn     = document.getElementById('mylist-browse-btn');
    const pageLoader          = document.getElementById('page-loader');

    // Player
    const playerModal         = document.getElementById('player-modal');
    const closePlayer         = document.getElementById('close-player');
    const vixPlayer           = document.getElementById('vix-player');
    const playerMovieTitle    = document.getElementById('player-movie-title');
    const playerMovieSubtitle = document.getElementById('player-movie-subtitle');
    const playerControlsTop   = document.getElementById('player-controls-top');
    const playerLoader        = document.getElementById('player-loader');
    const playerNextEp        = document.getElementById('player-next-ep');
    const playerEpListBtn     = document.getElementById('player-ep-list-btn');
    const playerEpPanel       = document.getElementById('player-ep-panel');
    const playerEpSeasonSelect= document.getElementById('player-ep-season-select');
    const playerEpPanelClose  = document.getElementById('player-ep-panel-close');
    const playerEpList        = document.getElementById('player-ep-list');
    const playerEpLoading     = document.getElementById('player-ep-loading');
    const playerServerSelect  = document.getElementById('player-server-select');
    const playerServerPicker  = document.getElementById('player-server-picker');
    const playerServerTrigger = document.getElementById('player-server-trigger');
    const playerServerCurrent = document.getElementById('player-server-current');
    const playerServerMenu    = document.getElementById('player-server-menu');
    const playerAudioPicker   = document.getElementById('player-audio-picker');
    const playerAudioTrigger  = document.getElementById('player-audio-trigger');
    const playerAudioCurrent  = document.getElementById('player-audio-current');
    const playerAudioMenu     = document.getElementById('player-audio-menu');
    const playerSubtitlePicker  = document.getElementById('player-subtitle-picker');
    const playerSubtitleTrigger = document.getElementById('player-subtitle-trigger');
    const playerSubtitleCurrent = document.getElementById('player-subtitle-current');
    const playerSubtitleMenu    = document.getElementById('player-subtitle-menu');
    const playerSeasonPicker  = document.getElementById('player-season-picker');
    const playerSeasonTrigger = document.getElementById('player-season-trigger');
    const playerSeasonCurrent = document.getElementById('player-season-current');
    const playerSeasonMenu    = document.getElementById('player-season-menu');
    const playerControlsBottom= document.getElementById('player-controls-bottom');
    const playerPlay          = document.getElementById('player-play');
    const playerPlayIcon      = document.getElementById('player-play-icon');
    const playerCenterPlay    = document.getElementById('player-center-play');
    const playerCenterPlayIcon= document.getElementById('player-center-play-icon');
    const playerProgress      = document.getElementById('player-progress');
    const playerTimeCurrent   = document.getElementById('player-time-current');
    const playerTimeDuration  = document.getElementById('player-time-duration');
    const playerMute          = document.getElementById('player-mute');
    const playerVolume        = document.getElementById('player-volume');
    const playerVolumeIcon    = document.getElementById('player-volume-icon');
    const playerSeekZoneLeft     = document.getElementById('player-seek-zone-left');
    const playerSeekZoneRight    = document.getElementById('player-seek-zone-right');
    const playerSeekIndicatorLeft   = document.getElementById('player-seek-indicator-left');
    const playerSeekIndicatorRight  = document.getElementById('player-seek-indicator-right');
    const playerSeekAmountLeft      = document.getElementById('player-seek-amount-left');
    const playerSeekAmountRight     = document.getElementById('player-seek-amount-right');
    const playerFullscreen    = document.getElementById('player-fullscreen');
    const playerPip           = document.getElementById('player-pip');

    // Detail modal
    const detailModal         = document.getElementById('detail-modal');
    const detailClose         = document.getElementById('detail-close');
    const detailBackdrop      = document.getElementById('detail-modal-backdrop');
    const detailModalCard     = document.getElementById('detail-modal-card');
    const detailHero          = document.getElementById('detail-hero');
    const detailTitle         = document.getElementById('detail-title');
    const detailTagline       = document.getElementById('detail-tagline');
    const detailDesc          = document.getElementById('detail-desc');
    const detailMetaBar       = document.getElementById('detail-meta-bar');
    const detailRating        = document.getElementById('detail-rating');
    const detailYear          = document.getElementById('detail-year');
    const detailRuntime       = document.getElementById('detail-runtime');
    const detailGenres        = document.getElementById('detail-genres');
    const detailPlay          = document.getElementById('detail-play');
    const detailAddList       = document.getElementById('detail-add-list');
    const detailListIconAdd   = document.getElementById('detail-list-icon-add');
    const detailListIconCheck = document.getElementById('detail-list-icon-check');
    const detailTrailerBtn    = document.getElementById('detail-trailer-btn');
    const detailTrailerWrap   = document.getElementById('detail-trailer-wrap');
    const detailTrailerIframe = document.getElementById('detail-trailer-iframe');
    const detailCastSection   = document.getElementById('detail-cast-section');
    const detailCastRow       = document.getElementById('detail-cast-row');
    const detailEpisodesSection = document.getElementById('detail-episodes-section');
    const detailSeasonSelect  = document.getElementById('detail-season-select');
    const detailEpSearch      = document.getElementById('detail-ep-search');
    const detailEpisodeList   = document.getElementById('detail-episode-list');
    const detailEpLoading     = document.getElementById('detail-ep-loading');
    const detailRelatedSection = document.getElementById('detail-related-section');
    const detailRelatedRow    = document.getElementById('detail-related-row');

    // Search
    const searchToggle        = document.getElementById('search-toggle');
    const searchOverlay       = document.getElementById('search-overlay');
    const searchClose         = document.getElementById('search-close');
    const searchInput         = document.getElementById('search-input');
    const searchResultsGrid   = document.getElementById('search-results-grid');
    const searchLoading       = document.getElementById('search-loading');
    const searchPlaceholder   = document.getElementById('search-placeholder');

    // Nav links
    const navLinks = document.querySelectorAll('.nav-links a[data-page]');
    const mobileNavToggle     = document.getElementById('mobile-nav-toggle');
    const primaryNavigation   = document.getElementById('primary-navigation');
    const appToast            = document.getElementById('app-toast');
    const logoHome            = document.querySelector('.logo');
    const footerYear          = document.getElementById('footer-year');

    // ─── State ───────────────────────────────────────────────────────────────
    let currentPage       = 'home';
    let heroMovies        = [];
    let heroIndex         = 0;
    let heroRotateTimer   = null;
    let controlsHideTimer = null;
    let playerCloseTimer  = null;
    let detailCloseTimer  = null; // delayed display:none after the close animation
    let searchDebounce    = null;
    let searchRequestId   = 0;
    let catalogRequestId  = 0;
    let detailRequestId   = 0;
    let playerRequestId   = 0;
    // Aborts an in-flight source-resolution fetch when the player closes or a
    // new title launches, so the server-side browser session is released
    // instead of running to completion for a viewer who already left.
    let sourceAbortController = null;
    let episodesRequestId = 0;   // guards against stale season-switch responses (detail modal)
    let playerEpRequestId = 0;   // same for the in-player episode panel
    let currentDetailMovie = null;
    let currentDetailData  = null;
    let currentEpisodes    = [];
    let trailerVisible     = false;
    let currentPlayerMovie    = null;
    let currentPlayerSeason   = null;
    let currentPlayerEpisode  = null;
    let vixHlsInstance        = null;
    // 'hlsjs' (MSE) or 'native' — native TV players expose manifest
    // subtitle renditions via the TextTrack API instead of hls.js.
    let playbackEngine        = 'hlsjs';
    let activePlayerServer   = 'vidking';
    let playerReady          = false;
    let playerAudioTracks    = [];
    let playerSubtitleTracks = [];
    let externalSubtitleTracks = []; // Subtitles fetched from Videasy/Vidlove via the backend
    let playerAudioInitialized = false;
    let playerSubtitlesForcedOff = false; // user's last explicit subtitle choice was "Off"
    let activeExternalSubtitleIdx = -1;   // External track the player intends to render
    let pendingResumePosition = 0;
    let lastSavedPlaybackSecond = 0;
    let toastTimer = null;
    let heroCrossfadeTimer = null;
    let scrubbing = false;           // user is dragging the seek bar — don't fight their preview
    let lastFocusedBeforeOverlay = null;



    // ─── Page Loading Screen ─────────────────────────────────────────────────
    // Hide the loader after animation completes, then reveal content with fade-in
    if (pageLoader) {
        setTimeout(() => {
            pageLoader.classList.add('hidden');
            document.body.classList.add('content-revealed');
        }, 1900);
    }

    // ─── Helpers ──────────────────────────────────────────────────────────────
    function syncBodyOverflow() {
        const lock = detailModal.classList.contains('show')
            || playerModal.classList.contains('show')
            || searchOverlay.classList.contains('show');
        document.body.style.overflow = lock ? 'hidden' : '';
    }
    function showToast(message) {
        if (!appToast) return;
        clearTimeout(toastTimer);
        appToast.textContent = message;
        appToast.classList.add('show');
        toastTimer = setTimeout(() => appToast.classList.remove('show'), 2600);
    }

    // ─── Focus management for modal overlays ─────────────────────────────────
    const FOCUSABLE_SELECTOR = 'a[href], button:not([disabled]), input:not([disabled]), ' +
        'select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

    function saveFocus() {
        lastFocusedBeforeOverlay = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    }

    function restoreFocus() {
        try { lastFocusedBeforeOverlay?.focus(); } catch (_) {}
        lastFocusedBeforeOverlay = null;
    }

    function focusFirstIn(container) {
        const target = container?.querySelector(FOCUSABLE_SELECTOR);
        if (target instanceof HTMLElement) target.focus();
    }

    // Keeps Tab / Shift+Tab cycling inside `container` while an overlay is open.
    // Keep Tab focus cycling strictly inside an open overlay: every Tab press
    // is redirected to the next (or previous, with Shift) focusable element
    // within the container, wrapping at both ends.
    function trapTab(e, container) {
        if (!container) return;
        const items = Array.from(container.querySelectorAll(FOCUSABLE_SELECTOR))
            .filter(el => el.offsetWidth > 0 || el.offsetHeight > 0 || el === document.activeElement);
        if (items.length === 0) { e.preventDefault(); return; }
        const currentIndex = items.indexOf(document.activeElement);
        let nextIndex;
        if (e.shiftKey) {
            nextIndex = currentIndex <= 0 ? items.length - 1 : currentIndex - 1;
        } else {
            nextIndex = (currentIndex === -1 || currentIndex === items.length - 1) ? 0 : currentIndex + 1;
        }
        e.preventDefault();
        items[nextIndex].focus();
    }


    // ─── Navbar Scroll ───────────────────────────────────────────────────────
    window.addEventListener('scroll', () => {
        navbar.classList.toggle('scrolled', window.scrollY > 50);
    }, { passive: true });

    // Logo returns home (it already looks clickable via its pointer cursor).
    logoHome?.addEventListener('click', () => switchPage('home'));

    if (footerYear) footerYear.textContent = String(new Date().getFullYear());

    // ─── Nav Tab Switching ───────────────────────────────────────────────────
    navLinks.forEach(link => {
        link.addEventListener('click', e => {
            e.preventDefault();
            primaryNavigation?.classList.remove('open');
            mobileNavToggle?.setAttribute('aria-expanded', 'false');
            switchPage(link.dataset.page);
        });
    });

    mobileNavToggle?.addEventListener('click', () => {
        const open = primaryNavigation?.classList.toggle('open');
        mobileNavToggle.setAttribute('aria-expanded', open ? 'true' : 'false');
        mobileNavToggle.setAttribute('aria-label', open ? 'Close navigation' : 'Open navigation');
    });

    // ─── Profile dropdown ───────────────────────────────────────────────────
    function closeProfileMenu() {
        profileWrap?.classList.remove('open');
        profileBtn?.setAttribute('aria-expanded', 'false');
    }
    profileBtn?.addEventListener('click', (e) => {
        e.stopPropagation();
        const open = profileWrap.classList.toggle('open');
        profileBtn.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
    profileBtn?.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') closeProfileMenu();
    });
    profileMenuMyList?.addEventListener('click', () => {
        closeProfileMenu();
        primaryNavigation?.classList.remove('open');
        switchPage('mylist');
    });
    profileMenuAccount?.addEventListener('click', () => {
        closeProfileMenu();
        showToast('Account settings aren\u2019t available in this demo');
    });
    profileMenuSignout?.addEventListener('click', () => {
        closeProfileMenu();
        showToast('Sign out isn\u2019t available in this demo');
    });
    document.addEventListener('click', (e) => {
        if (profileWrap && !profileWrap.contains(e.target)) closeProfileMenu();
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') closeProfileMenu();
    });

    function setActiveNav(page) {
        navLinks.forEach(l => l.classList.toggle('active', l.dataset.page === page));
    }

    function switchPage(page) {
        if (page === currentPage && page !== 'mylist') return;
        currentPage = page;
        setActiveNav(page);
        stopHeroRotation();
        // Drop any in-flight hero crossfade from the previous page so its
        // timer can't repaint a stale banner over the new page's hero.
        resetHeroCrossfade();
        mylistEmpty.style.display = 'none';
        carouselsContainer.style.display = '';

        // Smooth scroll to top when switching pages
        window.scrollTo({ top: 0, behavior: 'smooth' });

        if (page === 'mylist') {
            catalogRequestId++;
            loadMyListPage();
        } else {
            const endpoint = {
                home:    '/api/home',
                movies:  '/api/movies',
                tvshows: '/api/tvshows',
                popular: '/api/popular',
            }[page] || '/api/home';

            showLoadingSkeleton();
            fetchAndRender(endpoint, page);
        }
    }

    function fetchAndRender(endpoint, page) {
        const requestId = ++catalogRequestId;
        fetch(endpoint)
            .then(res => {
                if (!res.ok) throw new Error(`HTTP ${res.status}`);
                return res.json();
            })
            .then(movies => {
                if (requestId !== catalogRequestId) return;
                carouselsContainer.innerHTML = '';
                if (!movies || movies.length === 0) {
                    showError('No content found. Check your TMDB API key.');
                    heroTitle.textContent = 'Nothing here yet';
                    heroDesc.textContent  = 'Content could not be loaded.';
                    return;
                }
                heroMovies = movies.filter(m => m.banner).slice(0, 8);
                heroIndex  = 0;
                renderHeroMovies(heroMovies);
                startHeroRotation();

                const categories = {};
                if (page === 'home') {
                    const cw = getContinueWatching();
                    if (cw && cw.length > 0) {
                        renderRow('Continue Watching', cw, true);
                    }
                }
                movies.forEach(movie => {
                    (movie.categories || []).forEach(cat => {
                        if (!categories[cat]) categories[cat] = [];
                        categories[cat].push(movie);
                    });
                });
                Object.keys(categories).forEach(cat => {
                    let rowTitle = cat;
                    let items = categories[cat];
                    let top10 = false;
                    // Netflix-style "Top 10" rows for the trending categories on Home
                    if (page === 'home' && (cat === 'Trending Movies' || cat === 'Trending TV') && items.length >= 5) {
                        top10 = true;
                        rowTitle = cat === 'Trending Movies' ? 'Top 10 Movies Today' : 'Top 10 TV Shows Today';
                        items = items.slice(0, 10);
                    }
                    renderRow(rowTitle, items, false, top10);
                });
            })
            .catch(err => {
                if (requestId !== catalogRequestId) return;
                console.error('Error fetching content:', err);
                carouselsContainer.innerHTML = '';
                showError('Failed to connect to the server. Make sure the server is running.');
                heroTitle.textContent = 'Connection Error';
                heroDesc.textContent  = 'Could not reach the movie server.';
            });
    }

    // ─── My List Page ────────────────────────────────────────────────────────
    /**
     * Render the My List page with no saved titles: an explanatory panel
     * instead of the grid, plus a neutralized hero (the banner must not keep
     * pointing at a removed title).
     */
    function showMyListEmpty() {
        stopHeroRotation();
        mylistEmpty.style.display = 'flex';
        heroTitle.textContent = 'My List';
        heroDesc.textContent  = '';
        // Kill any in-flight crossfade first, or its pending timer would
        // repaint the previous page's banner right over this empty state.
        resetHeroCrossfade();
        hero.style.backgroundImage = '';
        heroMetaRow.innerHTML = '';
        heroDots.innerHTML = '';
        if (heroTypeBadge) heroTypeBadge.textContent = '';

        // This "hero" isn't a real title — it's just filling the banner
        // space while the list is empty. Hide the badge row and action
        // buttons, and strip any click handlers left over from the last
        // real movie shown, so Play/More Info/My List can't fire against
        // stale data (previously this could open the player on whatever
        // movie was last featured on the homepage).
        heroBadgeRowEl?.style.setProperty('display', 'none');
        heroButtonsEl?.style.setProperty('display', 'none');
        heroPlay.onclick = null;
        heroInfo.onclick = null;
        heroAddList.onclick = null;
        heroClickArea.onclick = null;
        heroClickArea.onkeydown = null;
    }

    /**
     * Rebuild the My List hero after a title was removed while viewing the
     * page. Keeps the rotation sensible: clamps the index into the new
     * candidate set, restarts or stops rotation to match its size.
     */
    function refreshMyListHero() {
        const remaining = getMyList();
        if (remaining.length === 0) {
            showMyListEmpty();
            return;
        }
        mylistEmpty.style.display = 'none';
        heroMovies = remaining.filter(m => m.banner).slice(0, 6);
        if (heroMovies.length === 0) heroMovies = remaining.slice(0, 6);
        heroIndex = 0;
        renderHeroMovies(heroMovies);
        if (heroMovies.length > 1) startHeroRotation();
        else stopHeroRotation();
    }

    function loadMyListPage() {
        carouselsContainer.innerHTML = '';
        stopHeroRotation();
        const list = getMyList();
        if (list.length === 0) {
            showMyListEmpty();
        } else {
            mylistEmpty.style.display = 'none';
            heroMovies = list.filter(m => m.banner).slice(0, 6);
            heroIndex  = 0;
            renderHeroMovies(heroMovies);
            if (heroMovies.length > 1) startHeroRotation();
            renderRow('My List', list);
        }
    }

    mylistBrowseBtn.addEventListener('click', () => switchPage('home'));

    // ─── Hero ────────────────────────────────────────────────────────────────
    function renderHeroMovies(movies) {
        if (!movies || movies.length === 0) return;
        setHero(movies[0]);
        buildHeroDots(movies.length, 0);
    }

    /**
     * Cancel any in-flight hero crossfade and clear the overlay layer.
     * Needed when leaving a page that showed a hero (e.g. switching to the
     * empty My List state) — otherwise the pending timer repaints the previous
     * title's banner over the new page, and the layer keeps its old image.
     */
    function resetHeroCrossfade() {
        clearTimeout(heroCrossfadeTimer);
        heroCrossfadeTimer = null;
        if (!heroBgLayer) return;
        heroBgLayer.classList.remove('fading-in');
        heroBgLayer.style.backgroundImage = '';
    }

    /**
     * Crossfade the hero background to a new image using the overlay layer.
     * The layer fades in with the new image, then the main hero bg swaps instantly,
     * and the layer fades back out — giving a smooth Netflix-style dissolve.
     */
    function crossfadeHero(newUrl) {
        if (!heroBgLayer || !newUrl) {
            if (newUrl) hero.style.backgroundImage = `url('${CSS.escape(newUrl)}')`;
            return;
        }

        // Set new image on the crossfade layer and reveal it
        heroBgLayer.style.backgroundImage = `url('${CSS.escape(newUrl)}')`;
        heroBgLayer.classList.add('fading-in');

        // After the fade completes, swap the main hero background silently
        clearTimeout(heroCrossfadeTimer);
        heroCrossfadeTimer = setTimeout(() => {
            hero.style.backgroundImage = `url('${CSS.escape(newUrl)}')`;
            // Fade the layer back out (it now matches main — invisible seam)
            heroBgLayer.classList.remove('fading-in');
        }, 950);
    }

    /** Re-triggers the fade-up entrance animation on the hero text elements. */
    function animateHeroContent() {
        [heroBadgeRowEl, heroTitle, heroDesc, heroMetaRow, heroButtonsEl].forEach(el => {
            if (!el) return;
            el.classList.remove('hero-anim');
            void el.offsetWidth; // restart the CSS animation
            el.classList.add('hero-anim');
        });
    }

    function setHero(movie) {
        // Coming back from an empty-state page (e.g. My List with nothing
        // saved) which hides the action buttons/badge row — make sure a
        // real movie always restores them.
        heroButtonsEl?.style.removeProperty('display');
        heroBadgeRowEl?.style.removeProperty('display');

        if (movie.banner) {
            crossfadeHero(movie.banner);
        }
        heroTitle.textContent = movie.title;
        heroDesc.textContent  = movie.description || '';

        // Type badge
        if (heroTypeBadge) {
            heroTypeBadge.textContent = movie.type === 'tv' ? 'SERIES' : 'FILM';
        }

        // Build meta row
        heroMetaRow.innerHTML = '';
        
        if (movie.rating) {
            const ratingEl = document.createElement('span');
            ratingEl.className = 'hero-meta-rating';
            ratingEl.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="#f5c518" stroke="none"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"></polygon></svg> ${movie.rating.toFixed(1)}`;
            heroMetaRow.appendChild(ratingEl);
        }

        if (movie.year) {
            if (movie.rating) {
                const sep = document.createElement('span');
                sep.className = 'hero-meta-dot';
                heroMetaRow.appendChild(sep);
            }
            const yearEl = document.createElement('span');
            yearEl.className = 'hero-meta-year';
            yearEl.textContent = movie.year;
            heroMetaRow.appendChild(yearEl);
        }

        if (movie.genres && movie.genres.length > 0) {
            if (movie.rating || movie.year) {
                const sep = document.createElement('span');
                sep.className = 'hero-meta-dot';
                heroMetaRow.appendChild(sep);
            }
            movie.genres.slice(0, 3).forEach(g => {
                const pill = document.createElement('span');
                pill.className = 'hero-genre-pill';
                pill.textContent = g;
                heroMetaRow.appendChild(pill);
            });
        }

        heroPlay.onclick = () => openPlayer(movie);
        heroInfo.onclick = () => openDetailModal(movie);

        // Banner click area → open detail modal
        heroClickArea.onclick = () => openDetailModal(movie);
        heroClickArea.onkeydown = e => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                openDetailModal(movie);
            }
        };

        heroAddList.onclick = () => {
            const added = toggleMyList(movie);
            heroAddList.classList.toggle('in-list', added);
            const svg = heroAddList.querySelector('svg');
            if (svg) svg.style.transform = added ? 'rotate(45deg)' : '';
            showToast(added ? `${movie.title} added to My List` : `${movie.title} removed from My List`);
        };
        const inList = isInMyList(movie);
        heroAddList.classList.toggle('in-list', inList);
        const heroListSvg = heroAddList.querySelector('svg');
        if (heroListSvg) heroListSvg.style.transform = inList ? 'rotate(45deg)' : '';

        animateHeroContent();
    }

    function buildHeroDots(count, active) {
        heroDots.innerHTML = '';
        if (count <= 1) return;
        for (let i = 0; i < count; i++) {
            const dot = document.createElement('button');
            dot.type = 'button';
            dot.className = 'hero-dot' + (i === active ? ' active' : '');
            // Name the target title — "Show hero 3" tells screen-reader users
            // nothing about what they'd actually jump to.
            const title = heroMovies[i]?.title || `Featured title ${i + 1}`;
            dot.setAttribute('aria-label', title);
            if (i === active) dot.setAttribute('aria-current', 'true');
            else dot.removeAttribute('aria-current');
            dot.title = title;
            dot.addEventListener('click', e => {
                e.stopPropagation();
                heroIndex = i;
                clearInterval(heroRotateTimer);
                setHero(heroMovies[heroIndex]);
                buildHeroDots(heroMovies.length, heroIndex);
                startHeroRotation();
            });
            heroDots.appendChild(dot);
        }
    }

    function startHeroRotation() {
        stopHeroRotation();
        if (heroMovies.length <= 1) return;
        heroRotateTimer = setInterval(() => {
            // Pause rotation while the tab is hidden or a modal/overlay is open
            if (document.hidden ||
                detailModal.classList.contains('show') ||
                playerModal.classList.contains('show') ||
                searchOverlay.classList.contains('show')) return;
            heroIndex = (heroIndex + 1) % heroMovies.length;
            setHero(heroMovies[heroIndex]);
            buildHeroDots(heroMovies.length, heroIndex);
        }, 8000);
    }

    function stopHeroRotation() {
        if (heroRotateTimer) { clearInterval(heroRotateTimer); heroRotateTimer = null; }
    }

    // ─── Row Rendering ────────────────────────────────────────────────────────
    function renderRow(title, movies, isContinueWatching = false, isTop10 = false) {
        const rowDiv = document.createElement('div');
        rowDiv.className = 'row' + (isTop10 ? ' row-top10' : '');

        const rowHeader = document.createElement('div');
        rowHeader.className = 'row-header';

        const titleEl = document.createElement('h3');
        titleEl.textContent = title;
        rowHeader.appendChild(titleEl);

        // Netflix-style "Explore All" affordance, revealed on row hover
        const exploreAll = document.createElement('button');
        exploreAll.type = 'button';
        exploreAll.className = 'row-explore-all';
        exploreAll.innerHTML = `Explore All <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"></polyline></svg>`;
        exploreAll.addEventListener('click', () => {
            const strip = rowDiv.querySelector('.row-posters');
            if (strip) strip.scrollTo({ left: strip.scrollWidth, behavior: 'smooth' });
        });
        rowHeader.appendChild(exploreAll);

        rowDiv.appendChild(rowHeader);

        // Scroll wrap with edge arrows
        const scrollWrap = document.createElement('div');
        scrollWrap.className = 'row-scroll-wrap';

        const edgeLeft = document.createElement('button');
        edgeLeft.className = 'row-edge-arrow row-edge-arrow-left';
        edgeLeft.setAttribute('aria-label', 'Scroll left');
        edgeLeft.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="15 18 9 12 15 6"></polyline></svg>`;

        const edgeRight = document.createElement('button');
        edgeRight.className = 'row-edge-arrow row-edge-arrow-right';
        edgeRight.setAttribute('aria-label', 'Scroll right');
        edgeRight.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="9 18 15 12 9 6"></polyline></svg>`;

        const postersDiv = document.createElement('div');
        postersDiv.className = 'row-posters';

        const scrollAmount = 700;
        const doScrollLeft  = () => postersDiv.scrollBy({ left: -scrollAmount, behavior: 'smooth' });
        const doScrollRight = () => postersDiv.scrollBy({ left:  scrollAmount, behavior: 'smooth' });

        edgeLeft.addEventListener('click', doScrollLeft);
        edgeRight.addEventListener('click', doScrollRight);

        movies.forEach((movie, idx) => postersDiv.appendChild(createPosterCard(movie, false, isContinueWatching, isTop10 ? idx + 1 : 0)));

        // ── Keyboard navigation for the poster row ───────────────────────────
        postersDiv.setAttribute('tabindex', '0');
        postersDiv.addEventListener('keydown', e => {
            if (e.key === 'ArrowRight') { e.preventDefault(); postersDiv.scrollBy({ left: 220, behavior: 'smooth' }); }
            if (e.key === 'ArrowLeft')  { e.preventDefault(); postersDiv.scrollBy({ left: -220, behavior: 'smooth' }); }
        });

        scrollWrap.appendChild(edgeLeft);
        scrollWrap.appendChild(postersDiv);
        scrollWrap.appendChild(edgeRight);

        rowDiv.appendChild(scrollWrap);
        carouselsContainer.appendChild(rowDiv);
    }

    function createPosterCard(movie, small = false, isContinueWatching = false, top10Rank = 0) {
        const card = document.createElement('div');
        card.className = 'poster-card' + (small ? ' poster-card-sm' : '');
        card.setAttribute('role', 'button');
        card.setAttribute('tabindex', '0');
        card.setAttribute('aria-label',
            isContinueWatching ? `Play ${movie.title}`
            : top10Rank > 0 ? `Number ${top10Rank}: view details for ${movie.title}`
            : `View details for ${movie.title}`);

        const activateCard = () => {
            if (isContinueWatching) {
                openPlayer(movie);
            } else {
                openDetailModal(movie);
            }
        };
        card.addEventListener('click', activateCard);
        card.addEventListener('keydown', e => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                activateCard();
            }
        });

        const wrapper = document.createElement('div');
        wrapper.className = 'poster-wrapper';

        const img = document.createElement('img');
        img.src = movie.thumbnail;
        img.className = 'poster';
        img.alt = movie.title;
        img.loading = 'lazy';
        img.onerror = () => { img.onerror = null; img.src = PLACEHOLDER_POSTER; };

        const overlay = document.createElement('div');
        overlay.className = 'poster-overlay';

        // Quick-action buttons inside the overlay
        const overlayActions = document.createElement('div');
        overlayActions.className = 'poster-overlay-actions';

        const quickPlay = document.createElement('button');
        quickPlay.className = 'poster-quick-play';
        quickPlay.setAttribute('aria-label', `Play ${movie.title}`);
        quickPlay.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>`;
        quickPlay.addEventListener('click', e => {
            e.stopPropagation();
            openPlayer(movie);
        });

        const quickList = document.createElement('button');
        quickList.className = 'poster-quick-list';
        const applyQuickListState = inList => {
            quickList.setAttribute('aria-label', inList ? 'Remove from My List' : 'Add to My List');
            quickList.innerHTML = inList ? LIST_CHECK_SVG : LIST_PLUS_SVG;
        };
        applyQuickListState(isInMyList(movie));
        quickList.addEventListener('click', e => {
            e.stopPropagation();
            const added = toggleMyList(movie);
            // Reflect the new state on this card's button immediately — other
            // cards for the same title stay stale until re-render, but this
            // one must not contradict what just happened.
            applyQuickListState(added);
            // Un-listing from the My List page should drop the card too —
            // otherwise it lingers until the next reload.
            if (!added && currentPage === 'mylist') {
                const card = quickList.closest('.poster-card');
                card?.remove();
                // The hero may be featuring the title just removed (or the
                // page may now be empty) — resync both.
                refreshMyListHero();
            }
            showToast(added ? `${movie.title} added to My List` : `${movie.title} removed from My List`);
        });

        // Netflix-style circular "more info" button
        const quickInfo = document.createElement('button');
        quickInfo.className = 'poster-info-btn';
        quickInfo.setAttribute('aria-label', `More info about ${movie.title}`);
        quickInfo.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"></polyline></svg>`;
        quickInfo.addEventListener('click', e => {
            e.stopPropagation();
            openDetailModal(movie);
        });

        overlayActions.appendChild(quickPlay);
        overlayActions.appendChild(quickList);
        overlayActions.appendChild(quickInfo);

        const pTitle = document.createElement('div');
        pTitle.className = 'poster-title';
        pTitle.textContent = movie.title;

        overlay.appendChild(overlayActions);
        overlay.appendChild(pTitle);

        // Meta line and genres inside the hover preview
        if (!isContinueWatching) {
            const metaLine = document.createElement('div');
            metaLine.className = 'poster-overlay-meta';
            if (movie.year) {
                const y = document.createElement('span');
                y.textContent = movie.year;
                metaLine.appendChild(y);
            }
            if (metaLine.children.length > 0) overlay.appendChild(metaLine);
            if (movie.genres && movie.genres.length) {
                const g = document.createElement('div');
                g.className = 'poster-overlay-genres';
                g.textContent = movie.genres.slice(0, 3).join(' • ');
                overlay.appendChild(g);
            }
        }

        wrapper.appendChild(img);
        wrapper.appendChild(overlay);

        // Continue Watching: progress bar + remove button
        if (isContinueWatching) {
            const removeBtn = document.createElement('button');
            removeBtn.className = 'poster-remove-btn';
            removeBtn.setAttribute('aria-label', 'Remove from Continue Watching');
            removeBtn.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>`;
            removeBtn.addEventListener('click', e => {
                e.stopPropagation();
                removeFromContinueWatching(movie);
                card.remove();
            });
            wrapper.appendChild(removeBtn);

            // Progress bar
            const prog = getProgress(movie);
            const progressWrap = document.createElement('div');
            progressWrap.className = 'poster-progress-wrap';
            const progressBar = document.createElement('div');
            progressBar.className = 'poster-progress-bar';
            // Try to estimate progress from saved position vs. a rough duration estimate
            // We don't have duration here so we just show a bar if there's a saved position > 0
            if (prog && prog.position && prog.position > 0) {
                // Real progress when the duration is known; fall back to a rough
                // estimate for entries saved before durations were tracked.
                const pct = prog.duration > 0
                    ? (prog.position / prog.duration) * 100
                    : (prog.position / 3600) * 30;
                progressBar.style.width = Math.min(95, Math.max(2, pct)) + '%';
            } else {
                progressBar.style.width = '0%';
            }
            progressWrap.appendChild(progressBar);
            wrapper.appendChild(progressWrap);
        }

        const metaBelow = document.createElement('div');
        metaBelow.className = 'poster-meta-below';

        const titleBelow = document.createElement('div');
        titleBelow.className = 'poster-meta-title';
        titleBelow.textContent = movie.title;

        const infoBelow = document.createElement('div');
        infoBelow.className = 'poster-meta-info';

        if (movie.year) {
            const yearEl = document.createElement('span');
            yearEl.textContent = movie.year;
            infoBelow.appendChild(yearEl);
        }

        if (movie.rating) {
            if (movie.year) {
                const dot = document.createElement('span');
                dot.textContent = '•';
                infoBelow.appendChild(dot);
            }
            const rateEl = document.createElement('span');
            rateEl.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 24 24" fill="#f5c518" stroke="none" style="margin-right:2px; vertical-align:-1px;"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"></polygon></svg>${movie.rating.toFixed(1)}`;
            infoBelow.appendChild(rateEl);
        }

        metaBelow.appendChild(titleBelow);
        metaBelow.appendChild(infoBelow);

        if (top10Rank > 0) {
            // Netflix "Top 10" layout: giant outlined rank number beside the poster
            const item = document.createElement('div');
            item.className = 'top10-item';
            const rank = document.createElement('span');
            rank.className = 'top10-rank';
            rank.setAttribute('aria-hidden', 'true');
            rank.textContent = String(top10Rank);
            item.appendChild(rank);
            item.appendChild(wrapper);
            card.appendChild(item);
        } else {
            card.appendChild(wrapper);
        }
        card.appendChild(metaBelow);
        return card;
    }

    // ─── Loading Skeleton ─────────────────────────────────────────────────────
    function showLoadingSkeleton() {
        carouselsContainer.innerHTML = '';
        for (let r = 0; r < 3; r++) {
            const row = document.createElement('div');
            row.className = 'row skeleton-row';
            const label = document.createElement('div');
            label.className = 'skeleton skeleton-label';
            row.appendChild(label);
            const strip = document.createElement('div');
            strip.className = 'row-posters';
            for (let i = 0; i < 8; i++) {
                const card = document.createElement('div');
                card.className = 'poster-wrapper skeleton skeleton-card';
                strip.appendChild(card);
            }
            row.appendChild(strip);
            carouselsContainer.appendChild(row);
        }
    }

    // ─── Error Banner ─────────────────────────────────────────────────────────
    function showError(msg) {
        const err = document.createElement('div');
        err.className = 'error-banner';
        // Use textContent for the message to prevent XSS injection.
        const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        icon.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
        icon.setAttribute('width', '20'); icon.setAttribute('height', '20');
        icon.setAttribute('viewBox', '0 0 24 24');
        icon.setAttribute('fill', 'none'); icon.setAttribute('stroke', 'currentColor');
        icon.setAttribute('stroke-width', '2'); icon.setAttribute('stroke-linecap', 'round');
        icon.setAttribute('stroke-linejoin', 'round');
        icon.innerHTML = '<circle cx="12" cy="12" r="10"></circle><line x1="12" y1="8" x2="12" y2="12"></line><line x1="12" y1="16" x2="12.01" y2="16"></line>';
        const msgEl = document.createElement('span');
        msgEl.textContent = msg;
        err.appendChild(icon);
        err.appendChild(msgEl);
        carouselsContainer.appendChild(err);
    }

    // ═══════════════════════════════════════════════════════════════════════════
    // ─── Detail Modal ────────────────────────────────────────────────────────
    // ═══════════════════════════════════════════════════════════════════════════

    function openDetailModal(movie) {
        const requestId = ++detailRequestId;
        currentDetailMovie = movie;
        currentDetailData  = null;
        trailerVisible     = false;

        // A close animation may still be pending from a rapid close→reopen —
        // cancel its delayed display:none or it would hide this fresh modal.
        clearTimeout(detailCloseTimer);
        detailCloseTimer = null;
        saveFocus();

        // Reset trailer
        detailTrailerWrap.style.display = 'none';
        detailTrailerIframe.src = '';
        detailHero.classList.remove('trailer-active');
        detailTrailerBtn.style.display = 'none';

        // Set backdrop image immediately
        if (movie.banner) {
            detailHero.style.backgroundImage = `url('${CSS.escape(movie.banner)}')`;
        } else {
            detailHero.style.backgroundImage = '';
        }

        // Basic fields
        detailTitle.textContent   = movie.title || '';
        detailTagline.textContent = '';
        detailDesc.textContent    = movie.description || 'No description available.';
        detailRating.innerHTML    = '';
        detailRating.style.display = 'none';
        detailYear.textContent    = '';
        detailRuntime.textContent = '';
        detailGenres.innerHTML    = '';
        detailCastRow.innerHTML   = '';
        detailRelatedRow.innerHTML = '';
        detailCastSection.style.display   = 'none';
        detailRelatedSection.style.display = 'none';
        detailEpisodesSection.style.display = 'none';
        detailEpSearch.value = '';

        // My list state
        const inList = isInMyList(movie);
        detailListIconAdd.style.display   = inList ? 'none' : '';
        detailListIconCheck.style.display = inList ? '' : 'none';
        detailAddList.classList.toggle('in-list', inList);
        detailAddList.title = inList ? 'Remove from My List' : 'Add to My List';

        detailPlay.onclick = () => { closeDetailModal(); openPlayer(movie); };
        detailAddList.onclick = () => {
            const added = toggleMyList(movie);
            detailListIconAdd.style.display   = added ? 'none' : '';
            detailListIconCheck.style.display = added ? '' : 'none';
            detailAddList.classList.toggle('in-list', added);
            detailAddList.title = added ? 'Remove from My List' : 'Add to My List';
            showToast(added ? `${movie.title} added to My List` : `${movie.title} removed from My List`);
        };

        // Show modal
        detailModal.style.display = 'flex';
        detailModal.setAttribute('aria-hidden', 'false');
        requestAnimationFrame(() => requestAnimationFrame(() => {
            detailModal.classList.add('show');
            syncBodyOverflow();
            focusFirstIn(detailModalCard);
        }));

        // Fetch full detail
        fetchDetailData(movie, requestId);
    }

    function fetchDetailData(movie, requestId) {
        fetch(`/api/detail?type=${encodeURIComponent(movie.type)}&id=${encodeURIComponent(movie.id)}`)
            .then(r => {
                if (!r.ok) throw new Error('detail fetch failed');
                return r.json();
            })
            .then(data => {
                if (requestId !== detailRequestId || currentDetailMovie !== movie) return;
                currentDetailData = data;
                populateDetailModal(movie, data);
            })
            .catch(err => {
                console.warn('Could not load detail data:', err);
            });
    }

    function populateDetailModal(movie, d) {
        // Title
        const title = d.title || d.name || movie.title;
        detailTitle.textContent = title;

        // Tagline
        if (d.tagline) {
            detailTagline.textContent = `"${d.tagline}"`;
        }

        // Rating
        if (d.vote_average) {
            detailRating.innerHTML = `
                <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="#f5c518" stroke="none"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"></polygon></svg>
                ${d.vote_average.toFixed(1)}`;
            detailRating.style.display = '';
        }

        // Year
        const dateStr = d.release_date || d.first_air_date || '';
        if (dateStr) {
            detailYear.textContent = dateStr.slice(0, 4);
        }

        // Runtime / Seasons
        if (movie.type === 'movie' && d.runtime) {
            const h = Math.floor(d.runtime / 60);
            const m = d.runtime % 60;
            detailRuntime.textContent = h > 0 ? `${h}h ${m}m` : `${m}m`;
        } else if (movie.type === 'tv' && d.number_of_seasons) {
            detailRuntime.textContent = `${d.number_of_seasons} Season${d.number_of_seasons !== 1 ? 's' : ''}`;
        }

        // Genres
        detailGenres.innerHTML = '';
        if (d.genres && d.genres.length) {
            d.genres.slice(0, 4).forEach(g => {
                const pill = document.createElement('span');
                pill.className = 'detail-genre-pill';
                pill.textContent = g.name;
                detailGenres.appendChild(pill);
            });
        }

        // Overview
        if (d.overview) {
            detailDesc.textContent = d.overview;
        }

        // Trailer
        let trailerKey = null;
        if (d.videos && d.videos.results) {
            const trailers = d.videos.results.filter(v => v.site === 'YouTube' && v.type === 'Trailer');
            const official = trailers.find(v => v.official) || trailers[0];
            if (official) trailerKey = official.key;

            if (!trailerKey) {
                const teaser = d.videos.results.find(v => v.site === 'YouTube' && v.type === 'Teaser');
                if (teaser) trailerKey = teaser.key;
            }
        }

        if (trailerKey) {
            // Validate the key is safe before embedding in a URL.
            const safeKey = /^[A-Za-z0-9_-]{5,20}$/.test(trailerKey) ? trailerKey : null;
            if (safeKey) {
                detailTrailerBtn.style.display = '';
                detailTrailerBtn.onclick = () => toggleTrailer(safeKey);
            }
        }

        // Cast
        if (d.credits && d.credits.cast && d.credits.cast.length) {
            detailCastSection.style.display = '';
            detailCastRow.innerHTML = '';
            d.credits.cast.slice(0, 15).forEach(actor => {
                const card = document.createElement('div');
                card.className = 'cast-card';

                const avatarSrc = actor.profile_path
                    ? `https://image.tmdb.org/t/p/w185${actor.profile_path}`
                    : PLACEHOLDER_AVATAR;

                const img = document.createElement('img');
                img.src = avatarSrc;
                img.alt = actor.name;
                img.className = 'cast-avatar';
                img.onerror = () => { img.onerror = null; img.src = PLACEHOLDER_AVATAR; };

                const name = document.createElement('div');
                name.className = 'cast-name';
                name.textContent = actor.name;

                const char = document.createElement('div');
                char.className = 'cast-char';
                char.textContent = actor.character || '';

                card.appendChild(img);
                card.appendChild(name);
                if (actor.character) card.appendChild(char);
                detailCastRow.appendChild(card);
            });
        }

        // TV Show Episodes section
        if (movie.type === 'tv' && d.seasons && d.seasons.length) {
            const seasons = d.seasons.filter(s => s.season_number > 0);
            if (seasons.length === 0 && d.seasons.length > 0) {
                seasons.push(...d.seasons);
            }
            if (seasons.length) {
                detailEpisodesSection.style.display = '';
                buildSeasonSelector(movie, seasons);
            }
        }

        // Related content
        if (d.recommendations && d.recommendations.results && d.recommendations.results.length) {
            detailRelatedSection.style.display = '';
            detailRelatedRow.innerHTML = '';
            d.recommendations.results.slice(0, 20).forEach(rec => {
                if (!rec.poster_path) return;
                const recTitle = rec.title || rec.name || '';
                const recType  = rec.title ? 'movie' : 'tv';
                const recMovie = {
                    id:          String(rec.id),
                    title:       recTitle,
                    description: rec.overview || '',
                    banner:      rec.backdrop_path ? `https://image.tmdb.org/t/p/original${rec.backdrop_path}` : '',
                    thumbnail:   `https://image.tmdb.org/t/p/w500${rec.poster_path}`,
                    categories:  ['Related'],
                    type:        recType,
                };

                const card = document.createElement('div');
                card.className = 'related-card';
                card.title = recTitle;

                const img = document.createElement('img');
                img.src = recMovie.thumbnail;
                img.alt = recTitle;
                img.className = 'related-poster';
                img.loading = 'lazy';
                img.onerror = () => { img.onerror = null; img.src = PLACEHOLDER_POSTER; };

                const lbl = document.createElement('div');
                lbl.className = 'related-title';
                lbl.textContent = recTitle;

                card.appendChild(img);
                card.appendChild(lbl);
                card.addEventListener('click', () => {
                    closeDetailModal();
                    setTimeout(() => openDetailModal(recMovie), 380);
                });
                detailRelatedRow.appendChild(card);
            });
        }
    }

    // ─── Season selector ─────────────────────────────────────────────────────
    function buildSeasonSelector(movie, seasons) {
        detailSeasonSelect.innerHTML = '';
        seasons.forEach(s => {
            const opt = document.createElement('option');
            opt.value = s.season_number;
            opt.textContent = s.name || `Season ${s.season_number}`;
            detailSeasonSelect.appendChild(opt);
        });

        detailSeasonSelect.onchange = () => {
            const seasonNum = parseInt(detailSeasonSelect.value);
            loadEpisodes(movie.id, seasonNum);
        };

        loadEpisodes(movie.id, seasons[0].season_number);
    }

    function loadEpisodes(tvId, seasonNum) {
        const requestId = ++episodesRequestId;
        detailEpLoading.style.display = 'flex';
        detailEpisodeList.innerHTML = '';
        detailEpisodeList.appendChild(detailEpLoading);
        detailEpSearch.value = '';
        currentEpisodes = [];

        fetch(`/api/episodes?id=${encodeURIComponent(tvId)}&season=${encodeURIComponent(seasonNum)}`)
            .then(r => {
                if (!r.ok) throw new Error('episodes fetch failed');
                return r.json();
            })
            .then(data => {
                if (requestId !== episodesRequestId) return; // a newer season was selected meanwhile
                currentEpisodes = (data.episodes || []).filter(ep => ep.episode_number > 0);
                renderEpisodeList(currentEpisodes);
            })
            .catch(() => {
                if (requestId !== episodesRequestId) return;
                detailEpisodeList.innerHTML = '<div class="episode-no-results">Could not load episodes.</div>';
            });
    }

    function renderEpisodeList(episodes) {
        detailEpisodeList.innerHTML = '';
        if (!episodes || episodes.length === 0) {
            detailEpisodeList.innerHTML = '<div class="episode-no-results">No episodes found.</div>';
            return;
        }

        const seasonNum = parseInt(detailSeasonSelect.value) || 1;

        episodes.forEach(ep => {
            const item = document.createElement('div');
            item.className = 'episode-list-item';
            item.title = `Play ${ep.name}`;

            const numBadge = document.createElement('div');
            numBadge.className = 'episode-num-badge';
            numBadge.textContent = ep.episode_number;

            const stillWrap = document.createElement('div');
            stillWrap.className = 'episode-still-wrap';

            const still = document.createElement('img');
            still.className = 'episode-still';
            still.alt = ep.name;
            still.loading = 'lazy';
            still.src = ep.still_path
                ? `https://image.tmdb.org/t/p/w300${ep.still_path}`
                : PLACEHOLDER_STILL;
            still.onerror = () => { still.onerror = null; still.src = PLACEHOLDER_STILL; };

            const stillPlay = document.createElement('div');
            stillPlay.className = 'episode-still-play';
            stillPlay.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24" fill="white"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>`;

            stillWrap.appendChild(still);
            stillWrap.appendChild(stillPlay);

            const info = document.createElement('div');
            info.className = 'episode-info';

            const titleRow = document.createElement('div');
            titleRow.className = 'episode-title-row';

            const titleEl = document.createElement('div');
            titleEl.className = 'episode-title';
            titleEl.textContent = ep.name || `Episode ${ep.episode_number}`;

            const runtimeEl = document.createElement('div');
            runtimeEl.className = 'episode-runtime';
            if (ep.runtime) {
                runtimeEl.textContent = ep.runtime >= 60
                    ? `${Math.floor(ep.runtime/60)}h ${ep.runtime%60}m`
                    : `${ep.runtime}m`;
            }

            titleRow.appendChild(titleEl);
            if (ep.runtime) titleRow.appendChild(runtimeEl);

            const desc = document.createElement('div');
            desc.className = 'episode-desc';
            desc.textContent = ep.overview || '';

            info.appendChild(titleRow);
            if (ep.overview) info.appendChild(desc);

            item.appendChild(numBadge);
            item.appendChild(stillWrap);
            item.appendChild(info);

            item.addEventListener('click', () => {
                if (!currentDetailMovie) return;
                closeDetailModal();
                setTimeout(() => launchPlayer(currentDetailMovie, seasonNum, ep.episode_number), 380);
            });

            detailEpisodeList.appendChild(item);
        });
    }

    // Episode search filter
    detailEpSearch.addEventListener('input', () => {
        const q = detailEpSearch.value.toLowerCase().trim();
        if (!q) {
            renderEpisodeList(currentEpisodes);
            return;
        }
        const filtered = currentEpisodes.filter(ep =>
            (ep.name || '').toLowerCase().includes(q) ||
            (ep.overview || '').toLowerCase().includes(q)
        );
        renderEpisodeList(filtered);
    });

    // ─── Trailer toggle ───────────────────────────────────────────────────────
    function toggleTrailer(key) {
        trailerVisible = !trailerVisible;
        if (trailerVisible) {
            detailTrailerIframe.src = `https://www.youtube.com/embed/${key}?autoplay=1&rel=0&modestbranding=1`;
            detailTrailerWrap.style.display = '';
            detailHero.classList.add('trailer-active');
            detailTrailerBtn.innerHTML = `
                <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"></rect><line x1="9" y1="9" x2="15" y2="15"></line><line x1="15" y1="9" x2="9" y2="15"></line></svg>
                Close Trailer`;
        } else {
            detailTrailerIframe.src = '';
            detailTrailerWrap.style.display = 'none';
            detailHero.classList.remove('trailer-active');
            detailTrailerBtn.innerHTML = `
                <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="23 7 16 12 23 17 23 7"></polygon><rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect></svg>
                Trailer`;
        }
    }

    // ─── Close detail modal ───────────────────────────────────────────────────
    function closeDetailModal() {
        detailRequestId++;
        detailModal.classList.remove('show');
        detailModal.setAttribute('aria-hidden', 'true');
        syncBodyOverflow();
        restoreFocus();
        detailTrailerIframe.src = '';
        detailTrailerWrap.style.display = 'none';
        detailHero.classList.remove('trailer-active');
        trailerVisible = false;
        // Tracked so a rapid reopen can cancel the pending display:none.
        detailCloseTimer = setTimeout(() => {
            detailModal.style.display = 'none';
            detailCloseTimer = null;
        }, 350);
    }

    detailClose.addEventListener('click', closeDetailModal);
    detailBackdrop.addEventListener('click', closeDetailModal);

    // ─── Search ──────────────────────────────────────────────────────────────
    searchToggle.addEventListener('click', openSearch);

    function openSearch() {
        saveFocus();
        searchOverlay.classList.add('show');
        searchOverlay.setAttribute('aria-hidden', 'false');
        syncBodyOverflow();
        setTimeout(() => searchInput.focus(), 100);
    }

    function closeSearch() {
        searchRequestId++;
        searchOverlay.classList.remove('show');
        searchOverlay.setAttribute('aria-hidden', 'true');
        syncBodyOverflow();
        restoreFocus();
        searchInput.value = '';
        searchResultsGrid.innerHTML = '';
        searchPlaceholder.style.display = '';
        searchLoading.style.display = 'none';
    }

    searchClose.addEventListener('click', closeSearch);

    searchInput.addEventListener('input', () => {
        clearTimeout(searchDebounce);
        const requestId = ++searchRequestId;
        const q = searchInput.value.trim();
        if (!q) {
            searchResultsGrid.innerHTML = '';
            searchPlaceholder.style.display = '';
            searchLoading.style.display = 'none';
            return;
        }
        searchPlaceholder.style.display = 'none';
        searchLoading.style.display = 'flex';
        searchResultsGrid.innerHTML = '';
        searchDebounce = setTimeout(() => doSearch(q, requestId), 400);
    });

    function doSearch(q, requestId) {
        fetch(`/api/search?q=${encodeURIComponent(q)}`)
            .then(r => {
                if (!r.ok) throw new Error(`HTTP ${r.status}`);
                return r.json();
            })
            .then(results => {
                if (requestId !== searchRequestId) return;
                searchLoading.style.display = 'none';
                searchResultsGrid.innerHTML = '';
                if (!results || results.length === 0) {
                    searchResultsGrid.innerHTML = `<div class="search-no-results"><p>No results for "<strong>${escapeHtml(q)}</strong>"</p></div>`;
                    return;
                }
                results.forEach(movie => {
                    const card = createPosterCard(movie, true);
                    card.addEventListener('click', () => closeSearch());
                    searchResultsGrid.appendChild(card);
                });
            })
            .catch(() => {
                if (requestId !== searchRequestId) return;
                searchLoading.style.display = 'none';
                searchResultsGrid.innerHTML = `<div class="search-no-results"><p>Search failed. Please try again.</p></div>`;
            });
    }


    // ─── Player ───────────────────────────────────────────────────────────────
    function openPlayer(movie) {
        if (movie.type === 'tv') {
            const prog = getProgress(movie);
            launchPlayer(movie, prog.season, prog.episode);
        } else {
            launchPlayer(movie);
        }
    }

    // ─── External Subtitle Fetching ────────────────────────────────────────────
    // Fetches external subtitles from the selected backend provider,
    // deduplicating by language/label.
    async function fetchExternalSubtitles(movie, season, episode, provider) {
        if (!movie || !movie.id) return [];
        const supportedProviders = ['vidking', 'vidlove'];
        if (!supportedProviders.includes(provider)) {
            return [];
        }

        try {
            let endpoint;
            if (movie.type === 'tv') {
                endpoint = `/api/subtitles/${provider}?type=tv&id=${encodeURIComponent(movie.id)}&season=${encodeURIComponent(season || 1)}&episode=${encodeURIComponent(episode || 1)}`;
            } else {
                endpoint = `/api/subtitles/${provider}?type=movie&id=${encodeURIComponent(movie.id)}`;
            }
            const res = await fetch(endpoint, { method: 'GET', credentials: 'same-origin' });
            if (!res.ok) return [];
            const data = await res.json();
            if (!data.success || !Array.isArray(data.subtitles)) return [];
            
            const combined = [];
            const seenLabels = new Map();
            const seenUrls = new Set();
            for (const entry of data.subtitles) {
                // An identical URL is the identical file — drop only that.
                if (!entry || !entry.url || seenUrls.has(entry.url)) continue;
                seenUrls.add(entry.url);
                // Parallel uploads sharing a language/label are kept as
                // numbered versions ("English", "English (2)") instead of
                // collapsed: they usually come from different releases and
                // sync differently against our stream.
                const key = `${(entry.language || '').toLowerCase().trim()}__${(entry.label || '').toLowerCase().trim()}`;
                const n = seenLabels.get(key) || 0;
                seenLabels.set(key, n + 1);
                const sub = n > 0
                    ? { ...entry, label: `${entry.label || entry.language || 'Subtitle'} (${n + 1})` }
                    : entry;
                combined.push(sub);
            }
            return combined;
        } catch (e) {
            console.warn(`[Subtitles] Fetch from ${provider} failed:`, e);
            return [];
        }
    }

    // Registers the external subtitle list against the source's proxy
    // session so native-HLS engines get them injected into the master
    // manifest as TYPE=SUBTITLES renditions (TV players ignore <track>).
    // The resolver ALSO runs this ladder server-side during Resolve(), so
    // this endpoint only tops up whatever the backend missed.
    async function registerNativeSubtitleRenditions(sourceUrl, subs) {
        const match = String(sourceUrl || '').match(/\/api\/media\/proxy\/([A-Za-z0-9_-]+)\.m3u8/);
        if (!match || !subs || !subs.length) return;
        const payload = JSON.stringify({
            subtitles: subs.map(s => ({
                label: s.label || '',
                language: s.language || '',
                url: s.url || ''
            }))
        });
        const post = (viaXhr) => new Promise((resolve) => {
            const done = (ok) => resolve(ok);
            if (!viaXhr) {
                fetch(`/api/media/subs/${match[1]}`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    credentials: 'same-origin',
                    body: payload
                }).then(r => done(r.ok)).catch(() => done(false));
                return;
            }
            try {
                const xhr = new XMLHttpRequest();
                xhr.open('POST', `/api/media/subs/${match[1]}`, true);
                xhr.setRequestHeader('Content-Type', 'application/json');
                xhr.onload = () => done(xhr.status >= 200 && xhr.status < 300);
                xhr.onerror = () => done(false);
                xhr.send(payload);
            } catch (e) {
                done(false);
            }
        });
        // Try fetch first, XHR as the fallback path for TV browsers whose
        // fetch implementations misbehave.
        const ok = await post(window.fetch ? false : true);
        window.__goflixNativeSubs = { token: match[1], count: subs.length, ok: !!ok, at: Date.now() };
        if (!ok) console.warn(`Subtitle rendition registration failed (token ${match[1].slice(0, 8)}...)`);
        return ok;
    }

    // Returns the <track data-external> element at index i, or null.
    function externalTrackElement(index) {
        return vixPlayer.querySelectorAll('track[data-external]')[index] || null;
    }

    function normalizeTrackLabel(track, fallback, index) {
        const attrs = track?.attrs || {};
        return track?.name || track?.lang || attrs.NAME || attrs.LANGUAGE || fallback || `Track ${index + 1}`;
    }

    function isEnglishTrack(track) {
        const attrs = track?.attrs || {};
        const value = String(track?.lang || attrs.LANGUAGE || track?.name || attrs.NAME || '').toLowerCase();
        return /(^|[-_])en(g|us|gb|ca|au)?([_-]|$)/i.test(value) || /english/.test(value);
    }

    function isDefaultTrack(track) {
        const attrs = track?.attrs || {};
        return track?.default === true || track?.default === 'YES' || attrs.DEFAULT === 'YES' || track?.autoselect === true || attrs.AUTOSELECT === 'YES';
    }

    function chooseDefaultAudioIndex(tracks) {
        if (!tracks.length) return -1;
        const english = tracks.findIndex(isEnglishTrack);
        if (english >= 0) return english;
        const preferred = tracks.findIndex(isDefaultTrack);
        return preferred >= 0 ? preferred : 0;
    }

    function chooseDefaultSubtitleIndex(tracks) {
        if (!tracks.length) return -1;
        const english = tracks.findIndex(isEnglishTrack);
        if (english >= 0) return english;
        const preferred = tracks.findIndex(isDefaultTrack);
        return preferred >= 0 ? preferred : 0;
    }

    // chooseDefaultExternalSubtitleIndex picks the best External subtitle using
    // the priority: English (non-HI) > English (HI) > first available.
    function chooseDefaultExternalSubtitleIndex(subs) {
        if (!subs || !subs.length) return -1;
        // Prefer English non-HI.
        const enNonHI = subs.findIndex(s => {
            const lang = (s.language || '').toLowerCase();
            const label = (s.label || '').toLowerCase();
            const isEn = lang.startsWith('en') || label.includes('english');
            return isEn && !label.includes('(hi)');
        });
        if (enNonHI >= 0) return enNonHI;
        // Then English HI.
        const enHI = subs.findIndex(s => {
            const lang = (s.language || '').toLowerCase();
            const label = (s.label || '').toLowerCase();
            return (lang.startsWith('en') || label.includes('english')) && label.includes('(hi)');
        });
        if (enHI >= 0) return enHI;
        // Fallback: first subtitle.
        return 0;
    }

    function renderTrackMenu(menu, tracks, currentIndex, type) {
        if (!menu) return;
        menu.innerHTML = '';
        if (type === 'subtitle' && tracks.length) {
            const off = document.createElement('button');
            off.type = 'button';
            off.role = 'option';
            off.dataset.trackIndex = '-1';
            off.setAttribute('aria-selected', currentIndex < 0 ? 'true' : 'false');
            off.innerHTML = '<span><strong>Off</strong></span><svg class="track-option-check" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"></polyline></svg>';
            off.addEventListener('click', () => selectPlayerTrack(type, -1));
            menu.appendChild(off);
        }
        if (!tracks.length) {
            const empty = document.createElement('div');
            empty.className = 'player-track-empty';
            empty.textContent = type === 'audio' ? 'No alternate audio from this server' : 'No subtitles from this server';
            menu.appendChild(empty);
            return;
        }
        tracks.forEach((track, index) => {
            const button = document.createElement('button');
            button.type = 'button';
            button.role = 'option';
            button.dataset.trackIndex = String(index);
            button.setAttribute('aria-selected', index === currentIndex ? 'true' : 'false');
            button.innerHTML = `<span><strong>${escapeHtml(normalizeTrackLabel(track, type === 'audio' ? 'Original' : 'Subtitles', index))}</strong></span><svg class="track-option-check" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
            button.addEventListener('click', () => selectPlayerTrack(type, index));
            menu.appendChild(button);
        });
    }

    function syncPlayerAudioMenu(index) {
        if (!playerAudioCurrent) return;
        const track = playerAudioTracks[index];
        const label = track ? normalizeTrackLabel(track, isEnglishTrack(track) ? 'English' : 'Original', index) : 'Original';
        playerAudioCurrent.textContent = label;
        if (playerAudioTrigger) {
            playerAudioTrigger.title = `Audio: ${label}`;
            playerAudioTrigger.setAttribute('aria-label', `Audio, ${label}`);
        }
        playerAudioMenu?.querySelectorAll('[data-track-index]').forEach(button => {
            button.setAttribute('aria-selected', Number(button.dataset.trackIndex) === index ? 'true' : 'false');
        });
    }

    function syncPlayerSubtitleMenu(labelOrIndex) {
        if (!playerSubtitleCurrent) return;
        // Accept both a plain label string and a numeric track index (legacy HLS path).
        let label;
        if (typeof labelOrIndex === 'string') {
            label = labelOrIndex;
        } else {
            const index = labelOrIndex;
            label = index >= 0 && playerSubtitleTracks[index]
                ? normalizeTrackLabel(playerSubtitleTracks[index], 'Subtitles', index)
                : 'Off';
        }
        playerSubtitleCurrent.textContent = label;
        if (playerSubtitleTrigger) {
            playerSubtitleTrigger.title = `Subtitles: ${label}`;
            playerSubtitleTrigger.setAttribute('aria-label', `Subtitles, ${label}`);
        }
    }

    function updatePlayerTrackMenus() {
        const audioIndex = vixHlsInstance ? vixHlsInstance.audioTrack : -1;
        let hlsSubIndex = vixHlsInstance ? vixHlsInstance.subtitleTrack : -1;

        // Native engine has no hls.js track index — the "active" embedded
        // subtitle is whichever TextTrack is currently showing.
        if (!vixHlsInstance && playbackEngine === 'native') {
            hlsSubIndex = playerSubtitleTracks.findIndex(t => t.nativeTT && t.nativeTT.mode === 'showing');
        }

        // Build the audio menu as before.
        renderTrackMenu(playerAudioMenu, playerAudioTracks, audioIndex, 'audio');
        syncPlayerAudioMenu(audioIndex);
        if (playerAudioPicker) playerAudioPicker.style.display = playerAudioTracks.length > 1 ? '' : 'none';

        // Build a unified subtitle menu: External tracks first, then HLS tracks.
        renderSubtitleTrackMenu(hlsSubIndex);
    }

    // Renders the subtitle picker with External tracks at the top and HLS.js
    // subtitle tracks below, merging them into a single unified list.
    function renderSubtitleTrackMenu(hlsSubIndex) {
        if (!playerSubtitleMenu) return;
        playerSubtitleMenu.innerHTML = '';

        const hasExternal = externalSubtitleTracks.length > 0;
        const hasHLS = playerSubtitleTracks.length > 0;

        if (!hasExternal && !hasHLS) {
            const empty = document.createElement('div');
            empty.className = 'player-track-empty';
            empty.textContent = 'No subtitles from this server';
            playerSubtitleMenu.appendChild(empty);
            if (playerSubtitlePicker) playerSubtitlePicker.style.display = 'none';
            return;
        }

        // "Off" button.
        const off = document.createElement('button');
        off.type = 'button';
        off.role = 'option';
        off.dataset.trackIndex = '-1';
        off.dataset.trackSource = 'off';
        const isOff = hlsSubIndex < 0 && activeExternalSubtitleIdx < 0;
        off.setAttribute('aria-selected', isOff ? 'true' : 'false');
        off.innerHTML = '<span><strong>Off</strong></span><svg class="track-option-check" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"></polyline></svg>';
        off.addEventListener('click', () => selectPlayerTrack('subtitle', -1));
        playerSubtitleMenu.appendChild(off);

        // External tracks.
        externalSubtitleTracks.forEach((sub, index) => {
            const activeIdx = activeExternalSubtitleIdx;
            const button = document.createElement('button');
            button.type = 'button';
            button.role = 'option';
            button.dataset.trackIndex = String(index);
            button.dataset.trackSource = 'external';
            const selected = activeIdx === index;
            button.setAttribute('aria-selected', selected ? 'true' : 'false');
            button.innerHTML = `<span><strong>${escapeHtml(sub.label || sub.language || `Subtitle ${index + 1}`)}</strong></span><svg class="track-option-check" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
            button.addEventListener('click', () => selectPlayerTrack('external-subtitle', index));
            playerSubtitleMenu.appendChild(button);
        });

        // HLS embedded subtitle tracks (if any, and if no External tracks are overriding).
        if (hasHLS && !hasExternal) {
            playerSubtitleTracks.forEach((track, index) => {
                const button = document.createElement('button');
                button.type = 'button';
                button.role = 'option';
                button.dataset.trackIndex = String(index);
                button.dataset.trackSource = 'hls';
                button.setAttribute('aria-selected', index === hlsSubIndex ? 'true' : 'false');
                button.innerHTML = `<span><strong>${escapeHtml(normalizeTrackLabel(track, 'Subtitles', index))}</strong></span><svg class="track-option-check" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
                button.addEventListener('click', () => selectPlayerTrack('subtitle', index));
                playerSubtitleMenu.appendChild(button);
            });
        }

        // Update the current-label display.
        const activeExternalIdx = activeExternalSubtitleIdx;
        let currentLabel = 'Off';
        if (activeExternalIdx >= 0 && externalSubtitleTracks[activeExternalIdx]) {
            currentLabel = externalSubtitleTracks[activeExternalIdx].label || externalSubtitleTracks[activeExternalIdx].language || 'Subtitle';
        } else if (hlsSubIndex >= 0 && playerSubtitleTracks[hlsSubIndex]) {
            currentLabel = normalizeTrackLabel(playerSubtitleTracks[hlsSubIndex], 'Subtitles', hlsSubIndex);
        }
        syncPlayerSubtitleMenu(currentLabel);
        if (playerSubtitlePicker) playerSubtitlePicker.style.display = '';
    }

    function selectPlayerTrack(type, index) {
        if (type === 'audio' && vixHlsInstance && playerAudioTracks[index]) {
            vixHlsInstance.audioTrack = index;
            playerAudioInitialized = true;
            syncPlayerAudioMenu(index);
        } else if (type === 'external-subtitle') {
            // Activate a External <track> element and disable all others.
            playerSubtitlesForcedOff = false;
            if (vixHlsInstance) vixHlsInstance.subtitleTrack = -1;
            activateExternalTrack(index);
            enforceSubtitlePriority();
            updatePlayerTrackMenus();
        } else if (type === 'subtitle' && (vixHlsInstance || playbackEngine === 'native')) {
            // Deactivate any External tracks, then enable the HLS/native one.
            deactivateAllExternalTracks();
            if (vixHlsInstance) {
                // Restore visible rendering only when an embedded track is
                // actually picked; "hidden" while off so hls.js can't surface
                // its DEFAULT track behind our back.
                vixHlsInstance.subtitleDisplay = index >= 0;
                vixHlsInstance.subtitleTrack = index >= 0 ? index : -1;
            } else {
                // Native engine: drive TextTrack modes directly — exactly one
                // embedded track showing (or none).
                playerSubtitleTracks.forEach((t, i) => {
                    if (t.nativeTT) t.nativeTT.mode = i === index ? 'showing' : 'disabled';
                });
            }
            // Remember an explicit "Off" so hls.js can't resurrect its
            // DEFAULT/FORCED embedded track on the next level switch (and so
            // enforceSubtitlePriority keeps native ones stripped too).
            playerSubtitlesForcedOff = index < 0;
            enforceSubtitlePriority();
            // updatePlayerTrackMenus() re-renders the menu and syncs the
            // current-track label to the newly selected track (or "Off").
            updatePlayerTrackMenus();
        }
        playerAudioPicker?.classList.remove('open');
        playerSubtitlePicker?.classList.remove('open');
        playerAudioTrigger?.setAttribute('aria-expanded', 'false');
        playerSubtitleTrigger?.setAttribute('aria-expanded', 'false');
    }

    // Activates a single External track by index, disabling all others.
    // activeExternalSubtitleIdx records the intent so enforceSubtitlePriority
    // can strip anything else — including tracks the browser flips behind our
    // back before their TextTrack objects even exist.
    function activateExternalTrack(index) {
        activeExternalSubtitleIdx = index;
        let anyBacked = false;
        vixPlayer.querySelectorAll('track[data-external]').forEach((el, i) => {
            if (!el.track) return;
            anyBacked = true;
            el.track.mode = (i === index) ? 'showing' : 'disabled';
        });
        if (anyBacked) {
            // A real TextTrack implementation drives the elements — keep
            // manifest-injected renditions from doubling them.
            playerSubtitleTracks.forEach(t => {
                if (t.nativeTT && t.nativeTT.mode === 'showing') t.nativeTT.mode = 'disabled';
            });
            return;
        }
        // Native TV engines never create TextTracks for <track> elements
        // (el.track stays null forever) but DO expose the manifest-injected
        // renditions. Renditions were registered in externalSubtitleTracks
        // order, so the index maps 1:1 onto the mirrored embedded list.
        if (playbackEngine === 'native') {
            playerSubtitleTracks.forEach((t, i) => {
                if (t.nativeTT) t.nativeTT.mode = (i === index) ? 'showing' : 'disabled';
            });
        }
    }

    // Disables all External <track> elements.
    function deactivateAllExternalTracks() {
        activeExternalSubtitleIdx = -1;
        vixPlayer.querySelectorAll('track[data-external]').forEach(el => {
            if (el.track) el.track.mode = 'disabled';
        });
        if (playbackEngine === 'native') {
            playerSubtitleTracks.forEach(t => {
                if (t.nativeTT && t.nativeTT.mode !== 'disabled') t.nativeTT.mode = 'disabled';
            });
        }
    }

    // Keeps at most one subtitle layer rendering. Two known offenders:
    //
    // 1. hls.js re-selects manifest DEFAULT/FORCED subtitle tracks every time
    //    the subtitle group is re-parsed (manifest parse, LEVEL_SWITCHING,
    //    audio-group change): it fires SUBTITLE_TRACKS_UPDATED and only
    //    afterwards calls its internal setSubtitleTrack() +
    //    createTracksInGroup(), force-flipping a "disabled" <track> node back
    //    to "showing" — embedded cues stack under the active External track.
    //
    // 2. Chromium processes newly inserted <track> elements in a queued task
    //    that runs AFTER our code and recomputes the initial mode, clobbering
    //    the "disabled" we just set on non-selected External tracks (its
    //    first-track heuristic lands on "showing") — two External subtitles
    //    render at once.
    //
    // This runs off the media element's textTracks "change" event (plus
    // timeupdate as a backstop), which fire AFTER those native mutations,
    // stripping everything that should not render.
    function enforceSubtitlePriority() {
        // External tracks: exactly the intended one may render.
        vixPlayer.querySelectorAll('track[data-external]').forEach((el, i) => {
            if (el.track && el.track.mode === 'showing' && i !== activeExternalSubtitleIdx) {
                el.track.mode = 'disabled';
            }
        });
        if (activeExternalSubtitleIdx < 0 && !playerSubtitlesForcedOff) return;
        if (vixHlsInstance && vixHlsInstance.subtitleTrack !== -1) {
            vixHlsInstance.subtitleTrack = -1;
        }
        // Native-engine embedded tracks may have no backing <track> node for
        // the DOM sweep below — disable their TextTrack modes directly. The
        // one exception: the rendition mirroring the SELECTED External entry
        // on engines that ignored its <track> element stays showing.
        if (!vixHlsInstance) {
            const kept = activeExternalSubtitleIdx >= 0 && !externalTrackElement(activeExternalSubtitleIdx)?.track
                ? activeExternalSubtitleIdx : -1;
            playerSubtitleTracks.forEach((t, i) => {
                if (t.nativeTT && t.nativeTT.mode === 'showing' && i !== kept) {
                    t.nativeTT.mode = 'disabled';
                }
            });
        }
        // Sweep hls.js's own (non-External) <track> nodes — covers leftovers
        // from a destroyed instance too, including the native-HLS fallback
        // path where Safari honors DEFAULT=YES without hls.js.
        vixPlayer.querySelectorAll('track:not([data-external])').forEach(el => {
            if (el.track && el.track.mode !== 'disabled') el.track.mode = 'disabled';
        });
    }

    function resetPlayerTracks() {
        playerAudioTracks = [];
        playerSubtitleTracks = [];
        externalSubtitleTracks = [];
        playerAudioInitialized = false;
        playerSubtitlesForcedOff = false;
        activeExternalSubtitleIdx = -1;
        playbackEngine = 'hlsjs';
        vixPlayer.removeEventListener('loadedmetadata', onNativeTextTracksChanged);
        if (vixPlayer.textTracks && vixPlayer.textTracks.removeEventListener) {
            vixPlayer.textTracks.removeEventListener('change', onNativeTextTracksChanged);
            vixPlayer.textTracks.removeEventListener('addtrack', onNativeTextTracksChanged);
        }
        if (playerAudioMenu) playerAudioMenu.innerHTML = '';
        if (playerSubtitleMenu) playerSubtitleMenu.innerHTML = '';
        if (playerAudioCurrent) playerAudioCurrent.textContent = 'Original';
        if (playerSubtitleCurrent) playerSubtitleCurrent.textContent = 'Off';
        if (playerAudioPicker) playerAudioPicker.style.display = 'none';
        if (playerSubtitlePicker) playerSubtitlePicker.style.display = 'none';
        
        // Disable and remove all track elements to clear leftover subtitles
        if (vixPlayer.textTracks) {
            Array.from(vixPlayer.textTracks).forEach(t => t.mode = 'disabled');
        }
        vixPlayer.querySelectorAll('track').forEach(t => t.remove());
    }

    async function launchPlayer(movie, season, episode) {
		clearTimeout(playerCloseTimer);
		playerCloseTimer = null;
        const requestId = ++playerRequestId;
        // Invalidate any still-running resolution fetch from a previous launch.
        if (sourceAbortController) sourceAbortController.abort();
        sourceAbortController = new AbortController();
        currentPlayerMovie = movie;
        currentPlayerSeason = season || 1;
        currentPlayerEpisode = episode || 1;
		const savedProgress = getProgress(movie);
		pendingResumePosition = savedProgress.season === currentPlayerSeason && savedProgress.episode === currentPlayerEpisode
			? Number(savedProgress.position) || 0 : 0;
		lastSavedPlaybackSecond = 0;
        vixPlayer.__resumeApplied = false; // reset one-shot resume guard for new launch

        if (movie.type === 'tv') {
            playerNextEp.style.display = 'flex';
            playerEpListBtn.style.display = 'flex';
        } else {
            playerNextEp.style.display = 'none';
            playerEpListBtn.style.display = 'none';
        }

        const server = playerServerSelect ? (playerServerSelect.value || 'vidking') : 'vidking';
        activePlayerServer = server;
        playerReady = false;
        playerMovieTitle.textContent = movie.title || '';
        if (playerMovieSubtitle) {
            if (movie.type === 'tv') {
                playerMovieSubtitle.textContent = `Season ${currentPlayerSeason}  ·  Episode ${currentPlayerEpisode}`;
                playerMovieSubtitle.hidden = false;
            } else {
                playerMovieSubtitle.textContent = '';
                playerMovieSubtitle.hidden = true;
            }
        }

        playerLoader.innerHTML = '<div class="player-spinner"></div>';
        playerLoader.classList.remove('buffering');
        playerLoader.style.display = 'flex';
        playerModal.classList.remove('player-ready');
        // Remember where focus came from, but only on a fresh open — server
        // switches and next-episode launches reuse an already-shown modal.
        if (!playerModal.classList.contains('show')) saveFocus();
        playerModal.style.display = 'block';
        playerModal.setAttribute('aria-hidden', 'false');
        requestAnimationFrame(() => requestAnimationFrame(() => {
            playerModal.classList.add('show');
            syncBodyOverflow();
            showControls();
        }));

        stopVixPlayback();
        resetPlayerTracks();
        resetPlayerUI();
        vixPlayer.playbackRate = 1;
        restoreSavedVolume();
        vixPlayer.style.display = 'block';

        const provider = server === 'vidking' ? 'vidking' : (server === 'vidlove' ? 'vidlove' : 'vixsrc');

        // Fire External subtitle fetch in parallel with HLS source resolution for the selected provider
        let externalPromise = fetchExternalSubtitles(movie, currentPlayerSeason, currentPlayerEpisode, provider);

        try {
            const endpoint = movie.type === 'tv'
                ? `/api/media/source/${provider}/tv/${encodeURIComponent(movie.id)}/${encodeURIComponent(currentPlayerSeason)}/${encodeURIComponent(currentPlayerEpisode)}`
                : `/api/media/source/${provider}/movie/${encodeURIComponent(movie.id)}`;

            const response = await fetch(endpoint, {
                method: 'GET',
                headers: { 'Accept': 'application/json' },
                credentials: 'same-origin',
                signal: sourceAbortController.signal
            });

            let data;
            try {
                data = await response.json();
            } catch (_) {
                throw new Error(`Resolver returned an invalid response (${response.status})`);
            }

            if (requestId !== playerRequestId) return;
            if (!response.ok || !data.success || !data.url || data.type !== 'hls') {
                throw new Error(data.error || 'Unable to resolve media source');
            }

            // Native engines (smart TV browsers) ignore DOM <track>
            // elements, so their subtitle list must reach the manifest
            // itself: register it on the proxy session BEFORE the player
            // fetches the master playlist, and the resolver injects the
            // entries as TYPE=SUBTITLES renditions. hls.js playback skips
            // this entirely — manifests stay unchanged for MSE playback.
            const wantsNativeRenditions = !(window.Hls && Hls.isSupported()) &&
                !!vixPlayer.canPlayType('application/vnd.apple.mpegurl');
            if (wantsNativeRenditions && data.url) {
                const earlySubs = await externalPromise;
                if (requestId !== playerRequestId) return;
                externalSubtitleTracks = Array.isArray(earlySubs) ? earlySubs : [];
                await registerNativeSubtitleRenditions(data.url, externalSubtitleTracks);
                if (requestId !== playerRequestId) return;
            }

            await loadVixSource(data.url, requestId, provider);

            // Apply External subtitles once the player is ready.
            if (requestId !== playerRequestId) return;
            const subs = await externalPromise;
            if (requestId !== playerRequestId) return;
            externalSubtitleTracks = Array.isArray(subs) ? subs : [];
            if (externalSubtitleTracks.length > 0) {
                applyExternalTracks();
            }
        } catch (error) {
            if (requestId !== playerRequestId) return;
            // Aborted fetches mean the player closed or a new title launched —
            // not a real failure, so stay quiet.
            if (error && error.name === 'AbortError') return;
            console.error('[Player] Source resolution failed:', error);
            showPlayerError(error.message || 'Unable to load this media.');
        }
    }

    function stopVixPlayback() {
        if (vixHlsInstance) {
            vixHlsInstance.destroy();
            vixHlsInstance = null;
        }
        vixPlayer.pause();
        vixPlayer.removeAttribute('src');
        // Do NOT call vixPlayer.load() here — removing the src attribute is
        // sufficient to reset the media element, and calling load() on an
        // empty source triggers a spurious browser media error.
        vixPlayer.onloadedmetadata = null;
        vixPlayer.onerror = null;
        // Remove all <track> elements.
        if (vixPlayer.textTracks) {
            Array.from(vixPlayer.textTracks).forEach(t => t.mode = 'disabled');
        }
        vixPlayer.querySelectorAll('track').forEach(t => t.remove());
    }

    function applyPlayerTracks(provider) {
        if (!vixHlsInstance) return;
        playerAudioTracks = vixHlsInstance.audioTracks || [];
        playerSubtitleTracks = vixHlsInstance.subtitleTracks || [];
        if (playerAudioTracks.length > 0 && !playerAudioInitialized) {
            vixHlsInstance.audioTrack = chooseDefaultAudioIndex(playerAudioTracks);
            playerAudioInitialized = true;
        }
        if (playerSubtitleTracks.length > 0) {
            if (externalSubtitleTracks.length > 0) {
                // External subtitles take priority. hls.js auto-enables
                // manifest DEFAULT/FORCED subtitle tracks, which would stack
                // on top of the external track and render two subtitles at
                // once — keep the HLS-managed track forced off AND switch its
                // rendering to "hidden", so even the force-flip its
                // createTracksInGroup() does on level switches stays invisible.
                vixHlsInstance.subtitleDisplay = false;
                if (vixHlsInstance.subtitleTrack !== -1) {
                    vixHlsInstance.subtitleTrack = -1;
                }
            } else {
                vixHlsInstance.subtitleDisplay = true;
                if (vixHlsInstance.subtitleTrack < 0) {
                    const defaultSub = chooseDefaultSubtitleIndex(playerSubtitleTracks);
                    if (defaultSub >= 0) {
                        vixHlsInstance.subtitleTrack = defaultSub;
                    }
                }
            }
        }
        updatePlayerTrackMenus();
    }

    // Injects External subtitle tracks as <track> elements into the video element
    // and re-renders the subtitle picker to include them alongside HLS tracks.
    function applyExternalTracks() {
        // Remove any previously injected External tracks.
        vixPlayer.querySelectorAll('track[data-external]').forEach(t => t.remove());

        const defaultIdx = chooseDefaultExternalSubtitleIndex(externalSubtitleTracks);

        // Turn off EVERY native text track — including ones hls.js auto-enabled
        // from DEFAULT/FORCED flags in the manifest — BEFORE appending the
        // external tracks, so External subtitles are the only ones rendered.
        // subtitleDisplay=false makes hls.js render its own tracks as
        // "hidden" instead of "showing" — its internal force-flip on level
        // switches then lands on an invisible mode instead of doubling up.
        if (vixHlsInstance) {
            vixHlsInstance.subtitleDisplay = false;
            vixHlsInstance.subtitleTrack = -1;
        }
        if (vixPlayer.textTracks) {
            Array.from(vixPlayer.textTracks).forEach(t => { t.mode = 'disabled'; });
        }

        externalSubtitleTracks.forEach((sub, i) => {
            const track = document.createElement('track');
            track.kind = 'subtitles';
            track.label = sub.label;
            track.srclang = sub.language || '';
            track.src = sub.url;
            track.setAttribute('data-external', sub.id || String(i));
            // VTT loading is async — by the time this fires, the user may have
            // picked another External track or turned subtitles off. Re-check
            // live state instead of blindly re-showing the launch default.
            track.addEventListener('load', function() {
                if (!track.track || playerSubtitlesForcedOff) return;
                const intended = activeExternalSubtitleIdx < 0 ? defaultIdx : activeExternalSubtitleIdx;
                if (i === intended && track.track.mode !== 'showing') {
                    track.track.mode = 'showing';
                }
            });
            // Native engines never see our JS defaults race — give TVs that
            // honor the HTML `default` attribute the intended launch pick.
            if (!vixHlsInstance && playbackEngine === 'native') {
                track.default = (i === defaultIdx);
            }
            vixPlayer.appendChild(track);
        });

        // Auto-select the best External subtitle track.
        if (defaultIdx >= 0) {
            activateExternalTrack(defaultIdx);
        }
        // Strip anything hls.js re-enabled before the textTracks "change"
        // event delivers (it is async on some browsers).
        enforceSubtitlePriority();

        // Chromium processes freshly inserted <track> elements in a queued
        // task that runs after this function and recomputes their initial
        // mode, clobbering the "disabled" set above (its first-track
        // heuristic lands on "showing" — two subtitles at once). Re-assert
        // the selection once that task has run. Skipped if the player was
        // reset or relaunched in the meantime.
        const injected = Array.from(vixPlayer.querySelectorAll('track[data-external]'));
        setTimeout(() => {
            if (!injected.length || !vixPlayer.contains(injected[0])) return;
            if (defaultIdx >= 0) activateExternalTrack(defaultIdx);
            enforceSubtitlePriority();
        }, 0);

        updatePlayerTrackMenus();
        syncNativeEmbeddedTracks();
    }

    // Native-HLS fallback (Safari, smart TV browsers): manifest subtitle
    // renditions show up as TextTrack objects on the media element instead of
    // hls.js track lists. Mirror every non-External subtitle TextTrack into
    // playerSubtitleTracks so the unified picker can drive their modes.
    // Returns true when the mirrored list changed.
    function syncNativeEmbeddedTracks() {
        if (playbackEngine !== 'native' || !vixPlayer.textTracks) return false;
        const externalBacked = new Set();
        vixPlayer.querySelectorAll('track[data-external]').forEach(el => {
            if (el.track) externalBacked.add(el.track);
        });
        const found = [];
        Array.from(vixPlayer.textTracks).forEach(tt => {
            if (!tt || tt.kind !== 'subtitles' || externalBacked.has(tt)) return;
            found.push({ name: tt.label || tt.language || '', lang: tt.language || '', nativeTT: tt });
        });
        const unchanged = found.length === playerSubtitleTracks.length &&
            found.every((t, i) => t.nativeTT === playerSubtitleTracks[i].nativeTT);
        if (!unchanged) playerSubtitleTracks = found;
        return !unchanged;
    }

    // loadedmetadata / textTracks addtrack+change handler while in native
    // engine mode: rebuild the embedded list, re-assert subtitle priority,
    // and refresh the picker when tracks appeared or disappeared. Renditions
    // are parsed progressively, so they can arrive well past loadedmetadata.
    function onNativeTextTracksChanged() {
        if (playbackEngine !== 'native') return;
        const changed = syncNativeEmbeddedTracks();
        // Renditions parsed late (after the External default was picked):
        // re-apply the intended selection onto the newly arrived TextTrack.
        const idx = activeExternalSubtitleIdx;
        if (idx >= 0 && playerSubtitleTracks[idx]?.nativeTT &&
            playerSubtitleTracks[idx].nativeTT.mode !== 'showing' &&
            !externalTrackElement(idx)?.track) {
            playerSubtitleTracks[idx].nativeTT.mode = 'showing';
        }
        enforceSubtitlePriority();
        if (changed) updatePlayerTrackMenus();
    }

    function loadVixSource(url, requestId, provider = 'vixsrc') {
        return new Promise((resolve, reject) => {
            playbackEngine = 'hlsjs';
            let settled = false;
			const playbackReadinessEvents = ['loadeddata', 'canplay'];
			const cleanupReadinessListeners = () => {
				playbackReadinessEvents.forEach(event => vixPlayer.removeEventListener(event, tryStartPlayback));
				vixPlayer.removeEventListener('playing', onPlaying);
			};
            const readyTimeout = setTimeout(() => {
                settleErr(new Error('Playback timed out while loading the stream'));
            }, 25000);

            const settleOk = () => {
                if (settled) return;
                settled = true;
                clearTimeout(readyTimeout);
				cleanupReadinessListeners();
                if (requestId !== playerRequestId) {
                    resolve();
                    return;
                }
                playerReady = true;
                // Record Continue Watching only after playback actually starts,
                // so failed launches don't pollute the row.
                if (currentPlayerMovie) {
                    addToContinueWatching(currentPlayerMovie);
                    if (currentPlayerMovie.type === 'tv') {
                        saveProgress(currentPlayerMovie, currentPlayerSeason, currentPlayerEpisode);
                    }
                }
                playerLoader.style.display = 'none';
                playerModal.classList.add('player-ready');
                updatePlayerPlayIcon();
                showControls();
                resolve();
            };

            const settleErr = (err) => {
                if (settled) return;
                settled = true;
                clearTimeout(readyTimeout);
				cleanupReadinessListeners();
                // Tear down the HLS instance so it stops downloading fragments
                // in the background after a fatal error.
                if (vixHlsInstance) {
                    const dead = vixHlsInstance;
                    vixHlsInstance = null;
                    try { dead.destroy(); } catch (_) {}
                }
                reject(err instanceof Error ? err : new Error(String(err)));
            };

            const hasVideoFrame = () => vixPlayer.videoWidth > 0 && vixPlayer.videoHeight > 0;

            // 'playing' settles unconditionally — real playback is the strongest
            // readiness signal, even if videoWidth isn't reported yet.
            const onPlaying = () => {
                if (!settled && requestId === playerRequestId) settleOk();
            };

            // Attempts to start playback right away so the video plays as soon
            // as the resolver finishes. If the browser's autoplay policy rejects
            // audible playback, retries once muted so playback still starts
            // automatically; a toast tells the user how to unmute.
            let mutedAutoplayTried = false;
            const attemptAutoplay = () => {
                if (settled || requestId !== playerRequestId) return;
                const p = vixPlayer.play();
                if (p && typeof p.catch === 'function') {
                    p.catch(err => {
                        const blocked = err && (err.name === 'NotAllowedError' || /play\(\)\s*failed/i.test(err.message || ''));
                        if (blocked && !mutedAutoplayTried) {
                            mutedAutoplayTried = true;
                            vixPlayer.muted = true;
                            // Don't persist this forced mute — it's the
                            // browser's doing, not the user's preference.
                            syncVolumeUI(false);
                            const retry = vixPlayer.play();
                            if (retry && typeof retry.catch === 'function') retry.catch(() => {});
                            showToast('Autoplay started muted — press M or tap the speaker to unmute');
                        }
                    });
                }
            };

            const tryStartPlayback = () => {
                if (settled || requestId !== playerRequestId) return;
                if (!hasVideoFrame()) {
                    return;
                }
                if (vixPlayer.paused) {
                    const playAttempt = vixPlayer.play();
                    if (playAttempt && typeof playAttempt.then === 'function') {
                        playAttempt.catch(() => {});
                    }
                }
                settleOk();
            };

            vixPlayer.onerror = () => settleErr(new Error('The resolved HLS source could not be loaded by the browser'));
            vixPlayer.addEventListener('loadeddata', tryStartPlayback);
            vixPlayer.addEventListener('canplay', tryStartPlayback);
            vixPlayer.addEventListener('playing', onPlaying);

            if (window.Hls && Hls.isSupported()) {
                // Aggressive ABR is only safe when playback is served by this
                // machine or the LAN: there the proxy pre-buffers into RAM and
                // every hop is short, so measured throughput runs hot and the
                // top tier is always sustainable. Over a public address each
                // fragment crosses a WAN where loss bursts stall individual
                // fetches mid-transfer — there hls.js's conservative profile
                // applies instead: ABR starts from its own bandwidth estimate
                // and climbs as measurements allow, rather than locking the
                // top tier and rebufferring while a shallow buffer outgrows
                // the stalls.
                const host = location.hostname.toLowerCase();
                const localPlayback = host === 'localhost' || host === '::1' ||
                    /^127\./.test(host) || /^10\./.test(host) ||
                    /^192\.168\./.test(host) || /^172\.(1[6-9]|2\d|3[01])\./.test(host);
                vixHlsInstance = new Hls({
                    enableWorker: true,
                    lowLatencyMode: false,
                    capLevelToPlayerSize: false,
                    renderTextTracksNatively: true,
                    autoStartLoad: true,
                    startLevel: localPlayback ? 999 : -1,
                    // Local profile biases toward the top tier: require less
                    // headroom before claiming a higher variant (hls.js
                    // defaults: 0.95 down-guard / 0.7 up-switch) and assume a
                    // healthier starting estimate. All variants stay in the
                    // manifest either way, so ABR always has headroom.
                    abrEwmaDefaultEstimate: localPlayback ? 2000000 : 1000000,
                    abrBandWidthFactor: localPlayback ? 1.0 : 0.95,
                    abrBandWidthUpFactor: localPlayback ? 0.9 : 0.7,
                    // Buffer optimization for buffer-free 4K UHD & 1080p playback.
                    // 120s forward buffer rides out upstream dips; RAM use is
                    // bounded by maxBufferSize below.
                    maxBufferLength: 120,
                    maxMaxBufferLength: 240,
                    maxBufferSize: 256 * 1024 * 1024,
                    maxBufferHole: 0.5,
                    highBufferWatchdogPeriod: 2,
                    nudgeOffset: 0.1,
                    nudgeMaxRetry: 5,
                    fragLoadingTimeOut: 30000,
                    fragLoadingMaxRetry: 6,
                    fragLoadingRetryDelay: 500,
                    backBufferLength: 60,
                    // NOTE: `progressive` is intentionally disabled — streaming
                    // fragment parsing through the media proxy caused
                    // fragParsingError failures with some providers.
                    xhrSetup: function(xhr) {
                        xhr.withCredentials = false;
                    }
                });

                vixHlsInstance.on(Hls.Events.MANIFEST_PARSED, function(event, data) {
                    if (requestId !== playerRequestId) return;
                    // Local playback: start on the highest available quality
                    // (4K 2160p, 1080p, or max bitrate). Remote playback leaves
                    // startLevel at -1 so ABR picks from its own estimate.
                    if (localPlayback && data.levels && data.levels.length > 0) {
                        let highestLevelIndex = 0;
                        let maxScore = -1;
                        for (let i = 0; i < data.levels.length; i++) {
                            const lvl = data.levels[i];
                            const pixels = (lvl.width || 0) * (lvl.height || 0);
                            const bitrate = lvl.bitrate || 0;
                            const score = (pixels > 0 ? pixels * 1000 : 0) + bitrate;
                            if (score > maxScore) {
                                maxScore = score;
                                highestLevelIndex = i;
                            }
                        }
                        // Start on the top tier but LEAVE hls.js in automatic
                        // ABR mode. Pinning currentLevel/nextLevel/loadLevel
                        // switches hls.js to manual level control — a connection
                        // that can't sustain the top bitrate then has nowhere
                        // down to go and buffers endlessly. With auto left on,
                        // playback starts at max quality and steps down only
                        // when measured throughput genuinely can't keep up.
                        vixHlsInstance.startLevel = highestLevelIndex;
                    }
                    applyPlayerTracks(provider);
                    // Start playback as soon as the manifest is parsed — don't
                    // wait for the first fragment to buffer.
                    attemptAutoplay();
                });
                vixHlsInstance.on(Hls.Events.FRAG_BUFFERED, function() {
                    if (requestId !== playerRequestId) return;
                    tryStartPlayback();
                    attemptAutoplay();
                });
                vixHlsInstance.on(Hls.Events.AUDIO_TRACKS_UPDATED, function() {
                    if (requestId !== playerRequestId) return;
                    applyPlayerTracks(provider);
                });
                vixHlsInstance.on(Hls.Events.SUBTITLE_TRACKS_UPDATED, function() {
                    if (requestId !== playerRequestId) return;
                    playerSubtitleTracks = vixHlsInstance.subtitleTracks || [];
                    if (playerSubtitleTracks.length > 0) {
                        if (externalSubtitleTracks.length > 0) {
                            // External subtitles take priority — counteract
                            // hls.js auto-enabling DEFAULT/FORCED tracks so
                            // only one subtitle ever renders. "hidden" mode
                            // keeps its force-flips invisible (see
                            // applyPlayerTracks).
                            vixHlsInstance.subtitleDisplay = false;
                            if (vixHlsInstance.subtitleTrack !== -1) {
                                vixHlsInstance.subtitleTrack = -1;
                            }
                        } else {
                            vixHlsInstance.subtitleDisplay = true;
                            if (vixHlsInstance.subtitleTrack < 0) {
                                vixHlsInstance.subtitleTrack = chooseDefaultSubtitleIndex(playerSubtitleTracks);
                            }
                        }
                    }
                    updatePlayerTrackMenus();
                });
                vixHlsInstance.on(Hls.Events.AUDIO_TRACK_SWITCHED, function() {
                    if (requestId !== playerRequestId) return;
                    syncPlayerAudioMenu(vixHlsInstance.audioTrack);
                });
                vixHlsInstance.on(Hls.Events.SUBTITLE_TRACK_SWITCH, function() {
                    if (requestId !== playerRequestId) return;
                    // If this switch was hls.js resurrecting its DEFAULT/FORCED
                    // track despite an active External track / explicit Off,
                    // strip it right back off.
                    enforceSubtitlePriority();
                    updatePlayerTrackMenus();
                });
                vixHlsInstance.on(Hls.Events.ERROR, function(event, data) {
                    if (requestId !== playerRequestId || settled) return;
                    console.error('[Player] HLS error:', data.type, data.details, data.error || '', data.response || '');
                    if (!data.fatal) return;
                    // Recovery ladder: gives fatal errors — including
                    // fragParsingError from corrupted/mismatched fragments —
                    // three escalating chances before surfacing an error UI.
                    const attempts = (vixHlsInstance.__goflixRecoverAttempts = (vixHlsInstance.__goflixRecoverAttempts || 0) + 1);
                    if (attempts <= 3) {
                        if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
                            vixHlsInstance.startLoad();
                        } else if (attempts === 1) {
                            vixHlsInstance.recoverMediaError();
                        } else if (attempts === 2) {
                            // Codec mismatch is a common fragParsingError cause —
                            // swapping the audio codec often fixes it.
                            vixHlsInstance.swapAudioCodec();
                            vixHlsInstance.recoverMediaError();
                        } else {
                            // Last resort: tear down and reload the source fresh.
                            vixHlsInstance.stopLoad();
                            vixHlsInstance.loadSource(url);
                            vixHlsInstance.startLoad();
                        }
                        tryStartPlayback();
                        return;
                    }
                    let detail = data.details || 'HLS playback failed';
                    if (data.response && data.response.code) {
                        detail += ` (${data.response.code})`;
                    }
                    settleErr(new Error(detail));
                });

                // Reset the recovery ladder once fragments load cleanly again.
                vixHlsInstance.on(Hls.Events.FRAG_LOADED, function() {
                    if (vixHlsInstance) vixHlsInstance.__goflixRecoverAttempts = 0;
                });

                vixHlsInstance.loadSource(url);
                vixHlsInstance.attachMedia(vixPlayer);
                return;
            }

            if (vixPlayer.canPlayType('application/vnd.apple.mpegurl')) {
                // Use an absolute URL. Some smart TV native players (like BrowseHere's)
                // cannot resolve relative URLs and will throw Player Error 3001.
                playbackEngine = 'native';
                vixPlayer.src = new URL(url, window.location.origin).href;
                // Native players expose manifest subtitle renditions as
                // TextTracks instead of hls.js track objects. Surface them in
                // the subtitle picker once metadata is available; also re-sync
                // after external <track> elements are injected, since that
                // fires a textTracks "change" of its own.
                vixPlayer.addEventListener('loadedmetadata', onNativeTextTracksChanged);
                if (vixPlayer.textTracks && vixPlayer.textTracks.addEventListener) {
                    vixPlayer.textTracks.addEventListener('change', onNativeTextTracksChanged);
                    vixPlayer.textTracks.addEventListener('addtrack', onNativeTextTracksChanged);
                }
                return;
            }

            settleErr(new Error('This browser does not support HLS playback'));
        });
    }

    // Episode panel logic
    async function playNextEpisode() {
        if (!currentPlayerMovie || currentPlayerMovie.type !== 'tv') return;
        const tvId = currentPlayerMovie.id;
        const season = currentPlayerSeason;
        const episode = currentPlayerEpisode;
        try {
            const epRes = await fetch(`/api/episodes?id=${encodeURIComponent(tvId)}&season=${encodeURIComponent(season)}`);
            if (!epRes.ok) throw new Error('episodes fetch failed');
            const data = await epRes.json();
            const episodes = (data.episodes || []).filter(ep => ep.episode_number > 0);
            const nextInSeason = episodes.find(ep => ep.episode_number === episode + 1)
                || episodes.find(ep => ep.episode_number > episode);
            if (nextInSeason) {
                launchPlayer(currentPlayerMovie, season, nextInSeason.episode_number);
                return;
            }
            const detailRes = await fetch(`/api/detail?id=${encodeURIComponent(tvId)}&type=tv`);
            if (!detailRes.ok) return;
            const detail = await detailRes.json();
            const seasons = (detail.seasons || [])
                .filter(s => s.season_number > season)
                .sort((a, b) => a.season_number - b.season_number);
            if (seasons.length) {
                launchPlayer(currentPlayerMovie, seasons[0].season_number, 1);
            }
        } catch (err) {
            console.warn('Could not resolve next episode:', err);
        }
    }

    playerNextEp.addEventListener('click', () => {
        playNextEpisode();
    });

    playerEpListBtn.addEventListener('click', () => {
        playerEpPanel.classList.add('show');
        playerEpListBtn.setAttribute('aria-expanded', 'true');
        loadPlayerEpisodesPanel();
    });

    function syncServerPicker() {
        if (!playerServerSelect) return;
        const value = playerServerSelect.value || 'vidking';
        const option = playerServerSelect.options[playerServerSelect.selectedIndex];
        const label = option ? option.textContent : value;
        if (playerServerCurrent) playerServerCurrent.textContent = label;
        if (playerServerTrigger) {
            playerServerTrigger.title = `Server: ${label}`;
            playerServerTrigger.setAttribute('aria-label', `Playback server, ${label}`);
        }
        playerServerMenu?.querySelectorAll('[data-server]').forEach(btn => {
            const selected = btn.dataset.server === value;
            btn.setAttribute('aria-selected', selected ? 'true' : 'false');
        });
    }

    if (playerServerSelect) {
        playerServerSelect.addEventListener('change', () => {
            syncServerPicker();
            playerServerPicker?.classList.remove('open');
            playerServerTrigger?.setAttribute('aria-expanded', 'false');
            if (currentPlayerMovie) {
                if (vixPlayer && Number.isFinite(vixPlayer.currentTime) && vixPlayer.currentTime > 0) {
                    saveProgress(currentPlayerMovie, currentPlayerSeason, currentPlayerEpisode, vixPlayer.currentTime, vixPlayer.duration);
                    pendingResumePosition = vixPlayer.currentTime;
                    vixPlayer.__resumeApplied = false;
                }
                const serverName = playerServerCurrent ? playerServerCurrent.textContent : 'server';
                showToast(`Switching to ${serverName} (Highest Quality)...`);
                launchPlayer(currentPlayerMovie, currentPlayerSeason, currentPlayerEpisode);
            }
        });
    }
    // Close every player picker menu (server/audio/subtitle/season), except
    // optionally the one about to be opened. Keeps the menus mutually
    // exclusive — previously the season trigger left others dangling open.
    function closeAllPickers(exceptPicker = null) {
        [[playerServerPicker, playerServerTrigger],
         [playerAudioPicker, playerAudioTrigger],
         [playerSubtitlePicker, playerSubtitleTrigger],
         [playerSeasonPicker, playerSeasonTrigger]].forEach(([picker, trigger]) => {
            if (picker === exceptPicker) return;
            picker?.classList.remove('open');
            trigger?.setAttribute('aria-expanded', 'false');
        });
    }

    function toggleTrackPicker(picker, trigger) {
        if (!picker) return;
        const willOpen = !picker.classList.contains('open');
        closeAllPickers(picker);
        picker.classList.toggle('open', willOpen);
        trigger?.setAttribute('aria-expanded', willOpen ? 'true' : 'false');
    }

    playerAudioTrigger?.addEventListener('click', e => {
        e.stopPropagation();
        toggleTrackPicker(playerAudioPicker, playerAudioTrigger);
    });
    playerSubtitleTrigger?.addEventListener('click', e => {
        e.stopPropagation();
        toggleTrackPicker(playerSubtitlePicker, playerSubtitleTrigger);
    });

    playerServerTrigger?.addEventListener('click', (e) => {
        e.stopPropagation();
        toggleTrackPicker(playerServerPicker, playerServerTrigger);
    });
    playerServerMenu?.querySelectorAll('[data-server]').forEach(btn => {
        btn.addEventListener('click', () => {
            if (!playerServerSelect) return;
            playerServerSelect.value = btn.dataset.server;
            playerServerSelect.dispatchEvent(new Event('change', { bubbles: true }));
        });
    });

    playerSeasonTrigger?.addEventListener('click', (e) => {
        e.stopPropagation();
        toggleTrackPicker(playerSeasonPicker, playerSeasonTrigger);
    });

    document.addEventListener('click', () => closeAllPickers());

    syncServerPicker();

    playerEpPanelClose.addEventListener('click', () => {
        playerEpPanel.classList.remove('show');
        playerEpListBtn.setAttribute('aria-expanded', 'false');
    });

    function loadPlayerEpisodesPanel() {
        playerEpSeasonSelect.innerHTML = '';
        if (playerSeasonMenu) playerSeasonMenu.innerHTML = '';
        playerEpLoading.style.display = 'flex';
        playerEpList.innerHTML = '';
        playerEpList.appendChild(playerEpLoading);

        fetch(`/api/detail?id=${encodeURIComponent(currentPlayerMovie.id)}&type=tv`)
            .then(r => {
                if (!r.ok) throw new Error(`HTTP ${r.status}`);
                return r.json();
            })
            .then(data => {
                if (!data || !data.seasons) throw new Error('No seasons');
                const seasons = data.seasons.filter(s => s.season_number > 0);
                seasons.forEach(s => {
                    const label = s.name || `Season ${s.season_number}`;
                    const opt = document.createElement('option');
                    opt.value = s.season_number;
                    opt.textContent = label;
                    if (s.season_number === currentPlayerSeason) opt.selected = true;
                    playerEpSeasonSelect.appendChild(opt);

                    const btn = document.createElement('button');
                    btn.type = 'button';
                    btn.dataset.season = s.season_number;
                    btn.textContent = label;
                    if (s.season_number === currentPlayerSeason) btn.classList.add('active');
                    btn.addEventListener('click', () => {
                        playerEpSeasonSelect.value = String(s.season_number);
                        playerSeasonCurrent.textContent = label;
                        playerSeasonMenu.querySelectorAll('button').forEach(b => b.classList.toggle('active', b === btn));
                        playerSeasonPicker.classList.remove('open');
                        playerSeasonTrigger.setAttribute('aria-expanded', 'false');
                        fetchPlayerEpisodes(s.season_number);
                    });
                    playerSeasonMenu.appendChild(btn);
                });
                playerSeasonCurrent.textContent = playerEpSeasonSelect.options[playerEpSeasonSelect.selectedIndex]?.textContent || `Season ${currentPlayerSeason}`;
                playerEpSeasonSelect.onchange = () => {
                    const season = parseInt(playerEpSeasonSelect.value);
                    playerSeasonCurrent.textContent = playerEpSeasonSelect.options[playerEpSeasonSelect.selectedIndex]?.textContent || `Season ${season}`;
                    playerSeasonMenu.querySelectorAll('button').forEach(b => b.classList.toggle('active', Number(b.dataset.season) === season));
                    fetchPlayerEpisodes(season);
                };
                fetchPlayerEpisodes(currentPlayerSeason);
            })
            .catch(() => {
                playerEpList.innerHTML = '<div class="episode-no-results" style="color:#fff;text-align:center;padding:20px;">Failed to load seasons</div>';
            });
    }

    function fetchPlayerEpisodes(seasonNum) {
        const requestId = ++playerEpRequestId;
        playerEpLoading.style.display = 'flex';
        playerEpList.innerHTML = '';
        playerEpList.appendChild(playerEpLoading);

        fetch(`/api/episodes?id=${encodeURIComponent(currentPlayerMovie.id)}&season=${encodeURIComponent(seasonNum)}`)
            .then(r => {
                if (!r.ok) throw new Error(`HTTP ${r.status}`);
                return r.json();
            })
            .then(data => {
                if (requestId !== playerEpRequestId) return; // a newer season was selected meanwhile
                playerEpList.innerHTML = '';
                const episodes = (data.episodes || []).filter(ep => ep.episode_number > 0);
                if (episodes.length === 0) {
                    playerEpList.innerHTML = '<div class="episode-no-results" style="color:#fff;text-align:center;padding:20px;">No episodes</div>';
                    return;
                }
                episodes.forEach(ep => {
                    const item = document.createElement('div');
                    item.className = 'player-ep-item';
                    if (seasonNum === currentPlayerSeason && ep.episode_number === currentPlayerEpisode) {
                        item.classList.add('active');
                    }
                    
                    const img = document.createElement('img');
                    img.className = 'player-ep-item-img';
                    img.src = ep.still_path ? `https://image.tmdb.org/t/p/w300${ep.still_path}` : PLACEHOLDER_STILL;
                    
                    const info = document.createElement('div');
                    info.className = 'player-ep-item-info';
                    
                    const title = document.createElement('div');
                    title.className = 'player-ep-item-title';
                    title.textContent = `${ep.episode_number}. ${ep.name}`;
                    
                    const meta = document.createElement('div');
                    meta.className = 'player-ep-item-meta';
                    meta.textContent = ep.runtime ? `${ep.runtime} min` : '';

                    info.appendChild(title);
                    if (ep.runtime) info.appendChild(meta);
                    
                    item.appendChild(img);
                    item.appendChild(info);

                    item.addEventListener('click', () => {
                        playerEpPanel.classList.remove('show');
                        launchPlayer(currentPlayerMovie, seasonNum, ep.episode_number);
                    });
                    
                    playerEpList.appendChild(item);
                });
            })
            .catch(() => {
                if (requestId !== playerEpRequestId) return;
                playerEpList.innerHTML = '<div class="episode-no-results" style="color:#fff;text-align:center;padding:20px;">Failed to load episodes</div>';
            });
    }

    function showPlayerError(message) {
        playerReady = false;
        // Drop both playback states so the error card is visible even when a
        // fatal error strikes after the stream already started ('player-ready'
        // hides the loader) or during a stall (the transparent buffering
        // overlay must not swallow the error card either).
        playerModal.classList.remove('player-ready');
        playerLoader.classList.remove('buffering');
        playerLoader.innerHTML = `
            <div class="player-error">
                <div class="player-error-icon">!</div>
                <strong>Unable to play this title</strong>
                <span>${escapeHtml(message)}</span>
                <button type="button" class="player-error-btn" id="player-retry">Try again</button>
            </div>`;
        playerLoader.style.display = 'flex';
        document.getElementById('player-retry')?.addEventListener('click', () => {
            if (currentPlayerMovie) launchPlayer(currentPlayerMovie, currentPlayerSeason, currentPlayerEpisode);
        }, { once: true });
    }

    function resetPlayerUI() {
        scrubbing = false;
        [playerControlsBottom, playerCenterPlay].forEach(el => {
            if (el) el.style.display = '';
        });
        if (playerProgress) { playerProgress.value = 0; playerProgress.style.setProperty('--progress', '0%'); playerProgress.style.setProperty('--buffered', '0%'); }
        if (playerTimeCurrent) playerTimeCurrent.textContent = '0:00';
        if (playerTimeDuration) playerTimeDuration.textContent = '0:00';
        if (playerVolume) playerVolume.value = vixPlayer.muted ? 0 : vixPlayer.volume || 1;
        syncVolumeUI();
        playerSeekIndicatorLeft?.classList.remove('show', 'pulse');
        playerSeekIndicatorRight?.classList.remove('show', 'pulse');
    }

    function syncVolumeUI(persist = true) {
        const volume = vixPlayer.muted ? 0 : vixPlayer.volume;
        if (playerVolume) {
            playerVolume.value = volume;
            playerVolume.style.setProperty('--volume', `${volume * 100}%`);
        }
        if (playerMute) {
            const label = volume === 0 ? 'Unmute' : 'Mute';
            playerMute.setAttribute('aria-label', label);
            playerMute.title = label;
        }
        // Remember the user's choice across sessions — but only when it came
        // from the user, not from a forced muted-autoplay fallback.
        if (persist) {
            try {
                localStorage.setItem('goflix_volume', JSON.stringify({
                    volume: typeof vixPlayer.volume === 'number' ? vixPlayer.volume : 1,
                    muted: !!vixPlayer.muted,
                }));
            } catch (_) {}
        }
        if (!playerVolumeIcon) return;
        const speaker = '<path d="M11 5 6 9H3v6h3l5 4V5Z"></path>';
        if (volume === 0) {
            playerVolumeIcon.innerHTML = speaker + '<path d="m16 9 5 5m0-5-5 5"></path>';
        } else if (volume < 0.5) {
            playerVolumeIcon.innerHTML = speaker + '<path d="M15.5 9.5a4 4 0 0 1 0 5"></path>';
        } else {
            playerVolumeIcon.innerHTML = speaker + '<path d="M15.5 8.5a5 5 0 0 1 0 7"></path><path d="M18.5 6a8.5 8.5 0 0 1 0 12"></path>';
        }
    }

    // Apply the previously saved volume/mute before playback starts so
    // returning users don't get blasted at full volume or force-muted again.
    const restoreSavedVolume = () => {
        try {
            const saved = JSON.parse(localStorage.getItem('goflix_volume') || 'null');
            if (saved && typeof saved.volume === 'number' && saved.volume >= 0 && saved.volume <= 1) {
                vixPlayer.volume = saved.volume;
            }
            if (saved && typeof saved.muted === 'boolean') vixPlayer.muted = saved.muted;
        } catch (_) {}
        syncVolumeUI();
    };

    function closePlayerModal() {
        playerRequestId++;
        if (sourceAbortController) {
            // Release the server-side resolution (browser session) for a
            // viewer who is no longer waiting on it.
            sourceAbortController.abort();
            sourceAbortController = null;
        }
        clearTimeout(controlsHideTimer);
        clearTimeout(playerCloseTimer);
        playerModal.classList.remove('show');
        playerModal.setAttribute('aria-hidden', 'true');
        syncBodyOverflow();
        restoreFocus();
		playerCloseTimer = setTimeout(() => {
            playerModal.style.display = 'none';
            playerModal.classList.remove('player-ready');
            stopVixPlayback();
            vixPlayer.style.display = 'none';
            playerLoader.innerHTML = '<div class="player-spinner"></div>';
            playerLoader.classList.remove('buffering');
            playerLoader.style.display = 'flex';
            resetPlayerUI();
		playerCloseTimer = null;
        }, 350);
    }

    function showControls() {
        playerModal.classList.add('player-controls-visible');
        playerControlsTop.classList.remove('hidden');
        if (playerControlsBottom) playerControlsBottom.classList.remove('hidden');
        playerModal.style.cursor = 'default';
        clearTimeout(controlsHideTimer);
        controlsHideTimer = setTimeout(hideControlsIdle, 3000);
    }

    // Auto-hides both bars after the idle delay — but only once playback is
    // actually running. While the loader covers the video (resolving source,
    // buffering mid-stream, or an error card), the hide keeps postponing so
    // Back / server switching stay reachable without moving the mouse.
    function hideControlsIdle() {
        if (!playerLoader || playerLoader.style.display !== 'none') {
            controlsHideTimer = setTimeout(hideControlsIdle, 3000);
            return;
        }
        playerControlsTop.classList.add('hidden');
        if (playerControlsBottom) playerControlsBottom.classList.add('hidden');
        playerModal.classList.remove('player-controls-visible');
        playerModal.style.cursor = 'none';
    }

    function updatePlayerPlayIcon() {
        const playing = !vixPlayer.paused;
        const pause = '<rect x="6" y="5" width="4" height="14" rx="1"></rect><rect x="14" y="5" width="4" height="14" rx="1"></rect>';
        const play = '<polygon points="8,5 19,12 8,19"></polygon>';
        if (playerPlayIcon) playerPlayIcon.innerHTML = playing ? pause : play;
        if (playerCenterPlayIcon) playerCenterPlayIcon.innerHTML = playing ? pause : play;
        if (playerCenterPlay) {
            playerCenterPlay.classList.toggle('is-paused', !playing);
            playerCenterPlay.classList.toggle('is-playing', playing);
        }
    }

    function toggleVixPlay() {
        if (!isHlsServer(activePlayerServer) || !playerReady) return;
        if (vixPlayer.paused) vixPlayer.play().catch(() => {}); else vixPlayer.pause();
        showControls();
    }

    if (playerPlay) playerPlay.addEventListener('click', toggleVixPlay);
    if (playerCenterPlay) playerCenterPlay.addEventListener('click', toggleVixPlay);
    // Delay single-click toggles slightly so a double-click (fullscreen) does
    // not first trigger a play/pause flicker before entering fullscreen.
    let videoClickTimer = null;
    vixPlayer.addEventListener('click', () => {
        if (!isHlsServer(activePlayerServer) || !playerReady) return;
        clearTimeout(videoClickTimer);
        videoClickTimer = setTimeout(toggleVixPlay, 240);
    });
    vixPlayer.addEventListener('dblclick', () => {
        if (!isHlsServer(activePlayerServer) || !playerReady) return;
        clearTimeout(videoClickTimer);
        togglePlayerFullscreen();
    });
    vixPlayer.addEventListener('play', updatePlayerPlayIcon);
    vixPlayer.addEventListener('pause', updatePlayerPlayIcon);
    // Subtitle watchdog: hls.js re-enables DEFAULT/FORCED embedded subtitle
    // tracks when a level switch re-parses the manifest's subtitle group,
    // stacking them under the active External track — see
    // enforceSubtitlePriority(). "change" fires on native track mode flips;
    // timeupdate backstops browsers where hls.js falls back to polling.
    if (vixPlayer.textTracks && typeof vixPlayer.textTracks.addEventListener === 'function') {
        vixPlayer.textTracks.addEventListener('change', enforceSubtitlePriority);
    }
    vixPlayer.addEventListener('timeupdate', enforceSubtitlePriority);
    // Buffering indicator: show the spinner (over a transparent backdrop so the
    // video stays visible) while the stream stalls.
    vixPlayer.addEventListener('waiting', () => {
        if (playerReady && playerModal.classList.contains('show')) {
            playerLoader.innerHTML = '<div class="player-spinner"></div>';
            playerLoader.style.display = 'flex';
            playerLoader.classList.add('buffering');
        }
    });
    vixPlayer.addEventListener('playing', () => {
        if (playerModal.classList.contains('show')) {
            playerLoader.style.display = 'none';
            playerLoader.classList.remove('buffering');
        }
    });
    vixPlayer.addEventListener('timeupdate', () => {
        // While the user is dragging the seek bar, their preview position wins
        // over playback updates.
        if (scrubbing) return;
        const duration = Number.isFinite(vixPlayer.duration) ? vixPlayer.duration : 0;
        if (playerProgress) { const pct = duration ? (vixPlayer.currentTime / duration) * 100 : 0; playerProgress.value = pct; playerProgress.style.setProperty('--progress', `${pct}%`); }
        if (playerTimeCurrent) playerTimeCurrent.textContent = formatPlayerTime(vixPlayer.currentTime);
        if (playerTimeDuration) playerTimeDuration.textContent = formatPlayerTime(duration);
        if (currentPlayerMovie && vixPlayer.currentTime - lastSavedPlaybackSecond >= 5) {
            saveProgress(currentPlayerMovie, currentPlayerSeason, currentPlayerEpisode, vixPlayer.currentTime, vixPlayer.duration);
            lastSavedPlaybackSecond = vixPlayer.currentTime;
        }
    });
    // Buffered-range indicator: show how much of the stream is already
    // downloaded ahead of the playhead as a lighter band on the seek bar.
    vixPlayer.addEventListener('progress', () => {
        if (!playerProgress) return;
        try {
            const duration = Number.isFinite(vixPlayer.duration) ? vixPlayer.duration : 0;
            if (!duration || !vixPlayer.buffered || vixPlayer.buffered.length === 0) return;
            const end = vixPlayer.buffered.end(vixPlayer.buffered.length - 1);
            playerProgress.style.setProperty('--buffered', `${Math.min(100, (end / duration) * 100)}%`);
        } catch (_) {}
    });
    // Seek to the saved position once the duration is known. Returns false when
    // metadata isn't usable yet so 'durationchange' can retry — zeroing the
    // position unconditionally here used to silently drop resumes when HLS
    // hadn't reported a duration by loadedmetadata time.
    const applyPendingResume = () => {
        if (vixPlayer.__resumeApplied || pendingResumePosition <= 5) return true;
        if (!Number.isFinite(vixPlayer.duration)) return false;
        if (pendingResumePosition >= vixPlayer.duration - 10) {
            // Practically finished — start over instead of resuming.
            pendingResumePosition = 0;
            return true;
        }
        vixPlayer.__resumeApplied = true;
        vixPlayer.currentTime = pendingResumePosition;
        showToast(`Resuming at ${formatPlayerTime(pendingResumePosition)}`);
        pendingResumePosition = 0;
        return true;
    };
    vixPlayer.addEventListener('loadedmetadata', () => {
        applyPendingResume();
        if (playerTimeCurrent && !scrubbing) playerTimeCurrent.textContent = formatPlayerTime(vixPlayer.currentTime);
        if (playerTimeDuration) playerTimeDuration.textContent = formatPlayerTime(vixPlayer.duration);
        updatePlayerPlayIcon();
    });
    vixPlayer.addEventListener('durationchange', () => {
        if (applyPendingResume() && !scrubbing && playerTimeDuration) {
            playerTimeDuration.textContent = formatPlayerTime(vixPlayer.duration);
        }
    });
    vixPlayer.addEventListener('ended', () => {
        updatePlayerPlayIcon();
        if (currentPlayerMovie?.type === 'tv') {
            setTimeout(() => { if (playerModal.classList.contains('show')) playNextEpisode(); }, 500);
        }
    });
    if (playerProgress) {
        // While dragging: preview only — move the fill and the time label
        // without seeking, so the video doesn't stutter on every input tick.
        playerProgress.addEventListener('input', () => {
            if (!isHlsServer(activePlayerServer)) return;
            scrubbing = true;
            showControls();
            const duration = Number.isFinite(vixPlayer.duration) ? vixPlayer.duration : 0;
            if (!duration) return;
            const pct = Number(playerProgress.value);
            playerProgress.style.setProperty('--progress', `${pct}%`);
            if (playerTimeCurrent) playerTimeCurrent.textContent = formatPlayerTime((pct / 100) * duration);
        });
        // On release: commit the seek once.
        playerProgress.addEventListener('change', () => {
            scrubbing = false;
            if (!isHlsServer(activePlayerServer) || !Number.isFinite(vixPlayer.duration)) return;
            vixPlayer.currentTime = (Number(playerProgress.value) / 100) * vixPlayer.duration;
            showControls();
        });
    }
    if (playerMute) playerMute.addEventListener('click', () => {
        if (!isHlsServer(activePlayerServer)) return;
        vixPlayer.muted = !vixPlayer.muted;
        syncVolumeUI();
        showControls();
    });

    // ─── Double-tap-to-seek zones (Netflix/YouTube-style) ─────────────────
    // Tapping once on the left/right edge of the video behaves just like
    // tapping the video itself (toggle play/pause). Tapping twice quickly in
    // the same zone instead seeks ±10s and shows a brief ripple indicator.
    // Consecutive double-taps within the zone accumulate ("20 seconds",
    // "30 seconds"...) before resetting, matching the real apps.
    function setupSeekZone(zoneEl, indicatorEl, amountEl, direction) {
        if (!zoneEl) return;
        const SINGLE_TAP_DELAY = 260;
        const ACCUM_RESET_DELAY = 700;
        let tapCount = 0;
        let singleTapTimer = null;
        let accumSeconds = 0;
        let accumResetTimer = null;

        function doSeek() {
            if (!isHlsServer(activePlayerServer) || !playerReady) return;
            if (direction === 'back') {
                vixPlayer.currentTime = Math.max(0, vixPlayer.currentTime - 10);
            } else if (Number.isFinite(vixPlayer.duration)) {
                vixPlayer.currentTime = Math.min(vixPlayer.duration, vixPlayer.currentTime + 10);
            }
            accumSeconds += 10;
            amountEl.textContent = `${accumSeconds} second${accumSeconds === 1 ? '' : 's'}`;
            indicatorEl.classList.remove('pulse');
            void indicatorEl.offsetWidth; // restart the pulse animation
            indicatorEl.classList.add('show', 'pulse');
            clearTimeout(accumResetTimer);
            accumResetTimer = setTimeout(() => {
                indicatorEl.classList.remove('show');
                accumSeconds = 0;
            }, ACCUM_RESET_DELAY);
            showControls();
        }

        zoneEl.addEventListener('click', () => {
            if (!isHlsServer(activePlayerServer) || !playerReady) return;
            tapCount++;
            if (tapCount === 1) {
                singleTapTimer = setTimeout(() => {
                    tapCount = 0;
                    toggleVixPlay();
                }, SINGLE_TAP_DELAY);
            } else {
                clearTimeout(singleTapTimer);
                tapCount = 0;
                doSeek();
            }
        });
    }
    setupSeekZone(playerSeekZoneLeft, playerSeekIndicatorLeft, playerSeekAmountLeft, 'back');
    setupSeekZone(playerSeekZoneRight, playerSeekIndicatorRight, playerSeekAmountRight, 'forward');

    if (playerVolume) playerVolume.addEventListener('input', () => {
        if (!isHlsServer(activePlayerServer)) return;
        vixPlayer.volume = Number(playerVolume.value);
        vixPlayer.muted = vixPlayer.volume === 0;
        syncVolumeUI();
        showControls();
    });
    vixPlayer.addEventListener('volumechange', syncVolumeUI);
    function syncFullscreenIcon() {
        if (!playerFullscreen) return;
        const isFullscreen = document.fullscreenElement || document.webkitFullscreenElement || document.mozFullScreenElement || document.msFullscreenElement;
        if (isFullscreen) {
            playerFullscreen.innerHTML = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><polyline points="8 3 8 8 3 8"></polyline><polyline points="16 3 16 8 21 8"></polyline><polyline points="8 21 8 16 3 16"></polyline><polyline points="16 21 16 16 21 16"></polyline></svg>';
        } else {
            playerFullscreen.innerHTML = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><polyline points="8 3 3 3 3 8"></polyline><polyline points="16 3 21 3 21 8"></polyline><polyline points="8 21 3 21 3 16"></polyline><polyline points="21 16 21 21 16 21"></polyline></svg>';
        }
    }
    document.addEventListener('fullscreenchange', syncFullscreenIcon);
    document.addEventListener('webkitfullscreenchange', syncFullscreenIcon);
    document.addEventListener('mozfullscreenchange', syncFullscreenIcon);
    document.addEventListener('MSFullscreenChange', syncFullscreenIcon);

    // Attempts to request fullscreen on `el` using whichever vendor-prefixed
    // API is available. Returns the resulting Promise (if any) or null if the
    // element has no fullscreen API at all.
    function requestFullscreenOn(el) {
        if (!el) return null;
        if (el.requestFullscreen) return el.requestFullscreen();
        if (el.webkitRequestFullscreen) { el.webkitRequestFullscreen(); return null; }
        if (el.mozRequestFullScreen) { el.mozRequestFullScreen(); return null; }
        if (el.msRequestFullscreen) { el.msRequestFullscreen(); return null; }
        return undefined; // signals "no API available on this element"
    }

    function togglePlayerFullscreen() {
        const target = playerModal;
        const isFullscreen = document.fullscreenElement || document.webkitFullscreenElement || document.mozFullScreenElement || document.msFullscreenElement;

        try {
            if (!isFullscreen) {
                // Some browsers (older mobile Safari, some in-app/webview
                // browsers, or pages embedded in an iframe without
                // `allow="fullscreen"`) report `fullscreenEnabled === false`.
                // In that case requesting fullscreen on any element will
                // always fail, so go straight to the one thing that *can*
                // still work on iOS: native fullscreen on the <video> itself.
                if (document.fullscreenEnabled === false && !document.webkitFullscreenEnabled) {
                    if (vixPlayer.webkitEnterFullscreen) {
                        vixPlayer.webkitEnterFullscreen();
                    } else {
                        showToast('Fullscreen is disabled in this browser/frame');
                    }
                    showControls();
                    return;
                }

                const result = requestFullscreenOn(target);
                if (result && result.catch) {
                    // Container fullscreen was rejected (permissions policy,
                    // user gesture lost, etc). Fall back to requesting
                    // fullscreen on the <video> element directly, which is
                    // more broadly supported.
                    result.catch(err => {
                        console.warn('Fullscreen failed on player container, trying video element:', err);
                        const videoResult = requestFullscreenOn(vixPlayer);
                        if (videoResult && videoResult.catch) {
                            videoResult.catch(err2 => {
                                console.warn('Fullscreen failed on video element too:', err2);
                                if (vixPlayer.webkitEnterFullscreen) {
                                    vixPlayer.webkitEnterFullscreen();
                                } else {
                                    showToast('Fullscreen blocked by browser (permissions)');
                                }
                            });
                        } else if (videoResult === undefined && vixPlayer.webkitEnterFullscreen) {
                            vixPlayer.webkitEnterFullscreen();
                        }
                    });
                } else if (result === undefined) {
                    // Container had no fullscreen API at all — try the video
                    // element before giving up.
                    const videoResult = requestFullscreenOn(vixPlayer);
                    if (videoResult === undefined) {
                        if (vixPlayer.webkitEnterFullscreen) {
                            vixPlayer.webkitEnterFullscreen();
                        } else {
                            showToast('Fullscreen not supported on this device');
                        }
                    }
                }
            } else {
                if (document.exitFullscreen) {
                    const p = document.exitFullscreen();
                    if (p && p.catch) p.catch(() => {});
                } else if (document.webkitExitFullscreen) {
                    document.webkitExitFullscreen();
                } else if (document.mozCancelFullScreen) {
                    document.mozCancelFullScreen();
                } else if (document.msExitFullscreen) {
                    document.msExitFullscreen();
                }
            }
        } catch (err) {
            console.warn('Fullscreen error:', err);
            showToast('Fullscreen error: ' + err.message);
        }
        showControls();
    }

    if (playerFullscreen) playerFullscreen.addEventListener('click', () => {
        togglePlayerFullscreen();
    });

    // ─── Picture-in-Picture ───────────────────────────────────────────────────
    const pipSupported = !!(document.pictureInPictureEnabled ||
        (vixPlayer.webkitSetPresentationMode && document.pictureInPictureEnabled !== false));
    const syncPipIcon = () => {
        if (!playerPip) return;
        const inPip = document.pictureInPictureElement === vixPlayer ||
            vixPlayer.webkitPresentationMode === 'picture-in-picture';
        playerPip.classList.toggle('active', inPip);
        const label = inPip ? 'Exit picture in picture' : 'Picture in picture';
        playerPip.setAttribute('aria-label', label);
        playerPip.title = label;
    };
    if (playerPip && pipSupported) {
        playerPip.addEventListener('click', async () => {
            try {
                if (document.pictureInPictureElement === vixPlayer ||
                    vixPlayer.webkitPresentationMode === 'picture-in-picture') {
                    if (document.exitPictureInPicture) await document.exitPictureInPicture();
                    else if (vixPlayer.webkitSetPresentationMode) vixPlayer.webkitSetPresentationMode('inline');
                } else {
                    if (vixPlayer.requestPictureInPicture) await vixPlayer.requestPictureInPicture();
                    else if (vixPlayer.webkitSetPresentationMode) vixPlayer.webkitSetPresentationMode('picture-in-picture');
                }
            } catch (err) {
                console.warn('PiP error:', err);
                showToast('Picture-in-picture is not available for this stream');
            }
            showControls();
        });
        // Keep the icon in sync no matter how PiP was toggled (OS UI, media
        // keys, Safari's presentation mode…).
        vixPlayer.addEventListener('enterpictureinpicture', syncPipIcon);
        vixPlayer.addEventListener('leavepictureinpicture', syncPipIcon);
        vixPlayer.addEventListener('webkitpresentationmodechanged', syncPipIcon);
    } else if (playerPip) {
        playerPip.style.display = 'none';
    }

    playerModal.addEventListener('mousemove', (e) => {
        if (!playerModal.classList.contains('show')) return;
        if (e.target.closest('.player-ep-panel')) return;
        showControls();
    });
    playerModal.addEventListener('mouseleave', () => {
        // Same rule as hideControlsIdle: bars only auto-hide once playback
        // is actually running, never while loading/buffering or on error.
        if (!playerLoader || playerLoader.style.display !== 'none') return;
        clearTimeout(controlsHideTimer);
        controlsHideTimer = setTimeout(hideControlsIdle, 3000);
    });
    closePlayer.addEventListener('click', closePlayerModal);

    // ─── Keyboard shortcuts ───────────────────────────────────────────────────
    document.addEventListener('keydown', e => {
        const playerOpen = playerModal.classList.contains('show');
        const detailOpen = detailModal.classList.contains('show');
        const searchOpen = searchOverlay.classList.contains('show');

        // Keep Tab cycling inside whichever overlay is open.
        if (e.key === 'Tab') {
            if (playerOpen) { trapTab(e, playerModal); return; }
            if (detailOpen) { trapTab(e, detailModalCard); return; }
            if (searchOpen) { trapTab(e, searchOverlay); return; }
        }

        if (e.key === 'Escape') {
            if (playerEpPanel.classList.contains('show')) { playerEpPanel.classList.remove('show'); playerEpListBtn.setAttribute('aria-expanded', 'false'); return; }
            // While video fullscreen is active, Esc belongs to the browser's
            // own exit-first handler — some engines fire this keydown alongside
            // the fullscreenchange, and tearing the whole player down on top of
            // that loses playback. The next Esc then closes the player.
            const inFullscreen = document.fullscreenElement || document.webkitFullscreenElement;
            if (playerOpen && !inFullscreen) { closePlayerModal(); return; }
            if (detailOpen) { closeDetailModal(); return; }
            if (searchOpen) { closeSearch(); return; }
        }
        if (playerOpen) {
            const tag = document.activeElement.tagName;
            if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
            if (e.key === ' ' || e.key === 'k' || e.key === 'K') {
                e.preventDefault();
                toggleVixPlay();
                return;
            }
            if (e.key === 'ArrowLeft') {
                e.preventDefault();
                if (Number.isFinite(vixPlayer.currentTime)) {
                    vixPlayer.currentTime = Math.max(0, vixPlayer.currentTime - 10);
                    showControls();
                }
                return;
            }
            if (e.key === 'ArrowRight' && Number.isFinite(vixPlayer.duration) && Number.isFinite(vixPlayer.currentTime)) {
                e.preventDefault();
                vixPlayer.currentTime = Math.min(vixPlayer.duration, vixPlayer.currentTime + 10);
                showControls();
                return;
            }
            if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
                e.preventDefault();
                vixPlayer.volume = Math.min(1, Math.max(0, Number(vixPlayer.volume || 0) + (e.key === 'ArrowUp' ? 0.05 : -0.05)));
                if (vixPlayer.volume > 0) vixPlayer.muted = false;
                syncVolumeUI();
                showControls();
                showToast(`Volume ${Math.round(vixPlayer.volume * 100)}%`);
                return;
            }
            if (e.key === 'm' || e.key === 'M') { e.preventDefault(); vixPlayer.muted = !vixPlayer.muted; syncVolumeUI(); showControls(); return; }
            if ((e.key === 'f' || e.key === 'F') && !e.ctrlKey && !e.metaKey) {
                e.preventDefault();
                togglePlayerFullscreen();
            }
            return;
        }
        if ((e.key === '/' || e.key === 'f') && !e.ctrlKey && !e.metaKey) {
            // Don't hijack the shortcut while the detail modal is open — the
            // search overlay would cover it confusingly.
            if (detailModal.classList.contains('show')) return;
            const tag = document.activeElement.tagName;
            if (tag !== 'INPUT' && tag !== 'TEXTAREA' && tag !== 'SELECT') {
                e.preventDefault();
                openSearch();
            }
        }
    });

    // ─── Initial Load ─────────────────────────────────────────────────────────
    showLoadingSkeleton();
    fetchAndRender('/api/home', 'home');
});
