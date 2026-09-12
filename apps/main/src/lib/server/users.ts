import { apiList } from './api';
import type { ApiContext } from './api';

type Authorish = {
	author_id?: string | null;
	author_username?: string | null;
};

type UserStub = {
	id: string;
	username: string;
};

const USER_PAGE_SIZE = 1000;

/**
 * Backfill `author_username` on a list of content items when the upstream
 * list endpoint (blog, stories, guides, etc.) returns `author_id` but omits
 * the public username. A missing username prevents `UserHoverCard` from
 * opening, so we hydrate it from the public `/users` directory.
 *
 * The public users list is fetched once with a large page size and paged
 * through only if the tenant has more than `USER_PAGE_SIZE` users.
 */
export async function attachAuthorUsernames<T extends Authorish>(
	ctx: ApiContext,
	items: T[]
): Promise<void> {
	const missing = items.filter((i) => i.author_id && !i.author_username);
	if (missing.length === 0) return;

	try {
		const all: UserStub[] = [];
		const first = await apiList<UserStub>(ctx, `/users?page_size=${USER_PAGE_SIZE}`);
		all.push(...first.items);

		const total = first.meta?.total ?? first.items.length;
		while (all.length < total) {
			const offset = all.length;
			const next = await apiList<UserStub>(
				ctx,
				`/users?page_size=${USER_PAGE_SIZE}&page_offset=${offset}`
			);
			if (next.items.length === 0) break;
			all.push(...next.items);
		}

		const map = new Map(all.map((u) => [u.id, u.username]));
		for (const item of items) {
			if (item.author_id && !item.author_username) {
				item.author_username = map.get(item.author_id) ?? item.author_username;
			}
		}
	} catch {
		// Degrade to the original payload — the card still renders, but the
		// author hover card will be skipped for items without a username.
	}
}
