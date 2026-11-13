import { api } from '../api.js';
import { initLayout } from '../app.js';
import { escapeHtml, money, emptyState, toast } from '../ui.js';

const CATEGORY_ICONS = {
    washer: '🧺',
    fridge: '❄️',
    dishwasher: '🍽️',
    stove: '🔥',
    coffee: '☕',
    tv: '📺',
};

async function main() {
    await initLayout({ active: 'home' });
    loadCategories();
    loadPopularServices();
}

async function loadCategories() {
    const grid = document.getElementById('categoriesGrid');
    if (!grid) return;
    try {
        const data = await api.get('/api/catalog/categories');
        const items = data.categories || [];
        if (!items.length) {
            grid.innerHTML = emptyState('Категории пока не добавлены');
            return;
        }
        grid.innerHTML = items.map((c) => `
            <a class="card service-card" href="prices.html?category=${encodeURIComponent(c.slug)}" style="text-decoration:none">
                <div class="service-card__icon">${CATEGORY_ICONS[c.slug] || '🔧'}</div>
                <h3>${escapeHtml(c.title)}</h3>
                <p>${escapeHtml(c.description)}</p>
                <div class="flex-between" style="margin-top:auto;padding-top:14px">
                    <span class="service-card__meta">${c.services} услуг в прайсе</span>
                    <span class="service-card__price">от ${money(c.base_price)}</span>
                </div>
            </a>`).join('');
    } catch (err) {
        grid.innerHTML = emptyState('Не удалось загрузить каталог', err.message);
        toast(err.message || 'Ошибка загрузки каталога', 'error');
    }
}

async function loadPopularServices() {
    const grid = document.getElementById('popularServices');
    if (!grid) return;
    try {
        const data = await api.get('/api/catalog/items');
        const seen = new Set();
        const list = (data.items || []).filter((s) => {
            if (seen.has(s.category_slug)) return false;
            seen.add(s.category_slug);
            return true;
        }).slice(0, 6);
        if (!list.length) {
            grid.innerHTML = emptyState('Прайс-лист пока пуст');
            return;
        }
        grid.innerHTML = list.map((s) => `
            <a class="card service-card" href="contacts.html?service=${s.id}&category=${encodeURIComponent(s.category_slug)}" style="text-decoration:none">
                <div class="flex-between">
                    <span class="badge badge--soft">${escapeHtml(s.category_title)}</span>
                    <span class="text-xs text-muted">Гарантия ${s.warranty_months} мес.</span>
                </div>
                <h3 style="margin-top:10px">${escapeHtml(s.title)}</h3>
                <p>${escapeHtml(s.description)}</p>
                <div class="flex-between" style="margin-top:auto;padding-top:14px">
                    <span class="service-card__price">${money(s.price)}</span>
                    <span class="text-xs text-muted">Оформить →</span>
                </div>
            </a>`).join('');
    } catch (err) {
        grid.innerHTML = emptyState('Не удалось загрузить услуги', err.message);
    }
}

main();
