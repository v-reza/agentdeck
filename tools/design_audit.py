#!/usr/bin/env python3
"""Inventory design vs implementasi AgentDeck. Jalankan dari root repo.

Alat ini ADA karena `docs/DESIGN-INVENTORY.md` sebelumnya dihitung manual dan
salah di dua tempat: dia bilang mockup memakai satu set icon Lucide (padahal 22
dari 51 file memakai ligature Material Symbols), dan dia menyebut palet status
"beku (DESIGN.md + mockup)" padahal DESIGN.md tidak punya palet status sama
sekali. Angka yang dihitung tangan akan drift; angka yang dihitung script tidak.

Sumber peta layar: `tools/screens.py` (SCREENS). Bukan peta kedua — kalau
screens.py berubah, laporan ini ikut berubah tanpa diedit.

Cara memetakan file impl ke layar, berurutan:
  1. docblock file menyebut screen-id (`28-agent-detail`) — 39 file melakukannya
  2. route layar di SCREENS cocok dengan `<Route path="...">` di router.tsx
  3. tidak ada yang cocok -> file dihitung sebagai "shell/komponen bersama"

Yang dilaporkan, semuanya dihitung:
  * icon: `<svg>` + ligature Material Symbols, di design vs impl
  * skeleton: blok abu `bg-[#e8ecea]` vs dot status `animate-pulse` (dua hal beda)
  * dropdown: `<select>` bawaan HTML yang masih hidup, plus lokasinya
  * warna status: DECISIONS.md paragraf "Status color" vs `index.css`
  * token spacing yang dideklarasikan tapi nol dipakai
  * state archived: line-through / opacity / border kiri
  * jargon (`US-AD`, `AC`, `M0`-`M6`) yang bocor ke UI yang dirender

Jalankan: python tools/design_audit.py            (tulis docs/DESIGN-INVENTORY.md)
          python tools/design_audit.py --check    (exit 1 kalau ada yang bocor)

ponytail: regex, bukan AST. Yang dicek semuanya pola teks yang tidak bisa
disamarkan; parser TS = dependency baru tanpa manfaat.
"""

import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, os.path.join(ROOT, "tools"))

DESIGN_DIR = os.path.join(ROOT, "design", "stitch-output", "v2")
SRC = os.path.join(ROOT, "frontend", "src")
ROUTER = os.path.join(SRC, "app", "router.tsx")
INDEX_CSS = os.path.join(SRC, "index.css")
OUT = os.path.join(ROOT, "docs", "DESIGN-INVENTORY.md")

FAILED = []

# Blok skeleton di mockup. 43-state-loading memakai `bg-[#e8ecea]` untuk semua
# placeholder barisnya, dan `#edeeed` untuk track progress; dot status memakai
# `animate-pulse` + `rounded-full` + 1.5-2px. Dua-duanya pernah dihitung sebagai
# "skeleton" oleh inventory lama, itu sebabnya angkanya 55 padahal blok asli
# cuma segelintir.
SKELETON_FILLS = ("#e8ecea", "#edeeed", "#eff2f1")

# `.hermes.md`: jargon dilarang di UI yang dirender, "`/github` pengecualian".
# Track milestone M0-M6 memang isi halaman itu. Dua file di bawah ini adalah
# data + render track tersebut. Pengecualiannya diverifikasi di `main()`: kalau
# `Roadmap` sampai dipasang di halaman lain, pengecualiannya gugur sendiri.
GITHUB_EXEMPT = {"data/content.ts", "components/landing/Roadmap.tsx"}

# ------------------------------------------------------------------ helpers --


def read(path):
    with open(path, "r", encoding="utf-8", errors="replace") as fh:
        return fh.read()


def strip_comments(text):
    """Buang komentar JS/JSX supaya jargon di komentar tidak dihitung bocor.

    Blok komentar diganti newline sebanyak baris yang dia tempati, bukan
    dihapus: nomor baris yang dilaporkan scanner (`select_at`, lokasi jargon)
    harus tetap menunjuk baris aslinya di file, bukan baris setelah komentar
    mengkerut.
    """
    text = re.sub(r"/\*.*?\*/", lambda m: "\n" * m.group(0).count("\n"), text, flags=re.S)
    return re.sub(r"^[ \t]*//.*$", "", text, flags=re.M)


def load_screens():
    from screens import SCREENS  # noqa: E402  (butuh ROOT di sys.path)

    return SCREENS


