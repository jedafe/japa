import { page } from '$app/state';

/** Default brand when general.site_name is unset. */
export const DEFAULT_SITE_NAME = 'Japa.to';

/** Configured site name from the host app's public site settings (root layout
 * data). Works inside SSR pages of host apps; falls back to the default. */
export function siteName(): string {
	return (page.data?.siteSettings?.['general.site_name'] as string | undefined) ?? DEFAULT_SITE_NAME;
}
