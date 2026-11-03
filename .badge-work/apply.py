import json, os, sys, time, urllib.request, mimetypes, uuid

TOK = open("token").read().strip()
API = "https://api.japa.to/api/v1"
mapping = json.load(open("mapping.json"))
done_path = "done.json"
done = json.load(open(done_path)) if os.path.exists(done_path) else {}

def boundary():
    return uuid.uuid4().hex

def upload(path):
    fname = os.path.basename(path).replace("\n", " ")
    data = open(path, "rb").read()
    ct = mimetypes.guess_type(path)[0] or "image/jpeg"
    b = boundary()
    body = (
        f"--{b}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{fname}\"\r\n"
        f"Content-Type: {ct}\r\n\r\n"
    ).encode() + data + f"\r\n--{b}--\r\n".encode()
    req = urllib.request.Request(f"{API}/admin/media/upload", data=body, method="POST",
        headers={"Authorization": f"Bearer {TOK}", "Content-Type": f"multipart/form-data; boundary={b}"})
    r = json.load(urllib.request.urlopen(req))
    return r["data"]["media_id"], r["data"].get("url")

def patch(badge_id, media_id, accent):
    payload = {"icon_media_id": media_id, "accent_color": accent}
    req = urllib.request.Request(f"{API}/admin/gamification/badges/{badge_id}",
        data=json.dumps(payload).encode(), method="PATCH",
        headers={"Authorization": f"Bearer {TOK}", "Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req))

ok = err = skip = 0
for i, m in enumerate(mapping):
    slug = m["slug"]
    if slug in done:
        skip += 1; continue
    try:
        mid, url = upload(m["file"])
        patch(m["badge_id"], mid, m["accent"])
        done[slug] = {"media_id": mid, "accent": m["accent"], "url": url}
        json.dump(done, open(done_path, "w"), indent=1)
        ok += 1
        print(f"[{i+1}/{len(mapping)}] OK {slug} -> {mid} {m['accent']}", flush=True)
    except Exception as e:
        err += 1
        print(f"[{i+1}/{len(mapping)}] ERR {slug}: {e}", flush=True)
    time.sleep(0.15)
print(f"done: {ok} ok, {err} err, {skip} skipped")
