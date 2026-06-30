import { vi } from 'vitest';
import en from './src/lib/locales/en.json';

vi.mock('$lib/stores/i18n.svelte', () => ({
    t: (key: string, fallback?: string) => en[key] ?? fallback ?? key,
    locale: { current: 'en', set: vi.fn(), init: vi.fn() },
    getLocale: () => ({ current: 'en', set: vi.fn(), init: vi.fn() }),
    loadTranslations: vi.fn().mockResolvedValue(en)
}));
