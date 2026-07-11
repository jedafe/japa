import { PUBLIC_JC_API_URL } from '$env/static/public';

const API_BASE = `${PUBLIC_JC_API_URL}/api/v1`;

interface ApiOk<T> {
	data: T;
}

interface ApiErr {
	error: { code: string; message: string };
}

let jwtToken: string | null = null;

export function setToken(token: string | null) {
	jwtToken = token;
}

export function getToken(): string | null {
	return jwtToken;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
		...Object.fromEntries(Object.entries(init?.headers ?? {}))
	};
	if (jwtToken) {
		headers['Authorization'] = `Bearer ${jwtToken}`;
	}

	const res = await fetch(`${API_BASE}${path}`, {
		...init,
		headers
	});

	const body = (await res.json()) as ApiOk<T> | ApiErr;
	if (!res.ok || 'error' in body) {
		const err = 'error' in body ? body.error : { code: 'unknown', message: res.statusText };
		throw new Error(`${err.code}: ${err.message}`);
	}
	return (body as ApiOk<T>).data;
}

export const api = {
	listTracks() {
		return request<Track[]>('/learn/tracks');
	},
	getTrack(slug: string) {
		return request<TrackDetail>('/learn/tracks/' + slug);
	},
	getLesson(trackSlug: string, moduleSlug: string, lessonSlug: string) {
		return request<Lesson>(
			`/learn/tracks/${trackSlug}/modules/${moduleSlug}/lessons/${lessonSlug}`
		);
	},
	updateProgress(lessonId: string, data: { status: string }) {
		return request<{ id: string; status: string; completed_at: string | null }>('/learn/lessons/' + lessonId + '/progress', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	completeLesson(lessonId: string) {
		return request<{ id: string; status: string; completed_at: string | null }>('/learn/lessons/' + lessonId + '/complete', {
			method: 'POST'
		});
	},
	getProgress(trackId: string) {
		return request<{ completed: number; total: number; percent: number }>(
			'/learn/progress/' + trackId
		);
	},
	getCertificates() {
		return request<Certificate[]>('/me/certificates');
	},
	enroll(trackId: string) {
		return request<{ enrollment: Enrollment }>('/me/enrollments', {
			method: 'POST',
			body: JSON.stringify({ track_id: trackId })
		});
	},
	submitQuiz(quizId: string, answers: Record<string, number>) {
		return request<QuizResult>('/learn/quizzes/' + quizId + '/submit', {
			method: 'POST',
			body: JSON.stringify({ answers })
		});
	},
	getCertificate(certId: string) {
		return request<CertificateDetail>('/certificates/' + certId);
	},
	downloadCertificate(certId: string) {
		return request<{ download_url?: string; certificate_number?: string; message?: string }>('/certificates/' + certId + '/download');
	}
};

// Types
export interface Track {
	id: string;
	slug: string;
	title: string;
	description: string;
	thumbnail_url?: string;
	module_count: number;
	lesson_count: number;
	difficulty: string;
	estimated_hours: number;
}

export interface TrackDetail extends Track {
	modules: Module[];
}

export interface Module {
	id: string;
	slug: string;
	title: string;
	description: string;
	order: number;
	lessons: LessonSummary[];
}

export interface LessonSummary {
	id: string;
	slug: string;
	title: string;
	duration_minutes: number;
	order: number;
}

export interface Lesson {
	id: string;
	slug: string;
	title: string;
	body: string;
	content_type: string;
	duration_minutes: number;
	order: number;
	module_id: string;
	video_url?: string;
	quiz_id?: string;
	quiz?: QuizQuestion[];
}

export interface QuizQuestion {
	id: string;
	question: string;
	options: string[];
	explanation?: string;
	correct_index?: number;
}

export interface Certificate {
	id: string;
	track_id: string;
	certificate_number: string;
	issued_at: string;
	media_id?: string;
}

export interface Enrollment {
	id: string;
	track_id: string;
	enrolled_at: string;
	status: string;
	progress_pct?: number;
}

export interface QuizResult {
	score: number;
	passed: boolean;
	correct_answers: number;
	total: number;
	correct: string[];
}

export interface CertificateDetail {
	id: string;
	track_id: string;
	track_title: string;
	user_id: string;
	username: string;
	issued_at: string;
	certificate_number: string;
	completed_at: string;
}
