// Login / registration page logic (served with login.html). Kept in a
// separate file so the Content-Security-Policy can stay inline-script-free.
const tabLogin = document.getElementById('tab-login');
const tabRegister = document.getElementById('tab-register');
const username = document.getElementById('username');
const password = document.getElementById('password');
const confirm = document.getElementById('confirm');
const confirmRow = document.getElementById('confirm-row');
const email = document.getElementById('email');
const emailRow = document.getElementById('email-row');
const inviteRow = document.getElementById('invite-row');
const invite = document.getElementById('invite');
const rememberRow = document.getElementById('remember-row');
const remember = document.getElementById('remember');
const pwHint = document.getElementById('pw-hint');
const submit = document.getElementById('submit');
const submitLabel = submit.querySelector('.btn-label');
const tabsEl = document.getElementById('tabs');
const heading = document.getElementById('heading');
const error = document.getElementById('error');
const sub = document.getElementById('sub');
const form = document.getElementById('form');
let mode = 'login';
let inviteRequired = false;

function setMode(m) {
    mode = m;
    const register = m === 'register';
    tabLogin.classList.toggle('active', !register);
    tabRegister.classList.toggle('active', register);
    inviteRow.classList.toggle('hidden', !register || !inviteRequired);
    emailRow.classList.toggle('hidden', !register);
    confirmRow.classList.toggle('hidden', !register);
    pwHint.classList.toggle('hidden', !register);
    rememberRow.classList.toggle('hidden', register);
    tabsEl.dataset.mode = m;
    submitLabel.textContent = register ? 'Create account' : 'Sign in';
    heading.textContent = register ? 'Create account' : 'Sign in';
    heading.classList.remove('swap');
    void heading.offsetWidth; // restart the swap animation
    heading.classList.add('swap');
    sub.textContent = register ? 'Pick a username and password.' : 'Sign in to your account.';
    password.setAttribute('autocomplete', register ? 'new-password' : 'current-password');
    error.textContent = '';
}
tabLogin.addEventListener('click', () => setMode('login'));
tabRegister.addEventListener('click', () => setMode('register'));

fetch('/api/auth/status').then(r => r.json()).then(s => {
    inviteRequired = !!s.inviteRequired;
    if (mode === 'register') inviteRow.classList.toggle('hidden', !inviteRequired);
    if (s.authed && s.user) { location.href = '/'; }
}).catch(() => {});

// Password visibility toggles — animated eye icons. Both password fields
// follow the same visibility so the confirmation stays in sync while typing.
const pwFields = [password, confirm];
function setPwVisible(show) {
    pwFields.forEach(f => { f.type = show ? 'text' : 'password'; });
    document.querySelectorAll('.eye').forEach(btn => {
        btn.classList.toggle('show', show);
        btn.setAttribute('aria-label', show ? 'Hide password' : 'Show password');
        btn.classList.remove('pop');
        void btn.offsetWidth; // restart the pop animation
        btn.classList.add('pop');
    });
}
document.querySelectorAll('.eye').forEach(btn => {
    btn.addEventListener('click', () => setPwVisible(password.type === 'password'));
});

// Show an error message with a small shake so failures are noticeable.
function flashError(msg) {
    error.textContent = msg;
    error.classList.remove('shake');
    void error.offsetWidth; // restart the animation
    error.classList.add('shake');
}

const emailOk = (v) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v);

form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const register = mode === 'register';
    if (register) {
        if (password.value !== confirm.value) {
            flashError('Passwords do not match.');
            return;
        }
        const mail = email.value.trim();
        if (mail && !emailOk(mail)) {
            flashError('Enter a valid email address, or leave it blank.');
            return;
        }
    }
    submit.disabled = true;
    submit.classList.add('loading');
    error.textContent = '';
    try {
        const body = { username: username.value.trim(), password: password.value };
        if (register) {
            body.invite = invite.value;
            body.email = email.value.trim();
        } else {
            body.remember = remember.checked;
        }
        const res = await fetch('/api/auth/' + (register ? 'register' : 'login'), {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });
        const data = await res.json().catch(() => ({}));
        if (res.ok && data.success) { location.href = '/'; return; }
        flashError(data.error || (register ? 'Registration failed.' : 'Sign in failed.'));
    } catch (_) {
        flashError('Could not reach the server.');
    } finally {
        submit.disabled = false;
        submit.classList.remove('loading');
    }
});
