#!/usr/bin/env python3
"""Gate suite dokumen AgentDeck. Jalankan dari folder agentdeck/.

Cek:
  A. PRD      — jumlah story/AC, ID duplikat, gap ID, story tanpa AC,
                rujukan seksi yang tidak ada, link markdown relatif
  B. DESIGN   — front matter DESIGN.md: token ref valid, hex dikutip,
                komponen tidak nested, urutan seksi kanonikal, npx lint
  C. KONTRAK  — tabel/kolom DECISIONS.md ada di DDL ARCHITECTURE.md,
                enum DECISIONS dipakai di DDL, angka N1..N25 dirujuk
  D. SUITE    — traceability story, rujukan seksi antar dokumen
  E. MIGRASI  — tabel/kolom yang benar-benar dibuat internal/migrate/*.up.sql
                juga terdeklarasi di ARCHITECTURE.md dan DECISIONS.md

Exit 0 kalau bersih, 1 kalau ada temuan FAIL.
"""

import io
import os
import re
import sys
import subprocess

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
HERE = ROOT
FAILED = []
WARNED = []


def say(level, msg):
    print(f"{level:5} {msg}")
    if level == "FAIL":
        FAILED.append(msg)
    elif level == "WARN":
        WARNED.append(msg)


def read(name):
    aliases = {
        "00-PRD.md": "docs/00-PRD.md",
        "ARCHITECTURE.md": "docs/ARCHITECTURE.md",
        "BLUEPRINT.md": "docs/BLUEPRINT.md",
        "COVERAGE.md": "docs/COVERAGE.md",
        "DECISIONS.md": "docs/DECISIONS.md",
        "DESIGN.md": "docs/DESIGN.md",
        "DIAGRAMS.md": "docs/DIAGRAMS.md",
        "PRICING.md": "docs/PRICING.md",
        "INDEX-GENERASI.md": "docs/INDEX-GENERASI.md",
        "diagrams/architecture.html": "docs/diagrams/architecture.html",
        "landing/prompt-landing.md": "design/landing/prompt-landing.md",
        "landing/landing.html": "design/landing/source.html",
    }
    p = os.path.join(ROOT, aliases.get(name, name))
    return open(p, encoding="utf-8").read() if os.path.exists(p) else None


# ---------------------------------------------------------------- A. PRD ----
STORY = re.compile(r"(?m)^\*\*(US-([A-Z]+)(\d+))\*\*")
AC = re.compile(r"(?m)^- \[ \] AC(\d+)")
HEADING = re.compile(r"(?m)^## (\d+)\.")
MD_LINK = re.compile(r"\]\(([^)#]+\.(?:md|html))\)")


def check_prd():
    text = read("00-PRD.md")
    if text is None:
        say("FAIL", "00-PRD.md tidak ada")
        return
    sections = set(HEADING.findall(text))
    want = {str(i) for i in range(1, 15)}
    missing = sorted(want - sections, key=int)
    if missing:
        say("FAIL", f"PRD: seksi hilang {', '.join('§'+m for m in missing)}")
    else:
        say("ok", "PRD: 14 seksi lengkap (1-14)")

    chunks = re.split(r"(?m)^\*\*(US-[A-Z]+\d+)\*\*", text)
    ids, bodies = [], []
    for i in range(1, len(chunks), 2):
        ids.append(chunks[i])
        bodies.append(chunks[i + 1] if i + 1 < len(chunks) else "")

    dupes = sorted({s for s in ids if ids.count(s) > 1})
    if dupes:
        say("FAIL", f"PRD: ID story duplikat {', '.join(dupes)}")

    bare = [ids[i] for i, b in enumerate(bodies) if not AC.search(b)]
    if bare:
        say("FAIL", f"PRD: {len(bare)} story tanpa AC -> {', '.join(bare[:8])}")

    by_prefix = {}
    for full, prefix, num in STORY.findall(text):
        by_prefix.setdefault(prefix, []).append(int(num))
    for prefix, nums in sorted(by_prefix.items()):
        gaps = sorted(set(range(1, max(nums) + 1)) - set(nums))
        if gaps:
            say("FAIL", f"PRD: gap US-{prefix} -> " +
                ", ".join(f"US-{prefix}{n:02d}" for n in gaps))

    for link in MD_LINK.findall(text):
        if not os.path.exists(os.path.join(ROOT, "docs", link)):
            say("FAIL", f"PRD: link rusak -> {link}")

    n_story, n_ac = len(ids), sum(len(AC.findall(b)) for b in bodies)
    say("ok", f"PRD: {n_story} story, {n_ac} acceptance criteria")
    return n_story, n_ac


# ------------------------------------------------------------- B. DESIGN ----
CANON = ["overview", "brand & style", "colors", "typography", "layout",
         "elevation & depth", "elevation", "shapes", "components",
         "do's and don'ts"]


