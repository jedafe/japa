#!/usr/bin/env python3
"""Provision JapaConnect on Coolify v4 — verified working endpoints.

Idempotent-ish: reuses generated creds from .coolify-provision.json.
Creates: staging env, registry (compose, htpasswd), 6 infra services
per env, 14 docker-image apps, then prints GitHub secret names+values.

API quirks on v4.4.6: services take ONLY docker_compose_raw (base64) —
never a `type`; domains on apps are a string field `domains`; service
start is POST /services/{uuid}/start; service domain is set in the UI
(`Domains` field, `https://registry.japa.to`, plus `expose: ["5000"]`
on the registry service so Traefik knows the port).
"""
import base64, json, os, secrets, urllib.request, urllib.error

BASE = "https://m.japa.to/api/v1"
TOKEN = open(".env").read().split("JC_COOLIFY_TOKEN=")[1].split("\n")[0].strip().strip('"\'')
PROJECT = "vwz6wiv89c8zs0ijlj0tokz2"
SERVER = "donaklqeq5sl5ko8xlr2wxu8"
REG_HOST = "registry.japa.to"
C = json.load(open(".coolify-provision.json")) if os.path.exists(".coolify-provision.json") else {}
RU = C.get("registry", {}).get("user", "ci")
RP = C.get("registry", {}).get("password") or secrets.token_urlsafe(24)
RS = C.get("registry", {}).get("secret") or secrets.token_urlsafe(48)

def req(m, p, b=None):
    r = urllib.request.Request(BASE + p, method=m)
    r.add_header("Authorization", f"Bearer {TOKEN}")
    r.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(r, json.dumps(b).encode() if b is not None else None, timeout=30) as x:
            try: return x.status, json.loads(x.read() or b"{}")
            except Exception: return x.status, {}
    except urllib.error.HTTPError as e: return e.code, e.read().decode()[:150]

def show(t, s, b): print(f"{s:>3} {t}: {str(b)[:130]}")
def b64(s): return base64.b64encode(s.encode()).decode()

# staging environment (created first run)
st, b = req("POST", f"/projects/{PROJECT}/environments", {"name": "staging"})
show("staging env", st, b)

REG = f"""services:
  registry-auth:
    image: httpd:2-alpine
    command: sh -c 'htpasswd -Bbn "{RU}" "{RP}" > /auth/htpasswd'
    volumes: [registry-auth:/auth]
  registry:
    image: registry:2
    depends_on: [registry-auth]
    expose: ["5000"]
    environment:
      REGISTRY_AUTH: htpasswd
      REGISTRY_AUTH_HTPASSWD_REALM: japa-registry
      REGISTRY_AUTH_HTPASSWD_PATH: /auth/htpasswd
      REGISTRY_HTTP_SECRET: {RS}
    volumes: [registry-data:/var/lib/registry, registry-auth:/auth]
volumes: {{registry-data: {{}}, registry-auth: {{}}}}"""

INFRA = {
 "postgres":    f"services:\n  postgres:\n    image: pgvector/pgvector:pg18\n    environment:\n      POSTGRES_USER: japa\n      POSTGRES_PASSWORD: '{C.get('pg') or secrets.token_urlsafe(24)}'\n      POSTGRES_DB: japaconnect\n    volumes: [pgdata:/var/lib/postgresql/data]\nvolumes:\n  pgdata:",
 "valkey":      "services:\n  valkey:\n    image: valkey/valkey:9-alpine\n    command: valkey-server --appendonly yes\n    volumes: [vdata:/data]\nvolumes:\n  vdata:",
 "meilisearch": f"services:\n  meilisearch:\n    image: getmeili/meilisearch:v1.45\n    environment:\n      MEILI_MASTER_KEY: '{C.get('meili') or secrets.token_urlsafe(24)}'\n    volumes: [mdata:/meili_data]\nvolumes:\n  mdata:",
 "seaweedfs":   "services:\n  seaweedfs:\n    image: chrislusf/seaweedfs:latest\n    command: server -s3 -dir=/data\n    volumes: [sdata:/data]\nvolumes:\n  sdata:",
 "imgproxy":    f"services:\n  imgproxy:\n    image: darthsim/imgproxy:latest\n    environment:\n      IMGPROXY_KEY: '{C.get('img_key') or secrets.token_hex(32)}'\n      IMGPROXY_SALT: '{C.get('img_salt') or secrets.token_hex(32)}'",
 "mailpit":     "services:\n  mailpit:\n    image: axllent/mailpit:latest",
}

def mk(env, name, comp):
    st, b = req("POST", "/services", {"project_uuid": PROJECT, "environment_name": env,
                "server_uuid": SERVER, "name": name, "docker_compose_raw": b64(comp)})
    show(f"svc {env}/{name}", st, b)
    return b.get("uuid") if isinstance(b, dict) else None

u = mk("production", "registry", REG)
if u:
    show("reg start", *req("POST", f"/services/{u}/start"))
    C["registry"]["uuid"] = u

for env in ["staging", "production"]:
    for nm, comp in INFRA.items():
        u = mk(env, f"{env}-{nm}", comp)
        if u: show(f"start {env}/{nm}", *req("POST", f"/services/{u}/start"))

APPS = [("api","api.b.japa.to","api.japa.to","8080"),("main","b.japa.to","japa.to","3000"),
        ("admin","admin.b.japa.to","admin.japa.to","3001"),("control","control.b.japa.to","control.japa.to","3002"),
        ("academy","academy.b.japa.to","academy.japa.to","3003"),("docs","docs.b.japa.to","docs.japa.to","3004"),
        ("tools","tools.b.japa.to","tools.japa.to","3005")]
wh = {}
for env, tag in [("staging","staging"),("production","production")]:
    for name, sh, ph, port in APPS:
        st, b = req("POST", "/applications/dockerimage",
                    {"project_uuid": PROJECT, "environment_name": env, "server_uuid": SERVER,
                     "name": f"japa-{name}", "docker_registry_image_name": f"{REG_HOST}/japa-{name}:{tag}",
                     "ports_exposes": port, "instant_deploy": False,
                     "domains": f"https://{sh if env=='staging' else ph}"})
        show(f"app {env}/japa-{name}", st, b)
        if isinstance(b, dict) and b.get("uuid"): wh[f"{name}:{env}"] = b["uuid"]

out = {}
for k, uuid in wh.items():
    st, b = req("GET", f"/applications/{uuid}")
    sec = b.get("manual_webhook_secret_github", "") if isinstance(b, dict) else ""
    n, e = k.split(":")
    out[f"COOLIFY_WEBHOOK_{n.upper()}{'_STAGING' if e=='staging' else ''}"] = f"{BASE}/deploy?uuid={uuid}&token={sec}"
print(json.dumps(out, indent=0)[:2000])

C.update({"webhooks": out, "registry": {"user": RU, "password": RP, "secret": RS, "uuid": C.get("registry", {}).get("uuid")}, "apps": wh})
json.dump(C, open(".coolify-provision.json", "w"), indent=1)
print("saved .coolify-provision.json")
