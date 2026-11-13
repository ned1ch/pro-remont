import { api, ApiError } from '../api.js';
import { initLayout } from '../app.js';
import {
    escapeHtml, money, toast, setBusy, clearFormErrors, showFormErrors,
    debounce, statusBadge, statusInfo,
} from '../ui.js';
import { openPayment } from '../payments.js';

let user = null;
let categories = [];
let services = [];
let currentRequest = null;

const params = new URLSearchParams(window.location.search);
const preCategory = params.get('category') || '';
const preService = params.get('service') || '';

async function main() {
    user = await initLayout({ active: 'contacts' });
    await Promise.all([loadCatalog(), prefill()]);
    applyQuerySelection();
    bindForm();
}

async function loadCatalog() {
    const categorySelect = document.getElementById('requestCategory');
    try {
        const [catData, itemData] = await Promise.all([
            api.get('/api/catalog/categories'),
            api.get('/api/catalog/items'),
        ]);
        categories = catData.categories || [];
        services = itemData.items || [];
        categorySelect.innerHTML = '<option value="">Выберите тип техники</option>' + categories.map((c) => `
            <option value="${c.id}">${escapeHtml(c.title)}</option>`).join('');
    } catch (err) {
        toast(err.message || 'Не удалось загрузить каталог услуг', 'error');
    }
}

function renderServices(categorySlug, selectedId = null) {
    const select = document.getElementById('requestService');
    const list = categorySlug ? services.filter((s) => s.category_slug === categorySlug) : services;
    select.innerHTML = '<option value="">Не выбрано — уточнит менеджер</option>' + list.map((s) => `
        <option value="${s.id}" ${selectedId && Number(selectedId) === s.id ? 'selected' : ''}>
            ${escapeHtml(s.title)} — от ${money(s.price)}
        </option>`).join('');
    updateServiceHint();
}

function updateServiceHint() {
    const select = document.getElementById('requestService');
    const hint = document.getElementById('serviceHint');
    const service = services.find((s) => s.id === Number(select.value));
    if (!service) {
        hint.textContent = '';
        return;
    }
    hint.textContent = `Стоимость работы: ${money(service.price)} · гарантия ${service.warranty_months} мес.`;
}

function applyQuerySelection() {
    const categorySelect = document.getElementById('requestCategory');
    const serviceSelect = document.getElementById('requestService');

    if (preService) {
        const service = services.find((s) => s.id === Number(preService));
        if (service) {
            const category = categories.find((c) => c.slug === service.category_slug);
            if (category) categorySelect.value = String(category.id);
            renderServices(service.category_slug, service.id);
            serviceSelect.value = String(service.id);
            updateServiceHint();
            return;
        }
    }
    if (preCategory) {
        const category = categories.find((c) => c.slug === preCategory);
        if (category) {
            categorySelect.value = String(category.id);
            renderServices(category.slug);
        }
    }
}

async function prefill() {
    const form = document.getElementById('requestForm');
    if (!form) return;
    if (user) {
        form.elements.customer_name.value = user.full_name || '';
        form.elements.phone.value = user.phone || '';
        form.elements.email.value = user.email || '';
        form.elements.address.value = user.address || '';
        return;
    }
    try {
        const data = await api.get('/api/session/requisites');
        if (data.saved && data.requisites) {
            form.elements.customer_name.value = data.requisites.customer_name || '';
            form.elements.phone.value = data.requisites.phone || '';
            form.elements.email.value = data.requisites.email || '';
            form.elements.address.value = data.requisites.address || '';
            setDraftStatus('Черновик восстановлен');
        }
    } catch { /* ignore */ }
}

