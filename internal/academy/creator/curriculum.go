// Curriculum authoring for Japa Academy creators (ADR-063): modules, lessons,
// and quiz questions on the instructor's own tracks. Platform admins
// (super_admin / academy.admin.manage) may manage the curriculum of ANY
// track — they are exempt from the ownership check.
//
// Route surface (wired in internal/server/server.go):
//
//	GET    /academy/my/tracks/{id}/curriculum
//	POST   /academy/my/tracks/{id}/modules
//	PATCH  /academy/my/modules/{id}
//	DELETE /academy/my/modules/{id}
//	POST   /academy/my/modules/{id}/lessons
//	PATCH  /academy/my/lessons/{id}
//	DELETE /academy/my/lessons/{id}
//	GET    /academy/my/lessons/{id}/questions
//	PUT    /academy/my/lessons/{id}/questions
package creator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Access control
// ---------------------------------------------------------------------------

// canManageAnyTrack reports whether the user holds platform-level academy
// admin rights — super_admin group membership or the academy.admin.manage
// node. Admins may author the curriculum of any track regardless of the
// instructor owner.
func (s *Service) canManageAnyTrack(ctx context.Context, userID pgtype.UUID) bool {
	if s.adminChecker == nil {
		return false
	}

	return s.adminChecker.IsSuperAdmin(ctx, userID) ||
		s.adminChecker.HasPermission(ctx, userID, "academy.admin.manage")
}

// resolveOwnedTrack returns the track row when the caller may manage its
// curriculum: platform admins always may; otherwise the caller's instructor
// profile must own the track (academy_tracks.instructor_id).
func (s *Service) resolveOwnedTrack(ctx context.Context, userID, trackID pgtype.UUID) (gen.GetInstructorTrackByIDRow, error) {
	track, err := s.store.read.GetInstructorTrackByID(ctx, trackID)
	if err != nil {
		return gen.GetInstructorTrackByIDRow{}, err
	}

	if s.canManageAnyTrack(ctx, userID) {
		return track, nil
	}

	prof, err := s.ResolveSelfProfile(ctx, userID)
	if err != nil {
		return gen.GetInstructorTrackByIDRow{}, err
	}

	if !track.InstructorID.Valid || track.InstructorID != prof.ID {
		return gen.GetInstructorTrackByIDRow{}, fmt.Errorf("%w: you do not own this track", errpkg.ErrForbidden)
	}

	return track, nil
}

// resolveModuleTrack resolves module → track and applies the ownership rule.
func (s *Service) resolveModuleTrack(ctx context.Context, userID, moduleID pgtype.UUID) (gen.AcademyModule, gen.GetInstructorTrackByIDRow, error) {
	mod, err := s.store.read.GetAcademyModuleByID(ctx, moduleID)
	if err != nil {
		return gen.AcademyModule{}, gen.GetInstructorTrackByIDRow{}, err
	}

	track, err := s.resolveOwnedTrack(ctx, userID, mod.TrackID)
	if err != nil {
		return gen.AcademyModule{}, gen.GetInstructorTrackByIDRow{}, err
	}

	return mod, track, nil
}

// resolveLessonTrack resolves lesson → module → track and applies the
// ownership rule.
func (s *Service) resolveLessonTrack(ctx context.Context, userID, lessonID pgtype.UUID) (gen.GetAcademyLessonByIDRow, gen.GetInstructorTrackByIDRow, error) {
	lesson, err := s.store.read.GetAcademyLessonByID(ctx, lessonID)
	if err != nil {
		return gen.GetAcademyLessonByIDRow{}, gen.GetInstructorTrackByIDRow{}, err
	}

	mod, err := s.store.read.GetAcademyModuleByID(ctx, lesson.ModuleID)
	if err != nil {
		return gen.GetAcademyLessonByIDRow{}, gen.GetInstructorTrackByIDRow{}, err
	}

	track, err := s.resolveOwnedTrack(ctx, userID, mod.TrackID)
	if err != nil {
		return gen.GetAcademyLessonByIDRow{}, gen.GetInstructorTrackByIDRow{}, err
	}

	return lesson, track, nil
}

