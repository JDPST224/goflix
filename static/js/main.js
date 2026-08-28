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
    const genreFilterBar      = document.getElementById('genre-filter-bar');
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
    const detailSeasonPicker  = document.getElementById('detail-season-picker');
    const detailSeasonTrigger = document.getElementById('detail-season-trigger');
    const detailSeasonCurrent = document.getElementById('detail-season-current');
    const detailSeasonMenu    = document.getElementById('detail-season-menu');
    const detailEpSearch      = document.getElementById('detail-ep-search');
    const detailEpisodeList   = document.getElementById('detail-episode-list');
    const detailEpLoading     = document.getElementById('detail-ep-loading');
    const detailRelatedSection = document.getElementById('detail-related-section');
    const detailRelatedRow    = document.getElementById('detail-related-row');
    const detailRelatedScrollLeft  = document.getElementById('detail-related-scroll-left');
    const detailRelatedScrollRight = document.getElementById('detail-related-scroll-right');
    if (detailRelatedScrollLeft && detailRelatedScrollRight && detailRelatedRow) {
        detailRelatedScrollLeft.addEventListener('click', () => detailRelatedRow.scrollBy({ left: -600, behavior: 'smooth' }));
        detailRelatedScrollRight.addEventListener('click', () => detailRelatedRow.scrollBy({ left: 600, behavior: 'smooth' }));
    }

    // Season picker trigger is static markup (unlike the provider/genre
    // pickers, which are rebuilt per render), so its open/close toggle is
    // wired once here; only the menu's option list gets rebuilt per show.
    if (detailSeasonPicker && detailSeasonTrigger) {
        detailSeasonTrigger.addEventListener('click', (e) => {
            e.stopPropagation();
            document.querySelectorAll('.provider-picker.open').forEach(p => { if (p !== detailSeasonPicker) p.classList.remove('open'); });
            const open = detailSeasonPicker.classList.toggle('open');
            detailSeasonTrigger.setAttribute('aria-expanded', String(open));
        });
    }

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
    let externalSubtitleTracks = []; // Subtitles fetched from OpenSubtitles/Vidlove via the backend
    let playerAudioInitialized = false;
    let playerSubtitlesForcedOff = false; // user's last explicit subtitle choice was "Off"
    let activeExternalSubtitleIdx = -1;   // External track the player intends to render
    let pendingResumePosition = 0;
    let lastSavedPlaybackSecond = 0;
    let toastTimer = null;
    let heroCrossfadeTimer = null;
    let scrubbing = false;           // user is dragging the seek bar — don't fight their preview
    let lastFocusedBeforeOverlay = null;
    let allCatalogMovies = [];       // full unfiltered payload from the last catalog fetch
    let activeGenre      = 'All';    // genre chip filter (Movies/TV Shows pages)
    // Server pagination for the poster grids (genre chips on Movies/TV Shows,
    // provider Explore-All on Home): once the cached matches are revealed,
    // further batches come from /api/discover one page at a time. Nulled by
    // renderCategoryRows on any rebuild (chip/page switch), which strands
    // in-flight fetches via the identity checks below.
    let gridFeed = null;            // { query, page, seen, done, inflight, dryRounds }
    // True while the Home rows are replaced by an Explore-All grid (provider
    // or genre); lets switchPage re-render Home when its nav entry is clicked
    // again.
    let homeGridActive = false;
    // Home's per-genre rows (Action, Comedy, Horror, ...) are consolidated
    // into one carousel with a dropdown instead of one row each.
    const HOME_GENRE_ROWS = ['Action', 'Action & Adventure', 'Comedy', 'Comedy Shows', 'Horror',
        'Sci-Fi', 'Sci-Fi & Fantasy', 'Drama', 'Mystery', 'Romance', 'Animation', 'Anime'];
    let activeHomeGenre = 'Action';
    const PROVIDERS = [
        { key: 'netflix',   label: 'Netflix' },
        { key: 'prime',     label: 'Prime Video' },
        { key: 'max',       label: 'Max' },
        { key: 'disney',    label: 'Disney+' },
        { key: 'apple',     label: 'Apple TV+' },
        { key: 'paramount', label: 'Paramount+' },
        { key: 'hulu',      label: 'Hulu' },
    ];
    let providersData     = null;
    let activeProvider     = 'netflix';
    let providersRequestId = 0;
    // Brand-ish colors + glyph for each provider's dropdown badge. Prime and
    // Paramount get a small generic icon (play triangle / mountain peaks)
    // instead of a bare letter so they're easier to tell apart at a glance;
    // these are original shapes, not reproductions of the services' actual
    // trademarked logos.
    const PROVIDER_BADGES = {
        netflix:   { text: '',   className: 'badge-netflix', icon: `<svg width="256px" height="256px" viewBox="0 0 256 256" version="1.1" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" preserveAspectRatio="xMidYMid"><defs><radialGradient cx="48.3397178%" cy="49.4186213%" fx="48.3397178%" fy="49.4186213%" r="70.4380887%" gradientTransform="translate(0.483397,0.494186),scale(1.000000,0.550875),translate(-0.483397,-0.494186)" id="radialGradient-1"><stop stop-color="#000000" offset="0%"></stop><stop stop-color="#000000" stop-opacity="0" offset="100%"></stop></radialGradient></defs><g><path d="M141.676338,41.2746569 L141.608906,79.6360396 L141.541118,117.997421 L138.385008,109.0921 C138.383912,109.089031 138.380021,109.077687 138.378943,109.074618 L134.30055,194.477217 C138.310388,205.800934 140.458766,211.845937 140.482897,211.870064 C140.51447,211.901649 142.799605,212.039499 145.561,212.176541 C153.927405,212.591702 164.29504,213.481319 172.159936,214.45853 C173.98096,214.684771 175.548152,214.800631 175.642501,214.716128 C175.736852,214.631624 175.788229,175.572658 175.756672,127.918505 L175.69923,41.2746569 L158.687784,41.2746569 L141.676338,41.2746569 Z" stroke="#000000" stroke-width="2.9562209" fill="#B1060F"></path><path d="M80.1382878,41.1604861 L80.1382878,127.892104 C80.1382878,175.594555 80.1849645,214.670743 80.242112,214.727902 C80.2992558,214.785042 83.2534257,214.50614 86.8069318,214.108168 C90.3604343,213.710178 95.2716247,213.215292 97.7205879,213.008561 C101.476527,212.691477 112.690651,211.970454 113.989212,211.962472 C114.366954,211.960097 114.391182,210.011234 114.445895,175.226595 L114.503693,138.493217 L117.217033,146.170131 C117.636362,147.356673 117.767894,147.727198 118.176424,148.883116 L122.253748,63.5015679 C121.389836,61.0590106 121.842711,62.3412234 120.852658,59.5419824 C117.521259,50.1228923 114.694332,42.1337874 114.570412,41.7884254 L114.344924,41.1604861 L97.2417849,41.1604861 L80.1382878,41.1604861 Z" stroke="#000000" stroke-width="2.9562209" fill="#B1060F"></path><path d="M80.1382787,41.1604861 L80.1382878,89.8454048 L114.434478,180.820963 C114.438058,178.736175 114.44215,177.609688 114.445895,175.226595 L114.503693,138.493217 L117.217033,146.170131 C132.320656,188.907688 140.435174,211.82235 140.482897,211.870064 C140.51447,211.901649 142.799605,212.039499 145.561,212.176541 C153.927405,212.591702 164.29504,213.481319 172.159936,214.45853 C173.98096,214.684771 175.548152,214.800631 175.642501,214.716128 C175.70735,214.658045 175.749683,195.506553 175.760954,168.489092 L141.625319,70.3489604 L141.608897,79.6360396 L141.541109,117.997421 L138.384999,109.0921 C135.301137,100.390624 133.24206,94.5714036 120.852649,59.5419824 C117.52125,50.1228923 114.694323,42.1337874 114.570403,41.7884254 L114.344915,41.1604861 L97.2417758,41.1604861 L80.1382787,41.1604861 Z" fill="url(#radialGradient-1)"></path><path d="M80.1390021,41.160477 L114.503693,138.537458 L114.503693,138.493217 L117.217033,146.170131 C132.320656,188.907688 140.435174,211.82235 140.482897,211.870064 C140.51447,211.901649 142.799605,212.039499 145.561,212.176541 C153.927405,212.591702 164.29504,213.481319 172.159936,214.45853 C173.971627,214.683611 175.530793,214.799226 175.639648,214.717197 L141.541118,117.979583 L141.541118,117.997412 L138.385008,109.092091 C135.301146,100.390615 133.242069,94.5713945 120.852658,59.5419732 C117.521259,50.1228832 114.694332,42.1337783 114.570412,41.7884163 L114.344924,41.160477 L97.2417849,41.160477 L80.1390021,41.160477 Z" fill="#E50914"></path></g></svg>` },
        prime:     { text: '',   className: 'badge-prime', icon: `<svg width="24px" height="24px" viewBox="0 0 24 24" role="img" xmlns="http://www.w3.org/2000/svg"><title>Prime icon</title><path d="M22.787 15.292c-.336-.43-2.222-.204-3.069-.103-.257.031-.296-.193-.065-.356 1.504-1.056 3.968-.75 4.255-.397.288.357-.076 2.827-1.485 4.007-.217.18-.423.084-.327-.155.317-.792 1.027-2.566.69-2.996m-1.093 1.248c-2.627 1.94-6.437 2.97-9.717 2.97-4.597 0-8.737-1.7-11.87-4.528-.246-.222-.026-.525.27-.353 3.38 1.967 7.559 3.151 11.876 3.151a23.63 23.63 0 0 0 9.06-1.854c.444-.188.816.293.381.614m.482-5.038c-.761 0-1.346-.209-1.755-.626-.409-.418-.613-1.017-.613-1.797 0-.799.209-1.425.627-1.88.418-.454.998-.682 1.741-.682.572 0 1.019.138 1.341.415.323.276.484.645.484 1.105 0 .461-.174.81-.52 1.046-.348.237-.86.355-1.535.355-.35 0-.654-.034-.912-.101.037.411.161.706.373.884.212.178.533.268.963.268.172 0 .34-.011.502-.033a6.208 6.208 0 0 0 .733-.157.304.304 0 0 1 .046-.004c.104 0 .156.07.156.212v.424c0 .098-.013.167-.04.207a.341.341 0 0 1-.162.106 3.954 3.954 0 0 1-1.429.258m-.304-2.893c.314 0 .541-.048.682-.143.142-.095.212-.241.212-.438 0-.387-.23-.58-.69-.58-.59 0-.931.362-1.024 1.087.246.05.52.074.82.074m-9.84 2.755c-.08 0-.139-.018-.176-.055-.036-.037-.055-.096-.055-.175V6.886c0-.086.019-.146.055-.18.037-.034.096-.05.176-.05h.663c.141 0 .227.067.258.202l.074.249c.325-.215.619-.367.88-.456.26-.09.53-.134.806-.134.553 0 .943.197 1.17.59a3.77 3.77 0 0 1 .885-.452c.276-.092.562-.138.857-.138.43 0 .763.12 1 .36.236.239.354.574.354 1.004v3.253c0 .08-.017.138-.05.175-.034.037-.094.055-.18.055h-.885c-.08 0-.138-.018-.175-.055-.037-.037-.055-.096-.055-.175V8.176c0-.418-.188-.627-.562-.627-.332 0-.667.08-1.005.24v3.345c0 .08-.017.138-.05.175-.034.037-.094.055-.18.055h-.884c-.08 0-.139-.018-.176-.055-.036-.037-.055-.096-.055-.175V8.176c0-.418-.187-.627-.562-.627-.344 0-.682.083-1.013.249v3.336c0 .08-.017.138-.051.175-.034.037-.094.055-.18.055zM9.987 5.927c-.234 0-.42-.064-.562-.193-.142-.129-.212-.304-.212-.525 0-.221.07-.397.212-.526.141-.129.328-.193.562-.193.233 0 .42.064.562.193a.676.676 0 0 1 .212.526c0 .22-.07.396-.212.525-.141.129-.329.193-.562.193m-.443 5.437c-.08 0-.138-.019-.175-.055-.037-.037-.055-.096-.055-.176V6.886c0-.086.018-.146.055-.18.037-.034.096-.05.175-.05h.885c.086 0 .146.016.18.05s.05.094.05.18v4.247c0 .08-.017.139-.05.176-.034.036-.094.055-.18.055zm-3.681 0c-.08 0-.139-.018-.176-.055-.036-.037-.055-.096-.055-.175V6.886c0-.086.019-.146.055-.18.037-.034.096-.05.176-.05h.663c.141 0 .227.067.258.202l.12.497c.245-.27.477-.462.695-.575.219-.114.45-.17.696-.17h.13c.085 0 .147.016.183.05.037.034.056.094.056.18v.773c0 .08-.017.139-.051.176-.034.036-.094.055-.18.055a1.93 1.93 0 0 1-.166-.01 2.968 2.968 0 0 0-.258-.009c-.14 0-.313.02-.516.06-.202.04-.374.091-.515.152v3.097c0 .08-.018.138-.051.175-.034.037-.094.055-.18.055zM.344 13.262c-.08 0-.138-.017-.175-.05-.037-.034-.055-.095-.055-.18V6.886c0-.086.018-.146.055-.18.037-.034.095-.05.175-.05h.664c.14 0 .227.067.258.202l.064.24a2.03 2.03 0 0 1 .668-.424 2.13 2.13 0 0 1 .797-.157c.596 0 1.067.218 1.414.654.348.437.521 1.026.521 1.77 0 .51-.086.955-.258 1.336-.172.38-.405.674-.7.88a1.727 1.727 0 0 1-1.014.308c-.252 0-.491-.04-.719-.12a1.74 1.74 0 0 1-.58-.331v2.018c0 .085-.017.146-.05.18-.034.033-.095.05-.18.05zm2.018-2.81c.344 0 .597-.117.76-.35.163-.234.245-.603.245-1.106 0-.51-.08-.882-.24-1.115-.16-.234-.415-.35-.765-.35-.32 0-.62.083-.903.248v2.424c.27.166.571.249.903.249Z"/></svg>` },
        max:       { text: '',   className: 'badge-max', icon: `<svg width="800px" height="800px" viewBox="0 0 192 192" xmlns="http://www.w3.org/2000/svg" id="Layer_1"><rect width="100%" height="100%" fill="#ffffff"/><defs><style>.cls-3,.cls-4{fill:none;stroke:#000000;stroke-linejoin:round}.cls-4{stroke-width:12px}.cls-3{stroke-width:8px}.cls-3,.cls-4{stroke-linecap:round}</style></defs><path d="M0 0h192v192H0z" style="fill:none"/><circle cx="137.82" cy="71.24" r="9.4"/><path d="M137.82 56.81c7.96 0 14.43 6.48 14.43 14.43s-6.48 14.43-14.43 14.43-14.43-6.48-14.43-14.43 6.48-14.43 14.43-14.43m0-12c-14.6 0-26.43 11.83-26.43 26.43s11.83 26.43 26.43 26.43 26.43-11.83 26.43-26.43-11.83-26.43-26.43-26.43Z"/><path d="M43.23 51.86v38.77m23.34-38.77v38.77M43.52 71.24h23.05" class="cls-4"/><path d="M82.91 51.86v38.77h16.55c4.41-.36 7.91-4.45 8.06-9.4.15-4.88-2.99-9.19-7.29-9.99 4.3-.79 7.44-5.1 7.29-9.99-.14-4.84-3.47-8.88-7.77-9.4H82.9Z" style="stroke-width:12px;stroke:#000000;stroke-linejoin:round;fill:none"/><path d="M82.91 71.24h17.33" class="cls-4"/><path d="M47.68 116.06c-8.48-13.02-15.72-12.3-20.67-8.71-4.5 3.27-7.01 8.82-7.01 14.58v24.68m27.68-.01v-30.54" class="cls-3"/><path d="M75.36 146.6v-23.47c0-6.09-2.65-11.94-7.38-15.45-12.13-9-20.31 8.38-20.31 8.38m76.49-10.97v42.1" class="cls-3"/><ellipse cx="105.28" cy="126.63" class="cls-3" rx="18.4" ry="20.56"/><path d="M135.76 104.9 172 147.19m0-42.29-36.24 42.29" class="cls-3"/></svg>` },
        disney:    { text: '',   className: 'badge-disney', icon: `<svg fill="#000000" width="800px" height="800px" viewBox="0 0 24 24" id="disney-plus" data-name="Flat Color" xmlns="http://www.w3.org/2000/svg" class="icon flat-color"><path id="secondary" d="M19,8a1,1,0,0,1-1-1V6H17a1,1,0,0,1,0-2h1V3a1,1,0,0,1,2,0V4h1a1,1,0,0,1,0,2H20V7A1,1,0,0,1,19,8Z" style="fill: rgb(44, 169, 188);"></path><path id="primary" d="M17.89,12.58C16.47,4.87,7.42,4.26,2.84,5a1,1,0,1,0,.32,2c.12,0,11.34-1.78,12.76,6a3.39,3.39,0,0,1-1,3.38C13.59,17.58,11.1,18,9.19,17.92l-1-6.19a5.66,5.66,0,0,1,3.21,1.07,1,1,0,1,0,1.19-1.6A7.59,7.59,0,0,0,7.82,9.74l-.15-.9a1,1,0,1,0-2,.32l.14.84a7.67,7.67,0,0,0-1.33.48A4.07,4.07,0,0,0,2,14.06c0,2.54,2,4.68,5.16,5.57l.29.06.25,1.47a1,1,0,0,0,1,.84h.17a1,1,0,0,0,.82-1.15l-.15-.92h.19a10.1,10.1,0,0,0,6.59-2.12A5.4,5.4,0,0,0,17.89,12.58ZM4,14.06c0-.94.88-1.52,1.41-1.79A5.22,5.22,0,0,1,6.17,12l.92,5.52C5.15,16.79,4,15.54,4,14.06Z" style="fill: rgb(0, 0, 0);"></path></svg>` },
        apple:     { text: '',   className: 'badge-apple', icon: `<svg width="24px" height="24px" viewBox="0 0 24 24" role="img" xmlns="http://www.w3.org/2000/svg"><path d="M20.57 17.735h-1.815l-3.34-9.203h1.633l2.02 5.987c.075.231.273.9.586 2.012l.297-.997.33-1.006 2.094-6.004H24zm-5.344-.066a5.76 5.76 0 0 1-1.55.207c-1.23 0-1.84-.693-1.84-2.087V9.646h-1.063V8.532h1.121V7.081l1.476-.602v2.062h1.707v1.113H13.38v5.805c0 .446.074.75.214.932.14.182.396.264.75.264.207 0 .495-.041.883-.115zm-7.29-5.343c.017 1.764 1.55 2.358 1.567 2.366-.017.042-.248.842-.808 1.658-.487.71-.99 1.418-1.79 1.435-.783.016-1.03-.462-1.93-.462-.89 0-1.17.445-1.913.478-.758.025-1.344-.775-1.838-1.484-.998-1.451-1.765-4.098-.734-5.88.51-.89 1.426-1.451 2.416-1.46.75-.016 1.468.512 1.93.512.461 0 1.327-.627 2.234-.536.38.016 1.452.157 2.136 1.154-.058.033-1.278.743-1.27 2.219M6.468 7.988c.404-.495.685-1.18.61-1.864-.585.025-1.294.388-1.723.883-.38.437-.71 1.138-.619 1.806.652.05 1.328-.338 1.732-.825z"/></svg>` },
        paramount: { text: '',   className: 'badge-paramount', icon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="100%" height="100%"><defs><linearGradient id="skyGrad" x1="0%" y1="0%" x2="0%" y2="100%"><stop offset="0%" stop-color="#0064d2"/><stop offset="100%" stop-color="#00185a"/></linearGradient></defs><rect width="256" height="256" fill="url(#skyGrad)"/><path d="M50 200 L128 80 L206 200 Z" fill="#ffffff"/><path d="M128 80 L160 145 L128 160 L95 145 Z" fill="#dbeafe" opacity="0.7"/><g fill="#ffffff"><circle cx="128" cy="40" r="5" /><circle cx="92" cy="47" r="5" /><circle cx="164" cy="47" r="5" /><circle cx="60" cy="67" r="5" /><circle cx="196" cy="67" r="5" /><circle cx="36" cy="98" r="5" /><circle cx="220" cy="98" r="5" /><circle cx="22" cy="138" r="5" /><circle cx="234" cy="138" r="5" /></g><path d="M 120 200 L 136 200 M 128 192 L 128 208" stroke="#0064d2" stroke-width="6" stroke-linecap="round"/></svg>` },
        hulu:      { text: '',   className: 'badge-hulu', icon: `<svg width="32px" height="32px" viewBox="-4 -4 40 40" xmlns="http://www.w3.org/2000/svg"><path d="M19.197 9.807h-4.807c-0.943 0.016-1.875 0.199-2.751 0.543v-10.391h-7.719v32.083h7.729v-12.681c-0.067-1.204 0.876-2.229 2.084-2.267h4.52c1.152 0 2.095 0.923 2.12 2.079v12.787h7.704v-13.907c0-5.88-3-8.213-7.865-8.213z"/></svg>` },
    };



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

    // Placeholder footer/social anchors shouldn't jump the page to the top.
    document.querySelector('footer')?.addEventListener('click', e => {
        if (e.target.closest('a[href="#"]')) e.preventDefault();
    });

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
    // The hamburger dropdown has no backdrop of its own, so dismiss it on
    // any tap outside it (and on Escape), like the other menus here.
    function closeMobileNav() {
        primaryNavigation?.classList.remove('open');
        mobileNavToggle?.setAttribute('aria-expanded', 'false');
        mobileNavToggle?.setAttribute('aria-label', 'Open navigation');
    }
    document.addEventListener('click', (e) => {
        if (profileWrap && !profileWrap.contains(e.target)) closeProfileMenu();
        if (mobileNavToggle && primaryNavigation &&
            !primaryNavigation.contains(e.target) &&
            !mobileNavToggle.contains(e.target)) {
            closeMobileNav();
        }
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') { closeProfileMenu(); closeMobileNav(); }
    });

    // Closes the "Only on …" provider dropdown when clicking elsewhere or
    // pressing Escape. Registered once here (not per-render) since the
    // picker itself is rebuilt every time the provider row re-renders.
    function closeProviderPicker() {
        const open = document.querySelector('.provider-picker.open');
        if (!open) return;
        open.classList.remove('open');
        open.querySelector('.provider-trigger')?.setAttribute('aria-expanded', 'false');
    }
    document.addEventListener('click', (e) => {
        const open = document.querySelector('.provider-picker.open');
        if (open && !open.contains(e.target)) closeProviderPicker();
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') closeProviderPicker();
    });

    function setActiveNav(page) {
        navLinks.forEach(l => l.classList.toggle('active', l.dataset.page === page));
    }

    function switchPage(page) {
        // homeGridActive: Home's nav entry must restore the rows when a
        // provider grid has replaced them, so don't early-out for same-page.
        if (page === currentPage && page !== 'mylist' && !homeGridActive) return;
        currentPage = page;
        setActiveNav(page);
        stopHeroRotation();
        // Drop any in-flight hero crossfade from the previous page so its
        // timer can't repaint a stale banner over the new page's hero.
        resetHeroCrossfade();
        mylistEmpty.style.display = 'none';
        carouselsContainer.style.display = '';
        // The bar shows only on Movies/TV Shows; each page has a different
        // genre-name set, so any filter chosen on the other page is stale.
        activeGenre = 'All';
        const showGenreBar = (page === 'movies' || page === 'tvshows');
        genreFilterBar.style.display = showGenreBar ? '' : 'none';
        genreFilterBar.innerHTML = '';
        // Swaps which element carries the -80px pull onto the hero: the bar
        // when it's showing, carousels-container itself when it's not.
        carouselsContainer.classList.toggle('with-genre-bar', showGenreBar);

        // Scroll to top when switching pages — via scrollTopInstant(), not
        // window.scrollTo (a no-op here: body is the effective scroller), and
        // instant so the smooth animation can't fight the DOM rebuild below.
        scrollTopInstant();

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
                allCatalogMovies = movies;

                // Hero rendering stays here (outside renderCategoryRows) so
                // genre-chip switches never touch the banner.
                heroMovies = page === 'home' ? pickHomeHeroes(movies) : movies.filter(m => m.banner).slice(0, 8);
                heroIndex  = 0;
                renderHeroMovies(heroMovies);
                startHeroRotation();

                if (page === 'movies' || page === 'tvshows') {
                    buildGenreChips();
                }
                renderCategoryRows(page, applyGenreFilter(movies));
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

    // Renders everything below the hero: Continue Watching, the provider
    // carousel and Genres picker rows (home only), and the category-grouped
    // rows. Called on every fetch and again, with just a filtered slice,
    // whenever the genre chip selection changes.
    function renderCategoryRows(page, movies) {
        carouselsContainer.innerHTML = '';
        gridFeed = null; // any in-flight discover fetch now appends nowhere
        homeGridActive = false; // rows are back, so the switchPage guard must not skip

        const categories = {};
        movies.forEach(movie => {
            (movie.categories || []).forEach(cat => {
                if (!categories[cat]) categories[cat] = [];
                categories[cat].push(movie);
            });
        });

        // Home row order: Continue Watching, Only on …, Genres, then the
        // category rows from /api/home (whose payload order matches).
        if (page === 'home') {
            const cw = getContinueWatching();
            if (cw && cw.length > 0) {
                renderRow('Continue Watching', cw, true);
            }

            // Reserved slot so the async provider carousel lands here
            // (second row) instead of wherever it happens to finish loading
            // relative to the rest of the page.
            const providerSlot = document.createElement('div');
            providerSlot.id = 'provider-row-slot';
            carouselsContainer.appendChild(providerSlot);
            loadProvidersRow();

            // Genres picker row sits third, above the catalog rows below.
            renderHomeGenreRow(categories);

            // Picked for You renders mid-loop, right after the Top-10 rows.
        }

        // Genre chip filtering (Movies/TV Shows pages): the per-source-category
        // rows below are great for unfiltered browsing, but a genre match can
        // land in any of ~15 source lists, so filtering them individually
        // leaves most rows near-empty. Instead, pool every match into one
        // deduplicated, well-populated row (source lists overlap, so the same
        // title can appear more than once in `movies`).
        if (activeGenre !== 'All') {
            const seen = new Set();
            const matches = [];
            movies.forEach(m => {
                const key = mediaKey(m);
                if (seen.has(key)) return;
                seen.add(key);
                matches.push(m);
            });

            if (matches.length === 0) {
                const notice = document.createElement('div');
                notice.className = 'no-titles-notice';
                notice.textContent = 'No titles found for this genre.';
                carouselsContainer.appendChild(notice);
                return;
            }

            matches.sort((a, b) => b.rating - a.rating);
            renderGenreGrid(activeGenre, matches, page);
            return;
        }

        if (Object.keys(categories).length === 0) {
            const notice = document.createElement('div');
            notice.className = 'no-titles-notice';
            notice.textContent = 'No titles found for this genre.';
            carouselsContainer.appendChild(notice);
            return;
        }

        // Picked for You slots in after the Top-10 rows (or after whichever of
        // them made it into the payload, should the other be missing).
        const picksAfterCat = page === 'home'
            ? ('Trending TV' in categories ? 'Trending TV' : 'Trending Movies')
            : null;
        const picks = page === 'home' ? buildPicksForYou(allCatalogMovies) : [];

        Object.keys(categories).forEach(cat => {
            if (page === 'home' && HOME_GENRE_ROWS.includes(cat)) return; // consolidated into the Genres picker row above
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
            if (cat === picksAfterCat && picks.length >= 5) {
                renderRow('Picked for You', picks);
            }
        });
    }

    // Renders genre-filtered results as a wrapping grid instead of a
    // horizontal carousel, revealing cached matches as the user scrolls near
    // the bottom; once they run out, further pages are fetched from
    // /api/discover and appended (deduplicated against everything shown).
    const GENRE_GRID_BATCH = 24;

    // Shared poster-grid renderer: cached matches first, then /api/discover
    // pages via feedQuery(page). headerNode (optional) replaces the plain
    // title — the provider grid puts its back button + provider picker there.
    function renderGrid(titleText, matches, feedQuery, headerNode) {
        const section = document.createElement('div');
        section.className = 'genre-grid-section';

        if (headerNode) {
            section.appendChild(headerNode);
        } else {
            const title = document.createElement('h3');
            title.className = 'genre-grid-title';
            title.textContent = titleText;
            section.appendChild(title);
        }

        const grid = document.createElement('div');
        grid.className = 'genre-grid';
        section.appendChild(grid);

        const sentinel = document.createElement('div');
        sentinel.className = 'genre-grid-sentinel';
        section.appendChild(sentinel);

        carouselsContainer.appendChild(section);

        // One feed per grid: identity-checked by the in-flight fetch so a
        // chip/page switch mid-request strands the stale response.
        gridFeed = {
            query: feedQuery,
            page: 1,
            seen: new Set(matches.map(mediaKey)),
            done: false,
            inflight: false,
            dryRounds: 0,
        };

        let loaded = 0;
        let loadingEl = null;
        const observer = new IntersectionObserver((entries) => {
            if (entries[0].isIntersecting) loadMoreGenreItems();
        }, { rootMargin: '800px' });

        function loadMoreGenreItems() {
            if (loaded < matches.length) {
                const next = matches.slice(loaded, loaded + GENRE_GRID_BATCH);
                next.forEach(m => grid.appendChild(createPosterCard(m)));
                loaded += next.length;
            }
            if (loaded >= matches.length) fetchMoreGenreItems();
        }

        // Fetches the next /api/discover page and appends the titles not
        // already shown. An empty page (or TMDB's 500-page cap) ends the
        // feed; a network failure keeps the sentinel so the next scroll
        // retries. Stale responses (grid rebuilt mid-fetch) bail on the
        // gridFeed identity check.
        async function fetchMoreGenreItems() {
            const feed = gridFeed;
            if (!feed || feed.done || feed.inflight) return;
            feed.inflight = true;
            toggleGridLoading(true);
            try {
                const res = await fetch(`/api/discover?${feed.query(feed.page)}`);
                if (!res.ok) throw new Error(`HTTP ${res.status}`);
                const items = await res.json();
                if (gridFeed !== feed) return; // grid was rebuilt mid-fetch
                if (!Array.isArray(items) || items.length === 0 || feed.page >= 500) {
                    finishFeed();
                    return;
                }
                feed.page++;
                let added = 0;
                for (const m of items) {
                    const key = mediaKey(m);
                    if (feed.seen.has(key)) continue;
                    feed.seen.add(key);
                    grid.appendChild(createPosterCard(m));
                    added++;
                }
                feed.dryRounds = added > 0 ? 0 : feed.dryRounds + 1;
                if (feed.dryRounds >= 4) { finishFeed(); return; } // pages keep returning only dupes
            } catch (err) {
                if (gridFeed !== feed) return;
                console.error('Discover fetch failed:', err);
            } finally {
                const stale = gridFeed !== feed;
                if (!stale) {
                    feed.inflight = false;
                    toggleGridLoading(false);
                    // The observer is edge-triggered: while the sentinel
                    // stays in view it never refires, so keep loading until
                    // the appended rows push it out of the 800px window.
                    if (!feed.done && sentinelNearViewport()) loadMoreGenreItems();
                } else {
                    loadingEl?.remove();
                }
            }
        }

        function finishFeed() {
            if (!gridFeed) return;
            gridFeed.done = true;
            toggleGridLoading(false);
            observer.disconnect();
            sentinel.remove();
        }

        function sentinelNearViewport() {
            const rect = sentinel.getBoundingClientRect();
            return rect.top < window.innerHeight + 800 && rect.bottom > 0;
        }

        function toggleGridLoading(show) {
            if (show && !loadingEl) {
                loadingEl = document.createElement('div');
                loadingEl.className = 'genre-grid-loading';
                sentinel.parentNode.insertBefore(loadingEl, sentinel);
            } else if (!show && loadingEl) {
                loadingEl.remove();
                loadingEl = null;
            }
        }

        loadMoreGenreItems();
        // Always observe: even when the cached matches all fit in the first
        // batch (loaded === matches.length) the discover fetch chain can
        // park with the sentinel off-screen, and scrolling is what resumes
        // it. Redundant callbacks are guarded by done/inflight.
        observer.observe(sentinel);
    }

    // Genre chips (Movies/TV Shows pages) feed the shared grid a fixed-type
    // discover query; the server resolves the genre name to its TMDB ID.
    function renderGenreGrid(genreName, matches, page) {
        const type = page === 'tvshows' ? 'tv' : 'movie';
        renderGrid(genreName, matches, p => `type=${type}&genre=${encodeURIComponent(genreName)}&page=${p}`);
    }

    // Renders Home's "Genres" carousel: one row with a dropdown (reusing the
    // "Only on …" picker's look) instead of a separate carousel per genre.
    function renderHomeGenreRow(categories) {
        const available = HOME_GENRE_ROWS.filter(g => (categories[g] || []).length > 0);
        if (available.length === 0) return;
        if (!available.includes(activeHomeGenre)) activeHomeGenre = available[0];

        const rowDiv = document.createElement('div');
        rowDiv.className = 'row';

        const rowHeader = document.createElement('div');
        rowHeader.className = 'row-header';

        // Pure client-side swap — the whole catalog is already in memory.
        rowHeader.appendChild(makeGenrePicker(available, () => {
            postersDiv.innerHTML = '';
            (categories[activeHomeGenre] || []).forEach(movie => postersDiv.appendChild(createPosterCard(movie)));
        }));

        // "Explore All" opens the genre's full catalog as a paginated grid,
        // same flow as the provider rows.
        const exploreAll = document.createElement('button');
        exploreAll.type = 'button';
        exploreAll.className = 'row-explore-all explore-all-static';
        exploreAll.innerHTML = `Explore All <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"></polyline></svg>`;
        exploreAll.addEventListener('click', () => openHomeGenreGrid(activeHomeGenre));
        rowHeader.appendChild(exploreAll);

        rowDiv.appendChild(rowHeader);

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
        edgeLeft.addEventListener('click', () => postersDiv.scrollBy({ left: -scrollAmount, behavior: 'smooth' }));
        edgeRight.addEventListener('click', () => postersDiv.scrollBy({ left: scrollAmount, behavior: 'smooth' }));

        (categories[activeHomeGenre] || []).forEach(movie => postersDiv.appendChild(createPosterCard(movie)));

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

    // ─── Genre chip filter (Movies & TV Shows pages) ───────────────────────────
    // Canonical display order for chips; anything not listed falls back to
    // alphabetical order after these.
    const GENRE_ORDER = ['Action', 'Adventure', 'Animation', 'Comedy', 'Crime', 'Documentary',
        'Drama', 'Family', 'Fantasy', 'History', 'Horror', 'Music', 'Mystery', 'Romance',
        'Science Fiction', 'Thriller', 'War', 'Western'];

    function buildGenreChips() {
        const genreSet = new Set();
        allCatalogMovies.forEach(m => (m.genres || []).forEach(g => genreSet.add(g)));
        const genres = Array.from(genreSet).sort((a, b) => {
            const ia = GENRE_ORDER.indexOf(a), ib = GENRE_ORDER.indexOf(b);
            if (ia === -1 && ib === -1) return a.localeCompare(b);
            if (ia === -1) return 1;
            if (ib === -1) return -1;
            return ia - ib;
        });

        genreFilterBar.innerHTML = '';
        const makeChip = (label) => {
            const chip = document.createElement('button');
            chip.type = 'button';
            chip.className = 'genre-chip' + (activeGenre === label ? ' active' : '');
            chip.textContent = label;
            chip.setAttribute('aria-pressed', String(activeGenre === label));
            chip.addEventListener('click', () => {
                if (activeGenre === label) return;
                activeGenre = label;
                genreFilterBar.querySelectorAll('.genre-chip').forEach(c => {
                    const isActive = c.textContent === label;
                    c.classList.toggle('active', isActive);
                    c.setAttribute('aria-pressed', String(isActive));
                });
                renderCategoryRows(currentPage, applyGenreFilter(allCatalogMovies));
            });
            return chip;
        };

        genreFilterBar.appendChild(makeChip('All'));
        genres.forEach(g => genreFilterBar.appendChild(makeChip(g)));
    }

    function applyGenreFilter(movies) {
        if (activeGenre === 'All') return movies;
        return movies.filter(m => (m.genres || []).includes(activeGenre));
    }

    // ─── Diversified hero (Home) ────────────────────────────────────────────
    // Alternates movie/TV picks from the first movie- and TV-type category
    // groups in payload order, instead of always drawing all 8 banners from
    // today's trending movies.
    function pickHomeHeroes(movies) {
        const movieCat = movies.find(m => m.type === 'movie' && (m.categories || []).length)?.categories?.[0];
        const tvCat    = movies.find(m => m.type === 'tv'    && (m.categories || []).length)?.categories?.[0];
        const movieItems = movies.filter(m => m.banner && m.type === 'movie' && (!movieCat || (m.categories || []).includes(movieCat)));
        const tvItems    = movies.filter(m => m.banner && m.type === 'tv'    && (!tvCat    || (m.categories || []).includes(tvCat)));

        const picks = [];
        let i = 0;
        while (picks.length < 8 && (i < movieItems.length || i < tvItems.length)) {
            if (i < movieItems.length) picks.push(movieItems[i]);
            if (picks.length < 8 && i < tvItems.length) picks.push(tvItems[i]);
            i++;
        }
        // Fall back to the plain banner slice if the payload didn't yield
        // enough diversified picks (e.g. seeded test data).
        if (picks.length < 8) {
            const seen = new Set(picks.map(mediaKey));
            for (const m of movies) {
                if (picks.length >= 8) break;
                if (m.banner && !seen.has(mediaKey(m))) { picks.push(m); seen.add(mediaKey(m)); }
            }
        }
        return picks.slice(0, 8);
    }

    // ─── Picked for You (Home) ──────────────────────────────────────────────
    function buildPicksForYou(catalog) {
        const signal = [...getMyList(), ...getContinueWatching()];
        if (signal.length === 0) return [];

        const genreCounts = {};
        signal.forEach(m => (m.genres || []).forEach(g => { genreCounts[g] = (genreCounts[g] || 0) + 1; }));
        const topGenres = Object.keys(genreCounts)
            .sort((a, b) => genreCounts[b] - genreCounts[a])
            .slice(0, 3);
        if (topGenres.length === 0) return [];

        const excludeKeys = new Set(signal.map(mediaKey));
        const candidates = catalog
            .filter(m => !excludeKeys.has(mediaKey(m)))
            .map(m => {
                const matchCount = (m.genres || []).filter(g => topGenres.includes(g)).length;
                return { m, matchCount };
            })
            .filter(({ matchCount }) => matchCount > 0);

        candidates.sort((a, b) => (b.matchCount - a.matchCount) || (b.m.rating - a.m.rating));
        return candidates.slice(0, 20).map(({ m }) => m);
    }

    // ─── "Only on …" provider carousel (Home) ──────────────────────────────
    // Server-cached per-provider lists; switching providers is a pure
    // client-side swap over data already fetched once per Home visit.
    function loadProvidersRow() {
        const requestId = ++providersRequestId;
        fetch('/api/providers')
            .then(res => {
                if (!res.ok) throw new Error(`HTTP ${res.status}`);
                return res.json();
            })
            .then(data => {
                if (requestId !== providersRequestId || currentPage !== 'home') return;
                if (!data || Object.keys(data).length === 0) return;
                providersData = data;
                if (!providersData[activeProvider] || providersData[activeProvider].length === 0) {
                    const fallback = PROVIDERS.find(p => (providersData[p.key] || []).length > 0);
                    if (!fallback) return;
                    activeProvider = fallback.key;
                }
                renderProviderRow(providersData[activeProvider]);
            })
            .catch(err => {
                console.warn('Could not load provider carousel:', err);
            });
    }

    function renderProviderRow(items) {
        const slot = document.getElementById('provider-row-slot');
        if (!slot || !items || items.length === 0) return;
        slot.innerHTML = '';

        const rowDiv = document.createElement('div');
        rowDiv.className = 'row';

        const rowHeader = document.createElement('div');
        rowHeader.className = 'row-header';

        rowHeader.appendChild(makeProviderPicker(() => {
            postersDiv.innerHTML = '';
            (providersData[activeProvider] || []).forEach(movie => postersDiv.appendChild(createPosterCard(movie)));
        }));

        // "Explore All" opens the provider's full catalog as a paginated grid
        // (movies + TV interleaved server-side, fetched page by page on scroll).
        const exploreAll = document.createElement('button');
        exploreAll.type = 'button';
        exploreAll.className = 'row-explore-all explore-all-static';
        exploreAll.innerHTML = `Explore All <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"></polyline></svg>`;
        exploreAll.addEventListener('click', () => openProviderGrid(activeProvider));
        rowHeader.appendChild(exploreAll);

        rowDiv.appendChild(rowHeader);

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
        edgeLeft.addEventListener('click', () => postersDiv.scrollBy({ left: -scrollAmount, behavior: 'smooth' }));
        edgeRight.addEventListener('click', () => postersDiv.scrollBy({ left: scrollAmount, behavior: 'smooth' }));

        items.forEach(movie => postersDiv.appendChild(createPosterCard(movie)));

        postersDiv.setAttribute('tabindex', '0');
        postersDiv.addEventListener('keydown', e => {
            if (e.key === 'ArrowRight') { e.preventDefault(); postersDiv.scrollBy({ left: 220, behavior: 'smooth' }); }
            if (e.key === 'ArrowLeft')  { e.preventDefault(); postersDiv.scrollBy({ left: -220, behavior: 'smooth' }); }
        });

        scrollWrap.appendChild(edgeLeft);
        scrollWrap.appendChild(postersDiv);
        scrollWrap.appendChild(edgeRight);
        rowDiv.appendChild(scrollWrap);

        slot.appendChild(rowDiv);
    }

    // Builds the "Only on …" dropdown (trigger + option menu) shared by the
    // Home carousel row and the provider Explore-All grid. Only lists
    // providers the cache has titles for — TMDB's regional coverage varies a
    // lot per provider, so an entry can come back empty even with a correct
    // ID; showing it anyway would just land the user on a blank strip.
    // onSelect(key) fires after a different provider is chosen (menu closed,
    // aria + trigger label updated); call sites re-render their content.
    function makeProviderPicker(onSelect) {
        const pickerWrap = document.createElement('div');
        pickerWrap.className = 'provider-picker';

        const trigger = document.createElement('button');
        trigger.type = 'button';
        trigger.className = 'provider-trigger';
        trigger.setAttribute('aria-haspopup', 'listbox');
        trigger.setAttribute('aria-expanded', 'false');
        const currentLabel = PROVIDERS.find(p => p.key === activeProvider)?.label || '';
        trigger.innerHTML = `<h3>Only on <span class="provider-current">${escapeHtml(currentLabel)}</span></h3>
            <svg class="provider-chevron" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"></polyline></svg>`;

        const menu = document.createElement('div');
        menu.className = 'provider-menu';
        menu.setAttribute('role', 'listbox');
        menu.setAttribute('aria-label', 'Choose streaming provider');
        PROVIDERS.filter(p => (providersData[p.key] || []).length > 0).forEach(p => {
            const badge = PROVIDER_BADGES[p.key];
            const opt = document.createElement('button');
            opt.type = 'button';
            opt.setAttribute('role', 'option');
            opt.className = 'provider-option';
            opt.dataset.provider = p.key;
            opt.setAttribute('aria-selected', String(p.key === activeProvider));
            opt.innerHTML = `<span class="provider-badge ${badge.className}">${badge.icon || badge.text}</span><span>${escapeHtml(p.label)}</span>`;
            menu.appendChild(opt);
        });

        trigger.addEventListener('click', (e) => {
            e.stopPropagation();
            document.querySelectorAll('.provider-picker.open').forEach(p => { if (p !== pickerWrap) p.classList.remove('open'); });
            const open = pickerWrap.classList.toggle('open');
            trigger.setAttribute('aria-expanded', String(open));
        });

        menu.querySelectorAll('.provider-option').forEach(opt => {
            opt.addEventListener('click', () => {
                const key = opt.dataset.provider;
                pickerWrap.classList.remove('open');
                trigger.setAttribute('aria-expanded', 'false');
                if (key === activeProvider) return;
                activeProvider = key;
                trigger.querySelector('.provider-current').textContent = PROVIDERS.find(p => p.key === activeProvider)?.label || '';
                menu.querySelectorAll('.provider-option').forEach(o => o.setAttribute('aria-selected', String(o.dataset.provider === activeProvider)));
                onSelect(key);
            });
        });

        pickerWrap.appendChild(trigger);
        pickerWrap.appendChild(menu);
        return pickerWrap;
    }

    // Builds the "Genres …" dropdown shared by Home's genre carousel row and
    // its Explore-All grid, mirroring makeProviderPicker's look. available is
    // the label list to offer (callers only pass genres with cached titles).
    // onSelect(label) fires after a different genre is chosen (menu closed,
    // aria + trigger label updated); call sites re-render their content.
    function makeGenrePicker(available, onSelect) {
        const pickerWrap = document.createElement('div');
        pickerWrap.className = 'provider-picker';

        const trigger = document.createElement('button');
        trigger.type = 'button';
        trigger.className = 'provider-trigger';
        trigger.setAttribute('aria-haspopup', 'listbox');
        trigger.setAttribute('aria-expanded', 'false');
        trigger.innerHTML = `<h3>Genres: <span class="provider-current">${escapeHtml(activeHomeGenre)}</span></h3>
            <svg class="provider-chevron" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"></polyline></svg>`;

        const menu = document.createElement('div');
        menu.className = 'provider-menu';
        menu.setAttribute('role', 'listbox');
        menu.setAttribute('aria-label', 'Choose genre');
        available.forEach(g => {
            const opt = document.createElement('button');
            opt.type = 'button';
            opt.setAttribute('role', 'option');
            opt.className = 'provider-option';
            opt.dataset.genre = g;
            opt.setAttribute('aria-selected', String(g === activeHomeGenre));
            opt.innerHTML = `<span>${escapeHtml(g)}</span>`;
            menu.appendChild(opt);
        });

        trigger.addEventListener('click', (e) => {
            e.stopPropagation();
            document.querySelectorAll('.provider-picker.open').forEach(p => { if (p !== pickerWrap) p.classList.remove('open'); });
            const open = pickerWrap.classList.toggle('open');
            trigger.setAttribute('aria-expanded', String(open));
        });

        menu.querySelectorAll('.provider-option').forEach(opt => {
            opt.addEventListener('click', () => {
                const g = opt.dataset.genre;
                pickerWrap.classList.remove('open');
                trigger.setAttribute('aria-expanded', 'false');
                if (g === activeHomeGenre) return;
                activeHomeGenre = g;
                trigger.querySelector('.provider-current').textContent = g;
                menu.querySelectorAll('.provider-option').forEach(o => o.setAttribute('aria-selected', String(o.dataset.genre === activeHomeGenre)));
                onSelect(g);
            });
        });

        pickerWrap.appendChild(trigger);
        pickerWrap.appendChild(menu);
        return pickerWrap;
    }

    // ─── Provider Explore-All grid (Home) ───────────────────────────────────
    // Replaces the Home rows with a paginated grid of everything on the
    // provider: the carousel's cached items first, then /api/discover pages
    // (movies + TV interleaved server-side) as the user scrolls. The header
    // keeps the provider picker so the user can hop providers without going
    // back first.
    function openProviderGrid(key) {
        homeGridActive = true;
        carouselsContainer.innerHTML = ''; // the grid replaces the Home rows
        const items = providersData[key] || [];

        const header = document.createElement('div');
        header.className = 'provider-grid-header';

        header.appendChild(makeGridBackButton());

        header.appendChild(makeProviderPicker((newKey) => openProviderGrid(newKey)));

        renderGrid(null, items, p => `provider=${encodeURIComponent(key)}&page=${p}`, header);

        scrollTopInstant();
    }

    // The pill that leaves an Explore-All grid and rebuilds the Home rows.
    function makeGridBackButton() {
        const back = document.createElement('button');
        back.type = 'button';
        back.className = 'grid-back-btn';
        back.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"></polyline></svg> Back`;
        back.addEventListener('click', () => renderCategoryRows('home', allCatalogMovies));
        return back;
    }

    // Explore All clicks land mid-page; their grids start at the top. Both
    // scroll paths are set because body is the effective scroll container.
    function scrollTopInstant() {
        const scroller = document.scrollingElement || document.documentElement;
        scroller.scrollTop = 0;
        document.body.scrollTop = 0;
    }

    // ─── Genre Explore-All grid (Home) ──────────────────────────────────────
    // Same flow as the provider grid: cached home-row matches first, then
    // /api/discover pages as the user scrolls. HOME_GENRE_ROWS labels are row
    // names, not all of them TMDB genre names ("Anime", "Comedy Shows",
    // "Sci-Fi"), and each home row is single-type, so each label maps to its
    // discover feed's type + server-resolvable TMDB genre name.
    const HOME_GENRE_FEEDS = {
        'Action':             { type: 'movie', genre: 'Action' },
        'Action & Adventure': { type: 'tv',    genre: 'Action & Adventure' },
        'Comedy':             { type: 'movie', genre: 'Comedy' },
        'Comedy Shows':       { type: 'tv',    genre: 'Comedy' },
        'Horror':             { type: 'movie', genre: 'Horror' },
        'Sci-Fi':             { type: 'movie', genre: 'Science Fiction' },
        'Sci-Fi & Fantasy':   { type: 'tv',    genre: 'Sci-Fi & Fantasy' },
        'Drama':              { type: 'tv',    genre: 'Drama' },
        'Mystery':            { type: 'tv',    genre: 'Mystery' },
        'Romance':            { type: 'movie', genre: 'Romance' },
        'Animation':          { type: 'movie', genre: 'Animation' },
        'Anime':              { type: 'tv',    genre: 'Animation' }
    };

    function openHomeGenreGrid(label) {
        homeGridActive = true;
        carouselsContainer.innerHTML = ''; // the grid replaces the Home rows
        activeHomeGenre = label;

        // Cached matches for this home row, deduplicated, best-rated first —
        // the same pooling the genre rows use.
        const seen = new Set();
        const items = [];
        allCatalogMovies.forEach(m => {
            if (!(m.categories || []).includes(label)) return;
            const key = mediaKey(m);
            if (seen.has(key)) return;
            seen.add(key);
            items.push(m);
        });
        items.sort((a, b) => b.rating - a.rating);

        const available = HOME_GENRE_ROWS.filter(g =>
            allCatalogMovies.some(m => (m.categories || []).includes(g)));

        const header = document.createElement('div');
        header.className = 'provider-grid-header';

        header.appendChild(makeGridBackButton());

        header.appendChild(makeGenrePicker(available, (g) => openHomeGenreGrid(g)));

        const feed = HOME_GENRE_FEEDS[label] || { type: 'movie', genre: label };
        renderGrid(null, items,
            p => `type=${feed.type}&genre=${encodeURIComponent(feed.genre)}&page=${p}`,
            header);

        scrollTopInstant();
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
        if (!heroMovies[0].banner) hero.style.backgroundImage = ''; // setHero won't repaint it
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
            if (heroMovies.length === 0) {
                // Bannerless entries: neutralize the banner area instead of
                // keeping the previous page's artwork here (setHero only
                // repaints when a banner exists).
                heroTitle.textContent = 'My List';
                heroDesc.textContent = '';
                heroMetaRow.innerHTML = '';
                resetHeroCrossfade();
                hero.style.backgroundImage = '';
            }
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
        // Auto-advancing banners are exactly the motion the reduce-motion
        // preference asks to skip; manual dot selection still works.
        if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
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
        detailTrailerIframe.src = 'about:blank'; // '' would resolve to this app's own URL and reload the whole page inside the hidden iframe
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
                // Continue Watching cards are stored without `banner` (trimmed
                // to keep localStorage small — see addToContinueWatching), so
                // the hero opened blank. Backfill it from the freshly-fetched
                // TMDB detail now that it's available.
                if (!movie.banner && data.backdrop_path) {
                    detailHero.style.backgroundImage = `url('${CSS.escape('https://image.tmdb.org/t/p/original' + data.backdrop_path)}')`;
                }
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
                card.setAttribute('role', 'button');
                card.setAttribute('tabindex', '0');
                card.setAttribute('aria-label', `View details for ${recTitle}`);

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
                const openRec = () => {
                    closeDetailModal();
                    setTimeout(() => openDetailModal(recMovie), 380);
                };
                card.addEventListener('click', openRec);
                card.addEventListener('keydown', e => {
                    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openRec(); }
                });
                detailRelatedRow.appendChild(card);
            });
        }
    }

    // ─── Season selector ─────────────────────────────────────────────────────
    function buildSeasonSelector(movie, seasons) {
        detailSeasonMenu.innerHTML = '';
        detailSeasonPicker.classList.remove('open');
        detailSeasonTrigger.setAttribute('aria-expanded', 'false');

        seasons.forEach((s, i) => {
            const label = s.name || `Season ${s.season_number}`;
            const opt = document.createElement('button');
            opt.type = 'button';
            opt.className = 'provider-option';
            opt.setAttribute('role', 'option');
            opt.dataset.season = s.season_number;
            opt.setAttribute('aria-selected', String(i === 0));
            opt.textContent = label;
            opt.addEventListener('click', () => {
                detailSeasonPicker.classList.remove('open');
                detailSeasonTrigger.setAttribute('aria-expanded', 'false');
                const alreadySelected = opt.getAttribute('aria-selected') === 'true';
                detailSeasonMenu.querySelectorAll('.provider-option').forEach(o => o.setAttribute('aria-selected', String(o === opt)));
                detailSeasonCurrent.textContent = label;
                if (!alreadySelected) loadEpisodes(movie.id, s.season_number);
            });
            detailSeasonMenu.appendChild(opt);
        });

        detailSeasonCurrent.textContent = seasons[0].name || `Season ${seasons[0].season_number}`;
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
        detailEpisodeList.classList.remove('scrollable');
        if (!episodes || episodes.length === 0) {
            detailEpisodeList.innerHTML = '<div class="episode-no-results">No episodes found.</div>';
            return;
        }

        const seasonNum = parseInt(detailSeasonMenu.querySelector('.provider-option[aria-selected="true"]')?.dataset.season) || 1;

        episodes.forEach(ep => {
            const item = document.createElement('div');
            item.className = 'episode-list-item';
            item.title = `Play ${ep.name}`;
            item.setAttribute('role', 'button');
            item.setAttribute('tabindex', '0');
            item.setAttribute('aria-label', `Play ${ep.name}`);

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

            const playThisEpisode = () => {
                if (!currentDetailMovie) return;
                closeDetailModal();
                setTimeout(() => launchPlayer(currentDetailMovie, seasonNum, ep.episode_number), 380);
            };
            item.addEventListener('click', playThisEpisode);
            item.addEventListener('keydown', e => {
                if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); playThisEpisode(); }
            });

            detailEpisodeList.appendChild(item);
        });

        // Re-enable the overflow fade hint only when this list can scroll.
        detailEpisodeList.classList.toggle('scrollable',
            detailEpisodeList.scrollHeight > detailEpisodeList.clientHeight + 40);
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
            detailTrailerIframe.src = 'about:blank'; // '' would resolve to this app's own URL and reload the whole page inside the hidden iframe
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
        detailTrailerIframe.src = 'about:blank'; // '' would resolve to this app's own URL and reload the whole page inside the hidden iframe
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
        const supportedProviders = ['cinesrc', 'vidking', 'vidlove', 'vidsrcme'];
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

    function isDescriptiveTrack(track) {
        const attrs = track?.attrs || {};
        const str = (String(track?.name || '') + ' ' + String(attrs.NAME || '') + ' ' + String(attrs.CHARACTERISTICS || '')).toLowerCase();
        return str.includes('descriptive') || str.includes('description') || str.includes('commentary') || str.includes('describes-video');
    }

    function scoreAudioTrack(track) {
        let score = 0;
        const attrs = track?.attrs || {};
        const name = (String(track?.name || '') + ' ' + String(attrs.NAME || '')).toLowerCase();
        const lang = (String(track?.lang || '') + ' ' + String(attrs.LANGUAGE || '')).toLowerCase();

        const isEn = /(^|[-_])en(g|us|gb|ca|au)?([_-]|$)/i.test(lang) || /english/.test(name) || /english/.test(lang);
        if (isEn) score += 1000;
        if (isDefaultTrack(track)) score += 100;
        if (name.includes('original')) score += 50;

        // Surround / multi-channel audio (5.1, 7.1, Atmos) gets high priority
        const channels = parseInt(track?.channels || attrs.CHANNELS || '2', 10);
        if (channels >= 6 || name.includes('5.1') || name.includes('surround') || name.includes('atmos')) {
            score += 300;
        } else if (channels >= 2) {
            score += 50;
        }

        // Codecs: Dolby Digital Plus (e-ac-3) / AC-3 / DTS
        const codec = String(track?.audioCodec || '').toLowerCase();
        if (codec.includes('ec-3') || codec.includes('ac-3') || name.includes('dolby') || name.includes('dts')) {
            score += 150;
        }

        // Penalize audio description / commentary
        if (isDescriptiveTrack(track)) {
            score -= 500;
        }
        return score;
    }

    function chooseDefaultAudioIndex(tracks) {
        if (!tracks.length) return -1;
        let bestIdx = 0;
        let bestScore = -Infinity;
        for (let i = 0; i < tracks.length; i++) {
            const s = scoreAudioTrack(tracks[i]);
            if (s > bestScore) {
                bestScore = s;
                bestIdx = i;
            }
        }
        return bestIdx;
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
            off.setAttribute('role', 'option'); // IDL .role isn't reflected on older TV WebKit
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
            button.setAttribute('role', 'option'); // IDL .role isn't reflected on older TV WebKit
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
        off.setAttribute('role', 'option'); // IDL .role isn't reflected on older TV WebKit
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
            button.setAttribute('role', 'option'); // IDL .role isn't reflected on older TV WebKit
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
                button.setAttribute('role', 'option'); // IDL .role isn't reflected on older TV WebKit
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
        playerLoader.classList.remove('buffering', 'has-error');
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

        const provider = server === 'cinesrc' ? 'cinesrc' : (server === 'vidking' ? 'vidking' : (server === 'vidlove' ? 'vidlove' : (server === 'vidsrcme' ? 'vidsrcme' : 'vixsrc')));

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
            //
            // The registration runs WITHOUT blocking playback start: the
            // subtitle ladder can take tens of seconds, and the resolver
            // already holds the first manifest fetch briefly (waitForSubs)
            // while its own server-side ladder completes, so the manifest is
            // complete either way. Awaiting it here used to add the whole
            // ladder delay to TV startup.
            const wantsNativeRenditions = !(window.Hls && Hls.isSupported()) &&
                !!vixPlayer.canPlayType('application/vnd.apple.mpegurl');
            if (wantsNativeRenditions && data.url) {
                externalPromise.then(earlySubs => {
                    if (requestId !== playerRequestId || !data.url) return;
                    externalSubtitleTracks = Array.isArray(earlySubs) ? earlySubs : [];
                    registerNativeSubtitleRenditions(data.url, externalSubtitleTracks);
                }).catch(() => {});
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

    // applyEmbeddedSubtitlePolicy enforces the External-subtitles-over-
    // embedded rule on the hls.js track state. When External tracks exist,
    // hls.js's own track is forced off AND its rendering switched to
    // "hidden" — so even the force-flip its createTracksInGroup() does on
    // level switches stays invisible (hls.js auto-enables manifest
    // DEFAULT/FORCED tracks, which would otherwise stack two subtitles).
    // Without External tracks, the best embedded track is auto-selected.
    // Shared by applyPlayerTracks and the SUBTITLE_TRACKS_UPDATED handler —
    // the logic was previously duplicated verbatim in both.
    function applyEmbeddedSubtitlePolicy() {
        if (!vixHlsInstance || playerSubtitleTracks.length === 0) return;
        if (externalSubtitleTracks.length > 0) {
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

    function applyPlayerTracks(provider) {
        if (!vixHlsInstance) return;
        playerAudioTracks = vixHlsInstance.audioTracks || [];
        playerSubtitleTracks = vixHlsInstance.subtitleTracks || [];
        if (playerAudioTracks.length > 0 && !playerAudioInitialized) {
            vixHlsInstance.audioTrack = chooseDefaultAudioIndex(playerAudioTracks);
            playerAudioInitialized = true;
        }
        applyEmbeddedSubtitlePolicy();
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
                if (settled || requestId !== playerRequestId) return;
                if (vixPlayer.readyState >= 3) {
                    // The stream is loaded but autoplay is still blocked
                    // pending a user gesture (TV browsers, strict autoplay
                    // policies). The stream itself is healthy — settle as
                    // ready and let the user press play instead of failing
                    // after a clean load.
                    settleOk();
                    return;
                }
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
                // LAN vs WAN ABR profiles. Served by this machine or the LAN,
                // the proxy pre-buffers into RAM and every hop is short, so
                // the aggressive profile applies — a hot bandwidth estimate
                // and full-factor ABR keep playback on the top tier. Over a
                // public address each fragment crosses a WAN where loss
                // bursts stall individual fetches: the estimate starts
                // conservative, ABR keeps headroom so it reacts to real
                // throughput instead of locking the top tier and rebufferring
                // while a shallow buffer outgrows the stalls.
                const host = location.hostname.toLowerCase();
                const isLan = host === 'localhost' || host === '127.0.0.1' || host === '::1' ||
                    host.endsWith('.local') || /^10\./.test(host) ||
                    /^192\.168\./.test(host) || /^172\.(1[6-9]|2\d|3[01])\./.test(host);
                vixHlsInstance = new Hls({
                    enableWorker: true,
                    lowLatencyMode: false,
                    capLevelToPlayerSize: false,
                    renderTextTracksNatively: true,
                    autoStartLoad: true,
                    // -1 = auto. MANIFEST_PARSED below pins the first level
                    // to the top tier. (999 was an out-of-range index that
                    // hls.js could observe before that handler ran.)
                    startLevel: -1,
                    abrEwmaDefaultEstimate: isLan ? 10000000 : 2500000,
                    abrBandWidthFactor: isLan ? 1.0 : 0.9,
                    abrBandWidthUpFactor: 0.9,
                    // Buffer targets sized for TV-class devices: a 256 MB MSE
                    // target OOMs 500 MB-class smart TVs, and the server-side
                    // read-ahead already keeps the proxy ahead of the player.
                    maxBufferLength: 60,
                    maxMaxBufferLength: 120,
                    maxBufferSize: 128 * 1024 * 1024,
                    maxBufferHole: 0.5,
                    highBufferWatchdogPeriod: 2,
                    nudgeOffset: 0.1,
                    nudgeMaxRetry: 5,
                    fragLoadingTimeOut: 30000,
                    fragLoadingMaxRetry: 6,
                    fragLoadingRetryDelay: 500,
                    backBufferLength: 60,
                    xhrSetup: function(xhr) {
                        xhr.withCredentials = false;
                    }
                });

                vixHlsInstance.on(Hls.Events.MANIFEST_PARSED, function(event, data) {
                    if (requestId !== playerRequestId) return;
                    // Start on the highest available quality (4K 2160p, 1080p, or max bitrate).
                    if (data.levels && data.levels.length > 0) {
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
                    applyEmbeddedSubtitlePolicy();
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
                    if (requestId !== playerRequestId || !vixHlsInstance) return;
                    if (!data.fatal) {
                        // Non-fatal issues (stalls, range errors, throttled
                        // retries) are logged so rebuffering is never silent,
                        // but recovery is left to hls.js's internal handling.
                        console.warn('[Player] HLS issue:', data.details, data.response || '');
                        return;
                    }
                    console.error('[Player] HLS fatal error:', data.type, data.details, data.error || '', data.response || '');
                    // Recovery ladder: gives fatal errors — including
                    // fragParsingError from corrupted/mismatched fragments —
                    // three escalating chances before surfacing an error UI.
                    // It runs both before AND after playback has started: a
                    // fatal error mid-stream must be recovered or surfaced,
                    // never swallowed (gating the whole ladder on !settled
                    // used to leave the buffering spinner up forever).
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
                    if (settled) {
                        // Playback already started: stop the background load
                        // and show the error card (settleErr is a no-op once
                        // settled, so without this the failure was invisible).
                        const dead = vixHlsInstance;
                        vixHlsInstance = null;
                        try { dead.destroy(); } catch (_) {}
                        showPlayerError(detail);
                    } else {
                        settleErr(new Error(detail));
                    }
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
            } else {
                showToast('You’ve reached the end of this show');
            }
        } catch (err) {
            console.warn('Could not resolve next episode:', err);
        }
    }

    playerNextEp.addEventListener('click', () => {
        playNextEpisode();
    });

    playerEpListBtn.addEventListener('click', () => {
        // Toggle: with the panel raised above the bars (z-60), this same icon
        // is also the touch target users reach for when dismissing it.
        if (playerEpPanel.classList.contains('show')) {
            playerEpPanel.classList.remove('show');
            playerEpListBtn.setAttribute('aria-expanded', 'false');
            return;
        }
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

                    item.setAttribute('role', 'button');
                    item.setAttribute('tabindex', '0');

                    const playFromPanel = () => {
                        playerEpPanel.classList.remove('show');
                        playerEpListBtn.setAttribute('aria-expanded', 'false');
                        launchPlayer(currentPlayerMovie, seasonNum, ep.episode_number);
                    };
                    item.addEventListener('click', playFromPanel);
                    item.addEventListener('keydown', e => {
                        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); playFromPanel(); }
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
        // Re-enable pointer events on the loader (:has() replacement — older
        // TV WebKit has no :has(), which left Try again unclickable there).
        playerLoader.classList.add('has-error');
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
        // Back means "leave the player entirely": drop any active fullscreen
        // (container or iOS native video fullscreen) as part of closing.
        // Tearing the player down while fullscreen is still engaged leaves
        // the fullscreen layer outliving the modal — a stuck black screen on
        // some TV browsers, and a second tap needed elsewhere. (Esc keeps its
        // exit-first behavior — see the keydown handler.)
        if (isPlayerFullscreen() || (vixPlayer.webkitDisplayingFullscreen && vixPlayer.webkitExitFullscreen)) {
            exitPlayerFullscreen();
        }
        if (sourceAbortController) {
            // Release the server-side resolution (browser session) for a
            // viewer who is no longer waiting on it.
            sourceAbortController.abort();
            sourceAbortController = null;
        }
        clearTimeout(controlsHideTimer);
        clearTimeout(playerCloseTimer);
        // Stop audio/video immediately — destroying hls.js here (rather than
        // in the timer below) means nothing keeps playing during the
        // fade-out. Element-level cleanup stays in the timer.
        if (vixHlsInstance) {
            vixHlsInstance.destroy();
            vixHlsInstance = null;
        }
        vixPlayer.pause();
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
            playerLoader.classList.remove('buffering', 'has-error');
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
    // Double-tap-to-seek is a touch gesture. On mouse devices the zones only
    // added a ~260ms delay to click-to-pause and swallowed double-click
    // fullscreen over most of the video, so they're never even created there.
    if (window.matchMedia && window.matchMedia('(hover: none)').matches) {
        setupSeekZone(playerSeekZoneLeft, playerSeekIndicatorLeft, playerSeekAmountLeft, 'back');
        setupSeekZone(playerSeekZoneRight, playerSeekIndicatorRight, playerSeekAmountRight, 'forward');
    }

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

    function isPlayerFullscreen() {
        return !!(document.fullscreenElement || document.webkitFullscreenElement ||
            document.mozFullScreenElement || document.msFullscreenElement);
    }

    // exitPlayerFullscreen leaves every fullscreen mode the player can be in:
    // document/container fullscreen via the vendor-prefixed exit APIs, plus
    // iOS's native video fullscreen, which lives outside the document
    // fullscreen API entirely (webkitDisplayingFullscreen/webkitExitFullscreen).
    function exitPlayerFullscreen() {
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
        if (vixPlayer.webkitDisplayingFullscreen && vixPlayer.webkitExitFullscreen) {
            try { vixPlayer.webkitExitFullscreen(); } catch (_) {}
        }
    }

    function togglePlayerFullscreen() {
        const target = playerModal;

        try {
            if (!isPlayerFullscreen()) {
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
                exitPlayerFullscreen();
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
            const inFullscreen = isPlayerFullscreen() ||
                (vixPlayer.webkitDisplayingFullscreen && vixPlayer.webkitExitFullscreen);
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
