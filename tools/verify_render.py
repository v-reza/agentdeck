#!/usr/bin/env python3
"""Gate HTML hasil render Stitch terhadap kontrak AgentDeck v2.

Dipakai SETELAH tiap generate: simpan HTML, lalu jalankan ini. Menangkap
pelanggaran yang lolos dari prompt, terutama state-switcher yang sering
dikarang Stitch ketika prompt menyebut beberapa varian state.

Usage:
    python verify_render.py stitch_output_v2/10-board-list.html
    python verify_render.py stitch_output_v2/*.html

Exit 0 kalau bersih, 1 kalau ada pelanggaran.

ponytail: geometri shell dicek PER BLUEPRINT. Blueprint D (login/register)
justru melarang rail/sidebar/cost rail, jadi menuntut 44/224/264px di situ
menghasilkan false positive di setiap layar yang sebenarnya patuh.

ponytail: token terlarang dinilai dari RENDER, bukan cuma dari CSS. Hex yang ada
di CSS tapi 0 piksel di render tidak kelihatan, jadi cukup peringatan — kalau
dihukum, kita membuang satu credit generate untuk drift yang tak kasat mata.
"""

import argparse
import glob
import os
import re
import sys

# Elemen yang HARAM ada sebagai elemen nyata (bukan sekadar disebut di komentar).
FORBIDDEN_ELEMENTS = {
    r"switchState\s*\(": "fungsi switchState() — state switcher",
    r"setDemoState\s*\(": "fungsi setDemoState()",
    r"data-demo-state": "atribut data-demo-state",
    r"State Variant": "label 'State Variant' (switcher bar)",
    r"state-switcher": "kelas state-switcher",
    r'class="state-tab': "tab varian state",
}

FORBIDDEN_TOKENS = {
    "#0f766e": "aksen v1 (teal lama)",
    "#6750a4": "Material purple",
    "#4f378a": "Material purple (primary)",
    "#f7f8f9": "neutral abu-biru Portico",
    "#f1f3f4": "neutral abu-biru Portico",
    "#eceeef": "neutral abu-biru Portico",
    "#14161a": "tinta v1",
    "rgba(15,23,42": "border biru-tint Portico",
}

FORBIDDEN_CSS = {
    "backdrop-filter": "glassmorphism",
    "linear-gradient": "gradien dekoratif",
    "radial-gradient": "gradien dekoratif",
}

# Geometri shell v2, per blueprint.
SHELL_FULL = ["44px", "224px", "264px", "52px"]            # A: rail + sidebar + costrail + topbar
SHELL_BOARD = ["44px", "224px", "264px", "268px", "52px"]  # B: A + kolom board 268px
SHELL_DRAWER = SHELL_FULL + ["420px"]                      # C: A + drawer/panel 420px
PUBLIC_FORBIDDEN = ["44px", "224px", "264px"]              # P/D: HARAM ada chrome aplikasi


def _blueprint_of(note):
    """Ambil huruf blueprint dari catatan layar.

    ponytail: keputusan gate diambil dari HURUF blueprint, bukan dari pencocokan
    kata di catatan. Versi sebelumnya mencocokkan kata ("panel", "drawer") dan
    jadi melemah sendiri begitu label C dinormalisasi jadi "Blueprint C + cost rail
    (panel 420px)" -- layar C berhenti dicek geometrinya.
    """
    low = note.lower()
    if "sub-shell" in low:
        return "SUB"
    m = re.search(r"Blueprint\s+(P-DOCS|P|[ABCD])\b", note)
    return m.group(1) if m else ""


def _note_for(path):
    """Cari catatan blueprint layar ini di screens.py. Lalu --note dipakai
    sebagai fallback bila file tidak dikenali (mis. diuji di luar direktori)."""
    try:
        import importlib.util
        spec = importlib.util.spec_from_file_location("_s", os.path.join(
            os.path.dirname(os.path.abspath(__file__)), "screens.py"))
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        stem = os.path.basename(path)[:-5]
        meta = mod.SCREENS.get(stem)
        return meta[4] if meta else None
    except Exception:
        return None


