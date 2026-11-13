/**
 * Общий каркас сайта: шапка, подвал, авторизация, тема CMS и уведомления.
 */
import { api, ApiError } from './api.js';
import {
    escapeHtml, initials, roleLabel, toast, openModal, setBusy,
    showFormErrors, clearFormErrors, icon, relativeTime, statusBadge,
} from './ui.js';

const NAV = [
    { key: 'home', href: 'index.html', label: 'Главная' },
    { key: 'prices', href: 'prices.html', label: 'Цены' },
    { key: 'masters', href: 'masters.html', label: 'Мастера' },
    { key: 'reviews', href: 'reviews.html', label: 'Отзывы' },
    { key: 'about', href: 'about.html', label: 'О сервисе' },
    { key: 'contacts', href: 'contacts.html', label: 'Контакты' },
];

const PHONE = '+7 (800) 123-45-67';
const PHONE_HREF = 'tel:+78001234567';

let currentUser = null;
let theme = null;
let notifications = [];
let unreadCount = 0;
let pollTimer = null;
let headerBound = false;
const shownReady = new Set();

export function getUser() {
    return currentUser;
}

export function isStaff() {
    return !!currentUser && (currentUser.role === 'manager' || currentUser.role === 'admin');
}

export function hasPermission(permission) {
    if (!currentUser) return false;
    if (currentUser.role === 'admin') return true;
    return (currentUser.permissions || []).includes(permission);
}

export async function refreshUser() {
    try {
        const data = await api.get('/api/auth/me');
        currentUser = data.user;
    } catch {
        currentUser = null;
    }
    updateHeaderUser();
    return currentUser;
}

export async function initLayout({ active = '' } = {}) {
    document.body.dataset.page = active;
    renderHeader(active);
    renderFooter();
    applyTheme(await loadTheme());

    await refreshUser();
    bindHeader();
    initGlobalHandlers();

    if (currentUser) {
        await loadNotifications();
        if (!pollTimer) {
            pollTimer = setInterval(() => {
                loadNotifications().catch(() => {});
            }, 30000);
        }
        maybeShowReadyNotification();
    }

    handleAuthQuery();
    return currentUser;
}

/* ------------------------------- Шапка ---------------------------------- */

function renderHeader(active) {
    const links = NAV.map((item) => `
        <li><a href="${item.href}" class="${item.key === active ? 'active' : ''}">${escapeHtml(item.label)}</a></li>
    `).join('');

    const header = document.createElement('header');
    header.className = 'site-header';
    header.innerHTML = `
        <div class="container header-inner">
            <a href="index.html" class="logo">ООО <span>«pro-remont»</span></a>
            <nav class="main-nav" id="mainNav" aria-label="Основная навигация">
                <ul>
                    ${links}
                    <li><a href="${PHONE_HREF}" class="header-phone">${PHONE}</a></li>
                </ul>
            </nav>
            <div class="header-actions">
                <div id="authSlot" class="flex-center"></div>
            </div>
            <button class="nav-toggle" id="navToggle" type="button" aria-label="Открыть меню" aria-expanded="false">
                <span></span><span></span><span></span>
            </button>
        </div>`;

    const placeholder = document.getElementById('site-header');
    if (placeholder) placeholder.replaceWith(header);
    else document.body.prepend(header);
}