def route_map():
    """path router -> nama komponen. Dipakai saat docblock tidak menyebut id."""
    text = read(ROUTER)
    return dict(re.findall(r'<Route\s+path="([^"]+)"\s+element=\{<(\w+)', text))


# --------------------------------------------------------------- design side --


def scan_design():
    out = {}
    if not os.path.isdir(DESIGN_DIR):
        return out
    for name in sorted(os.listdir(DESIGN_DIR)):
        if not name.endswith(".html"):
            continue
        sid = name[: -len(".html")]
        html = read(os.path.join(DESIGN_DIR, name))
        fills = re.findall(r'class="([^"]*)"', html)
        # Icon di dalam <main> saja. Mockup merender shell penuh (sidebar,
        # cost rail, topbar) di setiap file, jadi angka mentah "23 svg" di
        # 35-approval-inbox sebagian besar adalah chrome yang di produk hidup
        # di komponen bersama — membandingkannya dengan isi satu route itu
        # apel-vs-jeruk. Isi halamannya sendiri yang bisa dibandingkan.
        main_html = re.search(r"<main[\s>].*?</main>", html, re.S)
        page_html = main_html.group(0) if main_html else html
        svg = len(re.findall(r"<svg", page_html))
        ligature = len(re.findall(r"material-symbols-outlined", html))
        out[sid] = {
            "svg": svg,
            "ligature": ligature,
            "select": len(re.findall(r"<select\b", html)),
            "pulse": len(re.findall(r"animate-pulse", html)),
            "skeleton": sum(1 for c in fills if any(f in c for f in SKELETON_FILLS)),
            "line_through": len(re.findall(r"line-through", html)),
            "opacity": len(re.findall(r"opacity-\d+", html)),
            "hex": set(re.findall(r"#([0-9a-fA-F]{6})\b", html)),
            # Teks mentah mockup disimpan supaya laporan bisa menjawab "jargon ini
            # dibawa design-nya atau ditambah impl" — dua pekerjaan yang beda.
            "body": html,
        }
    return out


def owning_screen(rel, mapped):
    """Layar yang memiliki sebuah file impl; kosong kalau file itu shell/bersama."""
    ids = mapped.get(rel) or set()
    return sorted(ids)[0] if ids else ""


# ----------------------------------------------------------- implentation ---


def impl_files():
    files = []
    for base, _dirs, names in os.walk(SRC):
        for n in names:
            if n.endswith((".tsx", ".ts")) and not n.endswith(".test.ts"):
                files.append(os.path.join(base, n))
    return sorted(files)


def normalize_route(path):
    """Bentuk route yang bisa dibandingkan antara mockup dan router.

    `screens.py` menulis `/boards/:id`, router menulis `/boards/:boardID`;
    keduanya menunjuk halaman yang sama. Segmen dinamis disamakan jadi `:x`
    supaya perbandingannya soal struktur, bukan soal nama variabelnya.
    """
    path = path.split("?")[0].strip("/").lower()
    if path in ("(overlay)", "(drawer)", "(modal)", ""):
        return path
    return "/".join(":x" if seg.startswith(":") else seg for seg in path.split("/"))


def assign_screens(files, screens, routes):
    """file -> set(screen-id). Urutan: docblock, lalu komponen pemilik route.

    Versi lama punya bug yang merusak seluruh laporan: kalau docblock tidak
    menyebut id, ia menugaskan file ke SETIAP layar yang nama komponennya
    muncul di `router.tsx`. Karena router menyebut hampir semua komponen,
    `router.tsx` dan `Layout.tsx` jadi milik 25-30 layar sekaligus, dan tabel
    per-layar melaporkan file yang sama untuk semua baris. Akibatnya kolom
    "icon impl" nol di mana-mana: `AgentDetail.tsx` dihitung milik 28 layar,
    jadi tidak ada satu pun layar yang benar-benar memiliki file yang memakai
    icon.

    Sekarang komponen pemilik ditentukan dari `<Route element={<X />}>` yang
    route-nya cocok dengan route layar. Cocok, atau tidak dipetakan sama sekali.
    """
    # route ternormalisasi -> komponen yang merender route itu
    by_route = {}
    for route, comp in routes.items():
        by_route.setdefault(normalize_route(route), set()).add(comp)

    mapped = {}
    unmapped = []
    for path in files:
        rel = os.path.relpath(path, SRC).replace("\\", "/")
        head = "\n".join(read(path).splitlines()[:40])
        ids = set(re.findall(r"\b(\d{2}[a-c]?-[a-z0-9-]+)\b", head))
        ids = {i for i in ids if i in screens}
        if not ids:
            comp = os.path.basename(path).split(".")[0]
            for sid, entry in screens.items():
                route = entry[1]
                if comp in by_route.get(normalize_route(route), ()):
                    ids.add(sid)
        if ids:
            mapped[rel] = ids
        else:
            unmapped.append(rel)
    return mapped, unmapped


