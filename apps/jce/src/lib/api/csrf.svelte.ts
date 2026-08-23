import { browser } from '$app/environment';
import { PUBLIC_JC_API_URL } from '$env/static/public';

let csrfToken = $state<string | null>(null);

export function getCsrfToken(): string | null {
	return csrfToken;
}

export function seedCsrfToken(token: string | null): void {
	csrfToken = token;
}

export async function refreshCsrfToken(): Promise<boolean> {
	if (!browser) return false;
	try {
		const res = await fetch(`${PUBLIC_JC_API_URL}/csrf`, { credentials: 'include' });
		if (!res.ok) return false;
		const body = await res.json();
		const token = body?.data?.csrfToken ?? body?.csrfToken ?? null;
		if (token) {
			csrfToken = token;
			return true;
		}
		return false;
	} catch {
		return false;
	}
}
