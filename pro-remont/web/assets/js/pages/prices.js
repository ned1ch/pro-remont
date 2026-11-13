import { api } from '../api.js';
import { initLayout } from '../app.js';
import { escapeHtml, money, emptyState, toast } from '../ui.js';

let categories = [];
let services = [];
let activeCategory = 'all';
let query = '';

async function main() {
    await initLayout({ active: 'prices' });
    activeCategory = new URLSearchParams(window.location.search).get('category') || 'all';

    try {
        const [catData, itemData] = await Promise.all([
            api.get('/api/catalog/categories'),
            api.get('/api/catalog/items'),
        ]);
        categories = catData.categories || [];
        services = itemData.items || [];
        if (!categories.some((c) => c.slug === activeCategory)) activeCategory = 'all';
        renderTabs();
        renderTable();
    } catch (err) {
        document.getElementById('priceCount').textContent = 'Не удалось загрузить прайс-лист';
        document.querySelector('#priceTable tbody').innerHTML = `<tr><td colspan="5">${emptyState('Ошибка загрузки', err.message)}</td></tr>`;
        toast(err.message || 'Ошибка загрузки прайс-листа', 'error');
    }

    document.getElementById('priceSearch')?.addEventListener('input', (event) => {
        query = event.target.value.trim().toLowerCase();
        renderTable();
    });
}

function renderTabs() {
    const tabs = document.getElementById('priceTabs');
    const all = [{ slug: 'all', title: 'Все услуги' }, ...categories];
    tabs.innerHTML = all.map((c) => `
        <button type="button" class="tab-btn ${c.slug === activeCategory ? 'active' : ''}" data-cat="${escapeHtml(c.slug)}">
            ${escapeHtml(c.title)}
        </button>`).join('');
    tabs.querySelectorAll('[data-cat]').forEach((button) => {
        button.addEventListener('click', () => {
            activeCategory = button.dataset.cat;
            tabs.querySelectorAll('[data-cat]').forEach((b) => b.classList.toggle('active', b === button));
            renderTable();
            const url = new URL(window.location.href);
            if (activeCategory === 'all') url.searchParams.delete('category');
            else url.searchParams.set('category', activeCategory);
            history.replaceState(null, '', url);
        });
    });
}

function filtered() {
    return services.filter((s) => {
        if (activeCategory !== 'all' && s.category_slug !== activeCategory) return false;
        if (!query) return true;
        const haystack = `${s.title} ${s.description} ${s.category_title}`.toLowerCase();
        return haystack.includes(query);
    });
}

function renderTable() {
    const tbody = document.querySelector('#priceTable tbody');
    const count = document.getElementById('priceCount');
    const items = filtered();
    count.textContent = `${items.length} ${plural(items.length)} в прайс-листе`;

    if (!items.length) {
        tbody.innerHTML = `<tr><td colspan="5">${emptyState('Ничего не найдено', 'Попробуйте изменить запрос или сбросить фильтр.', '<button type="button" class="btn btn-outline btn-sm" id="resetFilter">Сбросить фильтр</button>')}</td></tr>`;
        document.getElementById('resetFilter')?.addEventListener('click', () => {
            query = '';
            activeCategory = 'all';
            document.getElementById('priceSearch').value = '';
            renderTabs();
            renderTable();
        });
        return;
    }

    tbody.innerHTML = items.map((s) => `
        <tr>
            <td>
                <strong>${escapeHtml(s.title)}</strong>
                <div class="text-xs text-muted">${escapeHtml(s.description)}</div>
            </td>
            <td><span class="badge badge--soft">${escapeHtml(s.category_title)}</span></td>
            <td class="nowrap">${s.warranty_months ? `${s.warranty_months} мес.` : '—'}</td>
            <td><span class="price-val">${money(s.price)}</span></td>
            <td class="nowrap">
                <a class="btn btn-outline btn-sm" href="contacts.html?service=${s.id}&category=${encodeURIComponent(s.category_slug)}">Оформить</a>
            </td>
        </tr>`).join('');
}

function plural(n) {
    const abs = Math.abs(n) % 100;
    const last = abs % 10;
    if (abs > 10 && abs < 20) return 'услуг';
    if (last > 1 && last < 5) return 'услуги';
    if (last === 1) return 'услуга';
    return 'услуг';
}

main();
