#!/usr/bin/env python3
"""Fix websocket tests for single-tenant hub/pubsub."""
from pathlib import Path
import re

ws = Path("internal/websocket")

# pubsub_test.go
pubsub_test = ws / "pubsub_test.go"
text = pubsub_test.read_text()

# Remove TargetTenantID and TenantBroadcast from PubSubMessage literals
text = re.sub(r'\n\t+TargetTenantID: "[^"]+",', '', text)
text = re.sub(r'\n\t+TenantBroadcast: true,', '', text)

# Update BroadcastToSpace calls from two args to one
text = re.sub(r'hub\.BroadcastToSpace\("[^"]+", "([^"]+)",', r'hub.BroadcastToSpace("\1",', text)

pubsub_test.write_text(text)
print("patched pubsub_test.go")

# websocket_test.go
wt = ws / "websocket_test.go"
text = wt.read_text()

# BroadcastToSpace
for i in range(1, 5):
    text = re.sub(r'hub\.BroadcastToSpace\("tenant-%d", "space-%d"' % (i, i), r'hub.BroadcastToSpace("space-%d"' % i, text)

# It might be "tenant-1" and "space-1" etc.
text = re.sub(r'hub\.BroadcastToSpace\("tenant-([^"]+)", "space-([^"]+)"', r'hub.BroadcastToSpace("space-\2"', text)

# Update hub.spaceSubscribers key lookups from "tenant-X:space-Y" to "space-Y"
text = re.sub(r'"tenant-\d+:', '"', text)

# SessionChecker in Client literal
text = re.sub(
    r'SessionChecker:\s+func\(userID, tenantID string\) bool \{\n\t\t\t\treturn true\n\t\t\t\}',
    'SessionChecker: func(userID string) bool {\n\t\t\t\treturn true\n\t\t\t}',
    text,
)

# SessionChecker call
text = text.replace('client.SessionChecker(client.UserID, client.TenantID)', 'client.SessionChecker(client.UserID)')

wt.write_text(text)
print("patched websocket_test.go")
