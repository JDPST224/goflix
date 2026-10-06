// Login / registration page logic (served with login.html). Kept in a
// separate file so the Content-Security-Policy can stay inline-script-free.
(function () {
    'use strict';

    const $ = (id) => document.getElementById(id);

    const tabLogin    = $('tab-login');
    const tabRegister = $('tab-register');
    const tabsEl      = $('tabs');
    const username    = $('username');
    const password    = $('password');
    // Named confirmPw, not `confirm` — the old top-level `const confirm`
    // shadowed window.confirm for the whole script.
    const confirmPw   = $('confirm');
    const confirmRow  = $('confirm-row');
    const email       = $('email');
    const emailRow    = $('email-row');
    const invite      = $('invite');
    const inviteRow   = $('invite-row');
    const remember    = $('remember');
    const rememberRow = $('remember-row');
    const pwHint      = $('pw-hint');
    const submit      = $('submit');
    const submitLabel = submit.querySelector('.btn-label');
    const heading     = $('heading');
    const sub         = $('sub');
    const error       = $('error');
    const form        = $('form');
    const authCard    = document.querySelector('.card');

    const MIN_PW = 4;
    const MAX_PW = 128;

    let mode = 'login';
    let inviteRequired = false;
    let redirecting = false;

    /* ── Mode switching ──────────────────────────────────────── */

    function setMode(m, focusField) {
        if (m === mode && focusField !== true) { /* still re-apply, cheap */ }
        mode = m;
        const register = m === 'register';

        tabLogin.setAttribute('aria-selected', String(!register));
        tabRegister.setAttribute('aria-selected', String(register));
        // Roving tabindex: only the selected tab is in the tab order.
        tabLogin.tabIndex = register ? -1 : 0;
        tabRegister.tabIndex = register ? 0 : -1;

        inviteRow.classList.toggle('hidden', !register || !inviteRequired);
        emailRow.classList.toggle('hidden', !register);
        confirmRow.classList.toggle('hidden', !register);
        pwHint.classList.toggle('hidden', !register);
        rememberRow.classList.toggle('hidden', register);

        tabsEl.dataset.mode = m;
        authCard.dataset.mode = m;

        submitLabel.textContent = register ? 'Create account' : 'Sign in';
        heading.textContent = register ? 'Create your account' : 'Welcome back';
        heading.classList.remove('swap');
        void heading.offsetWidth; // restart the swap animation
        heading.classList.add('swap');
        sub.textContent = register
            ? 'Set up a profile to save your list and progress.'
            : 'Sign in to your GoFlix account.';

        password.setAttribute('autocomplete', register ? 'new-password' : 'current-password');

        // Hidden fields must not keep stale values or a stale invalid state.
        if (!register) {
            confirmPw.value = '';
            email.value = '';
            invite.value = '';
        }
        setPwVisible(false);
        clearError();

        if (focusField) {
            const target = username.value.trim() ? password : username;
            target.focus();
        }
    }

    tabLogin.addEventListener('click', () => setMode('login', true));
    tabRegister.addEventListener('click', () => setMode('register', true));

    // Left/Right arrows move between tabs, as expected for role="tablist".
    tabsEl.addEventListener('keydown', (e) => {
        if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
        e.preventDefault();
        const next = mode === 'login' ? 'register' : 'login';
        setMode(next, true);
        (next === 'login' ? tabLogin : tabRegister).focus();
    });

    /* ── Auth status ─────────────────────────────────────────── */

    fetch('/api/auth/status', { credentials: 'same-origin' })
        .then((r) => (r.ok ? r.json() : null))
        .then((s) => {
            if (!s) return;
            inviteRequired = !!s.inviteRequired;
            if (mode === 'register') inviteRow.classList.toggle('hidden', !inviteRequired);
            if (s.authed && s.user) { redirecting = true; location.href = '/'; }
        })
        .catch(() => { /* offline or endpoint missing — the form still works */ });

    /* ── Password visibility ─────────────────────────────────── */

    const eyeButtons = Array.from(document.querySelectorAll('[data-eye]'));

    function setPwVisible(show) {
        [password, confirmPw].forEach((f) => { f.type = show ? 'text' : 'password'; });
        eyeButtons.forEach((btn) => {
            btn.classList.toggle('show', show);
            btn.setAttribute('aria-pressed', String(show));
            btn.setAttribute('aria-label', show ? 'Hide password' : 'Show password');
        });
    }

    eyeButtons.forEach((btn) => {
        btn.addEventListener('click', () => setPwVisible(password.type === 'password'));
    });

    /* ── Errors ──────────────────────────────────────────────── */

    const invalidFields = new Set();

    function clearError() {
        error.textContent = '';
        invalidFields.forEach((f) => f.removeAttribute('aria-invalid'));
        invalidFields.clear();
    }

    // Show a message, mark and focus the field that caused it, and shake.
    function flashError(msg, field) {
        error.textContent = msg;
        error.classList.remove('shake');
        void error.offsetWidth; // restart the animation
        error.classList.add('shake');
        if (field) {
            field.setAttribute('aria-invalid', 'true');
            invalidFields.add(field);
            field.focus();
        } else {
            // A server-side message has no field to focus, and on the taller
            // register form it can land below the fold. Bring it into view.
            error.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
        }
    }

    // Typing in a flagged field clears its error state.
    [username, password, confirmPw, email, invite].forEach((f) => {
        f.addEventListener('input', () => {
            if (invalidFields.has(f)) clearError();
        });
    });

    const emailOk = (v) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v);

    /* ── Validation ──────────────────────────────────────────────
       The form is novalidate, so these checks were entirely missing:
       an empty username or a 1-character password used to go to the
       server and come back as a generic failure. */

    function validate(register) {
        const name = username.value.trim();
        if (!name) return ['Enter your username.', username];
        if (name.length > 32) return ['Usernames can be at most 32 characters.', username];
        if (!password.value) return ['Enter your password.', password];

        if (!register) return null;

        if (password.value.length < MIN_PW) {
            return ['Use a password of at least ' + MIN_PW + ' characters.', password];
        }
        if (password.value.length > MAX_PW) {
            return ['Use a password of at most ' + MAX_PW + ' characters.', password];
        }
        if (password.value !== confirmPw.value) {
            return ['Those passwords do not match.', confirmPw];
        }
        const mail = email.value.trim();
        if (mail && !emailOk(mail)) {
            return ['Enter a valid email address, or leave it blank.', email];
        }
        if (inviteRequired && !invite.value.trim()) {
            return ['Enter the invite code from your server admin.', invite];
        }
        return null;
    }

    /* ── Submit ──────────────────────────────────────────────── */

    function setLoading(on) {
        submit.disabled = on;
        submit.classList.toggle('loading', on);
        submit.setAttribute('aria-busy', String(on));
    }

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        if (submit.disabled) return;

        const register = mode === 'register';
        clearError();

        const problem = validate(register);
        if (problem) { flashError(problem[0], problem[1]); return; }

        setLoading(true);

        // Don't hang forever on a stalled connection.
        const ctrl = new AbortController();
        const timer = setTimeout(() => ctrl.abort(), 15000);

        try {
            const body = { username: username.value.trim(), password: password.value };
            if (register) {
                body.invite = invite.value.trim();
                body.email = email.value.trim();
            } else {
                body.remember = remember.checked;
            }

            const res = await fetch('/api/auth/' + (register ? 'register' : 'login'), {
                method: 'POST',
                credentials: 'same-origin',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
                signal: ctrl.signal
            });
            const data = await res.json().catch(() => ({}));

            if (res.ok && data.success) {
                redirecting = true;
                location.href = '/';
                return;
            }
            flashError(data.error || (register ? 'Could not create that account.' : 'That username or password is not right.'));
        } catch (err) {
            flashError(err && err.name === 'AbortError'
                ? 'The server took too long to answer. Try again.'
                : 'Could not reach the server. Check your connection and try again.');
        } finally {
            clearTimeout(timer);
            // Leave the button in its loading state while the page navigates
            // away, so it can't be double-submitted mid-redirect.
            if (!redirecting) setLoading(false);
        }
    });

    setMode('login');
})();
