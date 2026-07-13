export type { User } from '../api';
export type { Enrollment } from '../api';

export interface FeatureFlags {
	[key: string]: boolean;
}

export interface PaginatedResponse<T> {
	data: T[];
	total: number;
	page: number;
	per_page: number;
	total_pages: number;
}
