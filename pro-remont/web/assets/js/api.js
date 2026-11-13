/**
 * Тонкая обёртка над fetch для работы с JSON API.
 * Все ошибки приводятся к ApiError с полями code/fields.
 */

export class ApiError extends Error {
    constructor(status, data) {
        const info = data && data.error ? data.error : null;
        super((info && info.message) || 'Не удалось выполнить запрос');
        this.name = 'ApiError';
        this.status = status;
        this.code = (info && info.code) || 'http_error';
        this.fields = (info && info.fields) || null;
        this.data = data;
    }
}

async function request(path, options = {}) {
    const opts = {
        method: options.method || 'GET',
        credentials: 'same-origin',
        headers: { Accept: 'application/json', ...(options.headers || {}) },
        signal: options.signal,
    };

    if (options.body !== undefined) {
        if (options.body instanceof FormData) {
            opts.body = options.body;
        } else if (typeof options.body === 'string') {
            opts.body = options.body;
            opts.headers['Content-Type'] = opts.headers['Content-Type'] || 'application/json';
        } else {
            opts.body = JSON.stringify(options.body);
            opts.headers['Content-Type'] = 'application/json';
        }
    }

    let response;
    try {
        response = await fetch(path, opts);
    } catch (err) {
        if (err.name === 'AbortError') throw err;
        throw new ApiError(0, { error: { code: 'network_error', message: 'Нет связи с сервером. Проверьте подключение.' } });
    }

    const contentType = response.headers.get('content-type') || '';
    let data = null;
    if (contentType.includes('application/json')) {
        data = await response.json().catch(() => null);
    } else {
        data = await response.text().catch(() => null);
    }

    if (!response.ok) {
        throw new ApiError(response.status, data);
    }
    return data;
}

function withQuery(path, params) {
    if (!params) return path;
    const query = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== '') {
            query.set(key, String(value));
        }
    });
    const qs = query.toString();
    return qs ? `${path}${path.includes('?') ? '&' : '?'}${qs}` : path;
}

export const api = {
    request,
    get: (path, params) => request(withQuery(path, params)),
    post: (path, body) => request(path, { method: 'POST', body }),
    put: (path, body) => request(path, { method: 'PUT', body }),
    patch: (path, body) => request(path, { method: 'PATCH', body }),
    del: (path) => request(path, { method: 'DELETE' }),
};
