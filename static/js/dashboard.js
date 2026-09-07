// Admin dashboard: instance stats + account management. The server gates
// this page (non-admins are redirected); the status check below is just a
// client-side safety net.
const statAccounts = document.getElementById('stat-accounts');
const statSessions = document.getElementById('stat-sessions');
const usersBody = document.getElementById('users-body');
const adminMsg = document.getElementById('admin-msg');

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
    if (!s.isAdmin) { location.href = '/'; return; }
    loadUsers();
})();

document.getElementById('back-btn').addEventListener('click', () => { location.href = '/'; });
document.getElementById('account-btn').addEventListener('click', () => { location.href = '/account'; });

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

async function loadUsers() {
    try {
        const data = await fetch('/api/admin/users').then(r => r.json());
        const users = data.users || [];
        statAccounts.textContent = users.length;
        statSessions.textContent = users.reduce((n, u) => n + (u.sessions || 0), 0);
        usersBody.innerHTML = '';
        if (!users.length) {
            const tr = document.createElement('tr');
            tr.innerHTML = '<td colspan="5" class="empty">No accounts yet.</td>';
            usersBody.appendChild(tr);
            return;
        }
        users.forEach(u => {
            const tr = document.createElement('tr');
            const created = new Date(u.created_at).toLocaleDateString();
            const badge = u.is_admin ? '<span class="role-badge">Admin</span>' : '';
            const avatarSrc = u.has_avatar ? `/api/admin/users/${u.id}/avatar` : DEFAULT_AVATAR;
            tr.innerHTML = `
                <td>
                    <div class="user-cell">
                        <img class="user-avatar" src="${avatarSrc}" alt="" width="30" height="30">
                        <span>${escapeHtml(u.username)}${badge}</span>
                    </div>
                </td>
                <td class="muted">${u.email ? escapeHtml(u.email) : '—'}</td>
                <td class="muted">${created}</td>
                <td class="muted">${u.sessions}</td>
                <td></td>`;
            const actions = tr.lastElementChild;
            const logoutBtn = document.createElement('button');
            logoutBtn.className = 'btn';
            logoutBtn.textContent = 'Force sign-out';
            logoutBtn.addEventListener('click', () => adminAction(`/api/admin/users/${u.id}/logout`, `Signed ${u.username} out everywhere.`));
            const delBtn = document.createElement('button');
            delBtn.className = 'btn danger';
            delBtn.textContent = 'Delete';
            delBtn.addEventListener('click', () => {
                if (confirm(`Delete account "${u.username}"? Its synced data is removed too.`)) {
                    adminAction(`/api/admin/users/${u.id}`, 'Account deleted.', 'DELETE');
                }
            });
            actions.append(logoutBtn, delBtn);
            usersBody.appendChild(tr);
        });
    } catch (_) {
        adminMsg.textContent = 'Could not load accounts.';
        adminMsg.className = 'msg err';
    }
}

function escapeHtml(v) {
    return String(v).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}

async function adminAction(url, okText, method = 'POST') {
    try {
        const res = await fetch(url, { method });
        const data = await res.json().catch(() => ({}));
        adminMsg.className = 'msg';
        if (res.ok && data.success) {
            adminMsg.textContent = okText;
            adminMsg.classList.add('ok');
            loadUsers();
        } else {
            adminMsg.textContent = data.error || 'Action failed.';
            adminMsg.classList.add('err');
        }
    } catch (_) {
        adminMsg.textContent = 'Could not reach the server.';
        adminMsg.classList.add('err');
    }
}
