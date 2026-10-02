# OVERNIGHT-BRIEF — AgentDeck

Kontrak kerja buat run semalaman. Dibaca **di awal tiap turn**, bukan sekali.
Kalau brief ini dan ingatan gw berbeda, brief ini yang benar.

- **Mulai:** 2026-09-27 malam (sesi `20260921_155418_6ef46c`)
- **Fase sebelumnya (endpoint §6.2):** SELESAI. **129 ✅ / 0 ⬜** — seluruh endpoint
  backend terpasang dan ter-push (`e450c48`). Brief lama (F1–F13) sudah tidak berlaku.
- **Gate:** `tools\gate-overnight.cmd` (wajib, sudah diuji dua arah)
- **Jejak:** `docs/OVERNIGHT-LOG.md` — satu entri per fase, ditulis sebelum lanjut

---

## 1. Misi malam ini

**Frontend.** Backend sudah tamat: 131/131 endpoint §6.2 terpasang, terverifikasi
lawan API nyata, ter-push (`verify_suite.py`: `STATUS ENDPOINT: 131 ✅ dan 0 ⬜`).

Fase 0–17 (tabel §2) sudah selesai. Blok lama di sini bilang tiga hal "ditulis lalu
tidak disambungkan" — **ketiganya sudah tidak berlaku**, diverifikasi ulang
2026-10-02:

| Yang dulu diklaim | Kenyataannya sekarang |
|---|---|
| `store/api/stream.ts` — "nol komponen yang memakainya" | Terdaftar di `store/index.ts`, ada `stream-wiring.test.ts`, dan `use-sse-cache.ts` dipakai `BoardToolbar.tsx`, `ApprovalDetail.tsx`, `TaskDetailDrawer.tsx`. |
| `hooks/use-sse-cache.ts` — "nol pemanggil" | Tiga pemanggil (di atas). |
| `TaskDetailDrawer.tsx:44-46` — tab butuh endpoint yang tidak ada | Ketiga endpoint ada; tab Logs/Artifacts/Approvals terpasang. |

Sisa kerja nyata ada di **§2b**: 8 layar yang backend-nya sudah jalan tapi UI-nya
belum ada, plus 3 utang yang butuh keputusan produk.

## 2. Urutan fase 0–17 (SELESAI — dipertahankan sebagai jejak)

> **Sumber progres yang SAH, sudah diverifikasi lawan kode (2026-09-27):**
> - `docs/CHECKLIST.md` → **68 PASS / 4 dikerjakan / 16 belum / 1 ditunda**
>   dari 89 story M0–M4. Dihasilkan `tools/update_checklist.py` dari
>   `tools/checklist_status.json`. **Jangan diedit tangan** — ubah sidecar, lalu
>   jalankan ulang tool-nya (`python tools/update_checklist.py`).
> - Tabel detail `docs/ARCHITECTURE.md` §6.2.1–§6.2.19 → **129 ✅ / 0 ⬜**,
>   dijaga `verify_suite.py`.
> - `docs/00-PRD.md` **bukan** pelacak progres: 387 item AC, nol tercentang.
>
> "PASS" berarti **layarnya ada filenya dan komponennya terpasang** (di router
> atau dirender induknya). **Bukan** berarti design match — itu belum diukur
> untuk semua layar.

**Yang pertama dikerjakan bukan story baru, tapi yang sudah ditulis lalu tidak
disambungkan.** Ini pola sebenarnya di repo ini:

| Ada | Kenyataannya |
|---|---|
| `store/api/stream.ts` 158 baris + test | SSE lengkap (EventSource, patch cache RTK, `Last-Event-ID`). **Nol komponen memakainya.** |
| `hooks/use-sse-cache.ts` | Hook siap. **Nol pemanggil.** |
| `settings/ApiKeys.tsx`, `settings/Webhooks.tsx` | Masing-masing 21 baris, bilang "not available yet". Backend 5 dan 7 endpoint jalan. |
| `TaskDetailDrawer.tsx` | Tab Artifacts/Logs/Approvals/assignee picker belum dibangun. Endpoint-nya semua ada. |

Maka urutannya:

