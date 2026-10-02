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
	/** §80 Itemized Pricing — the field's section token (e.g.
	 *  'transportation'); fields sharing one render under one item table. */
	section?: string;
}

/** §80 Itemized Pricing — one named section inside a field group: the
 *  fields sharing a custom_fields.section token, in definition order.
 *  §80 Wiki Topic Pages: a section synthesized from a child page
 *  template's bound group carries label (its section_label, falling back
 *  to the template label) and page_slug — the /wiki/{space}/{slug}
 *  drill-down target for the "View more" link. */
export interface WikiEntityFieldSection {
	key: string;
	label?: string;
	page_slug?: string;
	fields: WikiEntityField[];
}

/** A field group assigned to the entity's class, fields in display order. */
export interface WikiEntityFieldGroup {
	id: string;
	slug: string;
	name: string;
	fields: WikiEntityField[];
	/** Fields regrouped by section token for one-table-per-section UIs. */
	sections?: WikiEntityFieldSection[];
}

/** GET /wiki/entities/{type}/{id}/fields response payload. */
export interface WikiEntityFields {
	groups: WikiEntityFieldGroup[];
	pending_count: number;
	/** Caller holds wiki.fields.propose (or manage) — may submit
	 *  propose_field/propose_group ops. */
	can_propose?: boolean;
	/** Caller holds wiki.fields.manage — ops revisions auto-approve. */
	can_manage?: boolean;
	/** The entity's resolved custom-fields class (propose_group payload). */
	entity_class?: string;
	/** §58 — the entity's resolved country currency (its geo space's
	 *  country currency_code); display fallback for currency/price_range
	 *  fields that don't declare {currency_code} in options. */
	entity_currency_code?: string;
}

/** Field definition carried by propose_field / propose_group ops. */
export interface WikiFieldDefInput {
	key: string;
	label: string;
	field_type: string;
	/** Array for select/multiselect; object schema for composite types
	 *  (§80 Wiki Topic Pages — price_parts carries options.parts). */
	options?: unknown;
	/** §80 Itemized Pricing — the field's section token; groups the field
	 *  into an item-table section when set. */
	section?: string;
}

/** Group definition carried by propose_group ops. */
export interface WikiGroupDefInput {
	slug: string;
	name: string;
	description?: string;
	fields?: WikiFieldDefInput[];
}

/** §80 field op — set_value edits existing fields; propose_field /
 *  propose_group (propose tier) and upsert_field / assign_group (manage
 *  tier) extend the schema itself. */
export interface WikiFieldOp {
	op: 'set_value' | 'propose_field' | 'propose_group' | 'upsert_field' | 'assign_group';
	entity_type?: string;
	entity_id?: string;
	field_key?: string;
	group_slug?: string;
	entity_class?: string;
	source_url?: string;
	value?: unknown;
	field?: WikiFieldDefInput;
	group?: WikiGroupDefInput;
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
	/** Last ops-application failure on a pending fields/mixed revision (§80). */
	apply_error?: string;
}

export interface EntityRevisionsPage {
	items: WikiEntityRevision[];
	page: number;
	limit: number;
	total: number;
}

/** GET /wiki/field-types — the canonical proposable field types
 *  ({value, label}); labels are server-English, callers may override via
 *  i18n. Returns [] on failure — the propose form hides itself. */
export async function getFieldTypes(): Promise<Array<{ value: string; label: string }>> {
	const res = await api.get<Array<{ value: string; label: string }>>('/wiki/field-types');
	return res.data ?? [];
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
 *  `pagination` object carries page/limit/total. Returns null on error so
 *  callers can distinguish a failure from an empty history. */
export async function getEntityRevisions(
	entityType: string,
	entityId: string,
	params: { page?: number; limit?: number } = {}
): Promise<EntityRevisionsPage | null> {
	const qs = new URLSearchParams();
	qs.set('page', String(params.page ?? 1));
	qs.set('limit', String(params.limit ?? 20));
	const res = await api.getEnvelope<WikiEntityRevision[]>(
		`/wiki/entities/${encodeURIComponent(entityType)}/${encodeURIComponent(entityId)}/revisions?${qs.toString()}`
	);
	if (res.error) return null;
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
