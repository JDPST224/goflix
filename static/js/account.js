// Account self-service page: profile picture, password, danger zone.
// Admin account management lives on /dashboard; admins just get a link.
const who = document.getElementById('who');
const whoEmail = document.getElementById('who-email');
const roleBadge = document.getElementById('role-badge');
const backBtn = document.getElementById('back-btn');
const avatarBtn = document.getElementById('avatar-btn');
const avatarImg = document.getElementById('avatar-img');
const avatarInput = document.getElementById('avatar-input');
const avatarUploadBtn = document.getElementById('avatar-upload-btn');
const avatarRemoveBtn = document.getElementById('avatar-remove-btn');
const avatarMsg = document.getElementById('avatar-msg');
const pwForm = document.getElementById('pw-form');
const pwMsg = document.getElementById('pw-msg');
const pwSubmit = document.getElementById('pw-submit');
const clearDataBtn = document.getElementById('clear-data-btn');
const clearMsg = document.getElementById('clear-msg');
const deleteForm = document.getElementById('delete-form');
const deleteBtn = document.getElementById('delete-btn');
const deleteMsg = document.getElementById('delete-msg');

const DEFAULT_AVATAR = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='12' fill='%23e50914'/%3E%3Ctext x='32' y='45' font-family='Arial, sans-serif' font-size='38' font-weight='900' fill='%23ffffff' text-anchor='middle'%3EG%3C/text%3E%3C/svg%3E";

(async function init() {
    let s;
    try {
        s = await fetch('/api/auth/status').then(r => r.json());
    } catch (_) {
        location.href = '/login';
        return;
    }
    if (!s.authed || !s.user) { location.href = '/login'; return; }
    who.textContent = s.user;
    whoEmail.textContent = s.email || 'No email on file';
    if (s.isAdmin) {
        roleBadge.classList.remove('hidden');
    }
    if (s.hasAvatar) {
        avatarImg.src = '/api/auth/avatar?v=' + Date.now();
        avatarRemoveBtn.classList.remove('hidden');
    }
})();

backBtn.addEventListener('click', () => { location.href = '/'; });

// ── Profile picture ─────────────────────────────────────────────────────
// The browser downsizes the chosen image to 256×256 via canvas, then POSTs
// it as a data-URI — no multipart, no server-side image processing.
avatarBtn.addEventListener('click', () => avatarInput.click());
avatarUploadBtn.addEventListener('click', () => avatarInput.click());

avatarInput.addEventListener('change', async () => {
    const file = avatarInput.files && avatarInput.files[0];
    avatarInput.value = '';
    if (!file) return;
    if (!/^image\/(png|jpeg|webp)$/.test(file.type)) {
        avatarMsg.textContent = 'Please choose a PNG, JPEG or WebP image.';
        return;
    }
    avatarMsg.textContent = 'Processing…';
    try {
        const dataURI = await downscaleImage(file, 256);
        const res = await fetch('/api/auth/avatar', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ data: dataURI })
        });
        const data = await res.json().catch(() => ({}));
        if (res.ok && data.success) {
            avatarImg.src = dataURI;
            avatarRemoveBtn.classList.remove('hidden');
            avatarMsg.textContent = 'Picture updated.';
        } else {
            avatarMsg.textContent = data.error || 'Upload failed.';
        }
    } catch (_) {
        avatarMsg.textContent = 'Could not process that image.';
    }
});

avatarRemoveBtn.addEventListener('click', async () => {
    try {
        const res = await fetch('/api/auth/avatar', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ data: '' })
        });
        const data = await res.json().catch(() => ({}));
        if (res.ok && data.success) {
            avatarImg.src = DEFAULT_AVATAR;
            avatarRemoveBtn.classList.add('hidden');
            avatarMsg.textContent = 'Picture removed.';
        } else {
            avatarMsg.textContent = data.error || 'Remove failed.';
        }
    } catch (_) {
        avatarMsg.textContent = 'Could not reach the server.';
    }
});

