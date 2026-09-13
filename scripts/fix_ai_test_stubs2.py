#!/usr/bin/env python3
"""Remove remaining tenant checks in ai handler test stubs."""
from pathlib import Path
import re

p = Path("internal/ai/agent_handlers_test.go")
text = p.read_text()

# Remove tenant checks in GetAgentSessionForUser and ListAgentSessionsByUser
# Replace `if !ok || sess.UserID != arg.UserID || sess.TenantID != arg.TenantID {`
# with `if !ok || sess.UserID != arg.UserID {`
text = re.sub(
    r"if !ok \|\| sess\.UserID != arg\.UserID \|\| sess\.TenantID != arg\.TenantID \{",
    "if !ok || sess.UserID != arg.UserID {",
    text,
)

# Remove tenant check in ListAgentSessionsForAdmin
# `if sess.TenantID != arg.TenantID {` -> remove whole block
text = re.sub(
    r"\n\tfor _, sess := range s\.sessions \{\n\t\tif sess\.TenantID != arg\.TenantID \{\n\t\t\tcontinue\n\t\t\}\n\n",
    "\n\tfor _, sess := range s.sessions {\n\t\t",
    text,
)

# Reassign: `if !ok || row.TenantID != arg.TenantID {` -> `if !ok {`
text = re.sub(
    r"row, ok := s\.contents\[arg\.ID\.String\(\)\]\n\tif !ok \|\| row\.TenantID != arg\.TenantID \{",
    "row, ok := s.contents[arg.ID.String()]\n\tif !ok {",
    text,
)

# Delete `otherTID` declaration if unused
text = re.sub(
    r"\n\totherTID, _ := uuid\.Parse\(\"[0-9a-f-]+\"\)\n",
    "\n",
    text,
)
text = re.sub(
    r"\n\totherTID := pgtype\.UUID\{\}\n",
    "\n",
    text,
)

p.write_text(text)
print("patched agent_handlers_test.go")