def _blueprint_for(path):
    """Ambil catatan blueprint layar dari screens.py, kalau bisa."""
    sid = os.path.basename(path)[:-5] if path.endswith(".html") else os.path.basename(path)
    try:
        sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
        import screens as S
        meta = S.SCREENS.get(sid)
        if meta:
            return meta[4] if len(meta) > 4 else ""
    except Exception:
        pass
    return ""


_PIL_PROBE = """
import json, sys
try:
    from PIL import Image
except Exception:
    sys.exit(3)
hexes = json.loads(sys.argv[2])
im = Image.open(sys.argv[1]).convert("RGB")
hist = im.getcolors(maxcolors=1 << 24) or []
targets = {}
for hx in hexes:
    h6 = hx.lstrip("#")
    if len(h6) == 6:
        targets[hx] = (int(h6[0:2], 16), int(h6[2:4], 16), int(h6[4:6], 16))
counts = {hx: 0 for hx in targets}
for cnt, col in hist:
    for hx, t in targets.items():
        if col == t:
            counts[hx] += cnt
print(json.dumps({"size": im.size, "counts": counts}))
"""


def _pil_candidates():
    """Interpreter yang mungkin punya Pillow.

    ponytail: di mesin ini ada 3 python berbeda (kernel, subprocess kernel, venv)
    dan Pillow tidak ada di semuanya. Gate tidak boleh diam-diam jadi lemah cuma
    karena dijalankan dari interpreter yang salah.
    """
    out = [sys.executable]
    env_py = os.environ.get("HERMES_PYTHON")
    if env_py:
        out.append(env_py)
    for cand in ("python", "python3", "py"):
        out.append(cand)
    venv = os.path.join(os.path.expanduser("~"), "AppData", "Local", "hermes",
                        "hermes-agent", "venv", "Scripts", "python.exe")
    out.append(venv)
    seen, uniq = set(), []
    for c in out:
        if c and c not in seen:
            seen.add(c)
            uniq.append(c)
    return uniq


def _share_via_subprocess(render_path, hexes):
    """Hitung severity dengan interpreter lain yang punya Pillow."""
    import json
    import subprocess
    for exe in _pil_candidates():
        try:
            r = subprocess.run([exe, "-c", _PIL_PROBE, render_path, json.dumps(hexes)],
                               capture_output=True, text=True, timeout=180)
        except Exception:
            continue
        if r.returncode != 0 or not r.stdout.strip():
            continue
        try:
            got = json.loads(r.stdout.strip().splitlines()[-1])
        except Exception:
            continue
        w, h = got["size"]
        total = w * h
        return {hx: (got["counts"].get(hx, 0),
                     (got["counts"].get(hx, 0) / total if total else 0.0))
                for hx in hexes}
    return None


def _share_in_render(render_path, hexes):
    """Berapa persen piksel render yang benar-benar memakai tiap hex terlarang.

    Mengembalikan {hex: (jumlah_px, fraksi)} atau None kalau severity tidak bisa
    dinilai (render tidak ada, atau Pillow tidak terpasang di interpreter ini).

    ponytail: TIDAK ada decoder PNG buatan sendiri di sini. Percobaan pertama
    memakai decoder stdlib dan hasilnya beda 12px dari Pillow pada file yang sama
    (bug rekonstruksi filter Paeth). Angka severity yang salah lebih buruk daripada
    tidak ada angka, jadi kalau Pillow tidak ada kita jatuh ke mode ketat dan
    bilang begitu.
    """
    if not render_path or not os.path.exists(render_path):
        return None

    targets = {}
    for hx in hexes:
        h6 = hx.lstrip("#")
        if len(h6) == 6:
            targets[hx] = (int(h6[0:2], 16), int(h6[2:4], 16), int(h6[4:6], 16))
    if not targets:
        return None

    # 1) in-process (paling cepat)
    try:
        from PIL import Image
        im = Image.open(render_path).convert("RGB")
        hist = im.getcolors(maxcolors=1 << 24) or []
        counts = {hx: 0 for hx in targets}
        for cnt, col in hist:
            for hx, t in targets.items():
                if col == t:
                    counts[hx] += cnt
        total = im.size[0] * im.size[1]
        return {hx: (counts[hx], counts[hx] / total if total else 0.0) for hx in targets}
    except Exception:
        pass

    # 2) interpreter lain yang punya Pillow
    return _share_via_subprocess(render_path, hexes)