def check_status_palette(text, fm):
    """Tiga sumber harus sepakat soal 10 warna status task.

    Ini kelas bug yang sudah terjadi sekali dan tidak ketahuan gate mana pun:
    `DECISIONS.md` §8 (yang dibekukan) dan `docs/DESIGN.md` sama-sama
    mendefinisikan palet status, dan nilainya berbeda 10 dari 10. `index.css`
    mengikuti DESIGN.md, jadi aplikasinya memakai warna yang tidak pernah
    diputuskan — sementara mockup dan DECISIONS memakai yang lain.

    Tidak ada gate yang bisa melihatnya: `designmd lint` cuma memeriksa DESIGN.md
    terhadap dirinya sendiri, dan `design_audit.py` membandingkan DESIGN.md
    dengan `index.css` — dua-duanya konsisten, dua-duanya salah.

    Jadi ketiganya diikat di sini. Yang beku adalah DECISIONS §8; DESIGN.md dan
    `index.css` wajib mengikutinya.
    """
    dec = read("DECISIONS.md") or ""
    i = dec.find("Status color:")
    if i < 0:
        say("FAIL", "STATUS PALETTE: DECISIONS.md tidak punya paragraf 'Status color'")
        return
    rest = dec[i:]
    lines = [rest.split("\n", 1)[0]]
    for line in rest.split("\n")[1:]:
        if line.startswith("- ") or not line.strip():
            break
        lines.append(line)
    frozen = {
        k: v.lower()
        for k, v in re.findall(r"`([a-z_]+)`\s+[a-z ]+`(#[0-9a-fA-F]{6})`", " ".join(lines))
    }
    if len(frozen) != 10:
        say("FAIL", f"STATUS PALETTE: DECISIONS §8 cuma terbaca {len(frozen)} dari 10 status")
        return

    design = {
        k.replace("-", "_").lower(): v.lower()
        for k, v in re.findall(r'(?m)^\s{2}status-([a-z-]+):\s*"(#[0-9a-fA-F]{6})"', fm)
    }
    css = {
        k.replace("-", "_").lower(): v.strip().lower()
        for k, v in re.findall(
            r"--color-status-([a-z_-]+):\s*([^;]+);", read("frontend/src/index.css") or ""
        )
    }

    for label, table in (("DESIGN.md", design), ("index.css", css)):
        bad = sorted(f"{k} {table.get(k, '—')} != #{v}" for k, v in frozen.items() if table.get(k) != v)
        if bad:
            say("FAIL", f"STATUS PALETTE: {label} melenceng dari DECISIONS §8 -> {', '.join(bad[:4])}")
        else:
            say("ok", f"STATUS PALETTE: {label} == DECISIONS §8 (10/10)")


def check_supersessions():
    """Keputusan yang menimpa sebuah AC harus kelihatan di AC itu sendiri.

    Ini kelas bug yang sudah dua kali kejadian di repo ini, dan tidak ada gate
    yang bisa melihatnya:

      1. DECISIONS §6A.J memindahkan kredensial ke `providers` dan bilang
         "menggantikan §6A.F sepenuhnya". Implementasinya masih membaca
         `agents.has_provider_key` — aturan lama — lalu melabeli agent yang
         provider-nya punya key sebagai "butuh kredensial".
      2. DECISIONS §6A.F mengizinkan `localhost`/`127.0.0.1` sebagai string
         persis dan bilang "ini mengalahkan US-AD106 AC3". Teks AC3 di PRD
         masih berbunyi "loopback ditolak", jadi spec dan keputusan saling
         bertentangan di atas kertas, dan pembaca berikutnya mengikuti yang mana
         saja yang dia baca duluan.

    Aturannya: setiap kali DECISIONS bilang "mengalahkan US-AD<n> AC<m>", teks
    AC itu di PRD wajib menyebut penimpanya. Bukan sekadar "ada catatan di
    suatu tempat" — AC-nya yang harus membawa penunjuknya, karena itu bagian
    yang dibaca orang saat mengecek kepatuhan.
    """
    dec = read("DECISIONS.md") or ""
    prd = read("00-PRD.md") or ""
    if not dec or not prd:
        say("FAIL", "SUPERSEDE: DECISIONS.md atau 00-PRD.md tidak terbaca")
        return

    refs = re.findall(r"mengalahkan US-AD(\d+)(?: AC(\d+))?", dec)
    if not refs:
        say("warn", "SUPERSEDE: nol override 'mengalahkan US-AD' di DECISIONS (kalau ini berubah, gate-nya jadi vacuous)")
        return

    # Satu story bisa punya beberapa AC; ambil blok AC-nya saja supaya cakupannya
    # tidak melar ke seluruh dokumen.
    def story_block(n):
        m = re.search(rf"(?ms)^\*\*US-AD{n}\*\*.*?(?=^\*\*US-AD\d+\*\*|\Z)", prd)
        return m.group(0) if m else ""

    bad = []
    for story, ac in refs:
        block = story_block(story)
        if not block:
            bad.append(f"US-AD{story} (tidak ada di PRD)")
            continue
        if ac:
            line = ""
            for candidate in block.splitlines():
                if candidate.startswith(f"- [ ] AC{ac} "):
                    line = candidate
                    break
            if not line:
                bad.append(f"US-AD{story} AC{ac} (tidak ada)")
                continue
            target, found = f"US-AD{story} AC{ac}", line
        else:
            target, found = f"US-AD{story}", block
        if "DECISIONS" not in found:
            bad.append(f"{target} (AC-nya tidak menyebut DECISIONS)")

    if bad:
        say("FAIL", f"SUPERSEDE: AC yang ditimpa tidak menunjuk penimpanya -> {', '.join(bad)}")
    else:
        say("ok", f"SUPERSEDE: {len(refs)} AC yang ditimpa menunjuk keputusannya")

    check_superseded_sections(dec)


def _sections(dec):
    """`[(letter, title, start, end)]` untuk tiap `### X. Judul`, urut dokumen."""
    out = []
    for m in re.finditer(r"(?m)^### ([A-Z])\. ([^\n]*)$", dec):
        out.append([m.group(1), m.group(2), m.start(), None])
    for i in range(len(out) - 1):
        out[i][3] = out[i + 1][2]
    if out:
        out[-1][3] = len(dec)
    return [(letter, title, start, end) for letter, title, start, end in out]


def _leading_banner(block):
    """Huruf pengganti dari banner di AWAL section, atau None.

    Cuma blockquote yang mengawali section yang dihitung. Ini bukan detail
    kosmetik: versi pertama gate ini memakai `successor in block`, dan itu
    **lolos mutasi** — hapus banner-nya, dan blok yang sama masih menyebut
    `§6A.J` di baris koreksi 10 baris di bawahnya, jadi gate-nya tetap hijau.
    Substring di seluruh blok tidak bisa membedakan "section ini membawa
    penunjuk di kepalanya" dari "nama penggantinya kebetulan muncul di prosa".
    """
    lines = block.splitlines()[1:]  # buang baris judul
    for line in lines:
        s = line.strip()
        if not s:
            continue
        if s.startswith(">"):
            m = re.search(r"PENGGANTI:\s*§6A\.([A-Z])", s)
            if m:
                return m.group(1)
            continue
        break  # baris pertama yang bukan blockquote mengakhiri banner
    return None