LUCIDE_IMPORT = re.compile(r"import\s*\{([^}]*)\}\s*from\s*'lucide-react'")


def count_icons(text, jsx):
    """Elemen icon yang dirender: komponen Lucide + `<svg>` inline.

    Dihitung dari source yang sudah dibuang komentarnya, supaya docblock yang
    membahas `<svg>` tidak ikut terhitung sebagai markup.
    """
    n = len(re.findall(r"<svg", jsx))
    for match in LUCIDE_IMPORT.finditer(jsx):
        for name in match.group(1).split(","):
            name = name.strip().removeprefix("type ").strip()
            if name:
                n += len(re.findall(r"<" + re.escape(name) + r"\b", jsx))
    return n


def scan_impl(files):
    out = {}
    for path in files:
        rel = os.path.relpath(path, SRC).replace("\\", "/")
        text = read(path)
        jsx = strip_comments(text)
        # Scan the comment-stripped source, not the raw text: files here discuss
        # `<select>` in their docblocks, and a line-prefix check missed those
        # because only the first line of a block comment starts with `*`. The
        # count was reading prose as markup.
        stripped = jsx
        selects = [(i, ln.strip()[:70]) for i, ln in enumerate(stripped.splitlines(), 1) if "<select" in ln]
        loading = [
            (i, ln.strip()[:70])
            for i, ln in enumerate(stripped.splitlines(), 1)
            if re.search(r"""["'>](Loading|Memuat)[….\s]""", ln)
        ]
        out[rel] = {
            # `<svg>` saja menyesatkan: implementasi memakai komponen Lucide
            # (`<X size={18} />`), bukan SVG inline, jadi ia melaporkan "3 icon"
            # untuk aplikasi yang merender ratusan. Yang dihitung di sini adalah
            # elemen icon yang benar-benar dirender — komponen Lucide DAN `<svg>`
            # mentah — supaya bisa dibandingkan dengan 319 `<svg>` di mockup.
            "svg": count_icons(text, jsx),
            "select": len(selects),
            "select_at": selects,
            "skeleton": len(re.findall(r"<Skeleton", text)),
            # Bare "Loading" was too blunt: it counted the i18n *key* names
            # (`state.loading`), so a screen that rendered no text at all still
            # scored. What matters is a literal loading string reaching the DOM —
            # a quoted value, or the ellipsis form the screens actually used.
            "loading_at": loading,
            "loading_text": len(loading),
            "lucide": len(re.findall(r"from 'lucide-react'", text)),
            "line_through": len(re.findall(r"line-through", text)),
            "opacity": len(re.findall(r"opacity-\d+", text)),
            "jargon": sorted(
                set(
                    f"{m}"
                    for m in re.findall(
                        r"\b(US-AD\d+|AC\d+|M[0-6]\b)", jsx
                    )
                )
            )
            if rel not in GITHUB_EXEMPT
            else [],
        }
    return out


# ------------------------------------------------------------------ tokens --


def status_palette():
    """Palet status beku, dibaca dari DECISIONS.md §8 — bukan ditulis ulang.

    Sepuluh pasangan itu ditulis dalam SATU bullet yang di-wrap ke empat baris
    markdown. Versi lama cuma membaca baris pertama, jadi menemukan 3 dari 10
    dan selisihnya kelihatan lebih kecil daripada kenyataannya. Di sini bullet-nya
    disatukan dulu sampai bullet berikutnya.
    """
    text = read(os.path.join(ROOT, "docs", "DECISIONS.md"))
    i = text.find("Status color:")
    if i < 0:
        return {}
    rest = text[i:]
    lines = [rest.split("\n", 1)[0]]
    for line in rest.split("\n")[1:]:
        if line.startswith("- ") or not line.strip():
            break
        lines.append(line)
    joined = " ".join(lines)
    # `archived` slate muda `#cbd5e1` — deskriptornya dua kata, jadi `\w+`
    # tunggal melewatkannya dan paletnya cuma ketemu 9 dari 10.
    # `[a-z ]+` itu rakus dan bisa melahap beberapa entri sekaligus, jadi
    # kecocokan belakangan menimpa yang awal dan nilai yang dilaporkan bukan
    # nilai di dokumen. Batasi ke deskriptor maksimal tiga kata.
    return {
        k: v.lower()
        for k, v in re.findall(r"`([a-z_]+)`\s+[a-z]+(?:\s+[a-z]+){0,2}\s+`(#[0-9a-fA-F]{6})`", joined)
    }


