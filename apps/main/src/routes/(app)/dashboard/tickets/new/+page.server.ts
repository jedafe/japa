// The new-ticket form moved under the Messages support workbench —
// this route only forwards.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async () => {
	throw redirect(307, '/messages/support/new');
};