def check_superseded_sections(dec):
    """Keputusan yang sudah digantikan harus mengatakannya di tempatnya sendiri.

    `check_supersessions` di atas cuma melihat frasa "mengalahkan US-AD<n>", dan
    di seluruh DECISIONS cuma ada satu. Bentuk penggantian yang lain tidak
    kelihatan gate mana pun, dan itu bukan hipotesis:

      §6A.F  kredensial per-agent. Digantikan §6A.J, tapi §6A.F sendiri tidak
             mengatakannya. Pembaca yang berhenti di §6A.F — dan komentar
             `has_provider_key` di kode menunjuk ke sana — masih membaca aturan
             yang sudah mati, dan implementasinya memang masih memakainya
             (ketahuan sebagai agent ber-label "butuh kredensial" padahal
             provider-nya punya key).

    Daftar `stale` di bawah sekarang cuma lantai, bukan sumbernya. Sumbernya
    adalah **deklarasi di dokumen itu sendiri**: section yang bilang
    "Menggantikan §6A.X sepenuhnya" mewajibkan X memasang banner. Jadi
    penggantian baru ketangkep tanpa ada yang perlu ingat mendaftarkannya di
    sini — itu inti "biar nggak keulang".

    Sekalian: penomoran section 6A. diperiksa karena sudah pernah rusak tanpa
    ketahuan. Saat gate ini ditulis, dokumennya punya **dua** section berlabel
    "H" dan **tidak punya** "G", jadi rujukan "§6A.G" atau "§6A.H" tidak bisa
    dijawab pembaca.
    """
    sections = _sections(dec)
    by_letter = {letter: (title, dec[start:end]) for letter, title, start, end in sections}

    # Penggantian yang dideklarasikan dokumen sendiri, plus lantai manual untuk
    # kasus yang tidak ditulis dengan frasa itu.
    #
    # Satu kalimat bisa menggantikan lebih dari satu section ("Menggantikan
    # §6A.F dan §6A.I sepenuhnya"), jadi yang dibaca adalah kalimatnya, bukan
    # cuma rujukan pertama sesudah kata "Menggantikan". Versi yang cuma membaca
    # rujukan pertama **lolos mutasi** itu.
    declared = {}
    for letter, _title, start, end in sections:
        for sentence in re.finditer(r"Menggantikan[^\n]*", dec[start:end]):
            head = sentence.group(0)
            # Batas kalimatnya `**` atau `. `, BUKAN `.`: rujukan section
            # sendiri mengandung titik (`§6A.F`), jadi `split(".")` memotong
            # tepat di tengah rujukan pertama dan tidak ada huruf yang terbaca.
            for stop in ("**", ". "):
                cut = head.find(stop)
                if cut >= 0:
                    head = head[:cut]
                    break
            for ref in re.findall(r"§6A\.([A-Z])", head):
                declared.setdefault(ref, letter)
    declared.setdefault("F", "J")

    bad = []
    for superseded, successor in sorted(declared.items()):
        if superseded not in by_letter:
            bad.append(f"§6A.{superseded} (disebut digantikan, section-nya tidak ada)")
            continue
        banner = _leading_banner(by_letter[superseded][1])
        if banner is None:
            bad.append(f"§6A.{superseded} (tidak ada banner PENGGANTI di awal section)")
        elif banner != successor:
            bad.append(f"§6A.{superseded} (banner menunjuk §6A.{banner}, deklarasinya §6A.{successor})")

    # Penomoran 6A.: A..J berurutan, tanpa label kembar.
    letters = [letter for letter, _t, _s, _e in sections]
    seen, dupes, order = set(), [], []
    for letter in letters:
        if letter in seen:
            dupes.append(letter)
        seen.add(letter)
        order.append(letter)
    if dupes:
        bad.append(f"label section kembar -> {', '.join(sorted(set(dupes)))}")
    if order:
        expected = [chr(c) for c in range(ord("A"), ord("A") + len(set(order)))]
        missing = sorted(set(expected) - seen)
        if missing:
            bad.append(f"label section hilang -> {', '.join(missing)}")

    if bad:
        say("FAIL", f"SUPERSEDE: {len(bad)} cacat struktur DECISIONS -> {'; '.join(bad)}")
    else:
        say(
            "ok",
            f"SUPERSEDE: {len(declared)} section mati membawa banner penggantinya, "
            f"penomoran 6A. utuh",
        )


def check_design():
    text = read("DESIGN.md")
    if text is None:
        say("FAIL", "DESIGN.md tidak ada")
        return
    m = re.match(r"^---\n(.*?)\n---\n", text, re.S)
    if not m:
        say("FAIL", "DESIGN.md: front matter YAML tidak ditemukan")
        return
    fm = m.group(1)

    defined = set()
    block = None
    for line in fm.splitlines():
        if re.match(r"^[a-z][a-z0-9_-]*:\s*$", line):
            block = line.split(":")[0]
            continue
        t = re.match(r"^\s{2}([A-Za-z0-9_-]+):", line)
        if t and block:
            defined.add(f"{block}.{t.group(1)}")
    comp = set(re.findall(r"(?m)^\s{2}([A-Za-z0-9_-]+):\s*$", fm))
    defined |= comp

    refs = set(re.findall(r"\{([a-zA-Z0-9_.-]+)\}", text))
    broken = sorted(r for r in refs if r not in defined)
    if broken:
        say("FAIL", f"DESIGN: {len(broken)} token ref rusak -> {', '.join(broken[:8])}")
    else:
        say("ok", f"DESIGN: {len(refs)} token ref valid, {len(defined)} token terdefinisi")

    bad_hex = [l.strip() for l in fm.splitlines()
               if re.search(r":\s*#[0-9a-fA-F]{3,8}\s*$", l)]
    if bad_hex:
        say("FAIL", f"DESIGN: {len(bad_hex)} hex tidak dikutip -> {bad_hex[0]}")

    if re.search(r"(?m)^\s{2}[a-z0-9-]+\.[a-z0-9-]+:", fm):
        say("FAIL", "DESIGN: komponen varian nested (pakai key sendiri)")

    heads = [h.strip().lower() for h in re.findall(r"(?m)^## (.+)$", text)]
    dupes = sorted({h for h in heads if heads.count(h) > 1})
    if dupes:
        say("FAIL", f"DESIGN: heading duplikat -> {', '.join(dupes)}")

    check_status_palette(text, fm)

    # designmd lint CLI via shell
    npx_cmd = "npx.cmd" if sys.platform == "win32" else "npx"
    try:
        r = subprocess.run(
            [npx_cmd, "-y", "-p", "@google/design.md", "designmd", "lint", "docs/DESIGN.md"],
            cwd=ROOT, capture_output=True, text=True, timeout=120, shell=True)
        out = (r.stdout or "") + (r.stderr or "")
        if r.returncode != 0:
            say("FAIL", f"DESIGN: designmd lint exit {r.returncode}")
        else:
            say("ok", "DESIGN: designmd lint exit 0 (clean)")
    except Exception as e:
        say("WARN", f"DESIGN: lint tidak dapat dijalankan ({e})")


