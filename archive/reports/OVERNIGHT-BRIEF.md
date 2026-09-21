# OVERNIGHT BRIEF — AgentDeck, sisa CHECKLIST.md (non-stop)

> **Kamu jalan unattended.** Tidak ada manusia yang bisa menjawab. Jangan pernah
> `clarify`, jangan tunggu approval, jangan berhenti dengan pertanyaan. Kalau
> mentok, catat di `archive/reports/OVERNIGHT-LOG.md` sebagai `BLOCKED-HUMAN` lalu lanjut
> ke story berikutnya.
>
> Dibaca oleh cron job `agentdeck-m0-m4-worker` setiap 10 menit. Baca ulang
> seluruh file ini di setiap run — kamu tidak punya memori antar-run.

## Tujuan

Selesaikan story yang **belum PASS** di `docs/CHECKLIST.md`, **berurutan dari
milestone terendah** (M0 → M1 → M2 → M3 → M4), dengan gate hijau. Kualitas di
atas kecepatan: story yang belum lulus gate **tidak** boleh ditandai PASS.

**Mode non-stop.** Tidak ada jam berhenti. Kerjakan terus selama masih ada story
yang bisa dikerjakan. Satu-satunya alasan berhenti di tengah run adalah kamu
sudah kehabisan konteks — dan itu pun bukan alasan meninggalkan repo setengah
jadi tanpa catatan.

## Repo

- Root: `D:/Project/agentdeck` (branch `main`).
- **Jangan sentuh** repo lain, jangan sentuh board `portico-suite`, jangan
  sentuh `~/Desktop/portfolio-projects`.
- Jangan commit build output (`api.exe`), cache, screenshot sementara, atau
  secrets. Semua credential → `[REDACTED]`.

## Sumber status — BACA INI, jangan menebak

`docs/CHECKLIST.md` itu **file generated**. Jangan mengeditnya langsung; edit-nya
akan hilang di regenerasi berikutnya. Sumber sebenarnya:

- `tools/checklist_status.json` — status per story (`pass`/`wip`/`todo`/`defer`/`fail`).
- `docs/COVERAGE.md` — daftar story + layar, generated oleh `tools/screens.py`.

Ubah status **hanya** lewat:

```bash
cd /d/Project/agentdeck && python tools/update_checklist.py US-ADxx pass
```

Perintah itu menulis sidecar lalu meregenerasi `docs/CHECKLIST.md`. Cek status
satu story dengan `python tools/update_checklist.py US-ADxx`.

## Langkah tiap run (urut, jangan dilompati)

1. **Baca status.** Baca `tools/checklist_status.json`, `docs/CHECKLIST.md`, dan
   30 baris terakhir `archive/reports/OVERNIGHT-LOG.md`. Itu satu-satunya memori kamu.
2. **Pastikan lingkungan hidup.**
   ```bash
   cd /d/Project/agentdeck && docker compose ps
   docker compose up -d --force-recreate api   # kalau api tidak healthy
   curl -fsS http://localhost:8080/readyz
   ```
   E2E butuh API + web hidup. Jangan matikan container milik proyek lain.
3. **Pilih satu story** — yang paling awal belum PASS di milestone terendah yang
   belum selesai. Kerjakan **satu story sampai kelar**, jangan sebar.
4. **Kerjakan sesuai kontrak**, bukan versi yang lebih simpel:
   - Baca desain di `design/stitch-output/v2/<screen>.html` dan clone
     **class-for-class**. Jangan mengarang dari ingatan.
   - Backend Go: pgx v5 + sqlc, `gofmt`, satu statement per baris.
   - Frontend: React 19 + React Router v7 + Tailwind v4 + shadcn/ui + Redux
     Toolkit/RTK Query. Tidak ada `window.alert/confirm/prompt`, tidak ada
     `window.location.pathname`, tidak ada Context untuk application data.
   - Bug: tulis test yang **gagal dulu**, baru perbaiki.
5. **Jalankan gate lengkap** (SEKUENSIAL — Vitest timeout kalau barengan
   Playwright):
   ```bash
   cd /d/Project/agentdeck && gofmt -l . && go build ./... && go vet ./... && go test ./... -count=1
   cd frontend && npx prettier --check src e2e && npx tsc -b --pretty false \
     && npx vitest run && npx playwright test --workers=1 --reporter=line && npm run build
   cd /d/Project/agentdeck && python tools/verify_web.py   # wajib 0 FAIL 0 warn
   ```
6. **Buktikan visual** untuk story yang punya layar: screenshot ke
   `$LOCALAPPDATA/Temp/agentdeck-shots` (JANGAN di dalam repo), lalu
   `vision_analyze` dengan pertanyaan element-level, bandingkan dengan file
   desainnya. Kalau mismatch, perbaiki dulu.
7. **Update checklist** hanya kalau semua gate di atas hijau:
   ```bash
   python tools/update_checklist.py US-ADxx pass
   ```