function downscaleImage(file, size) {
    return new Promise((resolve, reject) => {
        const url = URL.createObjectURL(file);
        const img = new Image();
        img.onload = () => {
            URL.revokeObjectURL(url);
            const canvas = document.createElement('canvas');
            canvas.width = size;
            canvas.height = size;
            const ctx = canvas.getContext('2d');
            if (!ctx) { reject(new Error('no canvas')); return; }
            // Cover-fit crop: center square of the source image.
            const side = Math.min(img.naturalWidth, img.naturalHeight);
            const sx = (img.naturalWidth - side) / 2;
            const sy = (img.naturalHeight - side) / 2;
            ctx.drawImage(img, sx, sy, side, side, 0, 0, size, size);
            resolve(canvas.toDataURL('image/jpeg', 0.85));
        };
        img.onerror = () => { URL.revokeObjectURL(url); reject(new Error('decode failed')); };
        img.src = url;
    });
}

// ── Change password ─────────────────────────────────────────────────────
pwForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    pwMsg.className = 'msg';
    pwSubmit.disabled = true;
    const cur = document.getElementById('cur').value;
    const next = document.getElementById('next').value;
    try {
        const res = await fetch('/api/auth/password', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ current: cur, next })
        });
        const data = await res.json().catch(() => ({}));
        if (res.ok && data.success) {
            pwMsg.textContent = 'Password updated.';
            pwMsg.classList.add('ok');
            pwForm.reset();
        } else {
            pwMsg.textContent = data.error || 'Update failed.';
            pwMsg.classList.add('err');
        }
    } catch (_) {
        pwMsg.textContent = 'Could not reach the server.';
        pwMsg.classList.add('err');
    } finally {
        pwSubmit.disabled = false;
    }
});

// ── Danger zone ─────────────────────────────────────────────────────────
clearDataBtn.addEventListener('click', async () => {
    if (!confirm('Delete ALL your synced data (My List, progress, Continue Watching) from the server?')) return;
    clearDataBtn.disabled = true;
    clearMsg.className = 'msg';
    try {
        const res = await fetch('/api/userdata/clear', { method: 'POST' });
        const data = await res.json().catch(() => ({}));
        if (res.ok && data.success) {
            clearMsg.textContent = 'Server data deleted. This device will re-upload its local copy on the next sync.';
            clearMsg.classList.add('ok');
        } else {
            clearMsg.textContent = data.error || 'Delete failed.';
            clearMsg.classList.add('err');
        }
    } catch (_) {
        clearMsg.textContent = 'Could not reach the server.';
        clearMsg.classList.add('err');
    } finally {
        clearDataBtn.disabled = false;
    }
});

deleteForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const pw = document.getElementById('del-pw').value;
    if (!pw) {
        deleteMsg.textContent = 'Enter your current password to confirm.';
        deleteMsg.className = 'msg err';
        return;
    }
    if (!confirm('Permanently delete your account, sessions and synced data? This cannot be undone.')) return;
    deleteBtn.disabled = true;
    deleteMsg.className = 'msg';
    try {
        const res = await fetch('/api/auth/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ current: pw })
        });
        const data = await res.json().catch(() => ({}));
        if (res.ok && data.success) {
            clearLocalUserData();
            location.href = '/login';
            return;
        }
        deleteMsg.textContent = data.error || 'Delete failed.';
        deleteMsg.classList.add('err');
    } catch (_) {
        deleteMsg.textContent = 'Could not reach the server.';
        deleteMsg.classList.add('err');
    } finally {
        deleteBtn.disabled = false;
    }
});

function clearLocalUserData() {
    try {
        ['goflix_mylist', 'goflix_progress', 'goflix_cw', 'goflix_removed', 'goflix_avprefs', 'goflix_user']
            .forEach(k => localStorage.removeItem(k));
    } catch (_) {}
}

document.getElementById('logout-btn').addEventListener('click', async () => {
    try { await fetch('/api/auth/logout', { method: 'POST' }); } catch (_) {}
    clearLocalUserData();
    location.href = '/';
});
