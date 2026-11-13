/**
 * Общие UI-хелперы: форматирование, тосты, модальные окна, бейджи.
 */

export const STATUS = {
    new: { label: 'Новая', tone: 'new' },
    accepted: { label: 'Принята в обработку', tone: 'accepted' },
    assigned: { label: 'Мастер назначен', tone: 'assigned' },
    in_progress: { label: 'В работе', tone: 'in_progress' },
    ready: { label: 'Готово к выдаче', tone: 'ready' },
    completed: { label: 'Выполнена', tone: 'completed' },
    paid: { label: 'Оплачена', tone: 'paid' },
    cancelled: { label: 'Отменена', tone: 'cancelled' },
};

export const ROLE_LABELS = {
    client: 'Клиент',
    master: 'Мастер',
    manager: 'Менеджер',
    admin: 'Администратор',
};

export const INVOICE_STATUS = {
    pending: 'Ожидает оплаты',
    paid: 'Оплачен',
    failed: 'Отклонён',
    refunded: 'Возврат',
};

export const REVIEW_STATUS = {
    pending: 'На модерации',
    approved: 'Опубликован',
    rejected: 'Отклонён',
};

export function escapeHtml(value) {
    if (value === null || value === undefined) return '';
    return String(value)
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#039;');
}

export function escapeAttr(value) {
    return escapeHtml(value).replaceAll('`', '&#096;');
}

export function money(value) {
    const n = Number(value) || 0;
    return new Intl.NumberFormat('ru-RU', {
        style: 'currency',
        currency: 'RUB',
        maximumFractionDigits: Number.isInteger(n) ? 0 : 2,
    }).format(n);
}

export function num(value) {
    return new Intl.NumberFormat('ru-RU').format(Number(value) || 0);
}

export function date(value) {
    if (!value) return '—';
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return '—';
    return new Intl.DateTimeFormat('ru-RU', { day: '2-digit', month: 'short', year: 'numeric' }).format(d);
}

export function dateTime(value) {
    if (!value) return '—';
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return '—';
    return new Intl.DateTimeFormat('ru-RU', {
        day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit',
    }).format(d);
}

export function relativeTime(value) {
    if (!value) return '';
    const d = new Date(value).getTime();
    const diff = Date.now() - d;
    const min = Math.round(diff / 60000);
    if (min < 1) return 'только что';
    if (min < 60) return `${min} мин назад`;
    const hours = Math.round(min / 60);
    if (hours < 24) return `${hours} ч назад`;
    const days = Math.round(hours / 24);
    if (days < 7) return `${days} дн назад`;
    return date(value);
}

export function statusInfo(status) {
    return STATUS[status] || { label: status || 'Неизвестно', tone: 'cancelled' };
}

export function statusBadge(status) {
    const info = statusInfo(status);
    return `<span class="badge badge--${info.tone}">${escapeHtml(info.label)}</span>`;
}

export function invoiceBadge(status) {
    const tone = status === 'paid' ? 'paid' : status === 'failed' ? 'cancelled' : status === 'pending' ? 'pending' : 'soft';
    return `<span class="badge badge--${tone}">${escapeHtml(INVOICE_STATUS[status] || status)}</span>`;
}

export function reviewBadge(status) {
    const tone = status === 'approved' ? 'approved' : status === 'rejected' ? 'rejected' : 'pending';
    return `<span class="badge badge--${tone}">${escapeHtml(REVIEW_STATUS[status] || status)}</span>`;
}

export function roleLabel(role) {
    return ROLE_LABELS[role] || role;
}

export function initials(name) {
    const parts = String(name || '').trim().split(/\s+/).filter(Boolean);
    if (!parts.length) return '??';
    if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
    return (parts[0][0] + parts[1][0]).toUpperCase();
}

export function stars(rating, max = 5) {
    const r = Math.round(Number(rating) || 0);
    let out = '';
    for (let i = 1; i <= max; i += 1) out += i <= r ? '★' : '☆';
    return `<span class="stars" title="${r} из ${max}">${out}</span>`;
}