function updateHeaderUser() {
    const slot = document.getElementById('authSlot');
    if (!slot) return;

    if (!currentUser) {
        slot.innerHTML = `
            <button class="btn btn-outline btn-sm" type="button" data-auth="login">Вход</button>
            <button class="btn btn-primary btn-sm" type="button" data-auth="register">Зарегистрироваться</button>`;
        return;
    }

    const staff = currentUser.role === 'manager' || currentUser.role === 'admin';
    slot.innerHTML = `
        <div class="notif-wrap" style="position:relative">
            <button class="bell" id="bellBtn" type="button" aria-label="Уведомления" aria-expanded="false">
                ${icon('bell')}
                <span class="bell__count ${unreadCount ? '' : 'hidden'}" id="bellCount">${unreadCount > 99 ? '99+' : unreadCount}</span>
            </button>
            <div class="notif-panel" id="notifPanel">
                <div class="notif-panel__head">
                    <span>Уведомления</span>
                    <button type="button" id="readAllBtn">Прочитать все</button>
                </div>
                <div class="notif-panel__list" id="notifList"></div>
            </div>
        </div>
        <a class="user-chip" href="account.html" title="Личный кабинет">
            <span class="avatar">${initials(currentUser.full_name)}</span>
            <span>
                <span class="user-chip__name">${escapeHtml(currentUser.full_name)}</span>
                <small>${staff ? 'Панель управления' : roleLabel(currentUser.role)}</small>
            </span>
        </a>
        <button class="btn btn-ghost btn-sm btn-icon" type="button" data-logout title="Выйти" aria-label="Выйти">
            ${icon('logout')}
        </button>`;

    renderNotificationList();
    document.getElementById('bellBtn')?.addEventListener('click', (event) => {
        event.stopPropagation();
        toggleNotifications();
    });
    document.getElementById('readAllBtn')?.addEventListener('click', async (event) => {
        event.stopPropagation();
        try {
            await api.post('/api/notifications/read-all');
            notifications = notifications.map((n) => ({ ...n, is_read: true }));
            unreadCount = 0;
            renderNotifications();
            toast('Все уведомления прочитаны', 'success');
        } catch (err) {
            toast(err.message || 'Не удалось обновить уведомления', 'error');
        }
    });
}

function renderFooter() {
    const footer = document.createElement('footer');
    footer.className = 'site-footer';
    footer.innerHTML = `
        <div class="container">
            <div class="footer-grid">
                <div>
                    <h4>ООО «pro-remont»</h4>
                    <p data-footer-note>Профессиональный ремонт крупной и мелкой бытовой техники. Работаем ежедневно с 08:00 до 22:00.</p>
                    <p>Выезд инженера за 45 минут. Гарантия до 24 месяцев.</p>
                </div>
                <div>
                    <h4>Навигация</h4>
                    <a href="prices.html">Прайс-лист</a>
                    <a href="masters.html">Наши мастера</a>
                    <a href="reviews.html">Отзывы клиентов</a>
                    <a href="about.html">О компании</a>
                    <a href="account.html">Личный кабинет</a>
                </div>
                <div>
                    <h4>Контакты</h4>
                    <p>Телефон: <a href="${PHONE_HREF}" style="display:inline;color:#fff">${PHONE}</a></p>
                    <p>Email: info@pro-remont-service.ru</p>
                    <p>Адрес: г. Екатеринбург, ул. Малышева, 51, офис 204</p>
                    <p>Ежедневно с 08:00 до 22:00</p>
                </div>
            </div>
            <div class="footer-bottom">
                <span>© 2026 ООО «pro-remont». ИНН 6671098452. Все права защищены.</span>
                <span class="footer-note">Сервисный центр бытовой техники в Екатеринбурге</span>
            </div>
        </div>`;
    const placeholder = document.getElementById('site-footer');
    if (placeholder) placeholder.replaceWith(footer);
    else document.body.appendChild(footer);
}

/* ------------------------------ Меню ------------------------------------ */

function bindHeader() {
    if (headerBound) return;
    headerBound = true;

    document.getElementById('navToggle')?.addEventListener('click', () => {
        const nav = document.getElementById('mainNav');
        const toggle = document.getElementById('navToggle');
        const open = nav?.classList.toggle('open');
        toggle?.setAttribute('aria-expanded', open ? 'true' : 'false');
        toggle?.setAttribute('aria-label', open ? 'Закрыть меню' : 'Открыть меню');
    });

    document.getElementById('mainNav')?.addEventListener('click', (event) => {
        if (event.target.closest('a')) {
            document.getElementById('mainNav')?.classList.remove('open');
            document.getElementById('navToggle')?.setAttribute('aria-expanded', 'false');
        }
    });

    document.addEventListener('click', (event) => {
        const panel = document.getElementById('notifPanel');
        if (panel && panel.classList.contains('open') && !event.target.closest('.notif-wrap')) {
            panel.classList.remove('open');
            document.getElementById('bellBtn')?.setAttribute('aria-expanded', 'false');
        }
    });
}

function initGlobalHandlers() {
    if (initGlobalHandlers.done) return;
    initGlobalHandlers.done = true;

    document.addEventListener('click', async (event) => {
        const auth = event.target.closest('[data-auth]');
        if (auth) {
            event.preventDefault();
            openAuth(auth.dataset.auth);
            return;
        }
        const logout = event.target.closest('[data-logout]');
        if (logout) {
            event.preventDefault();
            try {
                await api.post('/api/auth/logout');
            } catch { /* ignore */ }
            currentUser = null;
            window.location.href = 'index.html';
        }
    });
}

