#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

PASS=0
FAIL=0
RESULTS=()

run_check() {
    local name="$1"
    shift
    echo -e "\n${YELLOW}=== $name ===${NC}"
    if "$@"; then
        echo -e "${GREEN}✓ $name passed${NC}"
        PASS=$((PASS + 1))
        RESULTS+=("PASS  $name")
    else
        echo -e "${RED}✗ $name failed${NC}"
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL  $name")
    fi
}

echo -e "${YELLOW}JapaConnect Full Test Suite${NC}"
echo "=========================="

# --- Go Backend ---
echo -e "\n${YELLOW}--- Go Backend ---${NC}"

run_check "Go vet" go vet ./...
run_check "Go tests" go test ./...

# --- Frontend Builds ---
echo -e "\n${YELLOW}--- Frontend Builds ---${NC}"

for app in main admin control learn docs; do
    run_check "Build $app" bash -c "cd apps/$app && bun run build"
done

# --- Frontend Tests ---
echo -e "\n${YELLOW}--- Frontend Tests ---${NC}"

for app in main admin control learn docs; do
    run_check "Test $app" bash -c "cd apps/$app && bun run test"
done

# --- Summary ---
echo -e "\n${YELLOW}=========================="
echo "       SUMMARY"
echo "==========================${NC}"

for result in "${RESULTS[@]}"; do
    if [[ "$result" == PASS* ]]; then
        echo -e "${GREEN}$result${NC}"
    else
        echo -e "${RED}$result${NC}"
    fi
done

echo ""
TOTAL=$((PASS + FAIL))
echo -e "Total: $TOTAL | ${GREEN}Passed: $PASS${NC} | ${RED}Failed: $FAIL${NC}"

if [ $FAIL -gt 0 ]; then
    echo -e "\n${RED}Some checks failed!${NC}"
    exit 1
else
    echo -e "\n${GREEN}All checks passed!${NC}"
    exit 0
fi
