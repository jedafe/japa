import type { PageLoad } from './$types';

export const load: PageLoad = ({ params }) => ({ tenantId: params.id });
