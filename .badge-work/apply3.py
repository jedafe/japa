import json, os, time, urllib.request, uuid

TOK = open("token").read().strip()
API = "https://api.japa.to/api/v1"
mapping = json.load(open("gen_mapping.json"))
done = json.load(open("done.json"))

def upload(path):
    data = open(path,"rb").read()
    b = uuid.uuid4().hex
    body = (f'--{b}\r\nContent-Disposition: form-data; name="file"; filename="{os.path.basename(path)}"\r\n'
            f'Content-Type: image/png\r\n\r\n').encode() + data + f'\r\n--{b}--\r\n'.encode()
    req = urllib.request.Request(f"{API}/admin/media/upload", data=body, method="POST",
        headers={"Authorization": f"Bearer {TOK}", "Content-Type": f"multipart/form-data; boundary={b}"})
    return json.load(urllib.request.urlopen(req))["data"]["media_id"]

def patch(badge_id, media_id):
    req = urllib.request.Request(f"{API}/admin/gamification/badges/{badge_id}",
        data=json.dumps({"icon_media_id": media_id}).encode(),
        method="PATCH", headers={"Authorization": f"Bearer {TOK}", "Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req))

ok = err = 0
for i, m in enumerate(mapping):
    try:
        mid = upload(m["file"])
        patch(m["badge_id"], mid)
        done[m["slug"]]["media_id"] = mid
        done[m["slug"]]["glyph"] = True
        ok += 1
        print(f"[{i+1}/{len(mapping)}] OK {m['slug']} -> {mid}", flush=True)
    except Exception as e:
        err += 1
        print(f"[{i+1}/{len(mapping)}] ERR {m['slug']}: {e}", flush=True)
    time.sleep(0.15)
json.dump(done, open("done.json","w"), indent=1)
print(f"done: {ok} ok, {err} err")
