import { api } from '../api.js';
import { initLayout } from '../app.js';
import { escapeHtml, initials, stars, emptyState, toast } from '../ui.js';

async function main() {
    await initLayout({ active: 'masters' });
    const grid = document.getElementById('mastersGrid');
    try {
        const data = await api.get('/api/masters');
        const items = data.items || [];
        document.getElementById('mastersCount').textContent = `✓ ${items.length} сертифицированных специалистов`;
        if (!items.length) {
            grid.innerHTML = emptyState('Мастера пока не добавлены');
            return;
        }
        grid.innerHTML = items.map((m) => `
            <div class="card master-card">
                <div class="master-avatar">${initials(m.full_name)}</div>
                <h3>${escapeHtml(m.full_name)}</h3>
                <div class="master-spec">${escapeHtml(m.specialization)}</div>
                <p>${escapeHtml(m.bio)}</p>
                <div class="master-stats">
                    <div class="master-stat"><strong>${m.experience_years} лет</strong><span>стаж</span></div>
                    <div class="master-stat"><strong>${Number(m.rating).toFixed(1)}</strong><span>рейтинг ${stars(m.rating)}</span></div>
                    <div class="master-stat"><strong>${m.completed_requests}</strong><span>ремонтов</span></div>
                </div>
                <div class="mt-2">
                    ${m.is_available
        ? '<span class="badge badge--ready">Свободен для выезда</span>'
        : '<span class="badge badge--in_progress">На заявке</span>'}
                </div>
                <div class="mt-2">
                    <a href="contacts.html" class="btn btn-outline btn-sm">Вызвать мастера</a>
                </div>
            </div>`).join('');
    } catch (err) {
        grid.innerHTML = emptyState('Не удалось загрузить мастеров', err.message);
        toast(err.message || 'Ошибка загрузки мастеров', 'error');
    }
}

main();
