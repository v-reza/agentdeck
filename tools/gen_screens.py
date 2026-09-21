#!/usr/bin/env python3
"""Generator prompt Stitch per layar AgentDeck.

Mengikat setiap layar ke acceptance criteria story-nya, supaya tidak ada
kebutuhan story yang hilang dari desain.

Untuk tiap layar di `screens.py`:
  1. ambil AC dari setiap story yang dipetakan ke layar itu (00-PRD.md)
  2. pisahkan AC yang punya konsekuensi visual dari yang murni server
  3. rakit prompt = blok konteks + token + shell + larangan + daftar kebutuhan
  4. tulis ke stitch_prompts_v2/<id>.md

Hasil: prompt yang isinya bisa ditelusuri balik ke story, bukan karangan.

Jalankan: python gen_screens.py            # tulis semua prompt
          python gen_screens.py --check    # exit 1 kalau ada layar tanpa AC
"""

import os
import re
import sys

import screens as S

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
HERE = ROOT
OUT = os.path.join(ROOT, "design", "prompts")
SHARED = os.path.join(OUT, "_shared")

# AC yang tidak punya konsekuensi visual — hanya menyebut perilaku server.
SERVER_ONLY = re.compile(
    r"\b(?:mengembalikan|dikembalikan|return|respons|endpoint|status code)\b.*"
    r"\b(?:20\d|40\d|41\d|42\d|50\d|403|404|409|410|422|429)\b",
    re.I,
)
VISUAL_HINT = re.compile(
    r"\b(?:menampilkan|ditampilkan|tampil|layar|halaman|panel|tombol|badge|kartu|drawer|"
    r"kolom|tabel|baris|meter|grafik|ikon|warna|label|placeholder|tooltip|modal|form|"
    r"font|monospace|skeleton|empty state|toast|header|top bar|rail|sidebar|checkbox|"
    r"field|input|dropdown|select|tautan|link|dialog|konfirmasi|pratinjau|preview|"
    r"menampilkan|daftar|list|filter|tab|toggle|switch|avatar|banner|pita|stepper|"
    # "indikator" menandai keadaan yang HARUS terlihat (mis. badge "terverifikasi"
    # di US-AD109 AC3) tapi kalimatnya sering tidak menyebut kata benda UI sama
    # sekali. Menambah "status" akan menarik 13 AC perilaku server; "indikator"
    # hanya mengenai 1 AC, dan itu memang butuh wujud visual.
    r"indikator)\b",
    re.I,
)


def load_stories():
    """story id -> {title, prio, ms, acs:[str]}"""
    text = open(os.path.join(ROOT, "docs", "00-PRD.md"), encoding="utf-8").read()
    out = {}
    parts = re.split(r"(?m)^(?=\*\*US-AD\d+\*\*)", text)
    for p in parts:
        m = re.match(
            r"\*\*(US-AD\d+)\*\*\s+—\s+(.+?)\s+`(Must|Should|Could|Won't)`\s+·\s+`(M\d)`", p
        )
        if not m:
            continue
        acs = re.findall(r"(?m)^- \[ \] (AC\d+)(?: \(([^)]+)\))?\s+(.+)$", p)
        out[m.group(1)] = {
            "title": m.group(2),
            "prio": m.group(3),
            "ms": m.group(4),
            "acs": [(a, tag, txt) for a, tag, txt in acs],
        }
    return out


def visual_acs(acs):
    """AC yang punya konsekuensi visual, plus yang bertanda B2C/jalur gagal."""
    keep = []
    for num, tag, txt in acs:
        if VISUAL_HINT.search(txt):
            keep.append((num, tag, txt))
        elif tag in ("B2C",):
            keep.append((num, tag, txt))
    return keep


def shared_block(name):
    """Blok shared WAJIB ada. Dulu fungsi ini balikin "" diam-diam saat folder
    pindah, sehingga 50 prompt kehilangan seluruh kontrak (token/shell/larangan)
    tanpa satu pun error — prompt-nya tinggal 1.3KB dan Stitch mengarang token
    v1 sendiri. Sekarang gagal keras."""
    p = os.path.join(SHARED, name)
    if not os.path.exists(p):
        raise SystemExit(f"FATAL: blok shared hilang: {p}\n"
                         f"Prompt tanpa blok ini kehilangan kontrak token/shell.")
    return open(p, encoding="utf-8").read().strip()


