import { describe, expect, it } from 'vitest';
import { entityPageHref } from './wikiEntityLinks';

describe('entityPageHref', () => {
	it('builds the entity overview path', () => {
		expect(entityPageHref('course', 'msc-cs-unilag')).toBe('/wiki/e/course/msc-cs-unilag/overview');
	});
	it('encodes segments', () => {
		expect(entityPageHref('student_visa', 'a b')).toBe('/wiki/e/student_visa/a%20b/overview');
	});
});
