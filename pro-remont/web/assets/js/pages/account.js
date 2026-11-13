/**
 * Личный кабинет: клиент, мастер, менеджер, администратор.
 */
import { api } from '../api.js';
import {
    initLayout, hasPermission, isStaff, refreshUser,
} from '../app.js';
import {
    escapeHtml, money, date, dateTime, relativeTime, statusBadge, statusInfo,
    invoiceBadge, reviewBadge, roleLabel, initials, stars, emptyState, toast,
    openModal, confirmDialog, setBusy, clearFormErrors, showFormErrors,
    debounce,
} from '../ui.js';
import { openPayment, showReceipt } from '../payments.js';

const state = {
    user: null,
    tab: 'requests',
    stats: null,
    managerStatus: 'active',
    managerQuery: '',
    usersRole: '',
    usersQuery: '',
    permissionCatalog: null,
};

const PERMISSION_LABELS = {
    manage_requests: 'Обработка заявок',
    assign_masters: 'Назначение мастеров',
    manage_prices: 'Прайс-лист',
    manage_users: 'Пользователи',
    manage_cms: 'Дизайн и CMS',
    view_reports: 'Отчёты',
};

async function main() {
    state.user = await initLayout({ active: '' });
    if (!state.user) {
        renderGate();
        return;
    }
    renderHero();
    renderLayout();
    await Promise.all([loadStats(), route()]);
    window.addEventListener('hashchange', () => route());
    openRequestFromQuery();
}

/* ---------------------------- Каркас ------------------------------------ */

function renderGate() {
    document.getElementById('accountTitle').textContent = 'Личный кабинет';
    document.getElementById('accountSubtitle').textContent = 'Войдите, чтобы управлять заявками и отслеживать ремонт';
    document.getElementById('accountRoot').innerHTML = `
        <div class="card card--static" style="max-width:560px;margin:36px auto;text-align:center">
            <div class="empty-state" style="padding-top:10px">
                <div class="empty-state__icon">🔐</div>
                <h3>Требуется вход</h3>
                <p>Личный кабинет доступен клиентам, мастерам, менеджерам и администраторам сервисного центра.</p>
                <div class="flex-center" style="justify-content:center;flex-wrap:wrap;gap:10px">
                    <button class="btn btn-primary" type="button" data-auth="login">Войти</button>
                    <button class="btn btn-outline" type="button" data-auth="register">Зарегистрироваться</button>
                </div>
            </div>
            <div class="form-hint mt-2">Демо-клиент: client@example.ru / Client123!</div>
        </div>`;
}

function renderHero() {
    const u = state.user;
    document.getElementById('accountTitle').textContent = 'Личный кабинет';
    document.getElementById('accountSubtitle').textContent = `${roleLabel(u.role)} · ${u.full_name}`;
    document.getElementById('accountUserBlock').classList.remove('hidden');
    document.getElementById('accountAvatar').textContent = initials(u.full_name);
    document.getElementById('accountName').textContent = u.full_name;
    document.getElementById('accountRole').textContent = roleLabel(u.role);
}

function renderLayout() {
    const tabs = tabsForUser();
    document.getElementById('accountRoot').innerHTML = `
        <div class="account-layout">
            <aside class="account-side">
                <div class="account-side__title">Меню кабинета</div>
                <nav class="account-nav" id="accountNav">
                    ${tabs.map((t) => `<button type="button" data-tab="${t.key}"><span class="nav-dot"></span>${escapeHtml(t.label)}</button>`).join('')}
                </nav>
                <div class="account-side__title" style="margin-top:8px">Сервис</div>
                <nav class="account-nav">
                    <a class="btn btn-ghost btn-sm" href="contacts.html" style="justify-content:flex-start;text-decoration:none">＋ Новая заявка</a>
                    <a class="btn btn-ghost btn-sm" href="index.html" style="justify-content:flex-start;text-decoration:none">← На сайт</a>
                    <button class="btn btn-ghost btn-sm" type="button" data-logout style="justify-content:flex-start">Выйти из аккаунта</button>
                </nav>
            </aside>
            <div class="account-main">
                <div class="stats-grid ${isStaff() ? 'stats-grid--staff' : ''}" id="accountStats"></div>
                <div id="accountContent">
                    <div class="panel"><div class="panel__body"><div class="skeleton" style="height:140px"></div></div></div>
                </div>
            </div>
        </div>`;

    document.getElementById('accountNav').addEventListener('click', (event) => {
        const button = event.target.closest('[data-tab]');
        if (!button) return;
        const tab = button.dataset.tab;
        if (location.hash === `#${tab}`) route();
        else location.hash = tab;
    });
}

function tabsForUser() {
    const u = state.user;
    const tabs = [{ key: 'requests', label: u.role === 'master' ? 'Мои заявки' : 'Заявки' }];
    if (u.role === 'client') {
        tabs.push({ key: 'history', label: 'История ремонтов' });
        tabs.push({ key: 'offers', label: 'Спецпредложения' });
        tabs.push({ key: 'payments', label: 'Платежи' });
    }
    if (hasPermission('manage_prices')) {
        tabs.push({ key: 'prices', label: 'Прайс-лист' });
    }
    if (hasPermission('manage_requests')) {
        tabs.push({ key: 'reviews', label: 'Отзывы' });
    }
    if (hasPermission('manage_users')) {
        tabs.push({ key: 'users', label: 'Пользователи' });
    }
    if (hasPermission('manage_cms')) {
        tabs.push({ key: 'theme', label: 'Оформление' });
    }
    tabs.push({ key: 'profile', label: 'Профиль' });
    tabs.push({ key: 'notifications', label: 'Уведомления' });
    return tabs;
}

async function route() {
    const tabs = tabsForUser();
    let tab = location.hash.replace('#', '') || 'requests';
    if (!tabs.some((t) => t.key === tab)) tab = 'requests';
    state.tab = tab;
    document.querySelectorAll('#accountNav [data-tab]').forEach((button) => {
        button.classList.toggle('active', button.dataset.tab === tab);
    });
    document.title = `${tabs.find((t) => t.key === tab)?.label || 'Кабинет'} — pro-remont`;

    const content = document.getElementById('accountContent');
    if (!content) return;
    content.innerHTML = '<div class="panel"><div class="panel__body"><div class="skeleton" style="height:140px"></div></div></div>';

    try {
        switch (tab) {
            case 'requests': await renderRequests(); break;
            case 'history': await renderClientHistory(); break;
            case 'offers': await renderOffers(); break;
            case 'payments': await renderPayments(); break;
            case 'prices': await renderPrices(); break;
            case 'reviews': await renderReviews(); break;
            case 'users': await renderUsers(); break;
            case 'theme': await renderTheme(); break;
            case 'profile': await renderProfile(); break;
            case 'notifications': await renderNotifications(); break;
            default: await renderRequests();
        }
    } catch (err) {
        content.innerHTML = panel('Ошибка', emptyState('Не удалось загрузить раздел', err.message,
            '<button type="button" class="btn btn-outline btn-sm" id="retryTab">Повторить</button>'));
        document.getElementById('retryTab')?.addEventListener('click', route);
        toast(err.message || 'Ошибка загрузки', 'error');
    }
}

function setContent(html) {
    const content = document.getElementById('accountContent');
    if (content) content.innerHTML = html;
}

function panel(title, body, { actions = '', subtitle = '', flush = false } = {}) {
    return `<section class="panel">
        <div class="panel__head">
            <div>
                <h2>${escapeHtml(title)}</h2>
                ${subtitle ? `<p>${escapeHtml(subtitle)}</p>` : ''}
            </div>
            ${actions ? `<div class="toolbar">${actions}</div>` : ''}
        </div>
        <div class="panel__body ${flush ? 'panel__body--flush' : ''}">${body}</div>
    </section>`;
}

