import { api, type ClientResult } from './client';

// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
// Browser-side helpers for the §80 structured-edit surface: the editable
// field schema (GET /wiki/entities/{type}/{id}/fields), entity-keyed op
// submissions (POST /wiki/suggest), and the entity revision history.

/** One field in an entity's editable schema — definition + current value +
 *  the requesting user's field-level permission flags. */
export interface WikiEntityField {
	id: string;
	key: string;
	label: string;
	field_type: string;
	/** Raw field options (array for select/multiselect, object config for
	 *  repeater/number) — same shape as CustomFieldDef.options. */
	options?: unknown;
	/** Current stored value as a JSON scalar/array (null when unset). */
	value?: unknown;
	can_edit: boolean;
	visible: boolean;
}

/** A field group assigned to the entity's class, fields in display order. */
export interface WikiEntityFieldGroup {
	id: string;
	slug: string;
	name: string;
	fields: WikiEntityField[];
}

/** GET /wiki/entities/{type}/{id}/fields response payload. */
export interface WikiEntityFields {
	groups: WikiEntityFieldGroup[];
	pending_count: number;
}

/** §80 field op — only set_value is produced by the suggest-edit UI; the
 *  other op kinds exist for propose/manage tiers. */
export interface WikiFieldOp {
	op: 'set_value' | 'propose_field' | 'propose_group' | 'upsert_field' | 'assign_group';
	entity_type?: string;
	entity_id?: string;
	field_key?: string;
	group_slug?: string;
	entity_class?: string;
	source_url?: string;
	value?: unknown;
	field?: unknown;
	group?: unknown;
}

/** POST /wiki/suggest response payload. */
export interface SuggestFieldsResult {
	revision_id: string;
	status: string;
	auto_approved: boolean;
}

/** One entry in an entity's revision history (fields/mixed revisions). */
export interface WikiEntityRevision {
	id: string;
	status: string;
	kind: string;
	summary?: string;
	ops?: unknown;
	submitted_by?: string;
	reviewed_by?: string;
	reviewed_at?: string;
	created_at: string;
	target_entity_type?: string;
	target_entity_id?: string;
}

export interface EntityRevisionsPage {
	items: WikiEntityRevision[];
	page: number;
	limit: number;
	total: number;
}

export async function getEntityFields(
	entityType: string,
	entityId: string
): Promise<WikiEntityFields | null> {
	const res = await api.get<WikiEntityFields>(
		`/wiki/entities/${encodeURIComponent(entityType)}/${encodeURIComponent(entityId)}/fields`
	);
	return res.data;
}

/** Submit field ops for review. Returns the full result so callers can
 *  surface the backend's structured error code. */
export function suggestFields(payload: {
	entity_type: string;
	entity_id: string;
	ops: WikiFieldOp[];
	summary?: string;
}): Promise<ClientResult<SuggestFieldsResult>> {
	return api.post<SuggestFieldsResult>('/wiki/suggest', payload);
}

/** Paginated revision history targeting an entity. The envelope's top-level
 *  `pagination` object carries page/limit/total. */
export async function getEntityRevisions(
	entityType: string,
	entityId: string,
	params: { page?: number; limit?: number } = {}
): Promise<EntityRevisionsPage> {
	const qs = new URLSearchParams();
	qs.set('page', String(params.page ?? 1));
	qs.set('limit', String(params.limit ?? 20));
	const res = await api.getEnvelope<WikiEntityRevision[]>(
		`/wiki/entities/${encodeURIComponent(entityType)}/${encodeURIComponent(entityId)}/revisions?${qs.toString()}`
	);
	const pg = (res.envelope?.pagination ?? {}) as {
		page?: number;
		limit?: number;
		total?: number;
	};
	return {
		items: res.data ?? [],
		page: pg.page ?? params.page ?? 1,
		limit: pg.limit ?? params.limit ?? 20,
		total: pg.total ?? 0
	};
}