8. **Commit lokal** (JANGAN push) setelah satu story lulus gate:
   ```bash
   git add -A && git commit -m "feat(<area>): <story> — <ringkas>"
   ```
   Commit per story supaya ada checkpoint dan bisa di-rollback. **Push ke remote
   publik DILARANG** — itu menunggu keputusan manusia.
9. **Tulis progres** ke `archive/reports/OVERNIGHT-LOG.md` (append, satu blok per run):
   timestamp, story, apa yang dikerjakan, hasil gate, hambatan. Ringkas.
10. **Ping WhatsApp** (lihat bagian bawah) kalau ada perubahan status story.
11. **Lanjut story berikutnya** selama masih ada konteks.

## Jebakan yang sudah pernah menggigit di repo ini — baca sebelum kerja

- **Route yang tidak dipanggil di `main.go` = 404 di runtime walau semua test
  Go hijau.** Ini pernah terjadi: 4 subagent, ~70 test hijau, tapi
  `/agent-catalog`, `PATCH /agents/{id}`, dan `/agent-skills` **404 semua** lewat
  HTTP. **Selalu verifikasi runtime** dengan `curl`, bukan cuma `go test`.
- **`vision_analyze` pada full-page screenshot TIDAK RELIABEL** untuk klaim
  overlap/hex/geometri/clipping (gambar di-downscale). Sudah 2× terbukti salah.
  Kalau mau mengklaim ada elemen kepotong/tumpang-tindih, WAJIB verifikasi lewat
  crop region ATAU ukur DOM (`scrollHeight` vs `clientHeight`,
  `getBoundingClientRect`).
- **`design/stitch-output/v2/*.html` adalah mockup SPEC**, bukan produk. Dia
  mengandung jargon internal (`US-AD<angka>`, `AC<angka>`, `HTTP <kode>`,
  `M1`-`M6`, `B2C`). **Jangan sampai jargon itu masuk ke kode produk.** Ada guard
  e2e permanen di `frontend/e2e/agents.spec.ts` yang akan GAGAL kalau jargon
  muncul — baca guard itu.
- **API di container bisa lebih tua dari source.** Kalau endpoint baru tidak ada,
  `docker compose build api && docker compose up -d api` dulu. Jangan salah
  diagnosa.
- **Test yang memakai fake repo in-memory bisa PASS padahal mapper-nya salah**,
  karena fake-nya mengosongkan kolom yang tidak di-SELECT statement asli. Kalau
  mengubah mapper, cek juga fake-nya.
- **Mutasi untuk membuktikan test load-bearing harus benar-benar mengubah
  semantik.** Cek diff-nya dulu — pernah kejadian mutasi cuma menyentuh komentar,
  jadi no-op palsu yang terlihat seperti CAUGHT.
- **`docs/CHECKLIST.md` generated** — jangan edit langsung (lihat bagian di atas).

## Aturan jujur

- Jangan tandai PASS kalau gate belum hijau. Jangan klaim screenshot/vision
  kalau belum benar-benar dijalankan.
- Jangan memalsukan angka, response API, atau hasil test.
- Kalau sebuah story butuh keputusan manusia (scope, UX, kredensial), catat di
  `archive/reports/OVERNIGHT-LOG.md` sebagai `BLOCKED-HUMAN` dan lanjut ke story lain.
  Jangan menebak.
- `US-AD92` (onboarding) **ditunda** atas keputusan user. Jangan kerjakan.
- Story `M5`/`M6` di luar scope. Jangan kerjakan.
- Story yang statusnya `wip` boleh dilanjutkan, tapi **jangan di-PASS-kan** kalau
  AC-nya belum bisa dibuktikan. Tulis apa yang kurang.

## Ping WhatsApp

Delivery cron ke WhatsApp sedang rusak (`WhatsApp send failed`), tapi bridge
lokal sehat. **Jangan tulis JID/chatId di file mana pun di dalam repo ini** —
repo ini publik dan brief ini ikut ter-commit. Pakai skrip yang sudah ada di
luar repo:

```bash
python "$LOCALAPPDATA/hermes/scripts/agentdeck-notify.py" "AGENTDECK PASS=<n> <US-ADxx> <status singkat>"
```

Skrip itu memegang JID-nya sendiri dan mengirim lewat bridge lokal. Format pesan:
satu baris, `AGENTDECK PASS=<n> <US-ADxx> <status singkat>`. Kirim **maksimal
sekali per run**, dan hanya kalau status berubah.
Jangan pernah memasukkan credential/token/secret ke pesan.

## Batas waktu

Tidak ada jam berhenti — jalan terus. Tapi:

- Kalau satu run **tidak menghasilkan perubahan status apa pun**, tetap tulis
  satu blok ringkas di `archive/reports/OVERNIGHT-LOG.md` (apa yang dicoba, kenapa gagal),
  kirim satu ping, lalu **lanjut** ke story berikutnya. Jangan diam.
- Jangan tinggalkan repo dalam keadaan setengah jadi tanpa catatan. Kalau kamu
  harus berhenti di tengah story, tulis di log: file apa yang sedang diubah dan
  langkah berikutnya.