| # | Fase | Story | Kenapa murah |
|---|---|---|---|
| 0 | ✅ **SELESAI** (`d4078f1`) — Pasang `useSseCache` di kanban + toolbar | US-AD39 | Board hidup tanpa refresh. |
| 1 | ✅ **SELESAI** — **Artifacts tab** (list + unduh) + tab shell 4 tab | US-AD48 | Upload dipisah ke fase 1b, lihat di bawah. |
| 2 | ✅ **SELESAI** (`F2`) — **Logs tab**: step timeline per run (grup per run, warna status, expand payload) | US-AD26, US-AD94 | AC1 (cache read/write) dan AC5 (masking viewer) TIDAK bisa: kolom/permukaan tidak ada. Dicatat di OPEN-ISSUES. |
| 1b | ✅ **SELESAI** (`F1b`) — **Upload artifact** di tab Artifacts | US-AD46 | File picker → `upload-url` → PUT → register. `run_id` diambil dari `useListTaskRunsQuery` (Fase 2 sudah menyediakannya). |
| 3 | ✅ **SELESAI** (`F3`) — **Approvals tab** di drawer | US-AD34, US-AD35 | `stream.ts` sudah punya `listApprovals`/`approveApproval`/`rejectApproval`. |
| 4 | ✅ **SELESAI** (`F4`) — **Approval detail** `36-approval-detail` | US-AD34, US-AD35 | Layar belum ada; datanya sudah. |
| 5 | ✅ **SELESAI** (`F5`) — **Run detail** `32-run-detail` | US-AD41 | `GET /runs/{id}` + `/summary` ada. |
| 6 | ✅ **SELESAI** (`F6`) — **Notifications** `13-notifications` | US-AD61 | Backend notifikasi ada sejak F9. |
| 7 | ✅ **SELESAI** (`f900251`) — **Webhooks** `40-webhooks` | US-AD52, US-AD53 | Backend 7 endpoint jalan sejak F12. Admin-only; secret tampil sekali. |
| 8 | ✅ **SELESAI** (`ba5faa4`) — **API Keys** `39-api-keys` | US-AD06 | Backend 5 endpoint jalan sejak F10. Key penuh tampil sekali. |
| 9 | ✅ **SELESAI** (`10b095c`) — **Dashboard** `42-dashboard` | US-AD76 | — |
| 10 | ✅ **SELESAI** (`00fd787`) — **Dependency view** `24-dependency-view` | US-AD19 | Endpoint per-task = N+1; ditambah `GET /boards/{id}/dependencies`. |
| 11 | ✅ **SELESAI** — **State screens** `43`/`44`/`45` | US-AD63, US-AD64, US-AD65 | Skeleton, empty, error boundary. Kecil-kecil. |
| 12 | **Skill library** `26b-agent-skills` | US-AD107 | ✅ SELESAI — `SkillLibrary.tsx` + markdown renderer sendiri (tanpa `marked`/`dompurify`), commit `d696c3e`. |
| 13 | **Security** `16-security` | US-AD90 | ✅ SELESAI — ganti password + sesi aktif; `status: 'PARSING_ERROR'` ditangani lewat `originalStatus`. |
| 14 | **Mobile board** `46-mobile-board` | US-AD60 | ✅ SELESAI — accordion + shell mobile (rail/sidebar/cost-rail lepas <768px, bottom nav). |
| 15 | **Ledger explorer** `30-ledger-explorer` | US-AD27 | ✅ SELESAI — endpoint `GET /orgs/{id}/ledger` baru + tabel AC4 + empty state AC5 + paging. |
| 16 | **Rate limit** | US-AD85 | ✅ SELESAI — `internal/ratelimit`, AC1–AC4 terbukti di wire, mutasi 11/0/0. |
| 17 | **Deteksi string keras di CI** | US-AD50 | ✅ SELESAI — `.github/workflows/ci.yml` + `frontend/scripts/check-i18n.cjs` (lexer TS, baseline per-file). |

| 1b | ✅ **SELESAI** (`8068758`) — **Upload artifact** dari UI | US-AD48 | File picker → `upload-url` → PUT → register; `run_id` diambil dari `useListTaskRunsQuery`. |

Yang sudah selesai tidak dihitung ulang. Tabel di atas **selesai semua (0–17)**.

Kalau fase 0–5 kelar, itu hasil yang bagus. Jangan mulai fase baru sebelum yang
lama ter-push.

## 2b. Urutan fase 18+ (SISA NYATA — diverifikasi ke kode 2026-10-02)

Backend tiap baris di bawah **sudah jalan**; yang kurang cuma UI. Urut dari
termurah. Satu fase = satu commit + push + satu entri `OVERNIGHT-LOG.md`.

| # | Layar | Story | Mockup | Backend (terverifikasi) |
|---|---|---|---|---|
| 18 | Audit log | US-AD95 | `41-audit-log` | `GET /api/v1/audit-log` ada; nol route `/audit` di `router.tsx` |
| 19 | Tutup akun sendiri | US-AD98 | `17-close-account` | `DELETE /api/v1/auth/me` ada; nol `closeAccount` di `frontend/src` |
| 20 | Ekspor CSV | US-AD56 | `31-cost-export` | ledger + cost-summary ada; nol `exportCsv`/`text/csv` di `frontend/src` |
| 21 | Command palette | US-AD55 | `14-command-palette` | murni klien. `uiSlice` sudah punya `commandPaletteOpen` + `toggleCommandPalette`; **komponennya belum ada** |
| 22 | Multi-sort header tabel | US-AD54 AC2 | `19-table-view` | murni klien. `TableView.tsx` ada; header belum bisa diklik |