# Layar yang punya spec halaman lengkap sendiri (bukan shell generik + AC).
# Tanpa ini, prompt-nya cuma shell + AC dan Stitch mengarang isi halaman
# berikut token v1-nya sendiri — terbukti pada 05-landing.
SPEC_EXTRA = {
    "05-landing": "landing/prompt-landing.md",
    # Layar yang AC-nya tidak cukup menentukan bentuk isi: Stitch mengarang
    # struktur sendiri (form detail padahal yang diminta daftar). Spec menulis
    # struktur pane konten secara eksplisit.
    "47-providers": "spec/47-providers.md",
}


def load_spec_extra(sid):
    """Isi spec halaman khusus, kalau ada."""
    rel = SPEC_EXTRA.get(sid)
    if not rel:
        return None
    path = os.path.join(ROOT, "design", rel)
    if not os.path.exists(path):
        return None
    text = open(path, encoding="utf-8").read()
    # buang judul H1 file spec; heading lain dibiarkan agar struktur tetap terbaca
    text = re.sub(r"\A#\s+.*?\n", "", text, count=1)
    return text.strip()


def build_prompt(sid, meta, stories):
    """Urutan mengikuti stitch-prompt-craft: konteks -> token -> shell -> larangan
    -> kebutuhan story -> instruksi layar. Instruksi layar di AKHIR karena Stitch
    memprioritaskan bagian akhir prompt."""
    title, route, sids, states, note = meta

    L = []
    L.append(f"# {sid} — {title}\n")
    L.append(f"Route: `{route}` · Shell: {note}\n")

    # Blok shell DIPILIH berdasarkan jenis halaman. Menempelkan shell aplikasi ke
    # halaman publik membuat Stitch menggambar rail+sidebar+cost rail di landing,
    # docs, pricing, changelog -- halaman yang justru dibaca sebelum orang login.
    publik = "standalone" in note.lower()
    # Blueprint P-DOCS (nav dokumen) sudah ada di dalam shell-publik.md, jadi
    # halaman docs memakai blok yang sama seperti halaman publik lain.
    shell = "shell-publik.md" if publik else "shell.md"

    L.append("## PROMPT\n")
    L.append(shared_block("konteks.md")); L.append("")
    L.append(shared_block("token.md")); L.append("")
    for blk in dict.fromkeys(shell.split("|")):
        L.append(shared_block(blk)); L.append("")
    L.append(shared_block("aturan-global.md")); L.append("")

    # --- kebutuhan yang diturunkan dari story, ditaruh dekat instruksi layar ---
    L.append("## Kebutuhan dari user story (WAJIB muncul semua)\n")
    L.append("Layar ini adalah wujud visual dari acceptance criteria berikut. "
             "Setiap butir di bawah harus kelihatan di layar. "
             "JANGAN mengarang kebutuhan yang tidak tertulis di sini.\n")
    n_vis = 0
    for s in sids:
        v = stories.get(s)
        if not v:
            continue
        L.append(f"### `{s}` — {v['title']} ({v['prio']}, {v['ms']})\n")
        vis = visual_acs(v["acs"])
        if not vis:
            L.append("- Layar ini hanya menampilkan ringkasan; tidak ada AC berimplikasi visual.\n")
        for num, tag, txt in vis:
            n_vis += 1
            tg = f" ({tag})" if tag else ""
            L.append(f"- **{num}**{tg} {txt}")
        L.append("")

    extra = load_spec_extra(sid)
    if extra:
        L.append("## Spec halaman (WAJIB diikuti, ini sumber kebenaran isi halaman)\n")
        L.append(extra + "\n")

    # Varian state ditulis sebagai INSTRUKSI GENERASI, bukan daftar state di dalam
    # layar. Kalau ditulis sebagai daftar, Stitch membuat "state switcher bar"
    # dengan tombol + switchState() — itu persis elemen yang dilarang.
    L.append(f"## State yang diminta\n")
    L.append(f"Bangun HANYA state `{states[0]}` sekarang. "
             f"JANGAN membangun state lain di frame ini.\n")
    if len(states) > 1:
        L.append(f"State lain dikirim sebagai permintaan TERPISAH nanti dengan "
                 f"shell identik, hanya isi pane yang berubah: "
                 + ", ".join(f"`{s}`" for s in states[1:]) + ".\n")
    L.append("")

    L.append("## Instruksi layar\n")
    if publik:
        L.append("Halaman ini **PUBLIK** -- pengunjung BELUM login. JANGAN gambar rail "
                 "ikon, sidebar aplikasi, cost rail, avatar pengguna, atau menu akun. "
                 "Halaman scroll normal dari atas ke bawah.\n")
    L.append(f"Buat layar **{title}** pada route `{route}`. {note}. "
             f"JANGAN membuat state switcher, tab varian, tombol demo, "
             f"toggle \"default/empty/loading/error\", atau fungsi `switchState()` "
             f"dalam bentuk apa pun. Layar ini hanya menampilkan satu state. "
             f"Semua butir di bagian 'Kebutuhan dari user story' wajib terlihat. "
             f"JANGAN menambah elemen, section, kartu, atau angka yang tidak diminta.\n")
    return "\n".join(L)