// ---------------------------------------------------------------------------
// DTOs
// ---------------------------------------------------------------------------

// LessonDTO is the editor-facing lesson record. The generated row types
// differ per query (list/get/create/update) but carry identical fields —
// this unifies them for the API contract.
type LessonDTO struct {
	ID              string             `json:"id"`
	ModuleID        string             `json:"module_id"`
	Title           string             `json:"title"`
	Slug            string             `json:"slug"`
	LessonType      string             `json:"lesson_type"`
	Content         *string            `json:"content,omitempty"`
	MediaID         *string            `json:"media_id,omitempty"`
	DurationMinutes *int32             `json:"duration_minutes,omitempty"`
	SortOrder       int32              `json:"sort_order"`
	IsPublished     bool               `json:"is_published"`
	IsPreview       bool               `json:"is_preview"`
	QuestionCount   int                `json:"question_count"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
	UpdatedAt       pgtype.Timestamptz `json:"updated_at"`
}

func lessonDTOFromGetRow(r gen.GetAcademyLessonByIDRow) LessonDTO {
	return LessonDTO{
		ID: r.ID.String(), ModuleID: r.ModuleID.String(), Title: r.Title,
		Slug: r.Slug, LessonType: r.LessonType, Content: r.Content,
		MediaID: uuidPtr(r.MediaID), DurationMinutes: r.DurationMinutes,
		SortOrder: r.SortOrder, IsPublished: r.IsPublished, IsPreview: r.IsPreview,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func lessonDTOFromListRow(r gen.ListAcademyLessonsByModuleRow) LessonDTO {
	return LessonDTO{
		ID: r.ID.String(), ModuleID: r.ModuleID.String(), Title: r.Title,
		Slug: r.Slug, LessonType: r.LessonType, Content: r.Content,
		MediaID: uuidPtr(r.MediaID), DurationMinutes: r.DurationMinutes,
		SortOrder: r.SortOrder, IsPublished: r.IsPublished, IsPreview: r.IsPreview,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func lessonDTOFromCreateRow(r gen.CreateAcademyLessonRow) LessonDTO {
	return LessonDTO{
		ID: r.ID.String(), ModuleID: r.ModuleID.String(), Title: r.Title,
		Slug: r.Slug, LessonType: r.LessonType, Content: r.Content,
		MediaID: uuidPtr(r.MediaID), DurationMinutes: r.DurationMinutes,
		SortOrder: r.SortOrder, IsPublished: r.IsPublished, IsPreview: r.IsPreview,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func lessonDTOFromUpdateRow(r gen.UpdateAcademyLessonRow) LessonDTO {
	return LessonDTO{
		ID: r.ID.String(), ModuleID: r.ModuleID.String(), Title: r.Title,
		Slug: r.Slug, LessonType: r.LessonType, Content: r.Content,
		MediaID: uuidPtr(r.MediaID), DurationMinutes: r.DurationMinutes,
		SortOrder: r.SortOrder, IsPublished: r.IsPublished, IsPreview: r.IsPreview,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func uuidPtr(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	s := id.String()
	return &s
}

// CurriculumModule is a module with its lessons for the curriculum editor.
type CurriculumModule struct {
	gen.AcademyModule
	Lessons []LessonDTO `json:"lessons"`
}

// Curriculum is the full editable curriculum for one track.
type Curriculum struct {
	Track   gen.GetInstructorTrackByIDRow `json:"track"`
	Modules []CurriculumModule            `json:"modules"`
}

// ModuleInput is the create/patch body for a module.
type ModuleInput struct {
	Title       string  `json:"title"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
	SortOrder   *int32  `json:"sort_order,omitempty"`
}

// Lesson types supported by the curriculum editor and the lesson player.
var validLessonTypes = map[string]bool{
	"article":    true,
	"quiz":       true,
	"video":      true,
	"document":   true,
	"youtube":    true,
	"screencast": true,
	"audio":      true,
	"checklist":  true,
	"link":       true,
}

