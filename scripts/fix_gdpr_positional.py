#!/usr/bin/env python3
"""Fix gdpr/store.go and gdpr/service.go for de-tenanted positional queries."""
from pathlib import Path
import re

STORE = Path("internal/gdpr/store.go")
SERVICE = Path("internal/gdpr/service.go")

# Map generated method name to the helper name in store.go.
METHODS = [
    ("createDataExportRequest", "CreateDataExportRequest", True),
    ("anonymizeUser", "AnonymizeUser", False),
    ("anonymizeUserProfile", "AnonymizeUserProfile", False),
    ("anonymizeMentorshipConnections", "AnonymizeMentorshipConnections", False),
    ("anonymizeAffiliateLinkEvents", "AnonymizeAffiliateLinkEvents", False),
    ("deleteUserTokens", "DeleteUserTokens", False),
    ("deleteUserTOTPCredentials", "DeleteUserTOTPCredentials", False),
    ("deleteUserOAuthAccounts", "DeleteUserOAuthAccounts", False),
    ("deleteUserPasswordResetTokens", "DeleteUserPasswordResetTokens", False),
    ("deleteUserEmailVerifications", "DeleteUserEmailVerifications", False),
]


def fix_store(text: str) -> str:
    for helper, gen_name, returns in METHODS:
        # signature
        text = re.sub(
            rf"func \(s \*Store\) {helper}\(ctx context\.Context, p gen\.{gen_name}Params\)",
            f"func (s *Store) {helper}(ctx context.Context, userID pgtype.UUID)",
            text,
        )
        # call
        text = re.sub(
            rf"s\.write\.{gen_name}\(ctx, p\)",
            f"s.write.{gen_name}(ctx, userID)",
            text,
        )
    return text


def fix_service(text: str) -> str:
    # createDataExportRequest is called with userID, not request.UserID
    text = re.sub(
        r"s\.store\.createDataExportRequest\(ctx, gen\.CreateDataExportRequestParams\{\s*UserID:\s*([^,\s]+),\s*\}\)",
        r"s.store.createDataExportRequest(ctx, \1)",
        text,
        flags=re.S,
    )
    # All other helpers are called with request.UserID
    for helper, gen_name, _ in METHODS[1:]:
        text = re.sub(
            rf"s\.store\.{helper}\(ctx, gen\.{gen_name}Params\{{\s*UserID:\s*request\.UserID,\s*\}}\)",
            f"s.store.{helper}(ctx, request.UserID)",
            text,
            flags=re.S,
        )
    return text


if __name__ == "__main__":
    for path, fn in [(STORE, fix_store), (SERVICE, fix_service)]:
        text = path.read_text()
        new_text = fn(text)
        if new_text != text:
            path.write_text(new_text)
            print(f"patched {path}")
