#!/usr/bin/env python3
"""Generate internal/publicapi/public_catalog_openapi.json from the public route list.

Reads a JSON array of {method, path} from stdin and writes an OpenAPI
paths object (map of path -> method -> operation) for every public,
non-infrastructure route.
"""
import json
import re
import sys

INFRA_PREFIXES = {
    "/health",
    "/ready",
    "/robots",
    "/llms",
    "/sitemap",
    "/og/",
    "/feed.xml",
    "/spaces/{slug}/feed.xml",
    "/tags/{slug}/feed.xml",
    "/users/{username}/feed.xml",
    "/site/settings/public",
    "/docs/",
    "/csrf",
    "/auth/",
    "/feature-flags",
    "/push/vapid-key",
    "/payments/webhook/",
    "/media/serve/",
    "/blog/feed.xml",
    "/r/{code}",
}


def should_include(path: str) -> bool:
    for prefix in INFRA_PREFIXES:
        if path.startswith(prefix) or path == prefix.rstrip("/"):
            return False
    return True


def operation_id(method: str, path: str) -> str:
    parts = [method.lower()] + [p for p in path.split("/") if p]
    parts = [re.sub(r"[{}]", "", p) for p in parts]
    new_parts = []
    for p in parts:
        p = re.sub(r"\.\w+$", "", p)
        for sub in re.split(r"[_-]", p):
            if sub:
                new_parts.append(sub)
    return "".join(p[:1].upper() + p[1:] for p in new_parts if p)


def path_params(path: str):
    return re.findall(r"\{(\w+)\}", path)


def build_operation(method: str, path: str):
    params = []
    for name in path_params(path):
        params.append({
            "name": name,
            "in": "path",
            "required": True,
            "schema": {"type": "string"},
            "description": f"{name} identifier",
        })

    op = {
        "operationId": operation_id(method, path),
        "summary": f"{method} {path}",
        "description": "Public route from the JEOS route catalog.",
        "tags": ["Public"],
        "security": [],
        "responses": {
            "200": {
                "description": "Success",
                "content": {
                    "application/json": {
                        "schema": {"type": "object"},
                    },
                },
            },
            "429": {"$ref": "#/components/responses/RateLimited"},
        },
    }
    if params:
        op["parameters"] = params
    return op


def main():
    routes = json.load(sys.stdin)
    routes = [r for r in routes if should_include(r["path"])]

    paths: dict[str, dict] = {}
    for r in routes:
        path = r["path"]
        method = r["method"].lower()
        paths.setdefault(path, {})[method] = build_operation(r["method"], path)

    with open("internal/publicapi/public_catalog_openapi.json", "w") as f:
        json.dump(paths, f, indent=2)

    print(f"Wrote {len(paths)} public path entries to internal/publicapi/public_catalog_openapi.json")


if __name__ == "__main__":
    main()