export function plural(n, one, few, many) {
    const abs = Math.abs(Number(n)) % 100;
    const last = abs % 10;
    if (abs > 10 && abs < 20) return many;
    if (last > 1 && last < 5) return few;
    if (last === 1) return one;
    return many;
}

let toastRoot = null;

export function toast(message, type = 'info', timeout = 4600) {
    if (!toastRoot) {
        toastRoot = document.getElementById('toast-root');
        if (!toastRoot) {
            toastRoot = document.createElement('div');
            toastRoot.id = 'toast-root';
            toastRoot.className = 'toast-root';
            toastRoot.setAttribute('aria-live', 'polite');
            document.body.appendChild(toastRoot);
        }
    }
    const el = document.createElement('div');
    el.className = `toast toast--${type}`;
    el.innerHTML = `<div class="toast__text">${escapeHtml(message)}</div>
        <button class="toast__close" type="button" aria-label="Закрыть">×</button>`;
    const close = () => {
        el.style.opacity = '0';
        el.style.transform = 'translateY(6px)';
        setTimeout(() => el.remove(), 180);
    };
    el.querySelector('.toast__close').addEventListener('click', close);
    toastRoot.appendChild(el);
    if (timeout > 0) setTimeout(close, timeout);
    return close;
}

/**
 * Открывает модальное окно.
 * @returns {{el: HTMLElement, close: Function, body: HTMLElement, foot: HTMLElement}}
 */
export function openModal({
    title = '',
    subtitle = '',
    body = '',
    footer = '',
    size = '',
    dismissible = true,
    onClose = null,
    className = '',
} = {}) {
    const overlay = document.createElement('div');
    overlay.className = 'modal open';
    overlay.innerHTML = `
        <div class="modal__dialog ${size ? `modal__dialog--${size}` : ''} ${className}" role="dialog" aria-modal="true">
            <div class="modal__head">
                <div>
                    <h3>${escapeHtml(title)}</h3>
                    ${subtitle ? `<p>${escapeHtml(subtitle)}</p>` : ''}
                </div>
                ${dismissible ? '<button class="modal-close" type="button" aria-label="Закрыть">×</button>' : ''}
            </div>
            <div class="modal__body"></div>
            <div class="modal__foot"></div>
        </div>`;
    const bodyEl = overlay.querySelector('.modal__body');
    const footEl = overlay.querySelector('.modal__foot');

    if (body instanceof HTMLElement) bodyEl.appendChild(body);
    else bodyEl.innerHTML = body || '';

    if (footer instanceof HTMLElement) footEl.appendChild(footer);
    else footEl.innerHTML = footer || '';

    if (!footEl.innerHTML.trim()) footEl.style.display = 'none';

    const previouslyFocused = document.activeElement;
    let closed = false;
    const close = () => {
        if (closed) return;
        closed = true;
        overlay.classList.remove('open');
        overlay.remove();
        document.body.style.overflow = '';
        if (previouslyFocused && previouslyFocused.focus) previouslyFocused.focus();
        if (onClose) onClose();
        document.removeEventListener('keydown', onKey);
    };
    const onKey = (event) => {
        if (event.key === 'Escape' && dismissible) close();
    };

    overlay.querySelector('.modal-close')?.addEventListener('click', close);
    overlay.addEventListener('mousedown', (event) => {
        if (event.target === overlay && dismissible) close();
    });
    document.addEventListener('keydown', onKey);
    document.body.style.overflow = 'hidden';
    document.body.appendChild(overlay);

    setTimeout(() => {
        const focusable = overlay.querySelector('input, select, textarea, button:not(.modal-close)');
        if (focusable) focusable.focus();
    }, 30);

    return { el: overlay, close, body: bodyEl, foot: footEl };
}