async function loadStats() {
    const el = document.getElementById('accountStats');
    if (!el) return;
    try {
        const data = await api.get('/api/stats');
        state.stats = data;
        const cards = (data.cards || []).map((c) => `
            <div class="stat-card stat-card--${escapeHtml(c.tone || 'default')}">
                <div class="stat-card__label">${escapeHtml(c.label)}</div>
                <div class="stat-card__value">${escapeHtml(c.value)}</div>
                <div class="stat-card__hint">${escapeHtml(c.hint || '')}</div>
            </div>`).join('');
        let extra = '';
        if (isStaff() && data.status_counts) {
            const chips = Object.entries(data.status_counts)
                .filter(([, n]) => n > 0)
                .map(([st, n]) => `<span class="badge badge--${statusInfo(st).tone}">${escapeHtml(statusInfo(st).label)}: ${n}</span>`)
                .join(' ');
            if (chips) extra = `<div class="stat-card" style="grid-column:1/-1"><div class="stat-card__label">Заявки по статусам</div><div class="flex gap-1" style="flex-wrap:wrap;margin-top:6px">${chips}</div></div>`;
        }
        el.innerHTML = cards + extra;
    } catch {
        el.innerHTML = '';
    }
}

async function afterAction(message) {
    if (message) toast(message, 'success');
    await loadStats();
    await route();
}

/* --------------------------- Заявки ------------------------------------- */

async function renderRequests() {
    if (state.user.role === 'master') return renderMasterRequests();
    if (state.user.role === 'manager' || state.user.role === 'admin') return renderManagerRequests();
    return renderClientRequests();
}

async function renderClientRequests() {
    const data = await api.get('/api/requests/my');
    const items = data.items || [];
    setContent(panel('Мои заявки', items.length
        ? `<div class="request-list">${items.map(requestCard).join('')}</div>`
        : emptyState('Активных заявок нет', 'Оформите заявку — мастер приедет в удобное время.',
            '<a class="btn btn-primary btn-sm" href="contacts.html">Оставить заявку</a>'),
    { subtitle: 'Текущие ремонты и их статусы', actions: '<a class="btn btn-primary btn-sm" href="contacts.html">＋ Новая заявка</a>' }));
    bindRequestActions(document.getElementById('accountContent'));
}

async function renderClientHistory() {
    const data = await api.get('/api/requests/my-history');
    const items = data.items || [];
    setContent(panel('История ремонтов', items.length
        ? `<div class="request-list">${items.map(requestCard).join('')}</div>`
        : emptyState('История пока пуста', 'Здесь появятся завершённые и оплаченные заявки.'),
    { subtitle: 'Завершённые и отменённые заявки' }));
    bindRequestActions(document.getElementById('accountContent'));
}

function requestCard(req) {
    const actions = requestActions(req);
    return `<div class="request-card">
        <div class="request-card__top">
            <div>
                <div class="request-card__number">${escapeHtml(req.public_number)}</div>
                <div class="request-card__title">${escapeHtml(req.service_title || req.equipment_type || 'Ремонт техники')}</div>
            </div>
            ${statusBadge(req.status)}
        </div>
        <div class="request-card__meta">
            <span>📅 ${date(req.created_at)}</span>
            <span>${req.master_name ? `👤 ${escapeHtml(req.master_name)}` : 'Мастер не назначен'}</span>
            <span>💳 ${money(req.total_amount)}</span>
            <span>📍 ${escapeHtml(req.address)}</span>
        </div>
        ${req.description ? `<p class="text-sm text-muted">${escapeHtml(truncate(req.description, 220))}</p>` : ''}
        <div class="request-card__actions">${actions}</div>
    </div>`;
}

function requestActions(req) {
    const u = state.user;
    const out = [`<button class="btn btn-outline btn-sm" type="button" data-action="detail" data-id="${req.id}">Подробнее</button>`];
    if (u.role === 'client') {
        if (req.status === 'new' || req.status === 'accepted') {
            out.push(`<button class="btn btn-ghost btn-sm" type="button" data-action="cancel" data-id="${req.id}">Отменить</button>`);
        }
        if (req.status === 'ready' || req.status === 'completed') {
            out.push(`<button class="btn btn-primary btn-sm" type="button" data-action="pay" data-id="${req.id}">Оплатить</button>`);
        }
        if (req.status === 'paid' || req.status === 'completed') {
            out.push(`<a class="btn btn-outline btn-sm" href="reviews.html?request=${req.id}">Оставить отзыв</a>`);
        }
        return out.join('');
    }
    if (u.role === 'master') {
        if (req.status === 'assigned') out.push(`<button class="btn btn-primary btn-sm" type="button" data-action="status" data-status="in_progress" data-id="${req.id}">Начать работу</button>`);
        if (req.status === 'in_progress') out.push(`<button class="btn btn-success btn-sm" type="button" data-action="status" data-status="ready" data-id="${req.id}">Завершить ремонт</button>`);
        return out.join('');
    }
    if (isStaff()) {
        if (req.status === 'new') out.push(`<button class="btn btn-primary btn-sm" type="button" data-action="accept" data-id="${req.id}">Принять</button>`);
        if (req.status === 'accepted') out.push(`<button class="btn btn-primary btn-sm" type="button" data-action="assign" data-id="${req.id}">Назначить</button>`);
        if (req.status === 'assigned') out.push(`<button class="btn btn-outline btn-sm" type="button" data-action="status" data-status="in_progress" data-id="${req.id}">Начать</button>`);
        if (req.status === 'in_progress') out.push(`<button class="btn btn-outline btn-sm" type="button" data-action="status" data-status="ready" data-id="${req.id}">Завершить</button>`);
        if (req.status === 'ready') {
            out.push(`<button class="btn btn-outline btn-sm" type="button" data-action="status" data-status="completed" data-id="${req.id}">Выполнено</button>`);
            out.push(`<button class="btn btn-primary btn-sm" type="button" data-action="pay" data-id="${req.id}">Оплата</button>`);
        }
        if (req.status === 'completed') out.push(`<button class="btn btn-primary btn-sm" type="button" data-action="pay" data-id="${req.id}">Оплата</button>`);
    }
    return out.join('');
}

async function renderMasterRequests() {
    const data = await api.get('/api/master/requests', { status: 'active' });
    const items = data.items || [];
    setContent(panel('Мои назначенные заявки', items.length
        ? `<div class="request-list">${items.map(masterRequestCard).join('')}</div>`
        : emptyState('Назначенных заявок нет', 'Когда менеджер назначит вас на заявку, она появится здесь.'),
    { subtitle: 'Клиенты, адреса и статусы ремонтов' }));
    bindRequestActions(document.getElementById('accountContent'));
}

function masterRequestCard(req) {
    return `<div class="request-card">
        <div class="request-card__top">
            <div>
                <div class="request-card__number">${escapeHtml(req.public_number)} · ${escapeHtml(req.equipment_type || 'Техника')}</div>
                <div class="request-card__title">${escapeHtml(req.service_title || 'Диагностика и ремонт')}</div>
            </div>
            ${statusBadge(req.status)}
        </div>
        <div class="request-card__meta">
            <span>👤 ${escapeHtml(req.customer_name)}</span>
            <span>📞 <a href="tel:${escapeHtml(req.phone)}">${escapeHtml(req.phone)}</a></span>
            <span>📍 ${escapeHtml(req.address)}</span>
            <span>💳 ${money(req.total_amount)}</span>
        </div>
        <p class="text-sm text-muted">${escapeHtml(req.description)}</p>
        <div class="request-card__actions">${requestActions(req)}</div>
    </div>`;
}