// LessonInput is the create/patch body for a lesson. Nullable fields are
// left untouched on PATCH (partial-update safe).
type LessonInput struct {
	Title           string  `json:"title"`
	Slug            *string `json:"slug,omitempty"`
	LessonType      *string `json:"lesson_type,omitempty"`
	Content         *string `json:"content,omitempty"`
	MediaID         *string `json:"media_id,omitempty"`
	DurationMinutes *int32  `json:"duration_minutes,omitempty"`
	SortOrder       *int32  `json:"sort_order,omitempty"`
	IsPublished     *bool   `json:"is_published,omitempty"`
	IsPreview       *bool   `json:"is_preview,omitempty"`
}

// QuizQuestionInput is one question in the replace-set quiz writer.
type QuizQuestionInput struct {
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Answer      string   `json:"answer"`
	Explanation *string  `json:"explanation,omitempty"`
	Position    int32    `json:"position"`
}

// QuizQuestionDTO is the editor-facing question record (includes the
// correct answer — creator endpoints only).
type QuizQuestionDTO struct {
	ID          string   `json:"id"`
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Answer      string   `json:"answer"`
	Explanation *string  `json:"explanation,omitempty"`
	Position    int32    `json:"position"`
}

// decodeQuizOptions normalizes the options text column to a plain string
// list (mirrors academy.parseQuizOptions — labels only, no correctness).
func decodeQuizOptions(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	var objects []struct {
		Text  string `json:"text"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal([]byte(trimmed), &objects); err == nil {
		out := make([]string, 0, len(objects))
		for _, o := range objects {
			l := o.Text
			if l == "" {
				l = o.Label
			}
			if l != "" {
				out = append(out, l)
			}
		}
		return out
	}

	var labels []string
	if err := json.Unmarshal([]byte(trimmed), &labels); err == nil {
		return labels
	}

	return nil
}

// ---------------------------------------------------------------------------
// Service — curriculum operations
// ---------------------------------------------------------------------------

// GetMyCurriculum returns the full module/lesson tree for the editor.
func (s *Service) GetMyCurriculum(ctx context.Context, userID, trackID pgtype.UUID) (*Curriculum, error) {
	track, err := s.resolveOwnedTrack(ctx, userID, trackID)
	if err != nil {
		return nil, err
	}

	mods, err := s.store.read.ListAcademyModulesByTrack(ctx, trackID)
	if err != nil {
		return nil, fmt.Errorf("list modules: %w", err)
	}

	cur := &Curriculum{Track: track, Modules: make([]CurriculumModule, 0, len(mods))}

	for _, m := range mods {
		lessons, lerr := s.store.read.ListAcademyLessonsByModule(ctx, m.ID)
		if lerr != nil {
			return nil, fmt.Errorf("list lessons: %w", lerr)
		}

		cm := CurriculumModule{AcademyModule: m, Lessons: make([]LessonDTO, 0, len(lessons))}

		for _, l := range lessons {
			dto := lessonDTOFromListRow(l)
			if l.LessonType == "quiz" {
				if qs, qerr := s.store.read.GetQuizQuestionsByLesson(ctx, l.ID); qerr == nil {
					dto.QuestionCount = len(qs)
				}
			}
			cm.Lessons = append(cm.Lessons, dto)
		}

		cur.Modules = append(cur.Modules, cm)
	}

	return cur, nil
}

// CreateMyModule creates a module on an owned track.
func (s *Service) CreateMyModule(ctx context.Context, userID, trackID pgtype.UUID, in ModuleInput) (gen.AcademyModule, error) {
	if in.Title == "" {
		return gen.AcademyModule{}, errors.New("title is required")
	}

	if _, err := s.resolveOwnedTrack(ctx, userID, trackID); err != nil {
		return gen.AcademyModule{}, err
	}

	slug := moduleSlug(in.Slug, in.Title)

	sortOrder := int32(0)
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	} else {
		// Default: append after the highest existing sort_order.
		if mods, err := s.store.read.ListAcademyModulesByTrack(ctx, trackID); err == nil && len(mods) > 0 {
			sortOrder = mods[len(mods)-1].SortOrder + 1
		}
	}

	var mod gen.AcademyModule
	err := s.withMutationAndReset(ctx, trackID, func(q *gen.Queries) error {
		var e error
		mod, e = q.CreateAcademyModule(ctx, gen.CreateAcademyModuleParams{
			TrackID:     trackID,
			Title:       in.Title,
			Slug:        slug,
			Description: in.Description,
			SortOrder:   sortOrder,
		})
		return e
	})
	if err != nil {
		return mod, err
	}

	return mod, nil
}

// UpdateMyModule patches a module (partial-update safe).
func (s *Service) UpdateMyModule(ctx context.Context, userID, moduleID pgtype.UUID, in ModuleInput) (gen.AcademyModule, error) {
	_, track, err := s.resolveModuleTrack(ctx, userID, moduleID)
	if err != nil {
		return gen.AcademyModule{}, err
	}

	var title *string
	if in.Title != "" {
		title = &in.Title
	}

	var mod gen.AcademyModule
	err = s.withMutationAndReset(ctx, track.ID, func(q *gen.Queries) error {
		var e error
		mod, e = q.UpdateAcademyModule(ctx, gen.UpdateAcademyModuleParams{
			ID:          moduleID,
			Title:       title,
			Description: in.Description,
			SortOrder:   in.SortOrder,
		})
		return e
	})
	if err != nil {
		return mod, err
	}

	return mod, nil
}

// DeleteMyModule deletes a module (cascades to its lessons).
func (s *Service) DeleteMyModule(ctx context.Context, userID, moduleID pgtype.UUID) error {
	_, track, err := s.resolveModuleTrack(ctx, userID, moduleID)
	if err != nil {
		return err
	}

	return s.withMutationAndReset(ctx, track.ID, func(q *gen.Queries) error {
		return q.DeleteAcademyModule(ctx, moduleID)
	})
}

// CreateMyLesson creates a lesson inside an owned module.
func (s *Service) CreateMyLesson(ctx context.Context, userID, moduleID pgtype.UUID, in LessonInput) (LessonDTO, error) {
	if in.Title == "" {
		return LessonDTO{}, errors.New("title is required")
	}

	_, track, err := s.resolveModuleTrack(ctx, userID, moduleID)
	if err != nil {
		return LessonDTO{}, err
	}

	lessonType := "article"
	if in.LessonType != nil && *in.LessonType != "" {
		lessonType = *in.LessonType
	}
	if !validLessonTypes[lessonType] {
		return LessonDTO{}, fmt.Errorf("invalid lesson_type %q", lessonType)
	}

	slug := moduleSlug(in.Slug, in.Title)

	sortOrder := int32(0)
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	} else {
		if lessons, err := s.store.read.ListAcademyLessonsByModule(ctx, moduleID); err == nil && len(lessons) > 0 {
			sortOrder = lessons[len(lessons)-1].SortOrder + 1
		}
	}

	var mediaID pgtype.UUID
	if in.MediaID != nil && *in.MediaID != "" {
		if err := mediaID.Scan(*in.MediaID); err != nil {
			return LessonDTO{}, errors.New("invalid media_id")
		}
	}

	duration := int32(0)
	if in.DurationMinutes != nil {
		duration = *in.DurationMinutes
	}

	var row gen.CreateAcademyLessonRow
	err = s.withMutationAndReset(ctx, track.ID, func(q *gen.Queries) error {
		var e error
		row, e = q.CreateAcademyLesson(ctx, gen.CreateAcademyLessonParams{
			ModuleID:        moduleID,
			Title:           in.Title,
			Slug:            slug,
			LessonType:      lessonType,
			Content:         in.Content,
			MediaID:         mediaID,
			DurationMinutes: &duration,
			SortOrder:       sortOrder,
			IsPublished:     in.IsPublished != nil && *in.IsPublished,
			IsPreview:       in.IsPreview != nil && *in.IsPreview,
		})
		return e
	})
	if err != nil {
		return LessonDTO{}, err
	}

	return lessonDTOFromCreateRow(row), nil
}

// UpdateMyLesson patches a lesson (partial-update safe).
func (s *Service) UpdateMyLesson(ctx context.Context, userID, lessonID pgtype.UUID, in LessonInput) (LessonDTO, error) {
	_, track, err := s.resolveLessonTrack(ctx, userID, lessonID)
	if err != nil {
		return LessonDTO{}, err
	}

	var title *string
	if in.Title != "" {
		title = &in.Title
	}

	if in.LessonType != nil && *in.LessonType != "" && !validLessonTypes[*in.LessonType] {
		return LessonDTO{}, fmt.Errorf("invalid lesson_type %q", *in.LessonType)
	}

	var mediaID pgtype.UUID
	if in.MediaID != nil && *in.MediaID != "" {
		if err := mediaID.Scan(*in.MediaID); err != nil {
			return LessonDTO{}, errors.New("invalid media_id")
		}
	}

	var row gen.UpdateAcademyLessonRow
	err = s.withMutationAndReset(ctx, track.ID, func(q *gen.Queries) error {
		var e error
		row, e = q.UpdateAcademyLesson(ctx, gen.UpdateAcademyLessonParams{
			ID:              lessonID,
			Title:           title,
			LessonType:      in.LessonType,
			Content:         in.Content,
			MediaID:         mediaID,
			DurationMinutes: in.DurationMinutes,
			SortOrder:       in.SortOrder,
			IsPublished:     in.IsPublished,
			IsPreview:       in.IsPreview,
		})
		return e
	})
	if err != nil {
		return LessonDTO{}, err
	}

	return lessonDTOFromUpdateRow(row), nil
}

// DeleteMyLesson deletes a lesson.
func (s *Service) DeleteMyLesson(ctx context.Context, userID, lessonID pgtype.UUID) error {
	_, track, err := s.resolveLessonTrack(ctx, userID, lessonID)
	if err != nil {
		return err
	}

	return s.withMutationAndReset(ctx, track.ID, func(q *gen.Queries) error {
		return q.DeleteAcademyLesson(ctx, lessonID)
	})
}

// ListMyQuizQuestions returns the quiz questions (with answers) for the
// curriculum editor.
func (s *Service) ListMyQuizQuestions(ctx context.Context, userID, lessonID pgtype.UUID) ([]QuizQuestionDTO, error) {
	if _, _, err := s.resolveLessonTrack(ctx, userID, lessonID); err != nil {
		return nil, err
	}

	rows, err := s.store.read.GetQuizQuestionsByLesson(ctx, lessonID)
	if err != nil {
		return nil, fmt.Errorf("list quiz questions: %w", err)
	}

	out := make([]QuizQuestionDTO, 0, len(rows))
	for _, q := range rows {
		out = append(out, QuizQuestionDTO{
			ID:          q.ID.String(),
			Question:    q.Question,
			Options:     decodeQuizOptions(q.Options),
			Answer:      q.Answer,
			Explanation: q.Explanation,
			Position:    q.Position,
		})
	}

	return out, nil
}

// ReplaceMyQuizQuestions rewrites a quiz lesson's whole question set in one
// call — the editor sends the full ordered list; deleted questions are
// removed, positions rewritten.
func (s *Service) ReplaceMyQuizQuestions(ctx context.Context, userID, lessonID pgtype.UUID, in []QuizQuestionInput) ([]QuizQuestionDTO, error) {
	_, track, err := s.resolveLessonTrack(ctx, userID, lessonID)
	if err != nil {
		return nil, err
	}

	for i, q := range in {
		if strings.TrimSpace(q.Question) == "" {
			return nil, fmt.Errorf("question %d: text is required", i+1)
		}
		if len(q.Options) < 2 {
			return nil, fmt.Errorf("question %d: at least 2 options are required", i+1)
		}
		if strings.TrimSpace(q.Answer) == "" {
			return nil, fmt.Errorf("question %d: a correct answer is required", i+1)
		}
		found := false
		for _, o := range q.Options {
			if strings.EqualFold(strings.TrimSpace(o), strings.TrimSpace(q.Answer)) {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("question %d: the answer must match one of the options", i+1)
		}
	}

	out := make([]QuizQuestionDTO, 0, len(in))
	err = s.withMutationAndReset(ctx, track.ID, func(q *gen.Queries) error {
		// Replace-set: delete all + re-insert must commit together or the
		// quiz is left gutted.
		if e := q.DeleteAcademyQuizQuestionsByLesson(ctx, lessonID); e != nil {
			return fmt.Errorf("clear quiz questions: %w", e)
		}

		for i, qu := range in {
			optionsJSON, _ := json.Marshal(qu.Options)
			position := qu.Position
			if position == 0 {
				position = int32(i + 1) // #nosec G115 -- bounded by question count
			}

			row, e := q.CreateAcademyQuizQuestion(ctx, gen.CreateAcademyQuizQuestionParams{
				LessonID:    lessonID,
				Question:    qu.Question,
				Options:     string(optionsJSON),
				Answer:      qu.Answer,
				Explanation: qu.Explanation,
				Position:    position,
			})
			if e != nil {
				return fmt.Errorf("create quiz question %d: %w", i+1, e)
			}

			out = append(out, QuizQuestionDTO{
				ID:          row.ID.String(),
				Question:    row.Question,
				Options:     qu.Options,
				Answer:      row.Answer,
				Explanation: row.Explanation,
				Position:    row.Position,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// moduleSlug normalizes a provided slug or derives one from the title.
func moduleSlug(provided *string, title string) string {
	if provided != nil && strings.TrimSpace(*provided) != "" {
		return content.Slugify(*provided)
	}

	return content.Slugify(title)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// GetMyCurriculum handles GET /academy/my/tracks/{id}/curriculum.
func (h *Handlers) GetMyCurriculum(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	trackID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	cur, err := h.service.GetMyCurriculum(r.Context(), user.UserID, trackID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cur})
}

// CreateMyModule handles POST /academy/my/tracks/{id}/modules.
func (h *Handlers) CreateMyModule(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	trackID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var in ModuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	mod, err := h.service.CreateMyModule(r.Context(), user.UserID, trackID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": mod})
}

// UpdateMyModule handles PATCH /academy/my/modules/{id}.
func (h *Handlers) UpdateMyModule(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	moduleID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var in ModuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	mod, err := h.service.UpdateMyModule(r.Context(), user.UserID, moduleID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": mod})
}

// DeleteMyModule handles DELETE /academy/my/modules/{id}.
func (h *Handlers) DeleteMyModule(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	moduleID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.DeleteMyModule(r.Context(), user.UserID, moduleID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// CreateMyLesson handles POST /academy/my/modules/{id}/lessons.
func (h *Handlers) CreateMyLesson(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	moduleID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var in LessonInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	lesson, err := h.service.CreateMyLesson(r.Context(), user.UserID, moduleID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": lesson})
}

// UpdateMyLesson handles PATCH /academy/my/lessons/{id}.
func (h *Handlers) UpdateMyLesson(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	lessonID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var in LessonInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	lesson, err := h.service.UpdateMyLesson(r.Context(), user.UserID, lessonID, in)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": lesson})
}

// DeleteMyLesson handles DELETE /academy/my/lessons/{id}.
func (h *Handlers) DeleteMyLesson(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	lessonID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.DeleteMyLesson(r.Context(), user.UserID, lessonID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// ListMyQuizQuestions handles GET /academy/my/lessons/{id}/questions.
func (h *Handlers) ListMyQuizQuestions(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	lessonID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	questions, err := h.service.ListMyQuizQuestions(r.Context(), user.UserID, lessonID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": questions})
}

// ReplaceMyQuizQuestions handles PUT /academy/my/lessons/{id}/questions.
// The body is the full ordered question set — a replace-set write so the
// editor can add/edit/delete/reorder in one shot.
func (h *Handlers) ReplaceMyQuizQuestions(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	lessonID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var in struct {
		Questions []QuizQuestionInput `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	questions, err := h.service.ReplaceMyQuizQuestions(r.Context(), user.UserID, lessonID, in.Questions)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": questions})
}