**Belum masuk daftar karena backend-nya belum ada** — jangan dikerjakan di bawah
goal ini, catat saja kalau kepepet: `US-AD57` bulk move, `US-AD78` invite link,
`US-AD72` prepopulate board dari template. Ketiganya **nol route** di `cmd/api`
(127 route total, nol yang cocok `bulk|move`, `invite`, `template`).

**Berhenti dan tulis di log, jangan dikerjakan**, untuk tiga utang yang butuh
keputusan produk (semuanya butuh migrasi atau keputusan, bukan kode):
US-AD94 AC1 cache read/write per step, US-AD94 AC5 masking per peran,
`decision_reason`.

## 3. Loop tiap fase

1. Baca kontraknya dulu: design `design/stitch-output/v2/*.html` (layout = sumber
   kebenaran, **teks mockup jangan dibawa ke produk**) + `docs/DESIGN.md` (token).
2. Tulis plan singkat di `docs/OVERNIGHT-LOG.md` (fase, layar, file, risiko).
3. RTK Query slice di `frontend/src/store/api/`. Server state **hanya** lewat RTK
   Query; Redux Toolkit = satu-satunya state management.
4. Bahasa: **`en` DAN `id` sekaligus** atau tidak compile. `t['key']`, bukan `t('key')`.
5. Ukur DOM / screenshot **elemen** buat kerjaan layout. **Nol unit test buat CSS.**
6. `tsc -b` + `prettier --check src/` tiap edit (gate cepat).
7. Commit + push + tulis hasilnya di `docs/OVERNIGHT-LOG.md`.

## 3b. Full e2e (diizinkan, TERUKUR ~5 menit)

User mengizinkan full e2e. Perintah yang benar, dan caranya **wajib** begini:

```bash
cd frontend
rm -rf .e2e-final
node node_modules/@playwright/test/cli.js test --shard=N/4 --workers=1 --trace=off --output=.e2e-final
```

- **Wajib dipecah `--shard=N/4`.** Full suite sekali jalan (~400s+) menembus cap
  tool terminal; per-shard ~47–170 detik.
- **`--workers=1`.** Config repo memang `workers: 1` + `fullyParallel: false`;
  menaikkannya bikin tes saling ganggu.
- **Jangan `npx`** — kadang nggak resolve. Pakai path `node_modules` langsung.
- **Jangan background** — background selalu `stdin is not a tty`. Foreground +
  redirect ke file.
- **`--output` unik per run.** Kalau dipakai ulang, artifact kegagalan run
  sebelumnya ketimpa dan buktinya hilang — ini pernah kejadian.
- **Hapus `.e2e-final/` setelah selesai**, jangan sampai ke-commit.

Angka acuan yang sudah diukur: **128 tes, 0 gagal**, shard 1/2/3/4 =
42/25/38/23, total ~307 detik.

`column-editor.spec.ts` (AC1) **pernah flake sekali** dengan pola
`locator.fill` timeout 60 detik padahal labelnya ada di komponen dan tesnya
lolos 2.6 detik saat dijalankan sendiri. Shard itu dua kali lebih lambat di run
pertama (168s vs 87s). Kalau ketemu lagi: **ulangi tes itu dulu sebelum
menyimpulkan regresi**, dan kalau gagal di bawah tekanan, catat sebagai flake —
jangan diamkan.

Container web (`agentdeck-web` :5173) dan API harus hidup. `reuseExistingServer`
membuat dev server yang sudah jalan dipakai ulang.

## 3c. Suite artifact butuh object storage

`e2e/artifacts.spec.ts` punya dua test dan **satu di antaranya bergantung
lingkungan**:

- **"dua empty state"** — jalan di mana saja. TIDAK butuh storage. Asertnya
  mengikuti jawaban API (503 = belum dikonfigurasi, 200 = belum ada artefak).
- **"round-trip byte"** — `test.skip` otomatis kalau API menjawab 503. Untuk
  menjalankannya sungguhan, API harus punya `S3_*`, **dan** presigned host-nya
  harus dijangkau browser yang menjalankan test.

### Kenapa butuh lebih dari sekadar mengisi `S3_*`

URL presigned punya satu host string, dan host itu dibakar ke dalam tanda
tangan. Yang menandatangani (API) dan yang memakainya (browser) harus setuju.
Container tidak bisa memenuhi keduanya: `minio:9000` tidak dikenal browser,
`127.0.0.1:9000` tidak berarti di dalam container.

