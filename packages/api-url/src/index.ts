export interface BuildApiUrlOptions {
	/**
	 * The API version prefix to anchor the URL under.
	 * Defaults to `/api/v1`.
	 * Set to an empty string to disable prefix handling entirely.
	 */
	version?: string;
}

export function buildApiUrl(
	path: string,
	base: string,
	options: BuildApiUrlOptions = {}
): string {
	const version = options.version ?? '/api/v1';
	const b = base.replace(/\/+$/, '');
	const p = path.startsWith('/') ? path : `/${path}`;

	if (!version) {
		return `${b}${p}`;
	}

	const v = version.replace(/\/+$/, '');
	const baseWithVersion = b.endsWith(v) ? b : `${b}${v}`;

	if (p.startsWith(`${v}/`)) {
		return `${baseWithVersion}${p.slice(v.length)}`;
	}

	if (p === v) {
		return baseWithVersion;
	}

	return `${baseWithVersion}${p}`;
}
