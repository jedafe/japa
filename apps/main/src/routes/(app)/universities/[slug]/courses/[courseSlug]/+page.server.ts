// Ref: japaconnect-engineering/sections/39-space-types-university.md
//      (university_course_offerings — public offering detail)
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { CourseOffering } from '$lib/types';

/**
 * Institutional course offering detail at
 * /universities/[slug]/courses/[courseSlug] — public, SSR, indexable.
 *
 * Fetches GET /universities/{slug}/courses/{offeringSlug}. The endpoint is
 * scoped to the university: a mismatched university slug returns 404.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const { slug, courseSlug } = event.params;

	let offering: CourseOffering;
	try {
		offering = await apiGet<CourseOffering>(ctx, `/universities/${slug}/courses/${courseSlug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Course offering not found' });
		}
		throw err;
	}

	// JCE course comparisons are scoped to a university; preselect resolves
	// the offering through the `course` item ref type resolver.
	const compareHref = `/compare?type=courses&source=course&university_id=${offering.university_id}&preselect=${offering.slug}`;

	return { offering, compareHref };
};
