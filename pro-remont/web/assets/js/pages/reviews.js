import { api } from '../api.js';
import { initLayout } from '../app.js';
import {
    escapeHtml, date, stars, emptyState, toast, setBusy, clearFormErrors, showFormErrors,
} from '../ui.js';

let user = null;
let requests = [];

async function main() {
    user = await initLayout({ active: 'reviews' });
    loadReviews();
    renderReviewForm();
}

async function loadReviews() {
    const list = document.getElementById('reviewsList');
    const summary = document.getElementById('ratingSummary');
    try {
        const data = await api.get('/api/reviews', { status: 'approved', limit: 50 });
        const items = data.items || [];
        summary.innerHTML = `
            <div class="rating-summary">
                <div>
                    <div class="rating-summary__value">${Number(data.average || 0).toFixed(1)}</div>
                    <div>${stars(data.average)}</div>
                </div>
                <div>
                    <div style="font-weight:700;color:var(--dark)">Средняя оценка сервиса</div>
                    <div class="rating-summary__meta">На основе ${data.approved || items.length} одобренных отзывов</div>
                </div>
            </div>`;
        if (!items.length) {
            list.innerHTML = emptyState('Отзывов пока нет', 'Станьте первым клиентом, кто поделится впечатлениями.');
            return;
        }
        list.innerHTML = items.map((r) => `
            <div class="card card--static review-card">
                <div class="review-header">
                    <div>
                        <strong>${escapeHtml(r.author_name)}</strong>
                        <div class="text-xs text-muted">${escapeHtml(r.device || 'Ремонт бытовой техники')}</div>
                    </div>
                    <div style="text-align:right">
                        ${stars(r.rating)}
                        <div class="review-date">${date(r.created_at)}</div>
                    </div>
                </div>
                <p>${escapeHtml(r.text)}</p>
            </div>`).join('');
    } catch (err) {
        list.innerHTML = emptyState('Не удалось загрузить отзывы', err.message);
        summary.innerHTML = '';
        toast(err.message || 'Ошибка загрузки отзывов', 'error');
    }
}

async function renderReviewForm() {
    const box = document.getElementById('reviewFormBody');
    if (!box) return;

    if (!user) {
        box.innerHTML = `
            <p class="text-sm mb-2">Войдите в личный кабинет, чтобы оставить отзыв о работе мастера.</p>
            <button type="button" class="btn btn-primary btn-block" data-auth="login">Войти в личный кабинет</button>`;
        return;
    }
    if (user.role !== 'client') {
        box.innerHTML = '<p class="text-sm">Модерация отзывов доступна в личном кабинете сотрудника.</p><a class="btn btn-outline btn-block mt-2" href="account.html#reviews">Перейти в кабинет</a>';
        return;
    }

    try {
        const [hist, mine] = await Promise.all([
            api.get('/api/requests/my-history'),
            api.get('/api/reviews/my'),
        ]);
        const reviewed = new Set((mine.items || []).map((r) => r.request_id).filter(Boolean));
        requests = (hist.items || []).filter((r) => (r.status === 'paid' || r.status === 'completed') && !reviewed.has(r.id));
    } catch {
        requests = [];
    }

    if (!requests.length) {
        box.innerHTML = `
            <p class="text-sm">У вас нет завершённых заявок без отзыва. После оплаты ремонта форма появится здесь.</p>
            <a class="btn btn-outline btn-block mt-2" href="account.html#requests">Мои заявки</a>`;
        return;
    }

    const preselected = new URLSearchParams(window.location.search).get('request');
    box.innerHTML = `
        <form id="reviewForm" novalidate>
            <div class="form-error"></div>
            <div class="form-group">
                <label for="reviewRequest">Заявка</label>
                <select id="reviewRequest" name="request_id">
                    ${requests.map((r) => `<option value="${r.id}" ${String(r.id) === preselected ? 'selected' : ''}>${escapeHtml(r.public_number)} — ${escapeHtml(r.service_title || r.equipment_type)}</option>`).join('')}
                </select>
                <div class="field-error" data-for="request_id"></div>
            </div>
            <div class="form-group">
                <label>Оценка</label>
                <div class="rating-input" id="ratingInput" role="group" aria-label="Оценка от 1 до 5">
                    ${[1, 2, 3, 4, 5].map((n) => `<button type="button" class="rating-star ${n <= 5 ? 'active' : ''}" data-rating="${n}" aria-label="${n} из 5">★</button>`).join('')}
                    <input type="hidden" name="rating" value="5">
                    <span class="rating-input__label" id="ratingLabel">5 из 5</span>
                </div>
            </div>
            <div class="form-group">
                <label for="reviewText">Ваш отзыв</label>
                <textarea id="reviewText" name="text" rows="4" minlength="10" maxlength="4000" required placeholder="Расскажите, что понравилось, как быстро приехал мастер и качество ремонта…"></textarea>
                <div class="field-error" data-for="text"></div>
            </div>
            <button type="submit" class="btn btn-primary btn-block">Отправить отзыв</button>
            <p class="form-hint">Публикуется после модерации. Не указывайте персональные данные.</p>
        </form>`;

    const ratingInput = box.querySelector('#ratingInput');
    ratingInput.querySelectorAll('[data-rating]').forEach((button) => {
        button.addEventListener('click', () => {
            const value = Number(button.dataset.rating);
            ratingInput.querySelector('input[name="rating"]').value = String(value);
            ratingInput.querySelectorAll('[data-rating]').forEach((b) => {
                b.classList.toggle('active', Number(b.dataset.rating) <= value);
            });
            box.querySelector('#ratingLabel').textContent = `${value} из 5`;
        });
    });
    // Все звёзды до выбранной подсвечиваются.
    ratingInput.querySelectorAll('[data-rating]').forEach((b) => {
        b.classList.toggle('active', Number(b.dataset.rating) <= 5);
    });

    const form = box.querySelector('#reviewForm');
    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        clearFormErrors(form);
        const text = form.elements.text.value.trim();
        if (text.length < 10) {
            showFormErrors(form, { text: 'Отзыв должен быть не короче 10 символов' });
            return;
        }
        const button = form.querySelector('[type="submit"]');
        setBusy(button, true);
        try {
            await api.post('/api/reviews', {
                request_id: Number(form.elements.request_id.value),
                rating: Number(form.elements.rating.value),
                text,
            });
            toast('Спасибо! Отзыв отправлен на модерацию.', 'success');
            form.reset();
            renderReviewForm();
        } catch (err) {
            if (err.fields) showFormErrors(form, err.fields, err.message);
            else showFormErrors(form, {}, err.message || 'Не удалось отправить отзыв');
            setBusy(button, false);
        }
    });
}

main();