def css_status_tokens():
    """Token status dari index.css.

    Nama token memakai tanda hubung (`--color-status-awaiting-approval`) sementara
    DECISIONS menulisnya dengan garis bawah (`awaiting_approval`), jadi kelas
    karakternya harus memuat keduanya lalu dinormalkan — tanpa itu
    `awaiting_approval` tidak pernah ketemu dan dilaporkan "beda" padahal nilainya
    sama.
    """
    text = read(INDEX_CSS)
    return {
        k.replace("-", "_").lower(): v.strip().lower()
        for k, v in re.findall(r"--color-status-([a-z_-]+):\s*([^;]+);", text)
    }


def dead_spacing_tokens(files):
    text = read(INDEX_CSS)
    declared = dict(re.findall(r"(--spacing-[a-z-]+):\s*([^;]+);", text))
    used = {name: 0 for name in declared}
    for path in files:
        body = read(path)
        for name in declared:
            used[name] += body.count(f"var({name})")
    return declared, used


# ------------------------------------------------------------------ report --


def build(screens, design, impl, mapped, unmapped):
    routes = route_map()
    lines = []
    add = lines.append

    add("# Design inventory — design vs implementasi")
    add("")
    add("**Dihasilkan `tools/design_audit.py`.** Jangan diedit tangan: angkanya")
    add("dihitung dari parse `design/stitch-output/v2/*.html` dan")
    add(f"`frontend/src/**` ({len(impl)} file), bukan dari ingatan. Jalankan ulang")
    add("scriptnya kalau ada yang berubah.")
    add("")

    total_design_svg = sum(d["svg"] for d in design.values())
    total_impl_svg = sum(i["svg"] for i in impl.values())
    total_design_skel = sum(d["skeleton"] for d in design.values())
    total_impl_skel = sum(i["skeleton"] for i in impl.values())
    ligature_files = [s for s, d in design.items() if d["ligature"]]
    select_files = [(r, i["select"], i["select_at"]) for r, i in impl.items() if i["select"]]
    loading_text = [(r, i["loading_text"], i["loading_at"]) for r, i in impl.items() if i["loading_text"]]

    add("## 1. Ringkasan")
    add("")
    add("| | design | impl |")
    add("|---|---|---|")
    add(f"| `<svg>` | {total_design_svg} | {total_impl_svg} |")
    add("")
    add("Dua angka icon di baris pertama itu **bukan** jumlah elemen unik. Mockup")
    add("menggambar ulang icon yang sama di tiap kartu (satu jam per kartu approval),")
    add("sementara implementasi mendeklarasikan komponennya sekali lalu merendernya")
    add("di dalam `.map()`. Jadi rasio mentahnya selalu terlihat lebih buruk daripada")
    add("kenyataan, dan satu-satunya cara membacanya adalah per jenis icon di layar")
    add("yang punya file impl — lihat §3.")
    add(f"| blok skeleton (abu berukuran) | {total_design_skel} | {total_impl_skel} |")
    add(f"| `<select>` bawaan HTML | {sum(d['select'] for d in design.values())} | {sum(i['select'] for i in impl.values())} |")
    add(f"| file memakai ligature Material Symbols | {len(ligature_files)} | 0 |")
    add(f"| file memakai `lucide-react` | 0 | {sum(1 for i in impl.values() if i['lucide'])} |")
    add("")

    add("## 2. Icon — dua set, bukan satu")
    add("")
    add("`docs/DESIGN-INVENTORY.md` yang lama bilang mockup memakai satu set")
    add("konsisten Lucide. Itu salah, dan script ini yang membuktikannya:")
    add("")
    add(f"- **{len(ligature_files)} dari {len(design)} file mockup** memakai ligature")
    add("  `<span class=\"material-symbols-outlined\">nama_icon</span>`.")
    add(f"- Sisanya memakai `<svg>` inline yang geometrinya cocok Lucide.")
    add("")
    add("Keputusan repo: **Lucide menang** (memory + konvensi repo). Ligature")
    add("Material Symbols tidak dibawa ke produk.")
    add("")
    add("File mockup yang memakai ligature:")
    add("")
    for sid in ligature_files:
        add(f"- `{sid}`")
    add("")

    add("## 3. Skeleton vs dot status")
    add("")
    add("Dua hal berbeda yang pernah dihitung jadi satu:")
    add("")
    add("- **blok skeleton** — `bg-[#e8ecea]` + ukuran, placeholder isi")
    add("- **dot status** — `animate-pulse` + `rounded-full` + 1.5-2px")
    add("")
    add("| layar | design skeleton | impl skeleton | design dot |")
    add("|---|---|---|---|")
    worst = sorted(design.items(), key=lambda kv: -kv[1]["skeleton"])[:12]
    for sid, d in worst:
        impl_skel = sum(i["skeleton"] for r, i in impl.items() if sid in mapped.get(r, ()))
        if d["skeleton"]:
            add(f"| {sid} | {d['skeleton']} | {impl_skel} | {d['pulse']} |")
    add("")

    add("## 4. Dropdown bawaan HTML yang masih hidup")
    add("")
    add("Kontrak repo: satu komponen `Combobox` buat semua select, token design")
    add("wajib, jangan token bawaan browser. Sisa native:")
    add("")
    add("| file | jumlah | baris |")
    add("|---|---|---|")
    for rel, n, at in select_files:
        where = ", ".join(str(i) for i, _ in at)
        add(f"| `{rel}` | {n} | {where} |")
    add("")

    add("## 5. Warna status: beku vs impl")
    add("")
    frozen = status_palette()
    current = css_status_tokens()
    add("Sumber beku: `DECISIONS.md` §8 paragraf \"Status color\".")
    add("")
    add("| status | beku (DECISIONS §8) | impl (`index.css`) | |")
    add("|---|---|---|---|")
    same = diff = 0
    for name, hexa in sorted(frozen.items()):
        got = current.get(name, "—")
        ok = got == hexa
        same += ok
        diff += not ok
        add(f"| {name} | `{hexa}` | `{got}` | {'sama' if ok else 'BEDA'} |")
    add("")
    add(f"**{same} sama, {diff} beda.**")
    add("")

    add("## 6. Token spacing")
    add("")
    declared, used = dead_spacing_tokens([os.path.join(SRC, r) for r in impl])
    dead = [n for n, c in used.items() if c == 0]
    add(f"Dideklarasikan: {len(declared)}. Nol dipakai: {len(dead)}.")
    add("")
    add("| token | nilai | dipakai |")
    add("|---|---|---|")
    for name, value in sorted(declared.items()):
        add(f"| `{name}` | {value} | {used[name]} |")
    add("")

    add("## 7. State archived")
    add("")
    add("| penanda | design | impl |")
    add("|---|---|---|")
    add(f"| `line-through` | {sum(d['line_through'] for d in design.values())} | {sum(i['line_through'] for i in impl.values())} |")
    add(f"| `opacity-*` | {sum(d['opacity'] for d in design.values())} | {sum(i['opacity'] for i in impl.values())} |")
    add("")

    jargon = {r: i["jargon"] for r, i in impl.items() if i["jargon"]}
    add("## 8. Jargon yang bocor ke UI yang dirender")
    add("")
    add("Kontrak: `US-AD`, `AC`, `M0`-`M6` dilarang masuk UI yang dirender.")
    add("Komentar kode boleh. Yang di bawah ini **bukan** komentar — dihitung")
    add("setelah semua komentar JS dan JSX dibuang.")
    add("")
    if jargon:
        add("Kolom `asal` memisahkan dua pekerjaan yang beda: `mockup` artinya")
        add("design-nya sendiri yang membawa jargon itu (perilaku repo: label dibuang,")
        add("section-nya tetap), `impl` artinya impl yang menambahkannya sendiri.")
        add("")
        add("| file | jargon | asal |")
        add("|---|---|---|")
        for rel, words in sorted(jargon.items()):
            design_body = design.get(owning_screen(rel, mapped), {}).get("body", "")
            origin = "mockup" if any(w in design_body for w in words) else "impl"
            add(f"| `{rel}` | {', '.join(words)} | {origin} |")
    else:
        add("Nol.")
    add("")

    add("## 9. Per layar")
    add("")
    add("| layar | route | file impl | svg design/impl | select impl | skeleton impl |")
    add("|---|---|---|---|---|---|")
    for sid in sorted(screens):
        title, route = screens[sid][0], screens[sid][1]
        if sid in design:
            dsvg = design[sid]["svg"]
        elif re.match(r"^\d+[a-c]?-", sid):
            # mockup bernama lain (mis. 06b-docs-api -> DocsPage)
            dsvg = "—"
        else:
            dsvg = "—"
        mine = [r for r, ids in mapped.items() if sid in ids]
        isvg = sum(impl[r]["svg"] for r in mine)
        isel = sum(impl[r]["select"] for r in mine)
        iskel = sum(impl[r]["skeleton"] for r in mine)
        files = ", ".join(f"`{os.path.basename(r)}`" for r in mine) or "—"
        add(f"| {sid} | `{route}` | {files} | {dsvg}/{isvg} | {isel} | {iskel} |")
    add("")

    add("## 10. File tanpa layar (shell & komponen bersama)")
    add("")
    add(f"{len(unmapped)} file tidak dipetakan ke satu layar. Ini wajar untuk")
    add("layout, komponen UI, dan store — tapi ikut dihitung di total impl.")
    add("")
    for rel in unmapped:
        add(f"- `{rel}`")
    add("")

    return "\n".join(lines) + "\n", jargon, select_files, dead, diff, loading_text