def check(path, note=None, render=None):
    raw = open(path, encoding="utf-8", errors="ignore").read()
    # buang komentar HTML supaya larangan yang ditulis sebagai komentar
    # ("JANGAN bikin state switcher") tidak dihitung sebagai pelanggaran.
    body = re.sub(r"<!--.*?-->", "", raw, flags=re.S)
    problems, warns = [], []

    for pat, why in FORBIDDEN_ELEMENTS.items():
        m = re.search(pat, body)
        if m:
            problems.append(f"elemen terlarang: {why} -> {m.group(0)!r}")

    # Token terlarang: nilai severity dari render kalau ada. Hex yang ada di CSS
    # tapi 0 piksel di render = tidak kelihatan, jadi cukup peringatan.
    present = [t for t in FORBIDDEN_TOKENS if t.lower() in body.lower()]
    shares = _share_in_render(render, present) if present else None
    for tok in present:
        why = FORBIDDEN_TOKENS[tok]
        if shares is None:
            problems.append(f"token v1/terlarang: {tok} ({why})")
        else:
            n, frac = shares.get(tok, (0, 0.0))
            if n == 0:
                warns.append(f"token v1 di CSS tapi 0 px di render: {tok} ({why})")
            elif frac < 0.0005:
                # <0.05% dari render = jejak tak terlihat (~1000px di layar 1600px).
                # Regenerasi untuk ini buang credit tanpa perubahan yang kelihatan.
                warns.append(
                    f"token v1 nyaris tak terlihat: {tok} ({why}) — {n}px ({frac*100:.3f}%)")
            else:
                problems.append(
                    f"token v1/terlarang: {tok} ({why}) — {n}px ({frac*100:.2f}% dari render)")

    for tok, why in FORBIDDEN_CSS.items():
        if tok in body:
            problems.append(f"CSS terlarang: {tok} ({why})")

    # --- geometri shell, sadar blueprint ---
    if note is None:
        note = _note_for(path)
    if not note:
        note = _blueprint_for(path)

    bp = _blueprint_of(note)
    if bp in ("P", "P-DOCS"):
        # Halaman publik: chrome aplikasi HARAM. Kegagalan keras, bukan peringatan --
        # 7 halaman publik (docs, changelog, github, community) pernah lolos gate
        # sambil menampilkan rail+sidebar+cost rail, sehingga dokumentasi terlihat
        # seperti aplikasi yang sudah login.
        leaked = [g for g in PUBLIC_FORBIDDEN if g in body]
        if leaked:
            problems.append(
                "halaman publik (" + bp + ") tapi memakai chrome aplikasi: "
                + ", ".join(leaked) + " (HARAM tanpa login)")
        if bp == "P-DOCS" and "720px" not in body and "240px" not in body:
            problems.append("nav dokumen: lebar 240px / artikel 720px tidak ditemukan")
        if render and os.path.exists(render):
            band = _share_in_render(render, ["#e8ecea"])
            if band:
                n, frac = band.get("#e8ecea", (0, 0.0))
                if frac > 0.02:
                    problems.append(
                        f"halaman publik: rail abu #e8ecea menutupi {frac*100:.1f}% render")
    elif bp == "D":
        leaked = [g for g in PUBLIC_FORBIDDEN if g in body]
        if leaked:
            problems.append("Blueprint D (tanpa shell) tapi ada: " + ", ".join(leaked))
        if "400px" not in body:
            problems.append("Blueprint D: kartu 400px tidak ditemukan")
    elif bp == "B":
        required = SHELL_BOARD
    elif bp == "C":
        required = SHELL_DRAWER
    elif bp == "A":
        required = SHELL_FULL
    elif bp == "SUB":
        required = []          # turunan shell induknya, tidak dicek sendiri
    else:
        required = SHELL_FULL  # catatan tak dikenal: pakai kontrak paling ketat
    if bp in ("A", "B", "C"):
        if "tanpa cost rail" in note.lower():
            required = [g for g in required if g != "264px"]
        missing = [g for g in required if g not in body]
        if missing:
            problems.append(
                f"geometri shell v2 hilang (blueprint {bp}): " + ", ".join(missing))
    elif bp == "":
        # Catatan tak dikenal: jangan tebak. Pakai kontrak paling ketat supaya
        # layar baru tidak lolos tanpa dicek.
        missing = [g for g in SHELL_FULL if g not in body]
        if missing:
            problems.append(
                "geometri shell v2 hilang (blueprint tak dikenal): " + ", ".join(missing))

    # --- resolusi render ---
    # PNG dari Stitch kadang turun sebagai thumbnail 250-500px (URL tanpa =s1600).
    # Screenshot sekecil itu tidak bisa dipakai untuk verifikasi pixel maupun
    # review visual, jadi ini kegagalan keras.
    if render and os.path.exists(render):
        try:
            from PIL import Image
            w, h = Image.open(render).size
        except Exception:
            w = h = 0
        if w < 1000 or h < 500:
            problems.append(
                f"render {w}x{h}px (di bawah 1000x500) — thumbnail, bukan screenshot")

    # --- emoji ---
    picto = re.findall(r"[\U0001F300-\U0001FAFF]", body)
    if picto:
        problems.append(f"emoji: {''.join(sorted(set(picto))[:5])}")
    ding = re.findall(r"[\u2600-\u27BF\u2B00-\u2BFF]", body)
    if ding:
        warns.append(f"dingbat sebagai glyph ikon: {''.join(sorted(set(ding))[:6])}")

    return problems, warns


