/** Locales written right-to-left. Used by hooks.server.ts (<html dir>) and
 * client-side lang/dir sync. Kept in a leaf module so server hooks can
 * import it without pulling in the runes store. */
export const RTL_LOCALES = new Set(['ar', 'he', 'fa', 'ur']);
export const isRtl = (locale: string): boolean => RTL_LOCALES.has(locale);