/* --------------------------- Авторизация -------------------------------- */

export function openAuth(mode = 'login', options = {}) {
    if (currentUser) {
        window.location.href = 'account.html';
        return;
    }
    const isLogin = mode !== 'register';
    const next = options.next || safeNext() || '';
    const modal = openModal({
        title: isLogin ? 'Вход в личный кабинет' : 'Регистрация клиента',
        subtitle: isLogin
            ? 'Отслеживайте статус ремонта и храните гарантийные талоны.'
            : 'Создайте аккаунт, чтобы оформить заявку и получать спецпредложения.',
        size: 'sm',
        body: isLogin ? loginFormHtml() : registerFormHtml(),
        footer: `<div class="text-sm text-muted" style="width:100%;text-align:center">
            ${isLogin ? 'Нет аккаунта?' : 'Уже зарегистрированы?'}
            <button type="button" class="btn btn-ghost btn-xs" data-switch-auth="${isLogin ? 'register' : 'login'}">
                ${isLogin ? 'Зарегистрироваться' : 'Войти'}
            </button>
        </div>`,
        onClose: () => {
            if (options.onClose) options.onClose();
        },
    });

    modal.el.querySelector('[data-switch-auth]')?.addEventListener('click', (event) => {
        const target = event.currentTarget.dataset.switchAuth;
        modal.close();
        openAuth(target, { next });
    });

        const form = modal.el.querySelector('form');
    form?.addEventListener('submit', async (event) => {
        event.preventDefault();
        clearFormErrors(form);
        if (!isLogin) {
            if (form.elements.password.value !== form.elements.password2.value) {
                showFormErrors(form, { password2: 'Пароли не совпадают' });
                return;
            }
            if (!form.elements.agree.checked) {
                showFormErrors(form, {}, 'Подтвердите согласие на обработку персональных данных');
                return;
            }
        }
        const button = form.querySelector('[type="submit"]');
        setBusy(button, true);
        try {
            const data = isLogin
                ? await api.post('/api/auth/login', {
                    contact: form.elements.contact.value.trim(),
                    password: form.elements.password.value,
                })
                : await api.post('/api/auth/register', {
                    full_name: form.elements.full_name.value.trim(),
                    contact: form.elements.contact.value.trim(),
                    password: form.elements.password.value,
                });

            currentUser = data.user;
            updateHeaderUser();
            toast(isLogin ? `Добро пожаловать, ${currentUser.full_name.split(' ')[0]}!` : 'Аккаунт создан. Добро пожаловать!', 'success');
            modal.close();
            const target = next || (currentUser.role === 'client' ? 'account.html' : 'account.html');
            setTimeout(() => {
                window.location.href = target;
            }, 250);
        } catch (err) {
            if (err instanceof ApiError && err.fields) {
                showFormErrors(form, err.fields, err.message);
            } else {
                showFormErrors(form, {}, err.message || 'Не удалось выполнить вход');
            }
            setBusy(button, false);
        }
    });
}

function loginFormHtml() {
    return `<form novalidate>
        <div class="form-error"></div>
        <div class="form-group">
            <label for="authContact">Телефон или Email</label>
            <input id="authContact" name="contact" type="text" autocomplete="username" required placeholder="client@example.ru">
            <div class="field-error" data-for="contact"></div>
        </div>
        <div class="form-group">
            <label for="authPassword">Пароль</label>
            <input id="authPassword" name="password" type="password" autocomplete="current-password" required placeholder="Введите пароль">
            <div class="field-error" data-for="password"></div>
        </div>
        <button type="submit" class="btn btn-primary btn-block">Войти</button>
        <p class="form-hint" style="text-align:center;margin-top:12px">
            Демо-клиент: client@example.ru / Client123!
        </p>
    </form>`;
}

