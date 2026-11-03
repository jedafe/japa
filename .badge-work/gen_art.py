import json, math, os, re, io
from PIL import Image, ImageDraw, ImageFilter
import cairosvg

ICONS_DIR = "../node_modules/.bun/lucide-svelte@1.0.1+93db4ba677d7b8de/node_modules/lucide-svelte/dist/icons"
OUT = "gen"
os.makedirs(OUT, exist_ok=True)

# ---------- lucide icon extraction ----------
_icon_cache = {}
def icon_svg(name):
    if name in _icon_cache: return _icon_cache[name]
    path = os.path.join(ICONS_DIR, name + ".svelte")
    if not os.path.exists(path): return None
    src = open(path, encoding="utf8").read()
    m = re.search(r'const iconNode = (\[.*?\]);', src, re.S)
    if not m: return None
    nodes = json.loads(m.group(1))
    elems = []
    for tag, attrs in nodes:
        a = " ".join(f'{k.replace("_","-")}="{v}"' for k, v in attrs.items())
        elems.append(f"<{tag} {a}/>")
    svg = ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" '
           'fill="none" stroke="#ffffff" stroke-width="2" '
           'stroke-linecap="round" stroke-linejoin="round">' + "".join(elems) + "</svg>")
    _icon_cache[name] = svg
    return svg

def icon_png(name, size):
    svg = icon_svg(name)
    if svg is None: return None
    return Image.open(io.BytesIO(cairosvg.svg2png(bytestring=svg.encode(),
                     output_width=size, output_height=size))).convert("RGBA")

# ---------- color helpers ----------
def hx(h):
    h = h.lstrip("#"); return tuple(int(h[i:i+2],16) for i in (0,2,4))
def mix(c1, c2, t):
    return tuple(int(a+(b-a)*t) for a,b in zip(c1,c2))

def radial_gradient(size, c_inner, c_outer):
    """RGB radial gradient image (computed small, upscaled smooth)."""
    s = 128
    im = Image.new("RGB", (s, s))
    px = im.load()
    cx = cy = s/2
    R = s/2
    for y in range(s):
        for x in range(s):
            t = min(1.0, math.hypot(x-cx, y-cy)/R)
            px[x,y] = mix(c_inner, c_outer, t)
    return im.resize((size, size), Image.BICUBIC)

def linear_gradient(size, c_top, c_bottom):
    col = Image.new("RGB", (1, 256))
    px = col.load()
    for y in range(256):
        px[0,y] = mix(c_top, c_bottom, y/255)
    return col.resize((size, size), Image.BICUBIC)

def circle_mask(size, r, cx=None, cy=None):
    m = Image.new("L", (size, size), 0)
    d = ImageDraw.Draw(m)
    cx = size/2 if cx is None else cx
    cy = size/2 if cy is None else cy
    d.ellipse([cx-r, cy-r, cx+r, cy+r], fill=255)
    return m

def sparkle(size=64, color=(255,255,255,200)):
    im = Image.new("RGBA", (size, size), (0,0,0,0))
    d = ImageDraw.Draw(im)
    c = size/2
    d.polygon([(c,0),(c+size*0.09,c-size*0.09),(size,c),(c+size*0.09,c+size*0.09),(c,size),(c-size*0.09,c+size*0.09),(0,c),(c-size*0.09,c-size*0.09)], fill=color)
    return im