async function renderManagerRequests() {
    const statusOptions = [
        ['active', 'Активные'], ['new', 'Новые'], ['accepted', 'Принятые'],
        ['assigned', 'Назначенные'], ['in_progress', 'В работе'], ['ready', 'Готовые'],
        ['completed', 'Выполненные'], ['paid', 'Оплаченные'], ['archive', 'Архив'], ['', 'Все'],
    ];
    setContent(`<section class="panel">
        <div class="panel__head">
            <div><h2>Все заявки</h2><p id="managerSummary">Загружаем…</p></div>
            <div class="toolbar">
                <select id="managerStatus" aria-label="Фильтр по статусу">
                    ${statusOptions.map(([v, l]) => `<option value="${v}" ${v === state.managerStatus ? 'selected' : ''}>${l}</option>`).join('')}
                </select>
                <div class="search-box">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
                    <input type="search" id="managerSearch" placeholder="Номер, клиент, телефон…" value="${escapeHtml(state.managerQuery)}">
                </div>
                <button class="btn btn-outline btn-sm" type="button" id="managerRefresh">Обновить</button>
            </div>
        </div>
        <div class="table-wrap" style="border:0;border-radius:0;box-shadow:none">
            <table class="table--compact">
                <thead><tr><th>№ / дата</th><th>Клиент</th><th>Техника</th><th>Мастер</th><th>Сумма</th><th>Статус</th><th>Действия</th></tr></thead>
                <tbody id="managerRows"><tr><td colspan="7"><div class="skeleton" style="height:22px"></div></td></tr></tbody>
            </table>
        </div>
    </section>`);

    document.getElementById('managerStatus').addEventListener('change', (event) => {
        state.managerStatus = event.target.value;
        loadManagerRows();
    });
    const search = document.getElementById('managerSearch');
    search.addEventListener('input', debounce(() => {
        state.managerQuery = search.value.trim();
        loadManagerRows();
    }, 350));
    document.getElementById('managerRefresh').addEventListener('click', loadManagerRows);
    await loadManagerRows();
}

async function loadManagerRows() {
    const tbody = document.getElementById('managerRows');
    if (!tbody) return;
    const params = { limit: 100 };
    if (state.managerStatus) params.status = state.managerStatus;
    if (state.managerQuery) params.q = state.managerQuery;
    try {
        const data = await api.get('/api/manager/requests', params);
        const items = data.items || [];
        document.getElementById('managerSummary').textContent = `${data.total} заявок · показано ${items.length}`;
        if (!items.length) {
            tbody.innerHTML = `<tr><td colspan="7">${emptyState('Заявок не найдено', 'Измените фильтр или обновите список.')}</td></tr>`;
            return;
        }
        tbody.innerHTML = items.map((req) => `
            <tr>
                <td class="nowrap"><strong>${escapeHtml(req.public_number)}</strong><div class="text-xs text-muted">${dateShort(req.created_at)}</div></td>
                <td>${escapeHtml(req.customer_name)}<div class="text-xs text-muted">${escapeHtml(req.phone)}</div></td>
                <td>${escapeHtml(req.equipment_type || req.category_title || '—')}<div class="text-xs text-muted">${escapeHtml(req.service_title || '')}</div></td>
                <td>${req.master_name ? escapeHtml(req.master_name) : '<span class="text-muted">не назначен</span>'}</td>
                <td class="nowrap">${money(req.total_amount)}</td>
                <td>${statusBadge(req.status)}</td>
                <td><div class="admin-table-actions">${requestActions(req)}</div></td>
            </tr>`).join('');
        bindRequestActions(tbody);
    } catch (err) {
        tbody.innerHTML = `<tr><td colspan="7">${emptyState('Ошибка загрузки', err.message)}</td></tr>`;
    }
}

function bindRequestActions(scope) {
    if (!scope) return;
    scope.querySelectorAll('[data-action]').forEach((button) => {
        if (button.dataset.bound) return;
        button.dataset.bound = '1';
        button.addEventListener('click', (event) => {
            event.preventDefault();
            handleRequestAction(button.dataset.action, Number(button.dataset.id), button.dataset.status);
        });
    });
}

async function handleRequestAction(action, id, status) {
    if (action === 'detail') {
        await openRequestDetail(id);
        return;
    }
    if (action === 'assign') {
        await openAssignModal(id);
        return;
    }
    if (action === 'review') {
        window.location.href = `reviews.html?request=${id}`;
        return;
    }
    if (action === 'pay') {
        await openPayment(id, {
            allowCash: isStaff(),
            onPaid: () => afterAction('Платёж проведён'),
        });
        return;
    }
    if (action === 'cancel') {
        const ok = await confirmDialog({
            title: 'Отменить заявку?',
            text: 'Заявка будет переведена в статус «Отменена». Это действие нельзя отменить.',
            confirmLabel: 'Отменить заявку',
            danger: true,
        });
        if (!ok) return;
        try {
            await api.post(`/api/requests/${id}/cancel`, { comment: 'Отменено клиентом' });
            await afterAction('Заявка отменена');
        } catch (err) {
            toast(err.message || 'Не удалось отменить заявку', 'error');
        }
        return;
    }
    if (action === 'accept') {
        try {
            await api.patch(`/api/requests/${id}/accept`, { comment: 'Принята менеджером' });
            await afterAction('Заявка принята в обработку');
        } catch (err) {
            toast(err.message || 'Не удалось принять заявку', 'error');
        }
        return;
    }
    if (action === 'status' && status) {
        try {
            await api.patch(`/api/requests/${id}/status`, { status, comment: '' });
            await afterAction(`Статус изменён: ${statusInfo(status).label}`);
        } catch (err) {
            toast(err.message || 'Не удалось изменить статус', 'error');
        }
    }
}

/* ----------------------- Детали и действия ------------------------------ */

