/** Default brand name when general.site_name is unset — the only place the
 * literal may appear; all UI copy must flow through t()/{site} or siteName().
 * Kept in a dependency-free module so server code can import it safely. */
export const DEFAULT_SITE_NAME = 'Japa.to';