SHELL_MINI_PUBLIK = """## Kontrak halaman PUBLIK AgentDeck (tanpa login)

Halaman ini **BUKAN aplikasi**. Pengunjung BELUM punya akun.

**HARAM ADA:** rail ikon 44px, sidebar aplikasi 224px, cost rail 264px, topbar 52px
di atas pane konten, avatar pengguna, menu akun, "Sign out", org/workspace switcher,
badge notifikasi, panel TODAY / 7-DAY / TOP SPENDERS / RUNNING NOW.

**WAJIB (Blueprint P):**
- top nav **64px** sticky, bg `#f6f7f6`, border-bottom `rgba(12,26,22,0.07)`
  - kiri: wordmark "AgentDeck" (`#0e1110`, 15px, 600) + mark aksen
  - tengah: link Inter 14 `#4d5553` -- Product, Pricing, Docs, GitHub
  - kanan: "Sign in" (teks polos) + "Get started free" (pill `#0d7a70`, teks `#ffffff`, radius 6px)
- konten **max-width 1120px, margin auto** (di tengah), bg halaman `#f6f7f6`
- footer: border-top `rgba(12,26,22,0.07)`, 4 kolom link Inter 13 `#4d5553`, padding 40px 0
- halaman **scroll normal** dari atas ke bawah; `body` JANGAN overflow hidden, JANGAN `h-screen`

**CEK DIRI SEBELUM KIRIM:** apakah layar ini terlihat seperti dashboard aplikasi
yang sudah login (rail kiri + sidebar + panel biaya kanan)? Kalau iya, SALAH. Hapus
semua chrome aplikasi itu dan ganti dengan top nav publik di atas.

**Blueprint P-DOCS (khusus halaman dokumentasi):** konten dua kolom --
nav dokumen **240px** + artikel **max-width 720px**.
- nav dokumen = **daftar link teks biasa**, TANPA background fill, TANPA border kotak, TANPA ikon
- grup pakai label uppercase kecil (`#98a09d`, 11px, 600, letter-spacing lebar)
- item aktif `#0d7a70` 600; item lain `#4d5553` 400
- nav dokumen tinggi **mengikuti isi**, JANGAN setinggi viewport
- blok kode: bg `#eef1f0`, radius 10px, JetBrains Mono 13px, padding 14px

**Token:** aksen tunggal Signal Teal `#0d7a70`, neutral hijau-tint `#f6f7f6`,
tinta `#0e1110`/`#4d5553`/`#6b7370`, radius maksimum 14px. JANGAN gradien dekoratif.
Satu section gelap (`#0e1110`) boleh, hanya kalau spec halaman memintanya.
"""