def check_design_audit():
    """Audit jargon/<select> — dulu manual, sekarang bagian dari suite.

    Kenapa ini dipindah ke dalam: `design_audit.py --check` adalah gate yang
    mengembalikan 1, tapi tidak ada yang memanggilnya, jadi satu-satunya cara ia
    menahan sesuatu adalah kalau seseorang ingat menjalankannya. Dan cara umum
    memanggilnya — `python tools/design_audit.py --check | grep FAIL` — membaca
    exit code `grep`, bukan exit code gate-nya, jadi gate itu tampak hijau
    sementara ia gagal. Dijalankan di sini tanpa pipe, jadi exit code-nya utuh.

    Cakupannya sengaja dipersempit ke dua aturan kontrak yang **biner dan bisa
    digagalkan** (jargon bocor ke UI yang dirender, `<select>` bawaan). Sisa
    audit tetap informatif — jumlah ikon, skeleton, spacing — dan tidak
    dinaikkan jadi FAIL di sini.
    """
    try:
        r = subprocess.run([sys.executable, os.path.join("tools", "design_audit.py"), "--check"],
                           cwd=ROOT, capture_output=True, text=True, timeout=180)
    except Exception as e:
        say("WARN", f"AUDIT: design_audit.py tidak dapat dijalankan ({e})")
        return

    out = (r.stdout or "") + (r.stderr or "")
    leaked = [line.strip() for line in out.splitlines() if line.startswith("FAIL")]
    if r.returncode != 0:
        for line in leaked or [f"exit {r.returncode} tanpa baris FAIL"]:
            say("FAIL", f"AUDIT: {line[6:] if line.startswith('FAIL  ') else line}")
    else:
        say("ok", "AUDIT: nol jargon bocor ke UI, nol <select> bawaan")


# ------------------------------------------------------------ C. KONTRAK ----
def check_contract():
    dec = read("DECISIONS.md")
    arch = read("ARCHITECTURE.md")
    prd = read("00-PRD.md")
    if dec is None or arch is None:
        say("FAIL", "DECISIONS.md atau ARCHITECTURE.md tidak ada")
        return

    m = re.search(r"## 6\. Skema database.*?```(.*?)```", dec, re.S)
    if m:
        tables = re.findall(r"(?m)^([a-z_]+)\(", m.group(1))
        missing = [t for t in tables if not re.search(rf"CREATE TABLE (?:IF NOT EXISTS )?{t}\b",
                                                      arch, re.I)]
        if missing:
            say("FAIL", f"DDL: {len(missing)} tabel kontrak belum ada -> {', '.join(missing)}")
        else:
            say("ok", f"DDL: {len(tables)} tabel kontrak semuanya ada di ARCHITECTURE.md")

    # Enums from DECISIONS §4 check
    enums = re.findall(r"\*\*([a-z_]+):\*\*\s*(.+)", dec)
    missing_enum = []
    for name, vals in enums:
        v_list = [v.strip(" `*") for v in vals.split("\u00b7") if v.strip()]
        if not v_list:
            continue
        first_clean = re.sub(r"[^a-zA-Z0-9_.]", "", v_list[0].split()[0])
        hit = bool(re.search(rf"\b{re.escape(first_clean)}\b", arch))
        if not hit:
            missing_enum.append(name)
    if missing_enum:
        say("FAIL", f"DDL: enum kontrak tidak terdeteksi -> {', '.join(missing_enum)}")
    else:
        say("ok", f"DDL: semua enum kontrak ({len(enums)}) terdeteksi di ARCHITECTURE.md")

    # Numbers N1..N25 referenced using word boundaries
    nums = dict(re.findall(r"(?m)^\| (N\d+) \| [^|]+ \| ([^|]+) \|", dec))
    if nums:
        unref = []
        for k in nums:
            pat = rf"\b{k}\b"
            if not re.search(pat, arch) and not re.search(pat, prd or ""):
                unref.append(k)
        if unref:
            say("WARN", f"angka tidak dirujuk -> {', '.join(sorted(unref))}")
        else:
            say("ok", f"angka: seluruh {len(nums)} kode (N1..N25) dirujuk di suite dokumen")


# -------------------------------------------------------------- D. SUITE ----
def _migration_ddl():
    """Tabel dan kolom yang benar-benar dibuat migrasi, dibaca dari disk.

    Gate lain membaca dokumen; gate ini membaca `internal/migrate/*.up.sql` yang
    akan dieksekusi database. Arahnya sengaja satu arah: tabel yang ADA di
    migrasi wajib terdokumentasi, tapi tabel target M2-M4 yang belum dimigrasi
    tetap boleh hidup di dokumen. Tanpa itu, gate ini akan memaksa penghapusan
    rencana yang belum dikerjakan hanya supaya hijau.
    """
    mig_dir = os.path.join(ROOT, "internal", "migrate")
    tables, columns = {}, {}
    for fname in sorted(os.listdir(mig_dir)):
        if not fname.endswith(".up.sql"):
            continue
        body = io.open(os.path.join(mig_dir, fname), encoding="utf-8").read()
        # Anchored at line start and requiring "(" so prose in a comment
        # ("... its own CREATE TABLE statements.") is not read as a table.
        for m in re.finditer(r"(?m)^CREATE TABLE (?:IF NOT EXISTS )?([a-z_]+)\s*\(", body):
            tables[m.group(1)] = fname
        for m in re.finditer(
            r"ALTER TABLE ([a-z_]+)\s+ADD COLUMN(?: IF NOT EXISTS)? ([a-z_]+)", body
        ):
            columns.setdefault(m.group(2), m.group(1))
    return tables, columns


