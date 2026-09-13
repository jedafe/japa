#!/usr/bin/env python3
"""Update ai agent handler test stubs to match the de-tenanted positional interfaces."""
from pathlib import Path
import re

AGENT_HANDLERS_TEST = Path("internal/ai/agent_handlers_test.go")
text = AGENT_HANDLERS_TEST.read_text()

# CountAgentSessionsByUser
replacements = [
    (r"func \(s \*stubAgentSessions\) CountAgentSessionsByUser\(_ context.Context, arg gen\.CountAgentSessionsByUserParams\) \(int64, error\) \{\n\trows, _ := s\.ListAgentSessionsByUser\(context\.Background\(\), gen\.ListAgentSessionsByUserParams\{\n\n\t\tUserID: arg\.UserID,\n\t\}\)",
     r"func (s *stubAgentSessions) CountAgentSessionsByUser(_ context.Context, userID pgtype.UUID) (int64, error) {\n\trows, _ := s.ListAgentSessionsByUser(context.Background(), gen.ListAgentSessionsByUserParams{\n\n\t\tUserID: userID,\n\t})",),

    (r"func \(s \*stubAgentSessions\) ListAgentToolCallsBySession\(_ context.Context, arg gen\.ListAgentToolCallsBySessionParams\) \(\[\]gen\.AiAgentToolCall, error\) \{\n\treturn s\.toolCalls\[arg\.SessionID\.String\(\)\], nil",
     r"func (s *stubAgentSessions) ListAgentToolCallsBySession(_ context.Context, sessionID pgtype.UUID) ([]gen.AiAgentToolCall, error) {\n\treturn s.toolCalls[sessionID.String()], nil"),

    (r"func \(s \*stubAgentSessions\) CancelAgentSession\(_ context.Context, arg gen\.CancelAgentSessionParams\) \(gen\.AiAgentSession, error\) \{\n\tsess, ok := s\.sessions\[arg\.ID\.String\(\)\]\n\tif !ok \|\| sess\.TenantID != arg\.TenantID \{\n\t\treturn gen\.AiAgentSession\{\}, pgx\.ErrNoRows\n\t\}\n\n\tif s\.cancelRace \{\n\t\treturn gen\.AiAgentSession\{\}, pgx\.ErrNoRows\n\t\}\n\n\tif sess\.Status != \"running\" \{\n\t\treturn gen\.AiAgentSession\{\}, pgx\.ErrNoRows\n\t\}\n\n\tsess\.Status = \"cancelled\"\n\ts\.sessions\[arg\.ID\.String\(\)\] = sess\n\ts\.cancelledID = &arg\.ID",
     r"func (s *stubAgentSessions) CancelAgentSession(_ context.Context, id pgtype.UUID) (gen.AiAgentSession, error) {\n\tsess, ok := s.sessions[id.String()]\n\tif !ok {\n\t\treturn gen.AiAgentSession{}, pgx.ErrNoRows\n\t}\n\n\tif s.cancelRace {\n\t\treturn gen.AiAgentSession{}, pgx.ErrNoRows\n\t}\n\n\tif sess.Status != \"running\" {\n\t\treturn gen.AiAgentSession{}, pgx.ErrNoRows\n\t}\n\n\tsess.Status = \"cancelled\"\n\ts.sessions[id.String()] = sess\n\ts.cancelledID = &id"),

    (r"func \(s \*stubAgentSessions\) GetAgentSession\(_ context.Context, arg gen\.GetAgentSessionParams\) \(gen\.AiAgentSession, error\) \{\n\tsess, ok := s\.sessions\[arg\.ID\.String\(\)\]\n\tif !ok \|\| sess\.TenantID != arg\.TenantID \{\n\t\treturn gen\.AiAgentSession\{\}, pgx\.ErrNoRows\n\t\}\n\n\treturn sess, nil\n\}",
     r"func (s *stubAgentSessions) GetAgentSession(_ context.Context, id pgtype.UUID) (gen.AiAgentSession, error) {\n\tsess, ok := s.sessions[id.String()]\n\tif !ok {\n\t\treturn gen.AiAgentSession{}, pgx.ErrNoRows\n\t}\n\n\treturn sess, nil\n}"),

    (r"func \(s \*stubAgentSessions\) CountAgentSessionsForAdmin\(_ context.Context, arg gen\.CountAgentSessionsForAdminParams\) \(int64, error\) \{\n\trows, _ := s\.ListAgentSessionsForAdmin\(context\.Background\(\), gen\.ListAgentSessionsForAdminParams\{\n\n\t\tStatusFilter: arg\.StatusFilter,\n\t\}\)",
     r"func (s *stubAgentSessions) CountAgentSessionsForAdmin(_ context.Context, statusFilter string) (int64, error) {\n\trows, _ := s.ListAgentSessionsForAdmin(context.Background(), gen.ListAgentSessionsForAdminParams{\n\n\t\tStatusFilter: statusFilter,\n\t})"),

    (r"func \(s \*stubAgentSessions\) GetContentAIExempt\(_ context.Context, _ gen\.GetContentAIExemptParams\) \(bool, error\) \{\n\treturn s\.contentExempt, nil\n\}",
     r"func (s *stubAgentSessions) GetContentAIExempt(_ context.Context, id pgtype.UUID) (bool, error) {\n\t_ = id\n\treturn s.contentExempt, nil\n}"),

    (r"func \(s \*stubAgentSessions\) GetCustomFieldValueAIExempt\(_ context.Context, _ gen\.GetCustomFieldValueAIExemptParams\) \(bool, error\) \{\n\treturn s\.cfvExempt, nil\n\}",
     r"func (s *stubAgentSessions) GetCustomFieldValueAIExempt(_ context.Context, id pgtype.UUID) (bool, error) {\n\t_ = id\n\treturn s.cfvExempt, nil\n}"),

    (r"func \(s \*stubAgentSessions\) GetCustomFieldValueEntity\(_ context.Context, arg gen\.GetCustomFieldValueEntityParams\) \(gen\.GetCustomFieldValueEntityRow, error\) \{\n\tif row, ok := s\.cfvEntities\[arg\.ID\.String\(\)\]; ok \{\n\t\treturn row, nil\n\t\}\n\n\treturn gen\.GetCustomFieldValueEntityRow\{\}, pgx\.ErrNoRows\n\}",
     r"func (s *stubAgentSessions) GetCustomFieldValueEntity(_ context.Context, id pgtype.UUID) (gen.GetCustomFieldValueEntityRow, error) {\n\tif row, ok := s.cfvEntities[id.String()]; ok {\n\t\treturn row, nil\n\t}\n\n\treturn gen.GetCustomFieldValueEntityRow{}, pgx.ErrNoRows\n}"),

    (r"func \(s \*stubAgentSessions\) GetUserAIExempt\(_ context.Context, _ gen\.GetUserAIExemptParams\) \(bool, error\) \{\n\treturn s\.userExempt, nil\n\}",
     r"func (s *stubAgentSessions) GetUserAIExempt(_ context.Context, id pgtype.UUID) (bool, error) {\n\t_ = id\n\treturn s.userExempt, nil\n}"),

    (r"func \(s \*stubContentStore\) GetContentByID\(_ context.Context, arg gen\.GetContentByIDParams\) \(gen\.GetContentByIDRow, error\) \{\n\trow, ok := s\.contents\[arg\.ID\.String\(\)\]\n\tif !ok \|\| row\.TenantID != arg\.TenantID \{\n\t\treturn gen\.GetContentByIDRow\{\}, pgx\.ErrNoRows\n\t\}\n\n\treturn row, nil\n\}",
     r"func (s *stubContentStore) GetContentByID(_ context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error) {\n\trow, ok := s.contents[id.String()]\n\tif !ok {\n\t\treturn gen.GetContentByIDRow{}, pgx.ErrNoRows\n\t}\n\n\treturn row, nil\n}"),
]

for old, new in replacements:
    text = re.sub(old, new, text)

AGENT_HANDLERS_TEST.write_text(text)
print("patched agent_handlers_test.go")
