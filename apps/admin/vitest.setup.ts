import { vi, afterEach } from 'vitest';
import en from './src/lib/locales/en.json';

vi.mock('$lib/stores/i18n.svelte', () => ({
    t: (key: string, fallback?: string) => en[key] ?? fallback ?? key,
    locale: { current: 'en', set: vi.fn(), init: vi.fn() },
    getLocale: () => ({ current: 'en', set: vi.fn(), init: vi.fn() }),
    loadTranslations: vi.fn().mockResolvedValue(en)
}));

// bits-ui's body-scroll-lock schedules a setTimeout cleanup (~24ms) that can
// fire after jsdom is torn down, causing "document is not defined" errors.
// Cancel pending timers after each test to prevent the race.
afterEach(() => {
    vi.clearAllTimers();
});
