/**
 * Модальное окно оплаты заявки (демо-платёжный шлюз).
 */
import { api, ApiError } from './api.js';
import { escapeHtml, money, openModal, setBusy, showFormErrors, clearFormErrors, toast } from './ui.js';

export async function openPayment(requestId, { onPaid = null, allowCash = false, request = null } = {}) {
    let data;
    try {
        data = await api.get(`/api/billing/invoice/${requestId}`);
    } catch (err) {
        toast(err.message || 'Не удалось загрузить счёт', 'error');
        return;
    }

    const invoice = data.invoice;
    const req = request || data.request;
    if (invoice.status === 'paid') {
        showReceipt(data);
        if (onPaid) onPaid(data);
        return;
    }

    const methods = [
        { key: 'card', label: 'Банковская карта' },
        { key: 'sbp', label: 'СБП / QR' },
    ];
    if (allowCash) methods.push({ key: 'cash', label: 'Наличные' });

    const modal = openModal({
        title: 'Оплата заявки',
        subtitle: `${req.public_number} · ${req.equipment_type || 'ремонт техники'}`,
        size: 'sm',
        body: `
            <form id="paymentForm" novalidate>
                <div class="form-error"></div>
                <div class="pay-summary">
                    <div class="pay-summary__row"><span>Заявка</span><strong>${escapeHtml(req.public_number)}</strong></div>
                    <div class="pay-summary__row"><span>Счёт</span><strong>${escapeHtml(invoice.number)}</strong></div>
                    <div class="pay-summary__row"><span>Услуга</span><span>${escapeHtml(req.service_title || req.category_title || 'Ремонт техники')}</span></div>
                    <div class="pay-summary__total"><span>К оплате</span><span>${money(invoice.amount)}</span></div>
                </div>
                <div class="pay-methods" id="payMethods">
                    ${methods.map((m, i) => `<button type="button" class="pay-method ${i === 0 ? 'active' : ''}" data-method="${m.key}">${m.label}</button>`).join('')}
                </div>
                <div id="cardFields">
                    <div class="form-group">
                        <label for="cardNumber">Номер карты</label>
                        <input id="cardNumber" name="card_number" inputmode="numeric" autocomplete="cc-number"
                               value="4111 1111 1111 1111" placeholder="0000 0000 0000 0000">
                        <div class="field-error" data-for="card_number"></div>
                    </div>
                    <div class="form-row">
                        <div class="form-group">
                            <label for="cardExpiry">Срок действия</label>
                            <input id="cardExpiry" name="expiry" inputmode="numeric" value="12/29" placeholder="MM/ГГ" autocomplete="cc-exp">
                        </div>
                        <div class="form-group">
                            <label for="cardCvc">CVC</label>
                            <input id="cardCvc" name="cvc" inputmode="numeric" value="123" placeholder="123" autocomplete="cc-csc">
                        </div>
                    </div>
                </div>
                <p class="pay-note">Демо-режим: карта 4111 1111 1111 1111 проходит успешно, карта с окончанием 0000 имитирует отказ банка.</p>
            </form>`,
        footer: `
            <button class="btn btn-outline" type="button" data-cancel>Отмена</button>
            <button class="btn btn-primary" type="submit" form="paymentForm" data-pay>Оплатить ${money(invoice.amount)}</button>`,
        onClose: () => {},
    });

    let method = 'card';
    modal.body.querySelectorAll('[data-method]').forEach((button) => {
        button.addEventListener('click', () => {
            method = button.dataset.method;
            modal.body.querySelectorAll('[data-method]').forEach((b) => b.classList.toggle('active', b === button));
            modal.body.querySelector('#cardFields').classList.toggle('hidden', method !== 'card');
        });
    });

    modal.foot.querySelector('[data-cancel]')?.addEventListener('click', () => modal.close());

    const form = modal.body.querySelector('#paymentForm');
    form.addEventListener('submit', async (event) => {
        event.preventDefault();
        clearFormErrors(form);
        const payButton = modal.foot.querySelector('[data-pay]');
        const cardNumber = form.elements.card_number.value.replace(/\D/g, '');
        if (method === 'card' && (cardNumber.length < 16 || cardNumber.length > 19)) {
            showFormErrors(form, { card_number: 'Введите 16–19 цифр номера карты' });
            return;
        }
        setBusy(payButton, true);
        try {
            const result = await api.post('/api/billing/pay', {
                request_id: requestId,
                method,
                card_number: method === 'card' ? cardNumber : '',
            });
            modal.close();
            toast('Платёж успешно проведён', 'success');
            if (onPaid) onPaid(result);
        } catch (err) {
            setBusy(payButton, false);
            if (err instanceof ApiError && err.code === 'payment_declined') {
                showFormErrors(form, {}, err.message);
                toast(err.message, 'error');
            } else if (err instanceof ApiError && err.fields) {
                showFormErrors(form, err.fields, err.message);
            } else {
                showFormErrors(form, {}, err.message || 'Не удалось провести платёж');
            }
        }
    });
}

export function showReceipt(data) {
    const invoice = data.invoice || data;
    const request = data.request || {};
    const modal = openModal({
        title: 'Электронный чек',
        subtitle: invoice.number || 'Оплаченный счёт',
        size: 'sm',
        body: `
            <div class="pay-summary">
                <div class="pay-summary__row"><span>Статус</span><span class="badge badge--paid">Оплачен</span></div>
                <div class="pay-summary__row"><span>Заявка</span><strong>${escapeHtml(request.public_number || invoice.request_number || '—')}</strong></div>
                <div class="pay-summary__row"><span>Способ</span><span>${escapeHtml(paymentMethod(invoice.method))}</span></div>
                ${invoice.card_last4 ? `<div class="pay-summary__row"><span>Карта</span><span>•••• ${escapeHtml(invoice.card_last4)}</span></div>` : ''}
                <div class="pay-summary__row"><span>Дата</span><span>${invoice.paid_at ? new Date(invoice.paid_at).toLocaleString('ru-RU') : '—'}</span></div>
                ${invoice.external_id ? `<div class="pay-summary__row"><span>Код операции</span><span class="text-xs">${escapeHtml(invoice.external_id)}</span></div>` : ''}
                <div class="pay-summary__total"><span>Итого</span><span>${money(invoice.amount)}</span></div>
            </div>
            <p class="form-hint">Чек сохранён в личном кабинете. Для получения бумажной копии обратитесь в сервисный центр.</p>`,
        footer: '<button class="btn btn-primary" type="button" data-close>Понятно</button>',
    });
    modal.foot.querySelector('[data-close]').addEventListener('click', () => modal.close());
}

function paymentMethod(method) {
    switch (method) {
        case 'card': return 'Банковская карта';
        case 'sbp': return 'СБП';
        case 'cash': return 'Наличные';
        default: return method || '—';
    }
}
