-- Quiz attempt limiting (security review residual): without a cap, an enrollee
-- could brute-force submissions indefinitely. 10 attempts per user per quiz
-- lesson bounds the blind-retake oracle to something that can't crack a quiz.
CREATE TABLE IF NOT EXISTS public.academy_quiz_attempts (
    user_id    uuid        NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    lesson_id  uuid        NOT NULL REFERENCES public.academy_lessons(id) ON DELETE CASCADE,
    attempts   integer     NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, lesson_id)
);
