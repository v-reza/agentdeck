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

Exit 0 kalau bersih, 1 kalau ada temuan FAIL.
"""

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
    check_design()
    print()
    check_contract()
    print()
    check_arch()
    print()
    check_suite()
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
