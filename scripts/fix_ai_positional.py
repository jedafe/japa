#!/usr/bin/env python3
"""Convert ai handler calls from gen.*Params to positional arguments."""
from pathlib import Path
import re

AGENT_HANDLERS = Path("internal/ai/agent_handlers.go")

CALLS = {
    "CountAgentSessionsByUser": ("CountAgentSessionsByUserParams", "UserID"),
    "ListAgentToolCallsBySession": ("ListAgentToolCallsBySessionParams", "SessionID"),
    "CancelAgentSession": ("CancelAgentSessionParams", "ID"),
    "GetAgentSession": ("GetAgentSessionParams", "ID"),
    "CountAgentSessionsForAdmin": ("CountAgentSessionsForAdminParams", "StatusFilter"),
    "GetContentAIExempt": ("GetContentAIExemptParams", "ID"),
    "GetCustomFieldValueAIExempt": ("GetCustomFieldValueAIExemptParams", "ID"),
    "GetCustomFieldValueEntity": ("GetCustomFieldValueEntityParams", "ID"),
    "GetUserAIExempt": ("GetUserAIExemptParams", "ID"),
}


def fix(text: str) -> str:
    for method, (params, field) in CALLS.items():
        pattern = rf"(h\.sessions\.{method}\([^\)]+\),\s*)gen\.{params}\{{\s*{field}:\s*([^,\s\}}]+),\s*\}}"
        text = re.sub(pattern, rf"\1\2", text, flags=re.S)
    return text


if __name__ == "__main__":
    text = AGENT_HANDLERS.read_text()
    new_text = fix(text)
    if new_text != text:
        AGENT_HANDLERS.write_text(new_text)
        print("patched internal/ai/agent_handlers.go")