SHELL_MINI = """## Kontrak shell AgentDeck v2 (angka dipatok, JANGAN diubah)

**Permukaan (WAJIB, JANGAN pakai putih untuk rail/sidebar/cost rail):**
- rail kiri: background `#e8ecea`
- sidebar: background `#f6f7f6`
- topbar + panel/kartu: `#ffffff`
- cost rail kanan: background `#eef1f0`
- halaman/kanvas: `#f6f7f6`

**Geometri:**
- rail **44px** (ikon saja, tanpa label) + sidebar **224px** + cost rail kanan **264px**
- topbar **52px**, HANYA membentang di atas pane konten - JANGAN sejajar rail+sidebar+costrail
- `body` overflow hidden; hanya pane konten yang scroll

| Blueprint | Dipakai untuk | Layout |
|---|---|---|
| **A** | halaman default | rail 44 + sidebar 224 + topbar 52 + konten (scroll) + cost rail 264 |
| **B** | board | seperti A, konten = 5 kolom x **268px**, gap **8px**, tiap kolom **top edge 3px** warna status, tiap kolom scroll sendiri |
| **C** | detail/drawer | A + drawer **420px** dari kanan, DI ANTARA konten dan cost rail |
| **D** | login/register saja | kartu **400px** tengah, TANPA rail/sidebar/costrail |

Ukuran lain: baris tabel **28px**, header tabel **32px**, padding kartu **10px**,
header kolom **30px**, avatar agent **20px**, avatar user **26px**, budget meter **6px**,
radius **6/10/14px**. Aksen tunggal Signal Teal `#0d7a70`. Tinta `#0e1110`/`#4d5553`/`#6b7370`.
Border `rgba(12,26,22,0.07)` subtle, `rgba(12,26,22,0.12)` standard.

Cost rail (blueprint A/B/C) isi tetap: TODAY (biaya + meter pagu), 7-DAY (sparkline),
TOP SPENDERS (3 baris), RUNNING NOW. JANGAN tambah isi lain.
"""


def main(argv):
    stories = load_stories()
    os.makedirs(OUT, exist_ok=True)
    written = []
    empty = []

    for sid in sorted(S.SCREENS):
        meta = S.SCREENS[sid]
        prompt = build_prompt(sid, meta, stories)
        path = os.path.join(OUT, f"{sid}.md")
        open(path, "w", encoding="utf-8", newline="").write(prompt)
        written.append((sid, len(prompt)))
        # prompt yang kehilangan blok shared akan jauh lebih pendek dari ini
        if len(prompt) < 8000:
            empty.append(f"{sid} (prompt cuma {len(prompt)} char — blok shared hilang?)")

        # layar wajib punya minimal 1 AC berimplikasi visual dari story-nya
        total_vis = 0
        for s in meta[2]:
            v = stories.get(s)
            if v:
                total_vis += len(visual_acs(v["acs"]))
        if total_vis == 0:
            empty.append(sid)

    # --- mode B: prompt ringkas, blok shared dikirim SEKALI di awal sesi ---
    gendir = os.path.join(OUT, "_gen")
    os.makedirs(gendir, exist_ok=True)
    for f in os.listdir(gendir):
        os.remove(os.path.join(gendir, f))
    tot_b = 0
    for sid in sorted(S.SCREENS):
        full = open(os.path.join(OUT, sid + ".md"), encoding="utf-8").read()
        i = full.find("## Kebutuhan dari user story")
        body = full[i:] if i > 0 else full
        mini = SHELL_MINI_PUBLIK if "standalone" in S.SCREENS[sid][4].lower() else SHELL_MINI
        txt = f"# {sid}\n\n{mini}\n" + body
        open(os.path.join(gendir, sid + ".md"), "w", encoding="utf-8", newline="").write(txt)
        tot_b += len(txt)
    print(f"mode B (_gen)  : {len(S.SCREENS)} prompt, {tot_b:,} chars (~{tot_b//4:,} tok)")

    print(f"prompt ditulis : {len(written)}")
    print(f"total chars    : {sum(n for _s, n in written):,}")
    if empty:
        print(f"\nGAP layar tanpa AC visual ({len(empty)}): {', '.join(empty)}")
        print("\nHASIL: ADA LAYAR TANPA KEBUTUHAN VISUAL")
        return 1
    print("\nHASIL: SEMUA LAYAR TERIKAT KE STORY (0 GAP)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