function bindForm() {
    const form = document.getElementById('requestForm');
    const categorySelect = document.getElementById('requestCategory');
    const serviceSelect = document.getElementById('requestService');

    form.elements.phone.addEventListener('blur', () => {
        form.elements.phone.value = normalizePhoneInput(form.elements.phone.value);
    });

    categorySelect.addEventListener('change', () => {
        const category = categories.find((c) => c.id === Number(categorySelect.value));
        renderServices(category ? category.slug : null);
    });

    serviceSelect.addEventListener('change', () => {
        const service = services.find((s) => s.id === Number(serviceSelect.value));
        if (service) {
            const category = categories.find((c) => c.slug === service.category_slug);
            if (category) categorySelect.value = String(category.id);
            renderServices(service.category_slug, service.id);
            serviceSelect.value = String(service.id);
        }
        updateServiceHint();
    });

    const scheduleDraft = debounce(saveDraft, 900);
    form.addEventListener('input', () => {
        if (!user) scheduleDraft();
    });

    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        clearFormErrors(form);
        if (!form.elements.agree.checked) {
            showFormErrors(form, {}, 'Подтвердите согласие на обработку персональных данных');
            return;
        }
        if (!form.elements.category_id.value) {
            showFormErrors(form, { category_id: 'Выберите тип техники' });
            return;
        }
        const phoneDigits = form.elements.phone.value.replace(/\D/g, '');
        if (phoneDigits.length < 10) {
            showFormErrors(form, { phone: 'Введите корректный телефон' });
            return;
        }

        const fd = new FormData(form);
        const payload = {
            customer_name: String(fd.get('customer_name') || '').trim(),
            phone: String(fd.get('phone') || '').trim(),
            email: String(fd.get('email') || '').trim(),
            address: String(fd.get('address') || '').trim(),
            category_id: Number(fd.get('category_id')) || null,
            service_id: Number(fd.get('service_id')) || null,
            brand_model: String(fd.get('brand_model') || '').trim(),
            description: String(fd.get('description') || '').trim(),
        };

        const button = form.querySelector('[type="submit"]');
        setBusy(button, true);
        try {
            const data = await api.post('/api/requests', payload);
            currentRequest = data.request;
            showSuccess(currentRequest);
            toast('Заявка создана. Менеджер скоро свяжется с вами.', 'success', 6000);
        } catch (err) {
            setBusy(button, false);
            if (err instanceof ApiError && err.fields) {
                showFormErrors(form, err.fields, err.message);
            } else {
                showFormErrors(form, {}, err.message || 'Не удалось создать заявку');
            }
        }
    });

    document.getElementById('newRequestBtn').addEventListener('click', () => {
        document.getElementById('successWrap').classList.add('hidden');
        document.getElementById('formWrap').classList.remove('hidden');
        form.reset();
        form.elements.customer_name.value = user ? (user.full_name || '') : '';
        form.elements.phone.value = user ? (user.phone || '') : '';
        form.elements.email.value = user ? (user.email || '') : '';
        form.elements.address.value = user ? (user.address || '') : '';
        document.getElementById('requestCategory').value = '';
        renderServices(null);
        currentRequest = null;
        window.scrollTo({ top: document.getElementById('requestCard').offsetTop - 100, behavior: 'smooth' });
    });

    document.getElementById('payNowBtn').addEventListener('click', () => {
        if (!currentRequest) return;
        openPayment(currentRequest.id, {
            request: currentRequest,
            onPaid: () => {
                document.getElementById('payNowBtn').classList.add('hidden');
                refreshRequest();
            },
        });
    });
}

async function saveDraft() {
    if (user) return;
    const form = document.getElementById('requestForm');
    const name = form.elements.customer_name.value.trim();
    const phone = form.elements.phone.value.replace(/\D/g, '');
    const address = form.elements.address.value.trim();
    if (name.length < 2 || phone.length < 10 || address.length < 5) {
        return;
    }
    try {
        await api.post('/api/session/requisites', {
            customer_name: name,
            phone: form.elements.phone.value.trim(),
            email: form.elements.email.value.trim(),
            address,
        });
        const now = new Date().toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
        setDraftStatus(`Черновик сохранён в ${now}`);
    } catch {
        setDraftStatus('Не удалось сохранить черновик');
    }
}

function setDraftStatus(text) {
    const el = document.getElementById('draftStatus');
    if (el) el.textContent = text;
}

function showSuccess(request) {
    document.getElementById('formWrap').classList.add('hidden');
    const wrap = document.getElementById('successWrap');
    wrap.classList.remove('hidden');
    document.getElementById('successNumber').textContent = request.public_number;
    document.getElementById('successSummary').innerHTML = `
        <div class="pay-summary__row"><span>Статус</span>${statusBadge(request.status)}</div>
        <div class="pay-summary__row"><span>Техника</span><span>${escapeHtml(request.equipment_type || '—')}</span></div>
        <div class="pay-summary__row"><span>Услуга</span><span>${escapeHtml(request.service_title || 'Уточнит менеджер')}</span></div>
        <div class="pay-summary__row"><span>Адрес</span><span>${escapeHtml(request.address)}</span></div>
        <div class="pay-summary__total"><span>Предварительно</span><span>${money(request.total_amount)}</span></div>`;

    const payBtn = document.getElementById('payNowBtn');
    const cabinetBtn = document.getElementById('cabinetBtn');
    const guestNote = document.getElementById('guestNote');
    const canPay = user && request.status !== 'cancelled' && request.status !== 'paid';
    payBtn.classList.toggle('hidden', !canPay);
    cabinetBtn.classList.toggle('hidden', !user);
    guestNote.classList.toggle('hidden', !!user);
    wrap.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

async function refreshRequest() {
    if (!currentRequest || !user) return;
    try {
        const data = await api.get(`/api/requests/${currentRequest.id}`);
        currentRequest = data.request;
        showSuccess(currentRequest);
        if (currentRequest.status === 'paid') {
            document.getElementById('payNowBtn').classList.add('hidden');
        }
    } catch { /* ignore */ }
}

function normalizePhoneInput(value) {
    let digits = String(value).replace(/\D/g, '');
    if (digits.length === 11 && digits.startsWith('8')) digits = `7${digits.slice(1)}`;
    if (digits.length === 10 && digits.startsWith('9')) digits = `7${digits}`;
    if (digits.length === 11 && digits.startsWith('7')) {
        return `+7 (${digits.slice(1, 4)}) ${digits.slice(4, 7)}-${digits.slice(7, 9)}-${digits.slice(9, 11)}`;
    }
    return value;
}

main();
