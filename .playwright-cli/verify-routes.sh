#!/bin/bash
# Control App Route Verification Script
# Tests all 13 routes for: successful load, no JS console errors, no raw i18n keys

BASE_URL="http://localhost:3002"
ROUTES=(
  "/"
  "/queue"
  "/dlq"
  "/db"
  "/audit"
  "/security"
  "/api-keys"
  "/cache"
  "/settings"
  "/feature-flags"
  "/tenants"
  "/ai-budget"
  "/cron"
)

ROUTE_NAMES=(
  "Dashboard"
  "Queue"
  "Dead Letter Queue"
  "DB Health"
  "Audit Log"
  "Security"
  "API Keys"
  "Cache"
  "Settings"
  "Feature Flags"
  "Tenants"
  "AI Budget"
  "Cron Jobs"
)

PASS_COUNT=0
FAIL_COUNT=0
TOTAL=${#ROUTES[@]}

echo "=========================================="
echo "  Control App Route Verification"
echo "=========================================="
echo ""

for i in "${!ROUTES[@]}"; do
  ROUTE="${ROUTES[$i]}"
  NAME="${ROUTE_NAMES[$i]}"
  FULL_URL="${BASE_URL}${ROUTE}"
  
  echo "--- [$((i+1))/$TOTAL] Testing: $NAME ($ROUTE) ---"
  
  # Navigate to route
  NAV_OUTPUT=$(playwright-cli goto "$FULL_URL" 2>&1)
  NAV_EXIT=$?
  
  # Check if navigation succeeded (redirect to login means auth failed)
  if echo "$NAV_OUTPUT" | grep -q "/login"; then
    echo "  FAIL: Redirected to login (auth issue)"
    FAIL_COUNT=$((FAIL_COUNT + 1))
    echo ""
    continue
  fi
  
  # Check HTTP status from page info
  HTTP_STATUS=$(echo "$NAV_OUTPUT" | grep -oP 'Page URL: \K.*' | head -1)
  
  # Take snapshot
  SNAP_OUTPUT=$(playwright-cli snapshot 2>&1)
  
  # Check for raw i18n keys (common patterns like "section.key" appearing as text)
  I18N_ISSUES=""
  
  # Check for common i18n key patterns in the snapshot text
  # Look for dotted keys that should be translated
  if echo "$SNAP_OUTPUT" | grep -qP '(?:text|generic)\s+"[a-z]+\.[a-z_]+'; then
    # Extract potential raw keys (excluding known UI patterns)
    RAW_KEYS=$(echo "$SNAP_OUTPUT" | grep -oP '(?:text|generic)\s+"\K[a-z]+\.[a-z_]+' | grep -vP '^(http|www|localhost|page|ref|url|data|aria|class|id|name|type|value|placeholder|label|title|role)' | head -5)
    if [ -n "$RAW_KEYS" ]; then
      I18N_ISSUES="Potential raw i18n keys: $RAW_KEYS"
    fi
  fi
  
  # Check console for errors
  CONSOLE_OUTPUT=$(playwright-cli console 2>&1)
  ERROR_COUNT=$(echo "$CONSOLE_OUTPUT" | grep -oP 'Errors: \K\d+' | head -1)
  WARN_COUNT=$(echo "$CONSOLE_OUTPUT" | grep -oP 'Warnings: \K\d+' | head -1)
  
  # Check for error-level console messages
  HAS_ERRORS=$(echo "$CONSOLE_OUTPUT" | grep -c "\[ERROR\]" || true)
  
  # Determine pass/fail
  STATUS="PASS"
  ISSUES=""
  
  if [ "$HAS_ERRORS" -gt 0 ]; then
    STATUS="FAIL"
    ERROR_MSGS=$(echo "$CONSOLE_OUTPUT" | grep "\[ERROR\]" | head -3)
    ISSUES="${ISSUES}Console errors: $ERROR_MSGS "
  fi
  
  if [ -n "$I18N_ISSUES" ]; then
    STATUS="FAIL"
    ISSUES="${ISSUES}${I18N_ISSUES} "
  fi
  
  # Check if we landed on the expected URL
  CURRENT_URL=$(echo "$NAV_OUTPUT" | grep -oP 'Page URL: \K.*' | head -1)
  if [ "$CURRENT_URL" != "$FULL_URL" ] && [ "$CURRENT_URL" != "${FULL_URL}/" ]; then
    STATUS="FAIL"
    ISSUES="${ISSUES}URL mismatch: expected $FULL_URL, got $CURRENT_URL "
  fi
  
  if [ "$STATUS" = "PASS" ]; then
    echo "  PASS - Loaded OK, no console errors, no raw i18n keys"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    echo "  FAIL - $ISSUES"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
  
  echo ""
done

echo "=========================================="
echo "  Results: $PASS_COUNT/$TOTAL passed, $FAIL_COUNT failed"
echo "=========================================="