def render(accent_hex, icon_name, out_path):
    S = 1024
    accent = hx(accent_hex)
    im = Image.new("RGBA", (S,S), (0,0,0,0))

    # soft tinted background wash (mostly transparent white)
    bg = Image.new("RGBA",(S,S),(255,255,255,255))

    # ground shadow
    sh = Image.new("RGBA",(S,S),(0,0,0,0))
    ImageDraw.Draw(sh).ellipse([S*0.28,S*0.80,S*0.72,S*0.92], fill=(0,0,0,90))
    bg.alpha_composite(sh.filter(ImageFilter.GaussianBlur(22)))

    medal = Image.new("RGBA",(S,S),(0,0,0,0))
    cx = cy = S//2

    # rosette bumps (scalloped edge) — metallic gold dark
    bumps = Image.new("RGBA",(S,S),(0,0,0,0))
    bd = ImageDraw.Draw(bumps)
    n = 26
    for i in range(n):
        a = 2*math.pi*i/n
        bx = cx + 372*math.cos(a); by = cy + 372*math.sin(a)
        bd.ellipse([bx-64,by-64,bx+64,by+64], fill=(140,102,24,255))
    medal.alpha_composite(bumps)

    # outer gold ring: linear metallic gradient masked to ring
    outer = Image.new("RGBA",(S,S),(0,0,0,0))
    g = linear_gradient(S, (247,222,120), (168,124,32))
    outer.paste(g, (0,0), circle_mask(S, 400, cx, cy))
    inner_cut = Image.new("L",(S,S),0)
    ImageDraw.Draw(inner_cut).ellipse([cx-330,cy-330,cx+330,cy+330], fill=0)
    ringm = circle_mask(S, 400, cx, cy)
    ImageDraw.Draw(ringm).ellipse([cx-330,cy-330,cx+330,cy+330], fill=0)
    outer.putalpha(ringm)
    medal.alpha_composite(outer)

    # rim light on outer ring
    rim = Image.new("RGBA",(S,S),(0,0,0,0))
    rd = ImageDraw.Draw(rim)
    rd.ellipse([cx-400,cy-400,cx+400,cy+400], outline=(120,86,18,255), width=10)
    rd.ellipse([cx-330,cy-330,cx+330,cy+330], outline=(120,86,18,255), width=8)
    medal.alpha_composite(rim)

    # inner disc: radial accent gradient (light center -> darker rim)
    c_in  = mix(accent, (255,255,255), 0.35)
    c_out = mix(accent, (0,0,0), 0.28)
    disc = radial_gradient(S, c_in, c_out).convert("RGBA")
    dm = circle_mask(S, 330, cx, cy)
    disc.putalpha(dm)
    medal.alpha_composite(disc)

    # inner disc top sheen
    sheen = Image.new("RGBA",(S,S),(0,0,0,0))
    ImageDraw.Draw(sheen).ellipse([cx-230,cy-300,cx+230,cy-60], fill=(255,255,255,60))
    medal.alpha_composite(sheen.filter(ImageFilter.GaussianBlur(30)))

    # icon drop shadow + glyph
    icon = icon_png(icon_name, 360)
    if icon:
        sh2 = Image.new("RGBA",(S,S),(0,0,0,0))
        dark = Image.new("RGBA", icon.size, (0,0,0,110))
        dark.putalpha(icon.split()[3].point(lambda a: a*0.5))
        sh2.alpha_composite(dark, (cx-180+8, cy-180+14))
        medal.alpha_composite(sh2.filter(ImageFilter.GaussianBlur(6)))
        medal.alpha_composite(icon, (cx-180, cy-180))

    # sparkles
    medal.alpha_composite(sparkle(80,(255,255,255,220)), (cx+240, cy-330))
    medal.alpha_composite(sparkle(56,(255,240,180,230)), (cx-330, cy+180))
    medal.alpha_composite(sparkle(44,(255,255,255,180)), (cx+300, cy+230))

    bg.alpha_composite(medal)
    bg.save(out_path)

# ---------------- badge → icon + accent ----------------
CAT = {
 "academy":"#6366f1","ai":"#8b5cf6","career":"#d97706","community":"#3b82f6",
 "comparison":"#0891b2","consistency":"#ea580c","content":"#65a30d",
 "documents":"#0f766e","financial":"#ca8a04","forum":"#0284c7",
 "getting_started":"#059669","growth":"#16a34a","housing":"#d97706",
 "journey":"#0d9488","mentorship":"#e11d48","readiness":"#7c3aed",
 "travel":"#2563eb","trust":"#15803d","services":"#db2777","special":"#b45309",
 "countries":"#059669","education":"#4f46e5",
}

