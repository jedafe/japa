import { test, expect } from '@playwright/test';

const corsHeaders = {
	'Access-Control-Allow-Origin': '*',
	'Access-Control-Allow-Methods': 'GET, POST, PATCH, DELETE, OPTIONS',
	'Access-Control-Allow-Headers': 'Authorization, Content-Type, X-API-Key, X-Device-Id'
};

function okJson(body: unknown) {
	return {
		status: 200,
		headers: { ...corsHeaders, 'Content-Type': 'application/json' },
		body: JSON.stringify({ data: body })
	};
}

function rawOk(body: unknown) {
	return {
		status: 200,
		headers: { ...corsHeaders, 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	};
}

function errorJson(status: number, code: string, message: string) {
	return {
		status,
		headers: { ...corsHeaders, 'Content-Type': 'application/json' },
		body: JSON.stringify({ error: { code, message } })
	};
}

const mockSettings = {
	projectName: 'Test Project',
	baseUrl: 'https://mock.japa.test',
	apiKey: 'test-api-key',
	projectRoot: '',
	mode: 'operator',
	autoSync: false,
	autoSyncContentTypes: [],
	default_content_type: 'guide',
	default_guide_category: 'relocation',
	default_persona_id: 'persona-1',
	default_voice_id: 'voice-1',
	default_space_id: 'space-1',
	default_status: 'draft',
	default_visibility: 'public',
	default_tags: [],
	accessToken: 'mock-access-token',
	tokenExpiresAt: Date.now() + 3600000,
	deviceId: 'test-device'
};

async function setupMockApi(
	page: any,
	overrides: {
		personaError?: { status: number; message: string };
		voiceError?: { status: number; message: string };
	} = {}
) {
	await page.route('https://mock.japa.test/api/v1/**', async (route: any, request: any) => {
		if (request.method() === 'OPTIONS') {
			return route.fulfill({ status: 204, headers: corsHeaders });
		}

		const url = new URL(request.url());
		const path = url.pathname;

		switch (path) {
			case '/api/v1/auth/exchange-key':
				return route.fulfill(
					rawOk({
						access_token: 'mock-access-token',
						refresh_token: 'mock-refresh-token',
						token_type: 'bearer',
						expires_in: 900
					})
				);
			case '/api/v1/me':
				return route.fulfill(okJson({ id: 'user-1', email: 'operator@example.com' }));
			case '/api/v1/content-types':
				return route.fulfill(
					okJson([
						{
							value: 'guide',
							label: 'Guide',
							has_category: true,
							categories: [{ Slug: 'relocation', Label: 'Relocation' }]
						}
					])
				);
			case '/api/v1/admin/personas':
				if (overrides.personaError) {
					const { status, message } = overrides.personaError;
					return route.fulfill(errorJson(status, 'forbidden', message));
				}
				return route.fulfill(
					okJson([
						{
							id: 'persona-1',
							profile_name: 'Ada',
							user_id: 'user-2'
						}
					])
				);
			case '/api/v1/admin/persona-voices':
				if (overrides.voiceError) {
					const { status, message } = overrides.voiceError;
					return route.fulfill(errorJson(status, 'forbidden', message));
				}
				return route.fulfill(
					okJson([
						{
							id: 'voice-1',
							name: 'Friendly',
							voice_prompt: 'Be friendly and approachable.'
						}
					])
				);
			case '/api/v1/spaces':
				return route.fulfill(okJson([{ id: 'space-1', name: 'Main Space' }]));
			case '/api/v1/content/status-options':
				return route.fulfill(okJson([{ value: 'draft', label: 'Draft' }]));
			case '/api/v1/content/visibility-options':
				return route.fulfill(okJson([{ value: 'public', label: 'Public' }]));
			default:
				return route.fulfill(errorJson(404, 'not_found', `Unexpected ${request.method()} ${path}`));
		}
	});
}

async function loadSettingsIntoPage(page: any, settings: typeof mockSettings) {
	await page.goto('/');
	await page.evaluate(
		(s: typeof mockSettings) => localStorage.setItem('japa-studio:settings', JSON.stringify(s)),
		settings
	);
}

test.describe('settings lookup wiring', () => {
	test('settings page populates all backend-driven dropdowns', async ({ page }) => {
		await setupMockApi(page);
		await loadSettingsIntoPage(page, mockSettings);
		await page.goto('/settings');

		await expect(page.getByLabel('Default content type')).toHaveValue('guide');
		await expect(page.getByLabel('Default content type').locator('option')).toHaveCount(2);

		await expect(page.getByLabel('Default guide category')).toHaveValue('relocation');
		await expect(page.getByLabel('Default guide category').locator('option')).toHaveCount(2);

		await expect(page.getByLabel('Default persona')).toHaveValue('persona-1');
		await expect(page.getByLabel('Default persona').locator('option')).toHaveCount(2);

		await expect(page.getByLabel('Default voice')).toHaveValue('voice-1');
		await expect(page.getByLabel('Default voice').locator('option')).toHaveCount(2);

		await expect(page.getByLabel('Default space')).toHaveValue('space-1');
		await expect(page.getByLabel('Default space').locator('option')).toHaveCount(2);

		await expect(page.getByLabel('Default status')).toHaveValue('draft');
		await expect(page.getByLabel('Default status').locator('option')).toHaveCount(1);

		await expect(page.getByLabel('Default visibility')).toHaveValue('public');
		await expect(page.getByLabel('Default visibility').locator('option')).toHaveCount(1);
	});

	test('settings page exchanges API key and then populates dropdowns', async ({ page }) => {
		await setupMockApi(page);
		const settingsWithoutToken = { ...mockSettings, accessToken: undefined, tokenExpiresAt: undefined };
		await loadSettingsIntoPage(page, settingsWithoutToken as unknown as typeof mockSettings);
		await page.goto('/settings');

		await expect(page.getByLabel('Default content type')).toHaveValue('guide', { timeout: 10000 });
		await expect(page.getByLabel('Default persona')).toHaveValue('persona-1');
	});

	test('settings page surfaces errors for denied lookups while still populating the rest', async ({ page }) => {
		await setupMockApi(page, {
			personaError: { status: 403, message: 'You do not have permission to view this directory' },
			voiceError: { status: 403, message: 'You do not have permission to view this directory' }
		});
		await loadSettingsIntoPage(page, mockSettings);
		await page.goto('/settings');

		await expect(page.getByLabel('Default content type')).toHaveValue('guide');
		await expect(page.getByLabel('Default space')).toHaveValue('space-1');
		await expect(page.getByLabel('Default status')).toHaveValue('draft');
		await expect(page.getByLabel('Default visibility')).toHaveValue('public');

		await expect(page.getByText('Some lookup options could not be loaded:')).toBeVisible();
		await expect(page.getByText('personas: You do not have permission to view this directory')).toBeVisible();
		await expect(page.getByText('voices: You do not have permission to view this directory')).toBeVisible();
	});
});
