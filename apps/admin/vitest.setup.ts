import { vi, afterEach } from 'vitest';
import { cleanup } from '@testing-library/svelte';
import en from './src/lib/locales/en.json';

vi.mock('$lib/stores/i18n.svelte', () => ({
    t: (key: string, fallback?: string) => en[key] ?? fallback ?? key,
    locale: { current: 'en', set: vi.fn(), init: vi.fn() },
    getLocale: () => ({ current: 'en', set: vi.fn(), init: vi.fn() }),
    loadTranslations: vi.fn().mockResolvedValue(en)
}));

// bits-ui's body-scroll-lock schedules a setTimeout cleanup that can fire
// after jsdom is torn down, causing "document is not defined" errors. Unmount
// all rendered components and wait for the ~24ms cleanup to finish before
// the next test/file runs.
afterEach(async () => {
    cleanup();
    await new Promise((resolve) => setTimeout(resolve, 50));
});