export function confirmDialog({ title = 'Подтвердите действие', text = '', confirmLabel = 'Подтвердить', danger = false } = {}) {
    return new Promise((resolve) => {
        const modal = openModal({
            title,
            body: `<p class="text-sm">${escapeHtml(text)}</p>`,
            size: 'sm',
            footer: `
                <button class="btn btn-outline" data-cancel type="button">Отмена</button>
                <button class="btn ${danger ? 'btn-danger' : ''}" data-confirm type="button">${escapeHtml(confirmLabel)}</button>`,
            onClose: () => resolve(false),
        });
        modal.foot.querySelector('[data-cancel]').addEventListener('click', () => {
            modal.close();
            resolve(false);
        });
        modal.foot.querySelector('[data-confirm]').addEventListener('click', () => {
            modal.close();
            resolve(true);
        });
    });
}

export function setBusy(button, busy) {
    if (!button) return;
    if (busy) {
        button.dataset.label = button.innerHTML;
        button.disabled = true;
        button.classList.add('is-busy');
        button.innerHTML = 'Подождите…';
    } else {
        button.disabled = false;
        button.classList.remove('is-busy');
        if (button.dataset.label) button.innerHTML = button.dataset.label;
    }
}

export function clearFormErrors(form) {
    if (!form) return;
    form.querySelectorAll('.field-error').forEach((el) => {
        el.classList.remove('show');
        el.textContent = '';
    });
    form.querySelectorAll('[aria-invalid="true"]').forEach((el) => el.removeAttribute('aria-invalid'));
    const general = form.querySelector('.form-error');
    if (general) {
        general.classList.remove('show');
        general.textContent = '';
    }
}

export function showFormErrors(form, fields = {}, message = '') {
    if (!form) return;
    clearFormErrors(form);
    let first = null;
    Object.entries(fields || {}).forEach(([name, text]) => {
        const input = form.querySelector(`[name="${CSS.escape(name)}"], [data-field="${CSS.escape(name)}"]`);
        if (!input) return;
        input.setAttribute('aria-invalid', 'true');
        let error = input.parentElement?.querySelector(`.field-error[data-for="${CSS.escape(name)}"]`);
        if (!error) {
            error = document.createElement('div');
            error.className = 'field-error';
            error.dataset.for = name;
            input.parentElement?.appendChild(error);
        }
        error.textContent = text;
        error.classList.add('show');
        if (!first) first = input;
    });
    if (message) {
        let box = form.querySelector('.form-error');
        if (!box) {
            box = document.createElement('div');
            box.className = 'form-error';
            form.prepend(box);
        }
        box.textContent = message;
        box.classList.add('show');
    }
    if (first) {
        first.focus();
        first.scrollIntoView({ block: 'center', behavior: 'smooth' });
    }
}

export function formValues(form) {
    return Object.fromEntries(new FormData(form).entries());
}

export function debounce(fn, delay = 400) {
    let timer = null;
    return (...args) => {
        clearTimeout(timer);
        timer = setTimeout(() => fn(...args), delay);
    };
}

export function emptyState(title, text = '', action = '') {
    return `<div class="empty-state">
        <div class="empty-state__icon">✦</div>
        <h3>${escapeHtml(title)}</h3>
        ${text ? `<p>${escapeHtml(text)}</p>` : ''}
        ${action}
    </div>`;
}

export function loadingBlock(text = 'Загружаем данные…') {
    return `<div class="empty-state"><div class="empty-state__icon">⏳</div><p>${escapeHtml(text)}</p></div>`;
}

export function icon(name, size = 18) {
    const paths = {
        bell: '<path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.7 21a2 2 0 0 1-3.4 0"/>',
        search: '<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>',
        user: '<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>',
        logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>',
        card: '<rect x="2" y="5" width="20" height="14" rx="2"/><path d="M2 10h20"/>',
        check: '<path d="M20 6 9 17l-5-5"/>',
        clock: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
        shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>',
        wrench: '<path d="M14.7 6.3a4 4 0 0 0-5.4 5.4L3 18l3 3 6.3-6.3a4 4 0 0 0 5.4-5.4l-2.3 2.3-2-2 2.3-2.3z"/>',
        plus: '<path d="M12 5v14M5 12h14"/>',
        trash: '<path d="M3 6h18"/><path d="M8 6V4h8v2"/><path d="m19 6-1 14H6L5 6"/><path d="M10 11v6M14 11v6"/>',
    };
    return `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] || ''}</svg>`;
}
