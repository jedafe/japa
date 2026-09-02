import { test, expect } from '@playwright/test';

/**
 * JCE geo-scoped comparison picker tests.
 * Verifies that the scope-chain dropdowns filter the /compare item list
 * for countries, universities, cost-of-living, courses, and visa types.
 */

test.describe('JCE scope chain picker', () => {
	test.beforeEach(async ({ page }) => {
		// A warm-up request to the compare page so the server is responsive
		// before the real test begins.
		await page.goto('/compare');
	});

	test('countries can be filtered by region and subregion', async ({ page }) => {
		await page.goto('/compare?type=countries&source=country');

		const regionSelect = page.getByRole('combobox').first();
		await expect(regionSelect).toBeVisible();
		await regionSelect.selectOption('Africa');
		await expect(page).toHaveURL(/region=Africa/);

		const subregionSelect = page.getByRole('combobox').nth(1);
		await expect(subregionSelect).toBeVisible();
		await subregionSelect.selectOption('West Africa');
		await expect(page).toHaveURL(/subregion=West%20Africa|subregion=West\+Africa/);

		await expect(page.getByText('Nigeria').first()).toBeVisible();
	});

	test('universities can be filtered by country, state and city', async ({ page }) => {
		await page.goto('/compare?type=universities&source=university');

		const countrySelect = page.getByRole('combobox').first();
		await expect(countrySelect).toBeVisible();
		await countrySelect.selectOption('Nigeria');
		await expect(page).toHaveURL(/country_id=/);

		const stateSelect = page.getByRole('combobox').nth(1);
		await expect(stateSelect).toBeVisible();
		await stateSelect.selectOption('Lagos State');
		await expect(page).toHaveURL(/state_id=/);

		const citySelect = page.getByRole('combobox').nth(2);
		await expect(citySelect).toBeVisible();
		await citySelect.selectOption('Lagos');
		await expect(page).toHaveURL(/city_id=/);

		await expect(page.getByText('University of Lagos').first()).toBeVisible();
	});

	test('cost of living can be filtered to a city', async ({ page }) => {
		await page.goto('/compare?type=cost_of_living&source=col_city');

		const countrySelect = page.getByRole('combobox').first();
		await expect(countrySelect).toBeVisible();
		await countrySelect.selectOption('Nigeria');

		const stateSelect = page.getByRole('combobox').nth(1);
		await expect(stateSelect).toBeVisible();
		await stateSelect.selectOption('Lagos State');

		const citySelect = page.getByRole('combobox').nth(2);
		await expect(citySelect).toBeVisible();
		await citySelect.selectOption('Lagos');

		await expect(page).toHaveURL(/city_id=/);
	});

	test('courses can be filtered by university', async ({ page }) => {
		await page.goto('/compare?type=courses&source=course');

		const universitySelect = page.getByRole('combobox').first();
		await expect(universitySelect).toBeVisible();
		await universitySelect.selectOption('University of Lagos');

		await expect(page).toHaveURL(/university_id=/);
		await expect(page.getByText('BSc Computer Science').first()).toBeVisible();
	});

	test('visa types can be filtered by destination country', async ({ page }) => {
		await page.goto('/compare?type=visa-types&source=visa_route');

		const countrySelect = page.getByRole('combobox').first();
		await expect(countrySelect).toBeVisible();
		await countrySelect.selectOption('Nigeria');

		await expect(page).toHaveURL(/country_id=/);
		await expect(page.getByText('Nigeria Work Visa').first()).toBeVisible();
	});
});