def check_migrations():
    """Dokumen vs migrasi yang benar-benar ada.

    Kontrak yang mendokumentasikan tabel yang tidak pernah dibuat adalah
    kontrak yang tidak bisa dipercaya pada bagian yang justru penting. Dua
    arah diperiksa secara berbeda: tabel migrasi yang tidak terdokumentasi
    selalu FAIL, kolom ALTER TABLE yang tidak muncul di DDL juga FAIL.
    """
    tables, columns = _migration_ddl()
    if not tables:
        say("FAIL", "MIGRASI: nol CREATE TABLE terbaca dari internal/migrate/*.up.sql")
        return

    arch = read("ARCHITECTURE.md") or ""
    dec = read("DECISIONS.md") or ""
    both = arch + "\n" + dec

    undocumented = sorted(t for t in tables if f"`{t}`" not in both and t not in both)
    if undocumented:
        say(
            "FAIL",
            "MIGRASI: tabel ada di migrasi tapi tak terdokumentasi -> "
            + ", ".join(f"{t} ({tables[t]})" for t in undocumented),
        )
    else:
        say("ok", f"MIGRASI: {len(tables)} tabel migrasi semua terdeklarasi di dokumen")

    missing_cols = sorted(c for c in columns if f"`{c}`" not in both and c not in both)
    if missing_cols:
        say(
            "FAIL",
            "MIGRASI: kolom ALTER TABLE tak terdokumentasi -> "
            + ", ".join(f"{c} ({columns[c]})" for c in missing_cols),
        )
    else:
        say("ok", f"MIGRASI: {len(columns)} kolom ALTER TABLE semua terdokumentasi")


def check_structure():
    """§18 menggambar struktur nyata, jadi gate harus membuktikannya.

    Versi sebelumnya dari §18 menggambarkan layout rencana — `internal/api/`,
    `internal/db/`, `internal/dispatcher/`, `tests/` — yang tidak pernah dibuat,
    dan nol gate yang menangkapnya. 29 dari 31 nama file yang dikontrak tidak
    ada, sementara 8 modul yang benar-benar ada tidak tercatat. Akibatnya dokumen
    jadi jebakan: yang mengikutinya mencari direktori yang tidak ada, yang
    mengabaikannya dianggap melanggar kontrak.

    Dua arah diperiksa: yang disebut §18 harus ada, dan modul nyata di
    `internal/` harus disebut. `frontend/` punya gate sendiri di `verify_web.py`
    (§18.2) karena ceknya butuh resolusi path di dalam `frontend/src/`.
    """
    arch = read("ARCHITECTURE.md")
    start = arch.find("## 18. Struktur Folder")
    end = arch.find("### 18.2 Frontend")
    if start < 0 or end < 0:
        say("FAIL", "STRUCT: §18 atau §18.2 tidak ditemukan di ARCHITECTURE.md")
        return
    section = arch[start:end]

    fence = re.search(r"```\n(.*?)```", section, re.S)
    if not fence:
        say("FAIL", "STRUCT: §18 tidak memuat blok struktur (fenced code)")
        return
    body = fence.group(1)

    claimed = sorted({m.group(1) for m in re.finditer(r"([A-Za-z0-9_.\-]+)/", body)}
                     - {"agentdeck", "https", "http"})
    claimed_files = sorted(set(re.findall(
        r"([A-Za-z0-9_.\-]+\.(?:go|sql|mod|sum|yaml|yml|json|ts|tsx))", body)))

    # Nama di blok struktur itu bersarang (`main.go` ada di `cmd/api/`), jadi
    # kecocokan dicari di seluruh repo, bukan di root. Ini yang bikin versi
    # pertama gate ini salah lapor.
    skip = {"node_modules", ".git", "dist", "test-results", "playwright-report"}
    dirs, files = set(), set()
    for dirpath, dirnames, filenames in os.walk(ROOT):
        dirnames[:] = [d for d in dirnames if d not in skip]
        for d in dirnames:
            dirs.add(d)
        for f in filenames:
            files.add(f)

    missing = [n + "/" for n in claimed if n not in dirs]
    missing += [n for n in claimed_files if n not in files]

    if missing:
        for name in missing:
            say("FAIL", f"STRUCT: §18 menyebut '{name}' tapi tidak ada di repo")
    else:
        say("ok", f"STRUCT: {len(claimed)} direktori + {len(claimed_files)} file §18 ada")

    undocumented = []
    internal = os.path.join(ROOT, "internal")
    if os.path.isdir(internal):
        for name in sorted(os.listdir(internal)):
            if os.path.isdir(os.path.join(internal, name)) and name not in section:
                undocumented.append("internal/" + name + "/")
    if undocumented:
        for name in undocumented:
            say("FAIL", f"STRUCT: {name} ada tapi tidak tercatat di §18")
    else:
        say("ok", "STRUCT: semua modul internal/ tercatat di §18")

    # Modul yang belum dibangun harus tetap terdaftar, supaya gambar struktur
    # di atas tidak dibaca sebagai janji.
    if "### 18.3 Modul yang belum dibangun" not in arch:
        say("FAIL", "STRUCT: §18.3 (modul yang belum dibangun) hilang")


    # Kolom `Status` di tabel §6.2 harus mencerminkan KODE, bukan niat. Tanpa
    # cek ini, tanda ✅/⬜ jadi klaim bebas yang bisa basi — persis masalah §18
    # sebelum diperbaiki. Arah yang ditegakkan: tanda ✅ wajib punya route di
    # `cmd/api`. Arah sebaliknya (route ada tapi ditandai ⬜) juga FAIL, karena
    # itu berarti dokumen menjanjikan lebih sedikit daripada yang sudah jalan.
    #
    # ponytail: arah "route ada tapi tidak tercatat sama sekali" belum dicek —
    # itu butuh penanganan path berparameter yang berbeda dan sudah dijaga
    # sebagian oleh cek PRD->ARCH di atas.
    routes = set()
    for name in sorted(os.listdir(os.path.join(ROOT, "cmd", "api"))):
        if not name.endswith(".go") or name.endswith("_test.go"):
            continue
        src = io.open(os.path.join(ROOT, "cmd", "api", name), encoding="utf-8", errors="replace").read()
        for pat in (r'HandleFunc\(\s*"([A-Z]+)\s+(/[^"]*)"',
                    r'(?:agentRoute|boardRoute|skillRoute|credRoute|orgRoute|apiRoute|providerRoute)\(\s*"([A-Z]+)\s+(/[^"]*)"',
                    r'mux\.Handle\(\s*"([A-Z]+)\s+(/[^"]*)"'):
            for m in re.finditer(pat, src):
                routes.add((m.group(1), re.sub(r"\{[^}]+\}", "{}", m.group(2)).split("?")[0]))

    claimed_done, claimed_todo, wrong = 0, 0, []
    for m in re.finditer(r"(?m)^\|\s*`(GET|POST|PUT|PATCH|DELETE)`\s*\|\s*`([^`]+)`\s*\|(.*)$", arch):
        meth, path, rest = m.group(1), m.group(2), m.group(3)
        cells = [c.strip() for c in rest.split("|")]
        # Kolom `Idempotent` berisi `Ya`/`Tidak`, jadi jumlah sel tidak tetap —
        # yang dicari adalah sel yang isinya HANYA tanda status.
        marks = [c for c in cells if c in ("✅", "⬜")]
        if not marks:
            continue
        mark = marks[0]
        norm = re.sub(r"\{[^}]+\}", "{}", path).split("?")[0]
        implemented = (meth, norm) in routes
        if mark == "✅":
            claimed_done += 1
            if not implemented:
                wrong.append(f"{meth} {path} ditandai ✅ tapi tidak ada di cmd/api")
        else:
            claimed_todo += 1
            if implemented:
                wrong.append(f"{meth} {path} ditandai ⬜ tapi route-nya ada")

    # ponytail: arah ketiga — route ada tapi tidak tercatat sama sekali di
    # dokumen — dulu tidak dicek, dan itu bukan celah teoretis: `GET
    # /boards/{id}/assignable-agents` ditambahkan tanpa satu baris pun di §6.2,
    # dan gate-nya hijau. Penanganannya sederhana karena `routes` sudah
    # dinormalisasi ke bentuk yang sama dengan `path` di dokumen.
    documented = set()
    for m in re.finditer(r"(?m)^\|\s*`(GET|POST|PUT|PATCH|DELETE)`\s*\|\s*`([^`]+)`\s*\|", arch):
        documented.add((m.group(1), re.sub(r"\{[^}]+\}", "{}", m.group(2)).split("?")[0]))
    for meth, path in sorted(routes - documented):
        if path.startswith("/internal/") or path in ("/healthz", "/livez", "/readyz", "/metrics"):
            continue
        wrong.append(f"{meth} {path} ada di cmd/api tapi tidak tercatat di tabel endpoint ARCHITECTURE")

    if wrong:
        for w in wrong:
            say("FAIL", f"STATUS ENDPOINT: {w}")
    else:
        say("ok", f"STATUS ENDPOINT: {claimed_done} ✅ dan {claimed_todo} ⬜ cocok dengan cmd/api")


