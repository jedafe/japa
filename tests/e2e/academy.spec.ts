import { test, expect } from '@playwright/test';

/**
 * Learn App Smoke Tests
 *
 * The learn app is an SSR SvelteKit app with client-side data fetching
 * via onMount. Without the Go backend running, the API call will hang
 * or fail, so skeletons or error states may persist indefinitely.
 *
 * These tests verify:
 * 1. The SvelteKit dev server starts and responds
 * 2. The page has the correct title
 * 3. The page renders the expected structural elements
 */

test.describe('Learn App — Smoke Tests', () => {
	test('page loads successfully', async ({ page }) => {
		const response = await page.goto('/');
		expect(response).not.toBeNull();
		await expect(page.locator('html')).toBeVisible();
	});

	test('page has correct title', async ({ page }) => {
		await page.goto('/');
		await expect(page).toHaveTitle('Learning Tracks | JapaConnect Academy');
	});

	test('hero section renders', async ({ page }) => {
		await page.goto('/');
		await expect(page.getByRole('heading', { name: 'Learn & Grow' })).toBeVisible();
	});

	test('track listing section renders', async ({ page }) => {
		await page.goto('/');

		// The Learning Tracks heading should always be visible regardless of data state
		await expect(page.getByRole('heading', { name: 'Learning Tracks' })).toBeVisible();

		// Verify one of the three content states is present:
		// 1. Track cards (href links to /tracks/*)
		// 2. Empty state message
		// 3. Error state with retry button
		// 4. Loading skeletons (API call still pending)
		const trackLinks = page.locator('a[href^="/tracks/"]');
		const emptyState = page.getByText('No tracks available yet');
		const errorState = page.getByRole('button', { name: 'Try again' });
		const skeletons = page.locator('.skeleton');

		// Wait up to 15s for any state to appear (covers slow API + error)
		const statesFound = await Promise.race([
			trackLinks.first().waitFor({ state: 'visible', timeout: 15_000 }).then(() => 'tracks'),
			emptyState.waitFor({ state: 'visible', timeout: 15_000 }).then(() => 'empty'),
			errorState.waitFor({ state: 'visible', timeout: 15_000 }).then(() => 'error'),
			skeletons.first().waitFor({ state: 'visible', timeout: 15_000 }).then(() => 'loading')
		]).catch(() => null);

		expect(statesFound).not.toBeNull();
	});
});
