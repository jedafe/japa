import { describe, it, expect } from 'vitest';
import {
	scopeChainOf,
	defaultItemRefType,
	requiredScopeStep,
	missingRequiredScope,
	scopeSummary,
	sourceOptions
} from './compareFlow';
import type { ComparisonType } from '$lib/types/jce';

const type = (config: Record<string, unknown>): ComparisonType => ({
	id: '1', name: 'T', slug: 't', item_label: 'Item', config: config as never
});

describe('compareFlow', () => {
	it('scopeChainOf returns config chain or []', () => {
		expect(scopeChainOf(type({ scope_chain: ['country', 'city'] }))).toEqual(['country', 'city']);
		expect(scopeChainOf(type({}))).toEqual([]);
		expect(scopeChainOf(null)).toEqual([]);
	});

	it('defaultItemRefType prefers config.default_item_ref_type then first item_ref_types then space', () => {
		expect(defaultItemRefType(type({ default_item_ref_type: 'country', item_ref_types: ['col_city', 'comparison'] }))).toBe('country');
		expect(defaultItemRefType(type({ item_ref_types: ['university', 'comparison'] }))).toBe('university');
		expect(defaultItemRefType(type({}))).toBe('space');
	});

	it('requiredScopeStep: course requires university, everything else null', () => {
		expect(requiredScopeStep('course', ['university'])).toBe('university');
		expect(requiredScopeStep('university', ['country', 'city'])).toBeNull();
		expect(requiredScopeStep('space', [])).toBeNull();
	});

	it('missingRequiredScope reports the missing step only while unset', () => {
		const chain = ['university'];
		expect(missingRequiredScope('course', chain, {})).toBe('university');
		expect(missingRequiredScope('course', chain, { university: 'u1' })).toBeNull();
		expect(missingRequiredScope('col_city', ['country', 'city'], {})).toBeNull();
	});

	it('scopeSummary joins selected labels in chain order', () => {
		const chain = ['country', 'state', 'city'];
		const selections = { country: 'c1', city: 'c3' };
		const labels = { country: 'Nigeria', city: 'Lagos' };
		expect(scopeSummary(chain, selections, labels)).toBe('Nigeria · Lagos');
		expect(scopeSummary(chain, {}, {})).toBe('');
	});

	it('scopeSummary falls back to raw value for string scopes, omits unlabeled id steps', () => {
		const chain = ['region', 'subregion', 'country'];
		expect(scopeSummary(chain, { region: 'Africa', subregion: 'West' }, {})).toBe('Africa · West');
		expect(scopeSummary(chain, { region: 'Africa', country: 'u1' }, {})).toBe('Africa');
		expect(scopeSummary(chain, { region: 'Africa' }, { region: 'Labeled' })).toBe('Labeled');
	});

	it('sourceOptions maps item_ref_types to labelled options', () => {
		const opts = sourceOptions(['col_city', 'comparison'], (k) => `L:${k}`);
		expect(opts).toEqual([
			{ value: 'col_city', label: 'L:col_city' },
			{ value: 'comparison', label: 'L:comparison' }
		]);
	});
});