ICON = {
 "first_lesson":"book-open","avid_learner":"book-marked","dedicated_student":"graduation-cap",
 "quiz_master":"circle-question-mark","perfect_score":"star","learning_champion":"trophy",
 "academy_graduate":"award","lifelong_learner":"infinity",
 "ai_explorer":"bot","ai_inquisitive":"message-circle-question-mark","ai_researcher":"scan-search",
 "ai_smart_planner":"calendar-cog","ai_collaborator":"messages-square","digital_strategist":"brain",
 "interview_champion":"mic",
 "space_explorer":"orbit","community_voice":"megaphone","social_butterfly":"share-2",
 "community_regular":"repeat","conversation_starter":"message-circle-plus",
 "people_person":"heart-handshake","group_starter":"user-round-plus","space_founder":"flag",
 "group_organizer":"calendar-days","event_participant":"ticket","active_participant":"party-popper",
 "community_host":"mic","space_leader":"crown","community_builder":"hammer",
 "smart_planner":"git-compare-arrows","comparison_rookie":"git-compare","decision_maker":"check-check",
 "destination_analyst":"chart-column","city_analyst":"building-2","comparison_expert":"scale",
 "ultimate_comparator":"trophy",
 "half_year_hero":"flame-kindling","year_of_dedication":"medal","challenge_champion":"trophy",
 "challenge_legend":"crown","comeback_kid":"undo-2","never_give_up":"mountain",
 "guide_creator":"notebook-pen","information_architect":"library-big",
 "trusted_contributor":"pen-tool","community_educator":"school",
 "document_explorer":"file-search","first_attestation":"signature",
 "document_detective":"search-check","verification_pro":"file-badge","paperwork_champion":"file-stack",
 "budget_master":"calculator",
 "first_discussion":"message-square-plus","helpful_member":"hand-helping","helpful_voice":"speech",
 "answer_champion":"circle-check-big","japa_champion":"award","poll_participant":"vote",
 "opinion_leader":"speech",
 "picture_perfect":"camera","voice_of_identity":"feather","social_links":"link-2",
 "email_verified":"mail-check","phone_verified":"smartphone","verified_member":"badge-check",
 "first_referral":"user-plus","referral_champion":"gift","growth_ambassador":"megaphone",
 "network_champion":"network","community_recruiter":"users-round","growth_legend":"rocket",
 "home_seeker":"house","housing_hunter":"hotel","neighbourhood_explorer":"map-pin-house",
 "rental_ready":"key-round","first_home":"house-plus","settlement_specialist":"clipboard-check",
 "home_sweet_home":"heart",
 "first_destination":"map-pin","waiting_game":"hourglass","boarding_pass":"plane",
 "new_beginnings":"sunrise","settled_abroad":"anchor",
 "guidance_seeker":"compass","mentor_in_training":"graduation-cap","transformational_mentor":"sparkles",
 "strategic_planner":"route","goal_setter":"target","japa_strategist":"waypoints",
 "mission_accomplished":"flag",
 "travel_planner":"map","travel_ready":"luggage","local_explorer":"footprints",
 "safety_advocate":"shield","trust_builder":"shield-plus",
}

FALLBACK = "award"

badges = {b["slug"]: b for b in json.load(open("badges_all.json"))}
done = set(json.load(open("done.json")).keys())
missing = [s for s in badges if s not in done]

report = []
for slug in missing:
    b = badges[slug]
    icon = ICON.get(slug, FALLBACK)
    if icon_svg(icon) is None:
        icon = FALLBACK
    accent = CAT.get(b["category"], "#b45309")
    out = f"{OUT}/{slug}.png"
    render(accent, icon, out)
    report.append({"slug": slug, "badge_id": b["id"], "file": out, "accent": accent, "icon": icon})
    print(slug, "->", icon, accent)

json.dump(report, open("gen_mapping.json","w"), indent=1)
print(len(report), "generated")
