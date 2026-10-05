// Ref: japaconnect-engineering/sections/120-mcp-agent-distribution.md
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { navigatePostLogin, safeRelativePath } from './redirect';

const gotoMock = vi.fn().mockResolvedValue(undefined);

vi.mock('$app/navigation', () => ({
	goto: (...args: unknown[]) => gotoMock(...args)
}));

const assignMock = vi.fn();

describe('safeRelativePath', () => {
	it('keeps relative site paths', () => {
		expect(safeRelativePath('/feed')).toBe('/feed');
		expect(safeRelativePath('/onboarding?step=2')).toBe('/onboarding?step=2');
	});

	it('falls back to / for nullish, empty, absolute, and protocol-relative targets', () => {
		expect(safeRelativePath(null)).toBe('/');
		expect(safeRelativePath(undefined)).toBe('/');
		expect(safeRelativePath('')).toBe('/');
		expect(safeRelativePath('//evil.com/phish')).toBe('/');
		expect(safeRelativePath('/\\evil.com')).toBe('/');
		expect(safeRelativePath('https://evil.com/')).toBe('/');
		expect(safeRelativePath('not-a-path')).toBe('/');
	});
});

describe('navigatePostLogin', () => {
	beforeEach(() => {
		gotoMock.mockClear();
		assignMock.mockClear();
		// jsdom location is read-only; stub just the assign call.
		Object.defineProperty(window, 'location', {
			value: { assign: assignMock },
			writable: true,
			configurable: true
		});
	});

	it('uses goto for relative paths', async () => {
		await navigatePostLogin('/feed');
		expect(gotoMock).toHaveBeenCalledWith('/feed');
		expect(assignMock).not.toHaveBeenCalled();
	});

	it('full-navigates for the allowlisted absolute API URL', async () => {
		await navigatePostLogin('http://localhost:8080/oauth/authorize?x=1');
		expect(assignMock).toHaveBeenCalledWith('http://localhost:8080/oauth/authorize?x=1');
		expect(gotoMock).not.toHaveBeenCalled();
	});

	it('never assigns javascript: or protocol-relative URLs', async () => {
		await navigatePostLogin('javascript:alert(1)');
		expect(assignMock).not.toHaveBeenCalled();
		expect(gotoMock).toHaveBeenCalledWith('/');

		gotoMock.mockClear();
		await navigatePostLogin('//evil.com/phish');
		expect(assignMock).not.toHaveBeenCalled();
		expect(gotoMock).toHaveBeenCalledWith('/');
	});

	it('defaults to / for empty input', async () => {
		await navigatePostLogin(null);
		expect(gotoMock).toHaveBeenCalledWith('/');
	});
});
