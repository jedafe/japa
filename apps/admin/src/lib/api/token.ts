let _accessToken: string | null = null;

export function setAccessToken(token: string | null): void {
	_accessToken = token;
}

export function getAccessToken(): string | null {
	return _accessToken;
}

export function clearAccessToken(): void {
	_accessToken = null;
}
