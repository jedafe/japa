export interface User {
	id: string;
	username: string;
	display_name: string;
	email: string;
	avatar_url: string | null;
	user_group_id: string;
	theme_preference: 'light' | 'dark' | 'system';
	timezone: string;
	onboarding_completed: boolean;
	is_verified: boolean;
	created_at: string;
}

export interface FeatureFlags {
	[key: string]: boolean;
}

export interface LearnTrack {
	id: string;
	slug: string;
	title: string;
	description: string;
	thumbnail_url: string | null;
	module_count: number;
	lesson_count: number;
	difficulty: 'beginner' | 'intermediate' | 'advanced';
	estimated_hours: number;
	is_featured: boolean;
	price: number;
	is_pro: boolean;
	instructor: {
		id: string;
		username: string;
		display_name: string;
		avatar_url: string | null;
	};
	modules: LearnModule[];
	created_at: string;
	updated_at: string;
}

export interface LearnModule {
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
	lesson_type: 'article' | 'video' | 'quiz' | 'checklist';
	duration_minutes: number;
	is_preview: boolean;
	order: number;
}

export interface LearnLesson {
	id: string;
	slug: string;
	title: string;
	body: Record<string, unknown>;
	lesson_type: 'article' | 'video' | 'quiz' | 'checklist';
	duration_minutes: number;
	is_preview: boolean;
	order: number;
	module_id: string;
	track_slug: string;
	module_slug: string;
	video_url: string | null;
	progress: LessonProgress | null;
}

export interface LessonProgress {
	status: 'not_started' | 'in_progress' | 'completed';
	progress_secs: number;
	quiz_passed: boolean;
	completed_at: string | null;
}

export interface Enrollment {
	id: string;
	track_id: string;
	track: LearnTrack;
	progress_pct: number;
	last_lesson_id: string | null;
	enrolled_at: string;
	completed_at: string | null;
}

export interface Certificate {
	id: string;
	track_id: string;
	track_title: string;
	student_name: string;
	issued_at: string;
	download_url: string;
	share_url: string;
}

export interface ReviewAggregate {
	average_rating: number;
	total_reviews: number;
	distribution: { 1: number; 2: number; 3: number; 4: number; 5: number };
}

export interface Review {
	id: string;
	user: { id: string; username: string; display_name: string; avatar_url: string | null };
	rating: number;
	title: string;
	body: string;
	created_at: string;
	updated_at: string;
}

export interface LessonQuiz {
	id: string;
	lesson_id: string;
	questions: QuizQuestion[];
}

export interface QuizQuestion {
	id: string;
	question: string;
	options: { id: string; text: string }[];
	correct_index?: number;
}

export interface QuizResult {
	score: number;
	passed: boolean;
	correct_answers: { question_id: string; correct_option_id: string; explanation: string }[];
}

export interface PaginatedResponse<T> {
	data: T[];
	total: number;
	page: number;
	per_page: number;
	total_pages: number;
}