async function openRequestDetail(id) {
    let data;
    try {
        data = await api.get(`/api/requests/${id}`);
    } catch (err) {
        toast(err.message || 'Не удалось загрузить заявку', 'error');
        return;
    }
    const req = data.request;
    const history = data.history || [];
    const modal = openModal({
        title: `Заявка ${req.public_number}`,
        subtitle: `${statusInfo(req.status).label} · создана ${dateTime(req.created_at)}`,
        size: 'xl',
        body: `
            <div class="flex-between mb-2">
                ${statusBadge(req.status)}
                <strong style="font-size:1.15rem">${money(req.total_amount)}</strong>
            </div>
            <div class="detail-grid">
                ${detailItem('Клиент', escapeHtml(req.customer_name))}
                ${detailItem('Телефон', `<a href="tel:${escapeHtml(req.phone)}">${escapeHtml(req.phone)}</a>`)}
                ${detailItem('Email', escapeHtml(req.email || '—'))}
                ${detailItem('Адрес', escapeHtml(req.address))}
                ${detailItem('Техника', escapeHtml(`${req.equipment_type || '—'}${req.brand_model ? ` · ${req.brand_model}` : ''}`))}
                ${detailItem('Услуга', escapeHtml(req.service_title || 'Уточняется'))}
                ${detailItem('Мастер', escapeHtml(req.master_name || 'Не назначен'))}
                ${detailItem('Категория', escapeHtml(req.category_title || '—'))}
                ${detailItem('Создана', dateTime(req.created_at))}
                ${detailItem('Принята', req.accepted_at ? dateTime(req.accepted_at) : '—')}
                ${detailItem('Завершена', req.completed_at ? dateTime(req.completed_at) : '—')}
                ${detailItem('Клиентский аккаунт', req.client_id ? `#${req.client_id}` : 'Гость')}
            </div>
            <div class="detail-section">
                <h4>Описание неисправности</h4>
                <p class="text-sm">${escapeHtml(req.description)}</p>
            </div>
            ${req.manager_comment ? `<div class="detail-section"><h4>Комментарий менеджера</h4><p class="text-sm">${escapeHtml(req.manager_comment)}</p></div>` : ''}
            <div class="detail-section">
                <h4>История заявки</h4>
                ${history.length ? `<div class="timeline">${history.map((h) => `
                    <div class="timeline__item">
                        <div class="timeline__title">${escapeHtml(h.from_status ? `${statusInfo(h.from_status).label} → ${statusInfo(h.to_status).label}` : statusInfo(h.to_status).label)}</div>
                        <div class="timeline__meta">${escapeHtml(h.changed_by || 'Система')} · ${dateTime(h.created_at)}</div>
                        ${h.comment ? `<div class="timeline__text">${escapeHtml(h.comment)}</div>` : ''}
                    </div>`).join('')}</div>` : '<p class="text-sm text-muted">История пуста</p>'}
            </div>`,
        footer: `
            <button class="btn btn-outline" type="button" data-close>Закрыть</button>
            ${isStaff() ? '<button class="btn btn-outline" type="button" data-amount>Изменить сумму</button>' : ''}
            ${requestActions(req)}`,
    });

    modal.foot.querySelector('[data-close]').addEventListener('click', () => modal.close());
    modal.foot.querySelector('[data-amount]')?.addEventListener('click', () => {
        modal.close();
        openAmountModal(req);
    });
    modal.foot.querySelectorAll('[data-action]').forEach((button) => {
        if (button.dataset.action === 'detail') {
            button.remove();
            return;
        }
        button.addEventListener('click', () => {
            const action = button.dataset.action;
            const status = button.dataset.status;
            modal.close();
            setTimeout(() => handleRequestAction(action, req.id, status), 30);
        });
    });
}

function detailItem(label, value) {
    return `<div class="detail-item">
        <div class="detail-item__label">${escapeHtml(label)}</div>
        <div class="detail-item__value">${value}</div>
    </div>`;
}

async function openAmountModal(req) {
    const modal = openModal({
        title: 'Изменить стоимость',
        subtitle: `Заявка ${req.public_number}`,
        size: 'sm',
        body: `<form id="amountForm" novalidate>
            <div class="form-error"></div>
            <div class="form-group">
                <label for="amountValue">Итоговая сумма, ₽</label>
                <input id="amountValue" name="total_amount" type="number" min="0" max="1000000" step="0.01" value="${Number(req.total_amount).toFixed(2)}" required>
                <div class="field-error" data-for="total_amount"></div>
            </div>
            <div class="form-group">
                <label for="amountComment">Комментарий</label>
                <input id="amountComment" name="comment" type="text" placeholder="Например: добавлена замена помпы">
            </div>
        </form>`,
        footer: '<button class="btn btn-outline" type="button" data-cancel>Отмена</button><button class="btn btn-primary" type="submit" form="amountForm" data-save>Сохранить</button>',
    });
    modal.foot.querySelector('[data-cancel]').addEventListener('click', () => modal.close());
    const form = modal.body.querySelector('#amountForm');
    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        clearFormErrors(form);
        const button = modal.foot.querySelector('[data-save]');
        setBusy(button, true);
        try {
            await api.patch(`/api/requests/${req.id}/amount`, {
                total_amount: Number(form.elements.total_amount.value),
                comment: form.elements.comment.value.trim(),
            });
            modal.close();
            await afterAction('Сумма заявки обновлена');
        } catch (err) {
            setBusy(button, false);
            if (err.fields) showFormErrors(form, err.fields, err.message);
            else showFormErrors(form, {}, err.message || 'Не удалось изменить сумму');
        }
    });
}

async function openAssignModal(requestId) {
    let data;
    try {
        data = await api.get('/api/masters', { all: 'true' });
    } catch (err) {
        toast(err.message || 'Не удалось загрузить мастеров', 'error');
        return;
    }
    const masters = data.items || [];
    if (!masters.length) {
        toast('Нет доступных мастеров', 'warning');
        return;
    }
    const modal = openModal({
        title: 'Назначить мастера',
        subtitle: 'Выберите сотрудника и подтвердите назначение',
        size: 'lg',
        body: `
            <form id="assignForm" novalidate>
                <div class="form-error"></div>
                <div class="perm-grid">
                    ${masters.map((m) => `
                        <label class="perm-item" style="cursor:pointer">
                            <input type="radio" name="master_id" value="${m.id}" ${m.is_available ? '' : 'data-busy="1"'}>
                            <div style="flex:1">
                                <strong>${escapeHtml(m.full_name)}</strong>
                                <span>${escapeHtml(m.specialization)} · ${m.experience_years} лет · ★ ${Number(m.rating).toFixed(1)}</span>
                                <div class="mt-1">
                                    ${m.is_available
        ? '<span class="badge badge--ready">Свободен</span>'
        : `<span class="badge badge--in_progress">Занят · ${m.active_requests} заявок</span>`}
                                </div>
                            </div>
                        </label>`).join('')}
                </div>
                <label class="checkbox mt-2">
                    <input type="checkbox" name="force">
                    <span>Назначить, даже если мастер занят другой заявкой</span>
                </label>
            </form>`,
        footer: '<button class="btn btn-outline" type="button" data-cancel>Отмена</button><button class="btn btn-primary" type="submit" form="assignForm" data-save>Назначить</button>',
    });
    modal.foot.querySelector('[data-cancel]').addEventListener('click', () => modal.close());
    const form = modal.body.querySelector('#assignForm');
    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        clearFormErrors(form);
        const selected = form.querySelector('input[name="master_id"]:checked');
        if (!selected) {
            showFormErrors(form, { master_id: 'Выберите мастера' });
            return;
        }
        const button = modal.foot.querySelector('[data-save]');
        setBusy(button, true);
        try {
            await api.post(`/api/requests/${requestId}/assign-master`, {
                master_id: Number(selected.value),
                force: form.elements.force.checked,
                comment: 'Мастер назначен менеджером',
            });
            modal.close();
            await afterAction('Мастер назначен');
        } catch (err) {
            setBusy(button, false);
            if (err.code === 'master_busy') {
                showFormErrors(form, {}, err.message);
                toast('Мастер занят. Поставьте галочку принудительного назначения.', 'warning');
            } else if (err.fields) {
                showFormErrors(form, err.fields, err.message);
            } else {
                showFormErrors(form, {}, err.message || 'Не удалось назначить мастера');
            }
        }
    });
}

/* ------------------------------ Клиент: предложения и платежи ----------- */

async function renderOffers() {
    const data = await api.get('/api/offers/personal');
    const items = data.items || [];
    setContent(panel('Специальные предложения', items.length
        ? `<div class="grid-2">${items.map((o) => `
            <div class="card card--static">
                <div class="flex-between">
                    <span class="badge badge--accent">−${o.discount_percent}%</span>
                    <span class="text-xs text-muted">${escapeHtml(o.reason)}</span>
                </div>
                <h3 class="mt-2">${escapeHtml(o.title)}</h3>
                <p class="text-sm">${escapeHtml(o.description)}</p>
                <div class="flex-between mt-2">
                    <code style="background:var(--light);padding:6px 10px;border-radius:6px;font-weight:700">${escapeHtml(o.code)}</code>
                    <button class="btn btn-outline btn-sm" type="button" data-copy="${escapeHtml(o.code)}">Скопировать код</button>
                </div>
            </div>`).join('')}</div>`
        : emptyState('Персональных предложений пока нет', 'Сделайте первый заказ — мы подготовим для вас скидку.'),
    { subtitle: `Учитываем историю ваших заказов: ${data.orders} ремонтов` }));

    document.querySelectorAll('[data-copy]').forEach((button) => {
        button.addEventListener('click', async () => {
            try {
                await navigator.clipboard.writeText(button.dataset.copy);
                toast('Код скопирован', 'success');
            } catch {
                toast(`Код: ${button.dataset.copy}`, 'info');
            }
        });
    });
}

async function renderPayments() {
    const data = await api.get('/api/billing/invoices/my');
    const items = data.items || [];
    setContent(`<section class="panel">
        <div class="panel__head"><div><h2>Платежи и счета</h2><p>История оплат и электронные чеки</p></div></div>
        ${items.length ? `<div class="table-wrap" style="border:0;box-shadow:none;border-radius:0">
            <table class="table--compact">
                <thead><tr><th>Счёт</th><th>Заявка</th><th>Сумма</th><th>Статус</th><th>Дата</th><th></th></tr></thead>
                <tbody>${items.map((inv) => `
                    <tr>
                        <td><strong>${escapeHtml(inv.number)}</strong></td>
                        <td>${escapeHtml(inv.request_number || `#${inv.request_id}`)}</td>
                        <td class="nowrap">${money(inv.amount)}</td>
                        <td>${invoiceBadge(inv.status)}</td>
                        <td class="nowrap">${inv.paid_at ? dateTime(inv.paid_at) : date(inv.created_at)}</td>
                        <td>${inv.status === 'paid'
        ? `<button class="btn btn-outline btn-sm" type="button" data-receipt="${inv.request_id}">Чек</button>`
        : `<button class="btn btn-primary btn-sm" type="button" data-pay="${inv.request_id}">Оплатить</button>`}</td>
                    </tr>`).join('')}</tbody>
            </table>
        </div>` : emptyState('Счетов пока нет', 'Счёт появится после оформления заявки.')}
    </section>`);

    document.querySelectorAll('[data-pay]').forEach((button) => {
        button.addEventListener('click', () => openPayment(Number(button.dataset.pay), {
            onPaid: () => afterAction('Платёж проведён'),
        }));
    });
    document.querySelectorAll('[data-receipt]').forEach((button) => {
        button.addEventListener('click', async () => {
            try {
                const data = await api.get(`/api/billing/invoice/${button.dataset.receipt}`);
                showReceipt(data);
            } catch (err) {
                toast(err.message || 'Не удалось загрузить чек', 'error');
            }
        });
    });
}

/* ------------------------------ Профиль -------------------------------- */

async function renderProfile() {
    const u = state.user;
    setContent(panel('Профиль', `
        <form id="profileForm" novalidate>
            <div class="form-error"></div>
            <div class="form-row">
                <div class="form-group">
                    <label for="profileName">ФИО</label>
                    <input id="profileName" name="full_name" type="text" value="${escapeHtml(u.full_name)}" required>
                    <div class="field-error" data-for="full_name"></div>
                </div>
                <div class="form-group">
                    <label for="profilePhone">Телефон</label>
                    <input id="profilePhone" name="phone" type="tel" value="${escapeHtml(u.phone || '')}">
                    <div class="field-error" data-for="phone"></div>
                </div>
            </div>
            <div class="form-group">
                <label for="profileEmail">Email</label>
                <input id="profileEmail" name="email" type="email" value="${escapeHtml(u.email || '')}">
                <div class="field-error" data-for="email"></div>
            </div>
            <div class="form-group">
                <label for="profileAddress">Адрес</label>
                <input id="profileAddress" name="address" type="text" value="${escapeHtml(u.address || '')}" placeholder="г. Екатеринбург, ул. Малышева, 51, кв. 12">
                <div class="field-error" data-for="address"></div>
            </div>
            <button class="btn btn-primary" type="submit">Сохранить изменения</button>
        </form>`,
    {
        subtitle: 'Контактные данные для связи и выезда мастера',
        actions: `<span class="badge badge--soft">${escapeHtml(roleLabel(u.role))}</span>`,
    }));

    const form = document.getElementById('profileForm');
    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        clearFormErrors(form);
        const button = form.querySelector('[type="submit"]');
        setBusy(button, true);
        try {
            await api.put('/api/profile', {
                full_name: form.elements.full_name.value.trim(),
                phone: form.elements.phone.value.trim(),
                email: form.elements.email.value.trim(),
                address: form.elements.address.value.trim(),
            });
            state.user = await refreshUser();
            renderHero();
            toast('Профиль сохранён', 'success');
            setBusy(button, false);
        } catch (err) {
            setBusy(button, false);
            if (err.fields) showFormErrors(form, err.fields, err.message);
            else showFormErrors(form, {}, err.message || 'Не удалось сохранить профиль');
        }
    });
}

/* ------------------------------ Прайс ----------------------------------- */

async function renderPrices() {
    const data = await api.get('/api/manager/price-list');
    const items = data.items || [];
    setContent(`<section class="panel">
        <div class="panel__head">
            <div><h2>Прайс-лист</h2><p>Изменения сразу отображаются на сайте</p></div>
            <a class="btn btn-outline btn-sm" href="prices.html" target="_blank" rel="noopener">Открыть на сайте</a>
        </div>
        <div class="table-wrap" style="border:0;border-radius:0;box-shadow:none">
            <table class="table--compact">
                <thead><tr><th>Услуга</th><th>Категория</th><th>Цена, ₽</th><th>Гарантия, мес.</th><th>Активна</th><th></th></tr></thead>
                <tbody>${items.map((s) => `
                    <tr data-price-row="${s.id}">
                        <td><strong>${escapeHtml(s.title)}</strong></td>
                        <td class="text-sm">${escapeHtml(s.category_title)}</td>
                        <td><input class="input" style="width:100px;padding:6px 8px" type="number" min="0" step="1" value="${Number(s.price).toFixed(0)}" data-price></td>
                        <td><input class="input" style="width:80px;padding:6px 8px" type="number" min="0" max="120" step="1" value="${s.warranty_months}" data-warranty></td>
                        <td><label class="toggle"><input type="checkbox" data-active ${s.is_active ? 'checked' : ''}><span class="toggle__track"></span></label></td>
                        <td><button class="btn btn-primary btn-sm" type="button" data-save-price="${s.id}">Сохранить</button></td>
                    </tr>`).join('')}</tbody>
            </table>
        </div>
    </section>`);

    document.querySelectorAll('[data-save-price]').forEach((button) => {
        button.addEventListener('click', async () => {
            const row = button.closest('[data-price-row]');
            const id = Number(button.dataset.savePrice);
            const price = Number(row.querySelector('[data-price]').value);
            const warranty = Number(row.querySelector('[data-warranty]').value);
            const active = row.querySelector('[data-active]').checked;
            setBusy(button, true);
            try {
                await api.patch(`/api/manager/price-list/${id}`, {
                    price, warranty_months: warranty, is_active: active,
                });
                toast('Цена обновлена', 'success');
            } catch (err) {
                toast(err.message || 'Не удалось обновить цену', 'error');
            } finally {
                setBusy(button, false);
            }
        });
    });
}

/* ------------------------------ Отзывы --------------------------------- */

async function renderReviews() {
    const [pendingData, approvedData] = await Promise.all([
        api.get('/api/reviews', { status: 'pending', limit: 50 }),
        api.get('/api/reviews', { status: 'approved', limit: 10 }),
    ]);
    const pending = pendingData.items || [];
    const approved = approvedData.items || [];
    setContent(`<section class="panel">
        <div class="panel__head"><div><h2>Модерация отзывов</h2><p>${pending.length} ожидают проверки · ${approvedData.total} опубликовано</p></div></div>
        <div class="panel__body">
            ${pending.length ? `<div class="request-list">${pending.map((r) => `
                <div class="card card--static">
                    <div class="review-header">
                        <div><strong>${escapeHtml(r.author_name)}</strong><div class="text-xs text-muted">${escapeHtml(r.device)} · ${dateTime(r.created_at)}</div></div>
                        ${reviewBadge(r.status)}
                    </div>
                    <p class="text-sm mb-2">${escapeHtml(r.text)}</p>
                    <div class="flex gap-1">
                        <button class="btn btn-success btn-sm" type="button" data-review="${r.id}" data-status="approved">Одобрить</button>
                        <button class="btn btn-ghost btn-sm" type="button" data-review="${r.id}" data-status="rejected">Отклонить</button>
                    </div>
                </div>`).join('')}</div>`
        : emptyState('Новых отзывов нет', 'Все отзывы проверены.')}
            <div class="detail-section"><h4>Последние опубликованные</h4>
                ${approved.length ? `<div class="request-list">${approved.map((r) => `
                    <div class="flex-between" style="padding:8px 0;border-bottom:1px solid var(--border)">
                        <div><strong>${escapeHtml(r.author_name)}</strong> ${stars(r.rating)}<div class="text-xs text-muted">${escapeHtml(r.device)}</div></div>
                        <span class="text-xs text-muted">${date(r.created_at)}</span>
                    </div>`).join('')}</div>` : '<p class="text-sm text-muted">Пока нет опубликованных отзывов</p>'}
            </div>
        </div>
    </section>`);

    document.querySelectorAll('[data-review]').forEach((button) => {
        button.addEventListener('click', async () => {
            setBusy(button, true);
            try {
                await api.patch(`/api/reviews/${button.dataset.review}/status`, { status: button.dataset.status });
                toast(button.dataset.status === 'approved' ? 'Отзыв опубликован' : 'Отзыв отклонён', 'success');
                await route();
            } catch (err) {
                toast(err.message || 'Не удалось обновить отзыв', 'error');
                setBusy(button, false);
            }
        });
    });
}

/* --------------------------- Пользователи ------------------------------- */

async function renderUsers() {
    setContent(`<section class="panel">
        <div class="panel__head">
            <div><h2>Пользователи и права</h2><p id="usersSummary">Загружаем…</p></div>
            <div class="toolbar">
                <select id="usersRole" aria-label="Фильтр по роли">
                    <option value="">Все роли</option>
                    <option value="client">Клиенты</option>
                    <option value="master">Мастера</option>
                    <option value="manager">Менеджеры</option>
                    <option value="admin">Администраторы</option>
                </select>
                <div class="search-box">
                    <input type="search" id="usersSearch" placeholder="Имя, email, телефон…" value="${escapeHtml(state.usersQuery)}">
                </div>
                <button class="btn btn-primary btn-sm" type="button" id="createStaffBtn">＋ Сотрудник</button>
            </div>
        </div>
        <div class="table-wrap" style="border:0;border-radius:0;box-shadow:none">
            <table class="table--compact">
                <thead><tr><th>Сотрудник</th><th>Роль</th><th>Права</th><th>Статус</th><th>Заявки</th><th>Действия</th></tr></thead>
                <tbody id="usersRows"><tr><td colspan="6"><div class="skeleton" style="height:22px"></div></td></tr></tbody>
            </table>
        </div>
    </section>`);

    document.getElementById('usersRole').value = state.usersRole;
    document.getElementById('usersRole').addEventListener('change', (event) => {
        state.usersRole = event.target.value;
        loadUsersRows();
    });
    const search = document.getElementById('usersSearch');
    search.addEventListener('input', debounce(() => {
        state.usersQuery = search.value.trim();
        loadUsersRows();
    }, 350));
    document.getElementById('createStaffBtn').addEventListener('click', openCreateStaffModal);
    await loadUsersRows();
}

async function loadUsersRows() {
    const tbody = document.getElementById('usersRows');
    if (!tbody) return;
    const params = {};
    if (state.usersRole) params.role = state.usersRole;
    if (state.usersQuery) params.q = state.usersQuery;
    try {
        const data = await api.get('/api/admin/users', params);
        const items = data.items || [];
        document.getElementById('usersSummary').textContent = `${data.total} пользователей`;
        if (!items.length) {
            tbody.innerHTML = `<tr><td colspan="6">${emptyState('Пользователи не найдены')}</td></tr>`;
            return;
        }
        const isAdminRole = state.user.role === 'admin';
        tbody.innerHTML = items.map((u) => `
            <tr>
                <td><div class="flex-center"><span class="avatar">${initials(u.full_name)}</span><div><strong>${escapeHtml(u.full_name)}</strong><div class="text-xs text-muted">${escapeHtml(u.email || u.phone || '')}</div></div></div></td>
                <td><span class="badge badge--soft">${escapeHtml(roleLabel(u.role))}</span></td>
                <td class="text-xs">${u.role === 'admin' ? 'Все права' : (u.permissions || []).map((p) => escapeHtml(PERMISSION_LABELS[p] || p)).join(', ') || '—'}</td>
                <td>${u.is_active ? '<span class="badge badge--ready">Активен</span>' : '<span class="badge badge--cancelled">Заблокирован</span>'}</td>
                <td>${u.active_requests || 0}</td>
                <td><div class="admin-table-actions">
                    <button class="btn btn-outline btn-sm" type="button" data-perms="${u.id}" data-name="${escapeHtml(u.full_name)}">Права</button>
                    ${u.id !== state.user.id ? `<button class="btn btn-ghost btn-sm" type="button" data-toggle-user="${u.id}" data-active="${u.is_active}">${u.is_active ? 'Заблокировать' : 'Активировать'}</button>
                    <button class="btn btn-ghost btn-sm text-danger" type="button" data-delete-user="${u.id}">Удалить</button>` : '<span class="text-xs text-muted">это вы</span>'}
                </div></td>
            </tr>`).join('');
        bindUserActions(tbody, isAdminRole);
    } catch (err) {
        tbody.innerHTML = `<tr><td colspan="6">${emptyState('Ошибка загрузки', err.message)}</td></tr>`;
    }
}

function bindUserActions(scope) {
    scope.querySelectorAll('[data-perms]').forEach((button) => {
        button.addEventListener('click', () => openPermissionsModal(Number(button.dataset.perms), button.dataset.name));
    });
    scope.querySelectorAll('[data-toggle-user]').forEach((button) => {
        button.addEventListener('click', async () => {
            try {
                await api.patch(`/api/admin/users/${button.dataset.toggleUser}`, {
                    is_active: button.dataset.active !== 'true',
                });
                toast('Статус пользователя обновлён', 'success');
                await loadUsersRows();
            } catch (err) {
                toast(err.message || 'Не удалось обновить пользователя', 'error');
            }
        });
    });
    scope.querySelectorAll('[data-delete-user]').forEach((button) => {
        button.addEventListener('click', async () => {
            const ok = await confirmDialog({
                title: 'Удалить учётную запись?',
                text: 'Пользователь будет переведён в архив и потеряет доступ к системе.',
                confirmLabel: 'Удалить',
                danger: true,
            });
            if (!ok) return;
            try {
                await api.del(`/api/admin/users/${button.dataset.deleteUser}`);
                toast('Учётная запись переведена в архив', 'success');
                await loadUsersRows();
            } catch (err) {
                toast(err.message || 'Не удалось удалить пользователя', 'error');
            }
        });
    });
}

async function openPermissionsModal(userId, name) {
    let data;
    try {
        data = await api.get(`/api/admin/users/${userId}/permissions`);
    } catch (err) {
        toast(err.message || 'Не удалось загрузить права', 'error');
        return;
    }
    const modal = openModal({
        title: 'Права доступа',
        subtitle: name,
        size: 'lg',
        body: `<form id="permForm" novalidate>
            <div class="form-error"></div>
            ${data.is_admin ? '<p class="form-hint mb-2">Администратор обладает всеми правами. Изменения флагов не ограничат доступ.</p>' : ''}
            <div class="perm-grid">
                ${(data.permissions || []).map((p) => `
                    <label class="perm-item">
                        <input type="checkbox" name="permissions" value="${escapeHtml(p.key)}" ${p.enabled ? 'checked' : ''}>
                        <div><strong>${escapeHtml(p.label)}</strong><span>${escapeHtml(p.description)}</span></div>
                    </label>`).join('')}
            </div>
        </form>`,
        footer: '<button class="btn btn-outline" type="button" data-cancel>Отмена</button><button class="btn btn-primary" type="submit" form="permForm" data-save>Сохранить</button>',
    });
    modal.foot.querySelector('[data-cancel]').addEventListener('click', () => modal.close());
    const form = modal.body.querySelector('#permForm');
    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        const button = modal.foot.querySelector('[data-save]');
        setBusy(button, true);
        try {
            const permissions = [...form.querySelectorAll('input[name="permissions"]:checked')].map((i) => i.value);
            await api.put(`/api/admin/users/${userId}/permissions`, { permissions });
            modal.close();
            toast('Права успешно обновлены', 'success');
            await loadUsersRows();
        } catch (err) {
            setBusy(button, false);
            if (err.fields) showFormErrors(form, err.fields, err.message);
            else showFormErrors(form, {}, err.message || 'Не удалось сохранить права');
        }
    });
}

async function openCreateStaffModal() {
    if (!state.permissionCatalog) {
        try {
            const data = await api.get('/api/admin/permissions');
            state.permissionCatalog = data.permissions || [];
        } catch (err) {
            toast(err.message || 'Не удалось загрузить список прав', 'error');
            return;
        }
    }
    const modal = openModal({
        title: 'Новый сотрудник',
        subtitle: 'Создайте учётную запись и выдайте права доступа',
        size: 'lg',
        body: `<form id="staffForm" novalidate>
            <div class="form-error"></div>
            <div class="form-row">
                <div class="form-group">
                    <label for="staffName">ФИО</label>
                    <input id="staffName" name="full_name" type="text" required placeholder="Иван Петров">
                    <div class="field-error" data-for="full_name"></div>
                </div>
                <div class="form-group">
                    <label for="staffEmail">Рабочий email</label>
                    <input id="staffEmail" name="email" type="email" required placeholder="master4@pro-remont.ru">
                    <div class="field-error" data-for="email"></div>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label for="staffRole">Роль</label>
                    <select id="staffRole" name="role">
                        <option value="master">Мастер</option>
                        <option value="manager">Менеджер</option>
                        <option value="admin">Администратор</option>
                    </select>
                    <div class="field-error" data-for="role"></div>
                </div>
                <div class="form-group">
                    <label for="staffPhone">Телефон</label>
                    <input id="staffPhone" name="phone" type="tel" placeholder="+7 (___) ___-__-__">
                    <div class="field-error" data-for="phone"></div>
                </div>
            </div>
            <div id="staffMasterFields">
                <div class="form-row">
                    <div class="form-group">
                        <label for="staffSpec">Специализация</label>
                        <input id="staffSpec" name="specialization" type="text" placeholder="Мастер по холодильному оборудованию">
                    </div>
                    <div class="form-group">
                        <label for="staffExp">Стаж, лет</label>
                        <input id="staffExp" name="experience_years" type="number" min="0" max="60" value="5">
                    </div>
                </div>
            </div>
            <div class="form-group">
                <label for="staffPassword">Пароль <span class="text-muted">(оставьте пустым — сгенерируем)</span></label>
                <input id="staffPassword" name="password" type="text" minlength="6" placeholder="Минимум 6 символов">
                <div class="field-error" data-for="password"></div>
            </div>
            <div class="form-group">
                <label>Права доступа</label>
                <div class="perm-grid" id="staffPerms">
                    ${state.permissionCatalog.map((p) => `
                        <label class="perm-item">
                            <input type="checkbox" name="permissions" value="${escapeHtml(p.key)}">
                            <div><strong>${escapeHtml(p.label)}</strong><span>${escapeHtml(p.description)}</span></div>
                        </label>`).join('')}
                </div>
            </div>
        </form>`,
        footer: '<button class="btn btn-outline" type="button" data-cancel>Отмена</button><button class="btn btn-primary" type="submit" form="staffForm" data-save>Создать</button>',
    });
    modal.foot.querySelector('[data-cancel]').addEventListener('click', () => modal.close());

    const form = modal.body.querySelector('#staffForm');
    const roleSelect = form.elements.role;
    const roleDefaults = {
        manager: ['manage_requests', 'assign_masters', 'manage_prices'],
        admin: [],
        master: [],
    };
    roleSelect.addEventListener('change', () => {
        form.querySelector('#staffMasterFields').classList.toggle('hidden', roleSelect.value !== 'master');
        form.querySelectorAll('input[name="permissions"]').forEach((input) => {
            input.checked = (roleDefaults[roleSelect.value] || []).includes(input.value);
        });
    });
    form.querySelector('#staffMasterFields').classList.toggle('hidden', roleSelect.value !== 'master');
    form.querySelectorAll('input[name="permissions"]').forEach((input) => {
        input.checked = roleDefaults[roleSelect.value].includes(input.value);
    });

    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        clearFormErrors(form);
        const button = modal.foot.querySelector('[data-save]');
        setBusy(button, true);
        const permissions = [...form.querySelectorAll('input[name="permissions"]:checked')].map((i) => i.value);
        const payload = {
            full_name: form.elements.full_name.value.trim(),
            email: form.elements.email.value.trim(),
            phone: form.elements.phone.value.trim(),
            role: form.elements.role.value,
            password: form.elements.password.value,
            permissions,
            specialization: form.elements.specialization?.value.trim() || '',
            experience_years: Number(form.elements.experience_years?.value || 0),
        };
        try {
            const data = await api.post('/api/admin/users', payload);
            modal.close();
            await loadUsersRows();
            if (data.temp_password) {
                showCredentials(payload.email, data.temp_password, data.user?.full_name || payload.full_name);
            } else {
                toast('Сотрудник создан', 'success');
            }
        } catch (err) {
            setBusy(button, false);
            if (err.fields) showFormErrors(form, err.fields, err.message);
            else showFormErrors(form, {}, err.message || 'Не удалось создать сотрудника');
        }
    });
}

function showCredentials(email, password, name) {
    const modal = openModal({
        title: 'Учётная запись создана',
        subtitle: name,
        size: 'sm',
        body: `
            <p class="text-sm mb-2">Передайте сотруднику данные для входа. Пароль показывается один раз.</p>
            <div class="pay-summary">
                <div class="pay-summary__row"><span>Email</span><strong>${escapeHtml(email)}</strong></div>
                <div class="pay-summary__row"><span>Временный пароль</span><code style="font-weight:800">${escapeHtml(password)}</code></div>
            </div>
            <button class="btn btn-outline btn-block" type="button" data-copy-pass>Скопировать пароль</button>`,
        footer: '<button class="btn btn-primary" type="button" data-close>Готово</button>',
    });
    modal.foot.querySelector('[data-close]').addEventListener('click', () => modal.close());
    modal.body.querySelector('[data-copy-pass]').addEventListener('click', async () => {
        try {
            await navigator.clipboard.writeText(password);
            toast('Пароль скопирован', 'success');
        } catch {
            toast(`Пароль: ${password}`, 'info');
        }
    });
}

/* ------------------------------- CMS ----------------------------------- */

async function renderTheme() {
    const data = await api.get('/api/admin/theme');
    const draft = data.draft || {};
    const published = data.published || {};
    setContent(`<section class="panel">
        <div class="panel__head">
            <div><h2>Оформление сайта</h2><p>Черновик и публикация темы · ${data.published_at ? `опубликовано ${dateTime(data.published_at)}` : 'ещё не публиковалось'}</p></div>
        </div>
        <div class="panel__body">
            <div class="theme-preview" id="themePreview">
                <div class="theme-preview__bar"></div>
                <h4 id="previewTitle">${escapeHtml(draft.hero_title || '')}</h4>
                <p id="previewSubtitle">${escapeHtml(draft.hero_subtitle || '')}</p>
            </div>
            <form id="themeForm" novalidate>
                <div class="form-error"></div>
                <div class="form-row">
                    <div class="form-group">
                        <label>Основной цвет</label>
                        <div class="color-row">
                            <input type="color" id="primaryPicker" value="${escapeHtml(draft.primary || '#E30613')}" aria-label="Основной цвет">
                            <input type="text" name="primary" value="${escapeHtml(draft.primary || '#E30613')}" maxlength="7">
                        </div>
                        <div class="field-error" data-for="primary"></div>
                    </div>
                    <div class="form-group">
                        <label>Акцентный цвет</label>
                        <div class="color-row">
                            <input type="color" id="accentPicker" value="${escapeHtml(draft.accent || '#0066CC')}" aria-label="Акцентный цвет">
                            <input type="text" name="accent" value="${escapeHtml(draft.accent || '#0066CC')}" maxlength="7">
                        </div>
                        <div class="field-error" data-for="accent"></div>
                    </div>
                </div>
                <div class="form-group">
                    <label for="heroTitle">Заголовок на главной</label>
                    <input id="heroTitle" name="hero_title" type="text" value="${escapeHtml(draft.hero_title || '')}" maxlength="200">
                    <div class="field-error" data-for="hero_title"></div>
                </div>
                <div class="form-group">
                    <label for="heroSubtitle">Подзаголовок на главной</label>
                    <textarea id="heroSubtitle" name="hero_subtitle" rows="3" maxlength="400">${escapeHtml(draft.hero_subtitle || '')}</textarea>
                    <div class="field-error" data-for="hero_subtitle"></div>
                </div>
                <div class="form-group">
                    <label for="footerNote">Текст в подвале</label>
                    <input id="footerNote" name="footer_note" type="text" value="${escapeHtml(draft.footer_note || '')}" maxlength="300">
                    <div class="field-error" data-for="footer_note"></div>
                </div>
                <div class="flex gap-1" style="flex-wrap:wrap">
                    <button class="btn btn-outline" type="button" id="saveTheme">Сохранить черновик</button>
                    <button class="btn btn-primary" type="button" id="publishTheme">Опубликовать изменения</button>
                    <span class="text-xs text-muted" style="align-self:center">Текущая версия: ${escapeHtml(published.primary || '—')} / ${escapeHtml(published.accent || '—')}</span>
                </div>
            </form>
        </div>
    </section>`);

    const form = document.getElementById('themeForm');
    const preview = document.getElementById('themePreview');
    const sync = () => {
        form.elements.primary.value = form.elements.primary.value.toUpperCase();
        form.elements.accent.value = form.elements.accent.value.toUpperCase();
        preview.style.setProperty('--preview-primary', form.elements.primary.value);
        document.getElementById('previewTitle').textContent = form.elements.hero_title.value;
        document.getElementById('previewSubtitle').textContent = form.elements.hero_subtitle.value;
    };
    form.elements.primary.addEventListener('input', sync);
    form.elements.accent.addEventListener('input', sync);
    form.elements.hero_title.addEventListener('input', sync);
    form.elements.hero_subtitle.addEventListener('input', sync);
    document.getElementById('primaryPicker').addEventListener('input', (event) => {
        form.elements.primary.value = event.target.value.toUpperCase();
        sync();
    });
    document.getElementById('accentPicker').addEventListener('input', (event) => {
        form.elements.accent.value = event.target.value.toUpperCase();
        sync();
    });
    sync();

    document.getElementById('saveTheme').addEventListener('click', async (event) => {
        const button = event.currentTarget;
        setBusy(button, true);
        try {
            await api.put('/api/admin/theme/draft', themePayload(form));
            toast('Черновик оформления сохранён', 'success');
        } catch (err) {
            if (err.fields) showFormErrors(form, err.fields, err.message);
            else toast(err.message || 'Не удалось сохранить черновик', 'error');
        } finally {
            setBusy(button, false);
        }
    });
    document.getElementById('publishTheme').addEventListener('click', async (event) => {
        const ok = await confirmDialog({
            title: 'Опубликовать оформление?',
            text: 'Изменения станут видны всем посетителям сайта после обновления страницы.',
            confirmLabel: 'Опубликовать',
        });
        if (!ok) return;
        const button = event.currentTarget;
        setBusy(button, true);
        try {
            await api.put('/api/admin/theme/draft', themePayload(form));
            await api.post('/api/admin/theme/publish');
            toast('Тема опубликована. Обновите страницу сайта.', 'success', 6000);
        } catch (err) {
            toast(err.message || 'Не удалось опубликовать тему', 'error');
        } finally {
            setBusy(button, false);
        }
    });
}

function themePayload(form) {
    return {
        primary: form.elements.primary.value.trim(),
        accent: form.elements.accent.value.trim(),
        hero_title: form.elements.hero_title.value.trim(),
        hero_subtitle: form.elements.hero_subtitle.value.trim(),
        footer_note: form.elements.footer_note.value.trim(),
    };
}

/* --------------------------- Уведомления -------------------------------- */

async function renderNotifications() {
    const data = await api.get('/api/notifications');
    const items = data.items || [];
    setContent(`<section class="panel">
        <div class="panel__head">
            <div><h2>Уведомления</h2><p>${data.unread} непрочитанных</p></div>
            <button class="btn btn-outline btn-sm" type="button" id="readAllNotifs">Прочитать все</button>
        </div>
        <div class="panel__body">
            ${items.length ? `<div class="request-list">${items.map((n) => `
                <div class="card card--static ${n.is_read ? '' : ''}" data-notification="${n.id}">
                    <div class="flex-between">
                        <strong>${escapeHtml(n.title)}</strong>
                        <span class="text-xs text-muted">${relativeTime(n.created_at)}</span>
                    </div>
                    <p class="text-sm mt-1">${escapeHtml(n.message)}</p>
                    <div class="flex gap-1 mt-2">
                        ${n.request_id ? `<button class="btn btn-outline btn-sm" type="button" data-open-request="${n.request_id}">Открыть заявку</button>` : ''}
                        ${n.is_read ? '' : `<button class="btn btn-ghost btn-sm" type="button" data-read-notification="${n.id}">Отметить прочитанным</button>`}
                    </div>
                </div>`).join('')}</div>` : emptyState('Уведомлений нет', 'Здесь появятся сообщения о статусах заявок и оплатах.')}
        </div>
    </section>`);

    document.getElementById('readAllNotifs')?.addEventListener('click', async () => {
        try {
            await api.post('/api/notifications/read-all');
            toast('Все уведомления прочитаны', 'success');
            await route();
        } catch (err) {
            toast(err.message || 'Не удалось обновить уведомления', 'error');
        }
    });
    document.querySelectorAll('[data-read-notification]').forEach((button) => {
        button.addEventListener('click', async () => {
            try {
                await api.post(`/api/notifications/${button.dataset.readNotification}/read`);
                await route();
            } catch (err) {
                toast(err.message || 'Не удалось обновить уведомление', 'error');
            }
        });
    });
    document.querySelectorAll('[data-open-request]').forEach((button) => {
        button.addEventListener('click', () => openRequestDetail(Number(button.dataset.openRequest)));
    });
}

/* --------------------------- URL-параметры ------------------------------ */

function openRequestFromQuery() {
    const url = new URL(window.location.href);
    const requestId = Number(url.searchParams.get('request'));
    if (!requestId) return;
    url.searchParams.delete('request');
    history.replaceState(null, '', `${url.pathname}${url.search}${url.hash}`);
    setTimeout(() => openRequestDetail(requestId), 350);
}

function truncate(value, max) {
    const text = String(value || '');
    if (text.length <= max) return text;
    return `${text.slice(0, max - 1)}…`;
}

function dateShort(value) {
    if (!value) return '—';
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString('ru-RU');
}

main();