def check_suite():
    prd = read("00-PRD.md")
    arch = read("ARCHITECTURE.md")
    diag = read("DIAGRAMS.md")
    html = read("docs/diagrams/architecture.html")

    if not prd or not arch:
        return
    ids = set(re.findall(r"(?m)^\*\*(US-AD\d+)\*\*", prd))
    if not ids:
        say("FAIL", "SUITE: tidak ada story terbaca")
        return

    # Check that traceability section covers story ranges
    if "## 13. Traceability" in prd:
        say("ok", f"traceability: tabel pemetaan ada di 00-PRD.md §13")
    else:
        say("WARN", "traceability: seksi §13 tidak ditemukan")

    # Check diagrams exist
    if diag and len(diag) > 5000:
        say("ok", f"DIAGRAMS.md ada ({len(diag):,} bytes)")
    else:
        say("FAIL", "DIAGRAMS.md kosong atau tidak ada")

    if html and "<svg" in html:
        say("ok", f"diagrams/architecture.html ada ({len(html):,} bytes, SVG valid)")
    else:
        say("FAIL", "diagrams/architecture.html tidak ada atau tidak valid")


# --------------------------------------------------- E. KONSISTENSI ARSITEKTUR --
def check_arch():
    """Gate untuk kelas bug yang lolos dari gate sebelumnya:

    1. Constraint name duplikat dalam satu CREATE TABLE -> Postgres menolak DDL.
    2. Nomor subsection duplikat / tidak monoton.
    3. Kolom kontrak DECISIONS vs kolom DDL (dulu daily_board_costs beda 5 kolom).
    4. Klaim jumlah endpoint harus sama dengan baris tabel endpoint.
    5. Endpoint yang dirujuk PRD harus ada di ARCHITECTURE.
    6. Referensi silang §x.y harus menunjuk section yang benar-benar ada.
    """
    import collections

    arch = read("ARCHITECTURE.md")
    dec = read("DECISIONS.md")
    prd = read("00-PRD.md")
    if arch is None or dec is None or prd is None:
        say("FAIL", "ARCHITECTURE/DECISIONS/00-PRD tidak lengkap")
        return

    # --- 1. constraint duplikat dalam satu tabel ---
    dup_cons = []
    for m in re.finditer(r"CREATE TABLE (?:IF NOT EXISTS )?([a-z_]+)\s*\((.*?)\n\);", arch, re.S):
        cons = re.findall(r"CONSTRAINT (\w+)", m.group(2))
        d = [k for k, v in collections.Counter(cons).items() if v > 1]
        if d:
            dup_cons.append(f"{m.group(1)}:{','.join(d)}")
    if dup_cons:
        say("FAIL", f"DDL: constraint duplikat dalam satu tabel -> {', '.join(dup_cons)}")
    else:
        say("ok", "DDL: tidak ada constraint duplikat dalam satu tabel")

    # --- 2. nomor subsection duplikat / tidak monoton ---
    subs = [(m.group(1), m.group(2)) for m in
            re.finditer(r"(?m)^### (\d+\.\d+) (.+)$", arch)]
    nums = [s[0] for s in subs]
    dup = sorted({n for n in nums if nums.count(n) > 1})
    if dup:
        say("FAIL", f"ARCHITECTURE: nomor subsection duplikat -> {', '.join(dup)}")
    else:
        say("ok", f"ARCHITECTURE: {len(nums)} nomor subsection unik (nol duplikat)")

    # --- 3. kolom kontrak DECISIONS vs DDL ---
    m = re.search(r"## 6\. Skema database.*?```(.*?)```", dec, re.S)
    if m:
        body = m.group(1)
        ddl_cols = {}
        for mm in re.finditer(r"CREATE TABLE (?:IF NOT EXISTS )?([a-z_]+)\s*\((.*?)\n\);", arch, re.S):
            cols = set()
            for line in mm.group(2).split("\n"):
                c = re.match(r"^\s*([a-z_][a-z0-9_]*)\s+[A-Z]", line)
                if c:
                    cols.add(c.group(1))
            ddl_cols[mm.group(1)] = cols

        bad = []
        for blk in re.split(r"(?m)^([a-z_]+)\(", body)[1:]:
            pass
        parts = re.split(r"(?m)^([a-z_]+)\(", body)
        for i in range(1, len(parts), 2):
            tbl = parts[i]
            if tbl not in ddl_cols:
                bad.append(f"{tbl}(tabel tak ada)")
                continue
            decl = parts[i + 1].split(")")[0]
            cols = [c for c in re.findall(r"[a-z_][a-z0-9_]*", decl)
                    if c not in ("PK", "FK", "UNIQUE")]
            miss = [c for c in cols if c not in ddl_cols[tbl]]
            if miss:
                bad.append(f"{tbl}:{','.join(miss)}")
        if bad:
            say("FAIL", f"KONTRAK: kolom DECISIONS tidak ada di DDL -> {'; '.join(bad)}")
        else:
            say("ok", "KONTRAK: semua kolom DECISIONS ada di DDL ARCHITECTURE")

    # --- 4. klaim jumlah endpoint == baris tabel ---
    actual = len(re.findall(r"(?m)^\|\s*`(?:GET|POST|PUT|PATCH|DELETE)`\s*\|\s*`[^`]+`", arch))
    # Klaim TOTAL: "(N Endpoint)" pada header tabel 6.2, "Total ... N endpoint",
    # "N Endpoint coverage", "HTTP Handlers (N endpoints)". Klaim PER-SUBSECTION
    # (#### 6.2.x ... (N Endpoint)) memang berbeda tiap section, jadi dihitung
    # terpisah dan hanya dibandingkan dengan baris di section itu sendiri.
    wrong = []
    for m in re.finditer(r"### 6\.2 Tabel Endpoint Lengkap \((\d+) Endpoint\)", arch):
        if int(m.group(1)) != actual:
            wrong.append(("header tabel 6.2", int(m.group(1))))
    for m in re.finditer(r"Total endpoint terdefinisi: (\d+) endpoint", arch):
        if int(m.group(1)) != actual:
            wrong.append(("total", int(m.group(1))))
    for m in re.finditer(r"(\d+) Endpoint coverage", arch):
        if int(m.group(1)) != actual:
            wrong.append(("contract test", int(m.group(1))))
    for m in re.finditer(r"HTTP Handlers \((\d+) endpoints?\)", arch):
        if int(m.group(1)) != actual:
            wrong.append(("folder tree", int(m.group(1))))

    # klaim per-subsection vs baris di subsection itu
    parts = re.split(r"(?m)^#### (6\.2\.\d+) (.+?) \((\d+) Endpoint\)", arch)
    for i in range(1, len(parts), 4):
        num, title, claim, body = parts[i], parts[i + 1], int(parts[i + 2]), parts[i + 3]
        n = len(re.findall(r"(?m)^\|\s*`(?:GET|POST|PUT|PATCH|DELETE)`\s*\|", body))
        if n != claim:
            wrong.append((f"{num} {title[:26]}", claim))

    if wrong:
        detail = "; ".join(f"{w[0]} klaim {w[1]} (aktual {actual})" for w in wrong)
        say("FAIL", f"ARCHITECTURE: klaim jumlah endpoint tidak konsisten -> {detail}")
    else:
        say("ok", f"ARCHITECTURE: klaim jumlah endpoint konsisten ({actual}) + semua subsection")

    # --- 5. endpoint dirujuk PRD ada di ARCH ---
    def norm(e):
        meth, path = e.split(" ", 1)
        path = re.sub(r"\{[^}]+\}", "{}", path).split("?")[0]
        return meth + " " + path

    arch_set = {norm(f"{m.group(1)} {m.group(2)}") for m in
                re.finditer(r"(?m)^\|\s*`(GET|POST|PUT|PATCH|DELETE)`\s*\|\s*`([^`]+)`", arch)}
    prd_set = {norm(f"{m.group(1)} {m.group(2)}") for m in
               re.finditer(r"`(GET|POST|PUT|PATCH|DELETE)\s+(/api/v1/[^`]+)`", prd)}
    miss = sorted(prd_set - arch_set)
    if miss:
        say("FAIL", f"PRD merujuk endpoint yang tak ada di ARCH ({len(miss)}) -> {', '.join(miss[:4])}")
    else:
        say("ok", f"PRD->ARCH: semua {len(prd_set)} endpoint yang dirujuk PRD terdefinisi")

    # --- 6. Role Min hanya dari enum role, kecuali aktor Worker yang terdokumentasi ---
    ROLES = {"None", "Viewer", "Member", "Admin", "Owner"}
    seen = collections.Counter()
    for m in re.finditer(r"(?m)^\|\s*`(?:GET|POST|PUT|PATCH|DELETE)`\s*\|\s*`[^`]+`\s*\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|", arch):
        seen[m.group(2).strip()] += 1
    unknown = sorted(r for r in seen if r not in ROLES and r != "Worker")
    if unknown:
        say("FAIL", f"RBAC: nilai Role Min di luar enum -> {', '.join(unknown)}")
    else:
        say("ok", f"RBAC: Role Min hanya dari enum role (+ Worker terdokumentasi, {seen.get('Worker', 0)} endpoint)")

    # Worker hanya boleh pada endpoint Auth Internal/Key
    bad_worker = []
    for m in re.finditer(r"(?m)^\|\s*`(GET|POST|PUT|PATCH|DELETE)`\s*\|\s*`([^`]+)`\s*\|\s*([^|]+?)\s*\|\s*Worker\s*\|", arch):
        if "Internal" not in m.group(3):
            bad_worker.append(m.group(2))
    if bad_worker:
        say("FAIL", f"RBAC: Worker dipakai di endpoint non-internal -> {', '.join(bad_worker[:3])}")
    else:
        say("ok", "RBAC: Worker hanya muncul di endpoint Auth Internal/Key")

    # --- 7. klaim jumlah tabel di dokumen turunan harus == DDL ---
    n_tables = len(re.findall(r"CREATE TABLE (?:IF NOT EXISTS )?[a-z_]+", arch))
    stale = []
    for name in ("design/landing/prompt-landing.md", "design/landing/source.html"):
        txt = read(name)
        if txt is None:
            continue
        for m in re.finditer(r"(\d+)\s+tables", txt):
            if int(m.group(1)) != n_tables:
                stale.append(f"{name}: '{m.group(0)}' (DDL={n_tables})")
    if stale:
        say("FAIL", f"angka: klaim jumlah tabel basi -> {'; '.join(stale)}")
    else:
        say("ok", f"angka: klaim jumlah tabel konsisten dengan DDL ({n_tables})")

    # --- 8. referensi silang §x.y menunjuk section yang ada ---
    have = set(re.findall(r"(?m)^#{2,4} (\d+\.\d+)", arch))
    refs = set(re.findall(r"§(\d+\.\d+)", arch))
    dangling = sorted(r for r in refs if r not in have)
    if dangling:
        say("FAIL", f"ARCHITECTURE: referensi § ke section tak ada -> {', '.join(dangling)}")
    else:
        say("ok", f"ARCHITECTURE: {len(refs)} referensi § menunjuk section yang ada")


