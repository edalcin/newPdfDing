// Cliente HTTP fino para a API Go: injeta o header X-CSRF-Token (double-
// submit cookie, ver refatoracao/08-seguranca.md, "CSRF") em todo método
// não-idempotente, e normaliza erros no envelope {"error": "..."} fixado em
// refatoracao/05-api.md, "Convenções gerais".

export class ApiError extends Error {
	status: number;
	body: unknown;

	constructor(status: number, message: string, body: unknown) {
		super(message);
		this.status = status;
		this.body = body;
	}
}

function readCookie(name: string): string | null {
	const match = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'));
	return match ? decodeURIComponent(match[1]) : null;
}

const IDEMPOTENT_METHODS: Record<string, true> = { GET: true, HEAD: true, OPTIONS: true };

export interface RequestOptions {
	method?: string;
	body?: BodyInit | Record<string, unknown> | null;
	headers?: Record<string, string>;
	signal?: AbortSignal;
}

/** Low-level request: returns the raw Response. Callers that need binary
 * content (file/thumbnail/preview streams) use this directly. */
export async function apiRequest(path: string, options: RequestOptions = {}): Promise<Response> {
	const method = options.method ?? 'GET';
	const headers = new Headers(options.headers);

	let body: BodyInit | null | undefined = options.body as BodyInit | null | undefined;
	if (options.body && typeof options.body === 'object' && !(options.body instanceof FormData) && !(options.body instanceof Blob)) {
		headers.set('Content-Type', 'application/json');
		body = JSON.stringify(options.body);
	}

	if (!IDEMPOTENT_METHODS[method.toUpperCase()]) {
		const csrf = readCookie('csrf');
		if (csrf) headers.set('X-CSRF-Token', csrf);
	}

	return fetch(`/api${path}`, { method, headers, body, credentials: 'same-origin', signal: options.signal });
}

/** JSON request/response helper. Throws ApiError on any non-2xx status. */
export async function apiJSON<T>(path: string, options: RequestOptions = {}): Promise<T> {
	const res = await apiRequest(path, options);
	const text = await res.text();
	const data = text ? JSON.parse(text) : undefined;
	if (!res.ok) {
		let message = res.statusText;
		if (data && typeof data === 'object' && 'error' in data && typeof data.error === 'string') {
			message = data.error;
		}
		throw new ApiError(res.status, message, data);
	}
	return data as T;
}
