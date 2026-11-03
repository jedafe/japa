import json, os, time, urllib.request, mimetypes, uuid

TOK = open("token").read().strip()
API = "https://api.japa.to/api/v1"
mapping = json.load(open("gen_mapping.json"))
done = json.load(open("done.json"))

def upload(path):
    fname = os.path.basename(path)
    data = open(path,"rb").read()
    b = uuid.uuid4().hex
    body = (f'--{b}\r\nContent-Disposition: form-data; name="file"; filename="{fname}"\r\n'
            f'Content-Type: image/png\r\n\r\n').encode() + data + f'\r\n--{b}--\r\n'.encode()
    req = urllib.request.Request(f"{API}/admin/media/upload", data=body, method="POST",
        headers={"Authorization": f"Bearer {TOK}", "Content-Type": f"multipart/form-data; boundary={b}"})
    return json.load(urllib.request.urlopen(req))["data"]["media_id"]

def patch(badge_id, media_id, accent):
    req = urllib.request.Request(f"{API}/admin/gamification/badges/{badge_id}",
        data=json.dumps({"icon_media_id": media_id, "accent_color": accent}).encode(),
        method="PATCH", headers={"Authorization": f"Bearer {TOK}", "Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req))

ok = err = skip = 0
for i, m in enumerate(mapping):
    slug = m["slug"]
    if slug in done:
        skip += 1; continue
    try:
        mid = upload(m["file"])
        patch(m["badge_id"], mid, m["accent"])
        done[slug] = {"media_id": mid, "accent": m["accent"], "generated": True}
        ok += 1
        print(f"[{i+1}/{len(mapping)}] OK {slug} -> {mid} {m['accent']}", flush=True)
    except Exception as e:
        err += 1
        print(f"[{i+1}/{len(mapping)}] ERR {slug}: {e}", flush=True)
    time.sleep(0.15)
json.dump(done, open("done.json","w"), indent=1)
print(f"done: {ok} ok, {err} err, {skip} skipped")