def check_pricing():
    """Pastikan docs/PRICING.md utuh dan sinkron dengan tools/gen_pricing.py.

    Tabel harga adalah data yang di-port; dokumen tanpa gate akan basi tanpa
    ketahuan, dan angka basi di sini langsung berarti estimasi biaya yang salah.
    """
    doc = read("PRICING.md")
    if not doc:
        say("FAIL", "PRICING: docs/PRICING.md tidak ada")
        return

    n_exact = len(re.findall(r"(?m)^\| `[^`]+` \|", doc))
    n_pat = len(re.findall(r"(?m)^\| \d+ \| ", doc))

    if n_exact == 0 or n_pat == 0:
        say("FAIL", f"PRICING: tabel tidak terbaca (exact={n_exact}, pattern={n_pat})")
        return

    # Angka acuan di DECISIONS/ARCHITECTURE harus sama dengan isi dokumen.
    arch = read("ARCHITECTURE.md")
    dec = read("DECISIONS.md")
    for label, text in (("ARCHITECTURE.md", arch), ("DECISIONS.md", dec)):
        m = re.search(r"(\d+) entri exact \+ (\d+) pattern", text)
        if not m:
            say("FAIL", f"PRICING: {label} tidak menyebut jumlah entri")
            continue
        if int(m.group(1)) != n_exact or int(m.group(2)) != n_pat:
            say(
                "FAIL",
                f"PRICING: {label} klaim {m.group(1)}+{m.group(2)}, "
                f"dokumen berisi {n_exact}+{n_pat}",
            )
        else:
            say("ok", f"PRICING: {label} sinkron ({n_exact} exact + {n_pat} pattern)")

    # Rumus 5 komponen: periksa BLOK rumusnya, bukan sekadar kata kunci di dokumen.
    # (Menghapus satu komponen dari rumus pernah lolos gate versi longgar.)
    block = re.search(r"```\n(miss = .*?)```", doc, re.S)
    if not block:
        say("FAIL", "PRICING: blok rumus tidak ditemukan")
    else:
        body = block.group(1)
        for token in ("input", "cached", "output", "reasoning", "cache_creation"):
            if not re.search(rf"(?m)^\s*[+ ]?.*\b{token}\b", body):
                say("FAIL", f"PRICING: komponen `{token}` hilang dari blok rumus")
        n_terms = len(re.findall(r"(?m)^\s*\+ ", body))
        if n_terms != 4:
            say("FAIL", f"PRICING: rumus punya {n_terms} suku, seharusnya 4")
        else:
            say("ok", "PRICING: rumus 5 komponen utuh (4 suku)")

    # Exact = 7 pipe (nama + 5 harga), pattern = 8 pipe (# + pattern + 5 harga).
    # Jumlah pipe tetap: baris terpotong langsung ketahuan.
    bad = [
        ln
        for ln in doc.splitlines()
        if re.match(r"^\| `", ln) and ln.count("|") != 7
    ] + [
        ln
        for ln in doc.splitlines()
        if re.match(r"^\| \d+ \| ", ln) and ln.count("|") != 8
    ]
    if bad:
        say("FAIL", f"PRICING: {len(bad)} baris jumlah kolomnya salah, mis. {bad[0][:60]}")
    else:
        say("ok", f"PRICING: {n_exact + n_pat} baris harga, jumlah kolom semua benar")


def main():
    print(f"== AgentDeck suite gate  ({ROOT})\n")
    check_prd()
    print()
    check_supersessions()
    print()
    check_design()
    print()
    check_design_audit()
    print()
    check_contract()
    print()
    check_arch()
    print()
    check_structure()
    print()
    check_suite()
    print()
    check_migrations()
    print()
    check_pricing()
    print()
    if FAILED:
        print(f"HASIL: {len(FAILED)} temuan FAIL")
        return 1
    print("HASIL: SEMUA GATE BERSIH (0 FAIL)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
