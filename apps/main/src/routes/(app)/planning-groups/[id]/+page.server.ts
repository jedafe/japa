// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { PlanningGroup, PlanningMilestone, PlanningCheckin, MemberDTO } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const currentUserId = event.locals.user?.id ?? null;
	const id = event.params.id;

	let group: PlanningGroup | null = null;
	let loadError = false;
	try {
		group = await apiGet<PlanningGroup>(ctx, `/planning-groups/${id}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Planning group not found' });
		}
		loadError = true;
	}

	// Members, milestones and check-ins are separate endpoints — fetch defensively.
	let members: MemberDTO[] = [];
	let milestones: PlanningMilestone[] = [];
	let checkins: PlanningCheckin[] = [];
	if (group && authed) {
		[members, milestones, checkins] = await Promise.all([
			apiList<MemberDTO>(ctx, `/planning-groups/${id}/members`)
				.then((r) => r.items)
				.catch(() => [] as MemberDTO[]),
			apiList<PlanningMilestone>(ctx, `/planning-groups/${id}/milestones`)
				.then((r) => r.items)
				.catch(() => [] as PlanningMilestone[]),
			apiList<PlanningCheckin>(ctx, `/planning-groups/${id}/checkins`)
				.then((r) => r.items)
				.catch(() => [] as PlanningCheckin[])
		]);
	}

	return { group, members, milestones, checkins, authed, currentUserId, loadError };
};
