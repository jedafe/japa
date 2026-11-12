import { describe, it, expect } from 'vitest';
import { kebabCandidates } from './icons';

describe('kebabCandidates', () => {
	it('kebab input passes through lowercased', () => {
		expect(kebabCandidates('book-open')).toEqual(['book-open']);
		expect(kebabCandidates('Building-2')).toEqual(['building-2']);
	});

	it('PascalCase hyphenates at case boundaries', () => {
		expect(kebabCandidates('BookOpen')).toEqual(['book-open']);
		expect(kebabCandidates('Handshake')).toEqual(['handshake']);
		expect(kebabCandidates('Globe')).toEqual(['globe']);
	});

	it('digit-bearing names also try the letter→digit split', () => {
		expect(kebabCandidates('Building2')).toEqual(['building2', 'building-2']);
		expect(kebabCandidates('Volume2')).toEqual(['volume2', 'volume-2']);
		expect(kebabCandidates('Heading6')).toEqual(['heading6', 'heading-6']);
		expect(kebabCandidates('Axis3d')).toEqual(['axis3d', 'axis-3d']);
		expect(kebabCandidates('ArrowDown01')).toEqual(['arrow-down01', 'arrow-down-01']);
	});

	it('keeps NxN-style digit groups intact', () => {
		expect(kebabCandidates('Grid2x2')).toEqual(['grid2x2', 'grid-2x2']);
		expect(kebabCandidates('Grid3x3')).toEqual(['grid3x3', 'grid-3x3']);
	});
});