Solusinya: **API di HOST** (seperti probe F13) + **dev server kedua** yang
proxy-nya diarahkan ke host. Ini yang dipakai F13 dan dipakai lagi di sini:

    # 1. API di host, MinIO hidup (container agentdeck-minio :9000)
    #    S3_ENDPOINT=http://127.0.0.1:9000 supaya browser bisa menjangkaunya
    bash "C:/Users/Reza/AppData/Local/hermes/cache/scratch/run-art-e2e.sh"

    # 2. Jalankan suite-nya lawan dev server kedua itu (:5174).
    #    JANGAN :5173 -- yang itu proxy ke API container yang tidak punya S3_*.
    cd frontend
    E2E_BASE_URL=http://127.0.0.1:5174 \
      node node_modules/@playwright/test/cli.js test e2e/artifacts.spec.ts \
      --workers=1 --trace=off --output=.e2e-art

`E2E_BASE_URL` menaikkan juga `webServer.url`, jadi Playwright tidak menyalakan
dev server ketiga. Variabelnya opsional: tanpa itu semuanya jalan di :5173
seperti sebelumnya.

**Sesudah selesai, matikan rig-nya.** Kalau tidak, suite biasa ikut gagal:

    powershell -NoProfile -Command "Get-Process agentdeck-api -EA SilentlyContinue | Stop-Process -Force"
    docker compose up -d api     # hidupkan lagi yang di container

### Kalau lupa mematikan rig

Gejalanya menyesatkan: container API mati, tapi :8080 tetap hidup karena API
host yang menjawab. Suite biasa (lewat :5173) gagal 502 karena proxy-nya
menunjuk container yang sudah mati. Cek dulu siapa yang memegang :8080.

## 4. Kontrak gate (terukur, bukan tebakan)

`tools\gate-overnight.cmd` = `gofmt -l .` → `go build ./...` → `tsc -b` →
`vitest run` → `prettier --check src/`. Terukur **20–52 detik**, di bawah plafon
300 detik. Sudah diuji dua arah (kotor → 1, bersih → 0).

**`go test ./...` DIKELUARKAN dari gate, dan itu disengaja** — terukur 490 detik,
mustahil masuk plafon 300. Konsekuensinya: gate hijau **tidak** berarti test Go
hijau. Kalau sebuah fase menyentuh Go, jalankan `go test` paket itu **manual** dan
tulis hasilnya di log. Jangan pernah bilang "gate hijau jadi aman".

`prettier` di-scope ke `src/` — `prettier --check .` merah karena file milik sesi
lain (`frontend/e2e/demo/measure-aksi.mjs`).

## 5. Aturan keras

- **Jangan** nyalain dispatcher AgentDeck (`AGENTDECK_DISPAT`). Keputusan user: OFF.
- **Jangan** sentuh board kanban, `tools/gate.cmd`, `frontend/e2e/demo/`,
  `tools/demo/`, atau `.gitignore`. Itu milik sesi lain.
- **Jangan** commit: `node_modules`, build output, cache, screenshot sementara, secrets.
- Kredensial/token/connection string → `[REDACTED]`. Nol nomor WA/JID/chatId di file.
- **Jargon spec (`US-AD`, `AC`, `M1`–`M6`, `HTTP \d{3}`) DILARANG masuk UI yang
  dirender.** Komentar kode boleh. `/github` pengecualian.
- **Combobox** buat semua select — token design wajib, bukan `<select>` bawaan browser.
- **Modal**: role/ARIA, focus trap, Escape, scroll lock, return focus, ikut `DESIGN.md`.
- Icon pakai **Lucide**.
- `git ls-remote origin refs/heads/main` = satu-satunya bukti push yang sah.
- Edit lewat python bisa ngubah line-ending → repo ini LF. Cek `file <path>` setelahnya.
- `curl` di MSYS itu program Windows native: `-o /tmp/x` nulis ke `C:\tmp\`. Pakai path native.

## 6. Kapan berhenti

- Fase 18–22 kelar dan masih ada waktu → lanjut, tetap satu commit per fase.
  Kalau 22 kelar, **stop**: sisa PRD butuh backend baru atau keputusan produk.
- Ada blocker yang butuh keputusan produk → **stop**, tulis di log, jangan ngarang
  jawaban. Kontrak yang bertabrakan: sebut konfliknya, pakai keputusan terbaru.
- Gate merah dan nggak kelar dalam 3 percobaan → stop, tulis apa adanya.
- Kehabisan turn (`goals.max_turns`) → goal auto-pause; watchdog yang ngabarin.

## 7. Yang TIDAK dijamin malam ini

- Semua layar yang belum ada. Di luar §2b masih ada belasan mockup yang
  **backend-nya juga belum ada** — itu bukan kerjaan malam ini.
- Full e2e Playwright (butuh ~5 menit dan Docker hidup; bukan gate).
- Design match penuh tiap layar — yang wajib: **inventory elemen** (ada / beda /
  **ilang**), dan yang ilang disebut, bukan didiemin (`docs/DESIGN-INVENTORY.md`).