def main(argv):
    ap = argparse.ArgumentParser(description="Gate render Stitch vs kontrak AgentDeck v2")
    ap.add_argument("files", nargs="*", help="file HTML atau direktori berisi HTML")
    ap.add_argument("--note", help="catatan blueprint (mis. 'Blueprint B + cost rail') "
                                   "bila file tidak dikenali dari screens.py")
    args = ap.parse_args(argv[1:])

    files = []
    for a in args.files:
        if os.path.isdir(a):
            files += sorted(glob.glob(os.path.join(a, "*.html")))
        elif os.path.isfile(a):
            files.append(a)
        else:
            print(f"FAIL {a}: tidak ada")
    if not files:
        ap.print_help()
        return 2

    files.sort()
    failed = warned = 0
    for f in files:
        if not os.path.exists(f):
            print(f"FAIL {f}: tidak ada")
            failed += 1
            continue
        stem = f[:-5] if f.endswith(".html") else f
        render = None
        for cand in (stem + "-render.png", stem + ".png"):
            if os.path.exists(cand):
                render = cand
                break
        probs, warns = check(f, render=render, note=args.note)
        name = os.path.basename(f)
        if probs:
            failed += 1
            print(f"FAIL {name}")
            for p in probs:
                print(f"       - {p}")
        else:
            print(f"ok   {name}")
        if warns:
            warned += 1
            for w in warns:
                print(f"       ~ {w}")

    print()
    if failed:
        print(f"HASIL: {failed} file MELANGGAR kontrak")
        return 1
    tail = f", {warned} dengan peringatan" if warned else ""
    print(f"HASIL: {len(files)} file bersih (0 pelanggaran{tail})")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