function registerFormHtml() {
    return `<form novalidate>
        <div class="form-error"></div>
        <div class="form-group">
            <label for="regName">Ваше имя</label>
            <input id="regName" name="full_name" type="text" autocomplete="name" required placeholder="Иван Иванов">
            <div class="field-error" data-for="full_name"></div>
        </div>
        <div class="form-group">
            <label for="regContact">Телефон или Email</label>
            <input id="regContact" name="contact" type="text" autocomplete="email" required placeholder="+7 (___) ___-__-__ или mail@example.ru">
            <div class="field-error" data-for="contact"></div>
        </div>
        <div class="form-group">
            <label for="regPassword">Придумайте пароль</label>
            <input id="regPassword" name="password" type="password" autocomplete="new-password" required minlength="6" placeholder="Минимум 6 символов">
            <div class="field-error" data-for="password"></div>
        </div>
        <div class="form-group">
            <label for="regPassword2">Повторите пароль</label>
            <input id="regPassword2" name="password2" type="password" autocomplete="new-password" required minlength="6" placeholder="Ещё раз">
            <div class="field-error" data-for="password2"></div>
        </div>
        <label class="checkbox mb-2">
            <input type="checkbox" name="agree" required checked>
            <span>Согласен с обработкой персональных данных и условиями сервиса</span>
        </label>
        <button type="submit" class="btn btn-primary btn-block">Создать аккаунт</button>
    </form>`;
}

// Проверка совпадения паролей для формы регистрации.
document.addEventListener('submit', (event) => {
    const form = event.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (!form.elements.password2) return;
    if (form.elements.password.value !== form.elements.password2.value) {
        event.preventDefault();
        event.stopImmediatePropagation();
        showFormErrors(form, { password2: 'Пароли не совпадают' });
    }
}, true);

function safeNext() {
    const raw = new URLSearchParams(window.location.search).get('next');
    if (!raw) return '';
    if (!raw.startsWith('/') || raw.startsWith('//')) return '';
    return raw;
}

function handleAuthQuery() {
    const params = new URLSearchParams(window.location.search);
    const mode = params.get('auth');
    if (!mode || currentUser) return;
    const next = params.get('next') || '';
    setTimeout(() => openAuth(mode === 'register' ? 'register' : 'login', { next }), 120);
}

/* ------------------------------ Тема CMS -------------------------------- */

async function loadTheme() {
    try {
        theme = await api.get('/api/cms/theme/config');
    } catch {
        theme = null;
    }
    return theme;
}

function applyTheme(config) {
    if (!config) return;
    const root = document.documentElement;
    if (/^#[0-9a-f]{6}$/i.test(config.primary || '')) {
        root.style.setProperty('--primary', config.primary);
        root.style.setProperty('--primary-hover', shade(config.primary, -14));
        root.style.setProperty('--primary-soft', mix(config.primary, '#ffffff', 0.88));
    }
    if (/^#[0-9a-f]{6}$/i.test(config.accent || '')) {
        root.style.setProperty('--accent', config.accent);
        root.style.setProperty('--accent-hover', shade(config.accent, -14));
        root.style.setProperty('--accent-soft', mix(config.accent, '#ffffff', 0.9));
    }
    document.querySelectorAll('[data-theme="hero-title"]').forEach((el) => {
        if (config.hero_title) el.textContent = config.hero_title;
    });
    document.querySelectorAll('[data-theme="hero-subtitle"]').forEach((el) => {
        if (config.hero_subtitle) el.textContent = config.hero_subtitle;
    });
    document.querySelectorAll('[data-footer-note]').forEach((el) => {
        if (config.footer_note) el.textContent = config.footer_note;
    });
}

function shade(hex, percent) {
    const { r, g, b } = hexToRgb(hex);
    const f = 1 + percent / 100;
    return rgbToHex(clamp(r * f), clamp(g * f), clamp(b * f));
}

function mix(hex, target, weight) {
    const a = hexToRgb(hex);
    const b = hexToRgb(target);
    return rgbToHex(
        Math.round(a.r * (1 - weight) + b.r * weight),
        Math.round(a.g * (1 - weight) + b.g * weight),
        Math.round(a.b * (1 - weight) + b.b * weight),
    );
}

function hexToRgb(hex) {
    const h = hex.replace('#', '');
    return {
        r: parseInt(h.slice(0, 2), 16),
        g: parseInt(h.slice(2, 4), 16),
        b: parseInt(h.slice(4, 6), 16),
    };
}

function rgbToHex(r, g, b) {
    return `#${[r, g, b].map((v) => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, '0')).join('')}`;
}