def github_exempt_active():
    """Cek pengecualian `/github` masih sah, bukan alasan buat nutupin temuan.

    `.hermes.md` mengecualikan halaman `/github` dari larangan jargon karena
    halaman itu memang menampilkan track milestone M0-M6. Pengecualian itu cuma
    berlaku selama `Roadmap` dipasang di sana dan di situ saja: begitu komponen
    itu muncul di halaman lain, jargonnya ikut bocor dan pengecualiannya gugur.
    """
    for path in impl_files():
        body = read(path)
        if "<Roadmap" in body:
            rel = os.path.relpath(path, SRC).replace("\\", "/")
            if rel != "routes/public/GitHubPage.tsx":
                return False, f"<Roadmap> juga dipasang di {rel}"
    return True, ""


def main():
    check = "--check" in sys.argv
    screens = load_screens()
    design = scan_design()
    files = impl_files()
    impl = scan_impl(files)
    mapped, unmapped = assign_screens(files, screens, route_map())

    exempt_ok, exempt_why = github_exempt_active()
    report, jargon, select_files, dead, diff, loading_text = build(screens, design, impl, mapped, unmapped)
    with open(OUT, "w", encoding="utf-8", newline="\n") as fh:
        fh.write(report)

    print(f"design   : {len(design)} file")
    print(f"impl     : {len(impl)} file ({len(mapped)} dipetakan, {len(unmapped)} shell/bersama)")
    print(f"laporan  : {os.path.relpath(OUT, ROOT)} ({len(report)} char)")
    print()
    if jargon:
        print(f"JARGON   : {len(jargon)} file bocor ke UI")
        for rel, words in sorted(jargon.items()):
            print(f"           {rel}: {', '.join(words)}")
    else:
        print("JARGON   : nol")
    if exempt_ok:
        print(f"           (+{len(GITHUB_EXEMPT)} file track M0-M6 di /github, dikecualikan kontrak)")
    else:
        print(f"           pengecualian /github GUGUR: {exempt_why}")
    print(f"SELECT   : {len(select_files)} file masih pakai <select> bawaan")
    for rel, n, at in select_files:
        print(f"           {rel}: {n} (baris {', '.join(str(i) for i, _ in at)})")
    print(f"WARNA    : {diff} token status beda dari DECISIONS §8")
    print(f"SPACING  : {len(dead)} token dideklarasikan tapi nol dipakai")
    # A screen whose loading state is a line of text is a screen the design
    # already solved (43-state-loading draws it as grey blocks). Reported, not
    # failed: the remaining hits are button labels mid-flight ("Memuat…" on a
    # submit), which are not the state this rule is about.
    if loading_text:
        print(f"LOADING  : {len(loading_text)} file masih merender teks loading")
        for rel, n, at in loading_text[:6]:
            print(f"           {rel}: {n} (baris {', '.join(str(i) for i, _ in at)})")

    if check:
        if not exempt_ok:
            FAILED.append(f"JARGON: pengecualian /github gugur — {exempt_why}")
        for rel, words in sorted(jargon.items()):
            FAILED.append(f"JARGON: {rel}: {', '.join(words)}")
        for rel, n, _at in select_files:
            FAILED.append(f"SELECT: {rel}: {n} <select> bawaan")
        if FAILED:
            print()
            for msg in FAILED:
                print(f"FAIL  {msg}")
            return 1
        print()
        print("BERSIH: nol jargon bocor, nol <select> bawaan")
    return 0


if __name__ == "__main__":
    sys.exit(main())