function clamp(v) {
    return Math.max(0, Math.min(255, Math.round(v)));
}

/* --------------------------- Уведомления -------------------------------- */

async function loadNotifications() {
    if (!currentUser) return;
    try {
        const data = await api.get('/api/notifications', { limit: 30 });
        notifications = data.items || [];
        unreadCount = data.unread || 0;
        renderNotifications();
        maybeShowReadyNotification();
    } catch {
        // Молча игнорируем сетевые сбои фонового опроса.
    }
}

function renderNotifications() {
    const count = document.getElementById('bellCount');
    if (count) {
        count.textContent = unreadCount > 99 ? '99+' : String(unreadCount);
        count.classList.toggle('hidden', unreadCount === 0);
    }
    renderNotificationList();
}

function renderNotificationList() {
    const list = document.getElementById('notifList');
    if (!list) return;
    if (!notifications.length) {
        list.innerHTML = '<div class="notif-panel__empty">У вас пока нет уведомлений</div>';
        return;
    }
    list.innerHTML = notifications.map((n) => `
        <div class="notif ${n.is_read ? '' : 'unread'}" data-notif-id="${n.id}" data-request-id="${n.request_id || ''}" data-notif-type="${escapeHtml(n.type)}">
            <div class="notif__title">${escapeHtml(n.title)}</div>
            <div class="notif__text">${escapeHtml(n.message)}</div>
            <div class="notif__time">${relativeTime(n.created_at)}</div>
        </div>`).join('');

    list.querySelectorAll('[data-notif-id]').forEach((el) => {
        el.addEventListener('click', () => handleNotificationClick(el));
    });
}

function toggleNotifications() {
    const panel = document.getElementById('notifPanel');
    const bell = document.getElementById('bellBtn');
    if (!panel) return;
    const open = panel.classList.toggle('open');
    bell?.setAttribute('aria-expanded', open ? 'true' : 'false');
}

async function handleNotificationClick(el) {
    const id = el.dataset.notifId;
    const requestId = el.dataset.requestId;
    if (el.classList.contains('unread')) {
        try {
            await api.post(`/api/notifications/${id}/read`);
            notifications = notifications.map((n) => (n.id === Number(id) ? { ...n, is_read: true } : n));
            unreadCount = Math.max(0, unreadCount - 1);
            renderNotifications();
        } catch { /* ignore */ }
    }
    if (requestId) {
        const tab = isStaff() ? 'requests' : 'requests';
        window.location.href = `account.html?request=${encodeURIComponent(requestId)}#${tab}`;
    }
}

function maybeShowReadyNotification() {
    if (!currentUser || currentUser.role !== 'client') return;
    const ready = notifications.find((n) => !n.is_read && n.type === 'ready');
    if (!ready) return;
    if (shownReady.has(ready.id)) return;
    shownReady.add(ready.id);
    const dismissed = JSON.parse(sessionStorage.getItem('dismissedReady') || '[]');
    if (dismissed.includes(ready.id)) return;

    const modal = openModal({
        title: 'Ремонт готов к выдаче',
        subtitle: 'Уведомление о готовности заказа',
        size: 'sm',
        body: `<div class="text-sm">
            <p class="mb-2">${escapeHtml(ready.message)}</p>
            <div class="badge badge--ready">Готово к выдаче</div>
        </div>`,
        footer: `
            <button class="btn btn-outline" type="button" data-read>Прочитано</button>
            <button class="btn btn-primary" type="button" data-open>Открыть заявку</button>`,
    });

    const markDismissed = () => {
        dismissed.push(ready.id);
        sessionStorage.setItem('dismissedReady', JSON.stringify(dismissed));
    };
    modal.foot.querySelector('[data-read]').addEventListener('click', async () => {
        try {
            await api.post(`/api/notifications/${ready.id}/read`);
            notifications = notifications.map((n) => (n.id === ready.id ? { ...n, is_read: true } : n));
            unreadCount = Math.max(0, unreadCount - 1);
            renderNotifications();
        } catch { /* ignore */ }
        markDismissed();
        modal.close();
    });
    modal.foot.querySelector('[data-open]').addEventListener('click', () => {
        markDismissed();
        window.location.href = `account.html?request=${ready.request_id}#requests`;
    });
}

/* ---------------------- Экспорт для страниц ----------------------------- */

export { statusBadge };
