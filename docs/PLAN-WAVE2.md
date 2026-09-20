# PLAN — Gelombang 2 WS-AGENT (spec siap eksekusi)

Dokumen ini adalah spec kerja **gelombang 2** workstream Agent Registry. Ditulis
sebelum gelombang 1 selesai supaya bisa langsung gas tanpa nunggu.

Rujukan mengikat: `DECISIONS.md` §6A (pricing/provider/skill), `ARCHITECTURE.md` §6.2.7
(12 endpoint), §9.1 (harga), §16 (kredensial), `00-PRD.md` US-AD86/96/106/107/108,
US-AD67/73.

---

## 1. Kenapa harus bergelombang

Gelombang 1 = fondasi + satu potongan UI yang nol dependency (item 1-4 PLAN WS-AGENT).

Gelombang 2 tidak bisa jalan bareng gelombang 1 karena **dua endpoint UI-nya belum ada**:

| Layar | Butuh endpoint | Status |
|---|---|---|
| `26-agent-form` (modal) | `GET /agent-catalog`, `GET {base_url}/models`, `POST /agents/{id}/validate` | belum ada |
| `27-agent-provider-key` | `PUT`/`DELETE /agents/{id}/provider-key` | belum ada |
| `28-agent-detail` | `PATCH /agents/{id}` (termasuk `archived_at`) | belum ada |
| Skill library | `GET`/`POST`/`PATCH /agent-skills` | belum ada |

Kalau UI dikerjakan lebih dulu, yang terjadi adalah UI memanggil endpoint yang tidak
ada → error, lalu harus dibongkar ulang saat endpoint asli muncul. Urutannya **backend
dulu, UI menyusul**.

---

## 2. Grafik dependency

```
GELOMBANG 1 (jalan)
  [1] migration 0008 (runs, ledger_entries, agent_skills, agents.base_url+archived_at)
  [2] internal/pricing (tabel 220+51, resolver, rumus 5 komponen)
  [3] UI 25: toolbar + footer + 2 card guidance
        │
        ▼
GELOMBANG 2
  [4] internal/crypto (AES-256-GCM)        ──┐
  [5] internal/provider (SSRF guard + /models) │
  [6] GET /agent-catalog  (butuh [2])      ──┤
  [7] provider-key endpoints (butuh [1],[4]) │  ← BACKEND, boleh paralel
  [8] agent-skills endpoints (butuh [1])   ──┘
        │
        ▼
  [9]  UI 26 modal (butuh [6],[7])
  [10] UI 27 panel kredensial (butuh [7])
  [11] UI 28 detail + arsip (butuh [7] PATCH)
  [12] label "estimate" + price_source di UI (butuh [2],[6])
```

**[4]-[8] semua backend dan saling lepas** → bisa jalan paralel sebagai satu gelombang.
**[9]-[12] semua UI dan menyentuh file yang sama** → **TIDAK boleh paralel sesama UI.**
Satu subagent mengerjakan [9]+[10]+[11]+[12] berurutan, atau dibagi per-file dengan
kepemilikan eksklusif (lihat §5).

---

## 3. Detail pekerjaan backend

### [4] `internal/crypto` — enkripsi kredensial provider (US-AD86)

- AES-256-GCM. Kunci dari env, **bukan** dari DB. Format kolom: `nonce(12B) || ciphertext || tag(16B)`
  (sudah ditetapkan di DDL `agents.provider_api_key_enc`).
- Fungsi: `Encrypt(plaintext []byte) ([]byte, error)`, `Decrypt(blob []byte) ([]byte, error)`.
- **Tulis-saja dari sisi API**: `GET /agents/{id}` **tidak pernah** mengembalikan kunci,
  hanya `has_provider_key bool`. Tidak ada endpoint yang mengembalikan plaintext.
- Env var wajib ada saat start; kalau tidak ada dan ada agent ber-kredensial → **fail closed**,
  jangan diam-diam pakai kunci kosong.
- Test wajib: roundtrip; nonce berbeda tiap enkripsi (dua enkripsi plaintext sama menghasilkan
  ciphertext berbeda); tag rusak → gagal dekripsi (bukan mengembalikan sampah); kunci salah → gagal.

### [5] `internal/provider` — SSRF guard + daftar model (US-AD106)

- `ValidateBaseURL(raw string) error` — hanya `https`. Tolak: IP private (10/8, 172.16/12,
  192.168/16), loopback (127/8, `::1`), link-local (169.254/16, `fe80::/10`), `0.0.0.0`,
  metadata cloud (169.254.169.254), dan hostname yang me-resolve ke salah satunya.
- **Redirect tidak boleh diikuti** ke alamat private: set `CheckRedirect` dan validasi tiap hop.
- `ListModels(ctx, baseURL, apiKey)` — `GET {base_url}/models`, format OpenAI-compatible
  (`{"data":[{"id":"..."}]}`). Timeout ≤ 10 s. Batasi ukuran respons.
- Test wajib: tabel kasus yang harus **ditolak** (semua kelas IP di atas) dan yang **diterima**
  (host publik https). Test redirect ke private harus gagal. **DNS rebinding**: hostname yang
  resolve ke private harus ditolak — resolusi dicek, bukan hanya bentuk string.
- Test ini load-bearing: ini satu-satunya yang mencegah server dipakai memindai jaringan internal.

### [6] `GET /api/v1/agent-catalog` (US-AD96, US-AD108)

- Auth: Session/Key, role min **Viewer**. Idempotent.
- Response: daftar model dari `internal/pricing` (220 exact + 51 pattern) + harga estimate
  per model, plus `price_version` dan penanda `price_source`.
- Model `unpriced` **tetap muncul** dengan `price_source: "unpriced"` — UI yang memutuskan
  menampilkannya sebagai "harga tidak diketahui", bukan menghapusnya.
- Test wajib: jumlah model = 220 + 51; `unpriced` tidak hilang; angka harga sama dengan
  yang di `docs/PRICING.md` (regresi bug 1000x).

### [7] Endpoint kredensial provider (US-AD86)

- `PUT /api/v1/agents/{id}/provider-key` — Session, role min **Admin**. Body membawa kunci
  transient + opsional `provider`/`model`/`base_url` (untuk agent yang belum tersimpan).
  Idempotent (Key).
- `DELETE /api/v1/agents/{id}/provider-key` — Session, **Admin**. Agent kembali ke env default.
- `POST /api/v1/agents/{id}/validate` — Session/Key, **Member**. **Manual, bukan otomatis**:
  hanya jalan kalau user menekan tombol uji. Menerima kunci transient di body (opsi (a) —
  tidak butuh ID agent tersimpan).
- **Kredensial TIDAK PERNAH masuk log, audit `before_json`/`after_json`, atau response.**
- Test wajib: `member`/`viewer` dapat 403 di `PUT`; `viewer` dapat 403 di `POST .../validate`;
  kunci tidak muncul di response `GET /agents/{id}`; `has_provider_key` jadi `true` setelah PUT;
  audit log tidak memuat kunci.

### [8] Endpoint skill library (US-AD107)

- `GET /api/v1/agent-skills` — Session/Key, **Viewer**.
- `POST /api/v1/agent-skills` — Session, **Admin**. Body `body_md`.
- `PATCH /api/v1/agent-skills/{id}` — Session, **Admin**. `version` naik; baris lama **tidak**
  diubah surut.
- **Agent tidak boleh memanggil ini.** Test wajib: auth sebagai agent (bukan user) → 403.
- Sanitasi markdown saat render (skill di-preview di UI) — jangan percaya `body_md`.
- Seed 8 skill bawaan saat org dibuat: `code_review`, `e2e_test`, `debug`, `refactor`,
  `test_write`, `docs`, `migration`, `security_review`.
- Test wajib: paritas EN/ID tidak relevan di sini (backend), tapi **slug unik per org** harus
  diuji: dua skill slug sama di org sama → 409; di org berbeda → boleh.

---

## 4. Detail pekerjaan UI

### [9] `26-agent-form` sebagai modal (US-AD96)

- Modal, bukan halaman. Isi form mengikuti `design/stitch-output/v2/26-agent-form.html`.
- **Ukuran modal harus pas**: tidak boleh ada label yang turun ke baris kedua. Ukur dengan DOM,
  jangan kira-kira.
- Field: nama, provider (termasuk `openai_compatible` untuk BYO), model, `base_url`
  (hanya muncul bila provider `openai_compatible`), kredensial (opsional), TOOLS, SKILL.
- Model diambil dari `GET /agent-catalog` — **integrasi nyata**, bukan daftar hardcoded.
- Bila provider BYO: tombol muat model memanggil `GET {base_url}/models` (lewat backend,
  bukan dari browser — kunci tidak boleh menyentuh browser).
- TOOLS: **daftar tertutup 9 primitif**. SKILL: dari `GET /agent-skills`.
- Kredensial **opsional** di create (menghormati US-AD20 AC5 + US-AD96 AC3).
- Uji kredensial = tombol manual, bukan otomatis saat submit.

### [10] `27-agent-provider-key` (US-AD86)

- Panel 420px, bukan halaman penuh. Dipakai **dua tempat**: dari form create (isi kredensial)
  dan dari halaman detail (rotasi/hapus).
- Selalu tampilkan status `has_provider_key`; kunci yang ada **tidak pernah** ditampilkan,
  hanya masked.

### [11] `28-agent-detail` + arsip (US-AD67, US-AD73)

- Klik **nama** agent di tabel 25 → route ke halaman detail ini (keputusan user).
- Arsip lewat `PATCH /agents/{id}` dengan `archived_at`.
- AC3 US-AD73: arsip saat agent masih memegang run aktif → **409**, bukan memutus run.
  Surface inline, jangan toast (aturan repo: error yang sudah punya inline surface tidak
  diduplikasi sebagai toast).
- AC4: arsip butuh owner/admin; member/viewer → 403.

### [12] Label "estimate" (US-AD108)

- **Setiap** angka biaya di UI ditandai estimasi, bukan tagihan. Tidak boleh ada satu pun
  angka biaya tanpa penanda ini.
- Bila `price_source == "unpriced"` → tampilkan "harga tidak diketahui", **bukan** `$0.00`.

---

## 5. Kepemilikan file (wajib, biar tidak tabrakan)

Satu file hanya boleh dimiliki **satu** pekerja dalam satu gelombang.

| Pekerjaan | File yang boleh disentuh |
|---|---|
| [4] crypto | `internal/crypto/**` |
| [5] provider | `internal/provider/**` |
| [6] catalog | `cmd/api/agent_catalog.go`, `cmd/api/agent_catalog_test.go` |
| [7] kredensial | `cmd/api/agents_provider_key.go`, `cmd/api/agents_provider_key_test.go` |
| [8] skills | `cmd/api/agent_skills.go`, `cmd/api/agent_skills_test.go`, `internal/skill/**` |
| [9]-[12] UI | `frontend/src/routes/dashboard/agents/**`, `frontend/src/lib/i18n.ts`, `frontend/e2e/agents.spec.ts` |

**`cmd/api/router.go` (atau padanan pendaftaran route) dan `internal/store/queries/*` adalah
titik tabrakan.** Gelombang 2 backend punya 3 pekerja yang butuh mendaftarkan route/query.
Pilih salah satu sebelum mulai:
- **(i)** satu pekerja mendaftarkan semua route lebih dulu, atau
- **(ii)** route didaftarkan di file per-domain yang di-`include` router.

Jangan biarkan tiga pekerja mengedit `router.go` bersamaan.

---

## 6. Verifikasi wajib (tiap pekerjaan, bukan opsional)

- `gofmt -l .` → **kosong**. Satu statement per baris.
- `go test ./<package-yang-dimiliki>/...` → lulus. **Jangan** `go build ./...` repo-wide saat
  pekerja lain sedang menulis.
- Bug wajib punya **test gagal dulu** sebelum diperbaiki.
- Guard wajib dibuktikan **load-bearing** lewat mutasi: rusak guard-nya → test harus GAGAL.
  Guard yang tidak bisa dibuat gagal berarti tidak menjaga apa pun.
- Frontend: `npx prettier --check src e2e`, `npx tsc -b`, `npx vitest run`,
  `npx playwright test e2e/agents.spec.ts --workers=1`.
- UI: screenshot + **dilihat** (`vision_analyze`). Dilarang mengklaim hasil visual tanpa
  melihat gambar. Verifikasi layout via ukur DOM, bukan dari screenshot full-page.
- `python tools/verify_suite.py` → 0 FAIL.
- `python tools/verify_web.py` → 0 FAIL, 0 warning.
- `python tools/update_checklist.py` → tandai story yang lulus.

---

## 7. Keputusan yang sudah diambil

### 7.1 Skill library (US-AD107) — desain UI/UX, tanpa mockup Stitch

Keputusan user 2026-09-20: **pakai token design yang ada**, dan UX-nya dipikirkan
supaya friendly. Karena belum ada mockup, layar ini **bukan** "match 100%" — dia
**konsisten** dengan design system. Perbedaan itu harus jujur, jangan diklaim match.

**Prinsip UX (kenapa begini, bukan sekadar "biar rapi"):**

Skill itu *teks yang bisa dieksekusi agent*. Dua kesalahan UX yang harus dihindari:
(a) user mengedit skill tanpa tahu agent mana yang memakainya → perubahan tak sengaja
merusak agent; (b) user menulis markdown dan tidak tahu hasilnya → skill rusak saat
dipakai. Jadi desainnya harus menjawab keduanya di layar yang sama.

**Tata letak — split view, bukan modal:**

```
┌──────────────────────────┬────────────────────────────────────────┐
│  Daftar skill            │  Pratinjau / Editor                    │
│  ┌────────────────────┐  │  ┌──────────────────────────────────┐  │
│  │ code_review        │  │  │  [Pratinjau] [Editor]   ← tab    │  │
│  │ v3 · dipakai 4 agt │◀ │  │                                  │  │
│  ├────────────────────┤  │  │  # Code Review                   │  │
│  │ e2e_test           │  │  │                                  │  │
│  │ v1 · dipakai 2 agt │  │  │  Periksa ...                     │  │
│  ├────────────────────┤  │  │                                  │  │
│  │ debug              │  │  └──────────────────────────────────┘  │
│  │ v2 · belum dipakai │  │  Dipakai oleh: Agent A, Agent B       │
│  └────────────────────┘  │  [Simpan]  v3 → v4                    │
└──────────────────────────┴────────────────────────────────────────┘
```

- Panel kiri `240px`, memakai `surface-well` + `rounded.md` (pola `board-column`).
- Panel kanan `flex-1`, memakai `surface-panel` + `rounded.md` + border `border-subtle`
  (pola `card-task`).
- Pratinjau markdown memakai `mono-code` + `code-block` (`surface-sunken`) — sudah ada.
- **Kolom "dipakai N agent" itu wajib**, bukan hiasan: itu yang mencegah user merusak
  agent tanpa sadar (masalah (a) di atas).
- Badge versi memakai `badge-info`; skill yang belum dipakai siapa pun memakai
  `badge-warning` untuk menarik perhatian sebelum dipakai.
- Simpan memakai `button-primary`; tombol hapus `button-danger` dengan konfirmasi
  modal (`modal` + `overlay-backdrop`).

**Alur yang harus mulus:**

1. **Buat skill** — tombol di header panel kiri → editor kosong + template awal
   (heading + satu paragraf contoh), supaya user tidak mulai dari nol.
2. **Edit** — tab `Pratinjau`/`Editor`. Perubahan yang belum disimpan ditandai
   `badge-warning` "belum disimpan"; pindah skill dengan perubahan tertunda → konfirmasi.
3. **Simpan** — `version` naik (v3 → v4), tampilkan `toast-success` dan badge versi baru.
   Baris lama **tidak** berubah surut (AC6 US-AD107).
4. **Dipakai siapa** — daftar nama agent yang merujuk skill ini, di bawah editor.
   Klik nama agent → route ke `28-agent-detail`.
5. **Kosong** — `empty-state` (padding 32px) dengan ajakan "Buat skill pertama", bukan
   tabel kosong.
6. **Sanitasi markdown** — `body_md` dari user **tidak boleh** dipercaya. Render lewat
   sanitizer; `<script>` dan handler inline dibuang. Ini trust boundary, bukan kosmetik.

**Yang TIDAK boleh ada di layar ini:** tombol apa pun yang membiarkan agent menulis
skill. Skill itu milik org; agent hanya memakainya (keputusan user).

### 7.2 Titik tabrakan route — sudah diputuskan

Repo ini punya konvensi jelas: tiap domain mendaftarkan route-nya sendiri lewat
`registerXxxRoutes(mux, api, svc)` di file domain masing-masing
(`cmd/api/boards.go:registerBoardRoutes` adalah contohnya).

**Keputusan: pekerja gelombang 2 TIDAK menyentuh `cmd/api/main.go`.**

- Tiap pekerja menulis `registerXxxRoutes(...)` di file miliknya sendiri.
- Test-nya membangun `mux` sendiri dan memanggil register function itu langsung —
  jadi tidak perlu menunggu `main.go`.
- **Orchestrator (gw) yang menyambungkan ketiganya ke `main.go`** saat integrasi.
  Ini 3 baris, dikerjakan sekali, setelah semua pekerja selesai → nol tabrakan.

`internal/store/queries/queries.sql` dan hasil `sqlc generate` juga titik tabrakan:
kalau dua pekerja butuh query baru, salah satu menunggu. Karena itu **hanya pekerja
yang benar-benar butuh query baru** yang menyentuhnya, dan tidak bersamaan.

### 7.3 Keputusan lain yang masih terbuka

1. **`glm-5.3-flash` tidak ada di 220 entri exact** — hanya kena pattern `glm-5*`
   (1/4/0.5/6). Keputusan user: pakai pattern dulu, override manual nanti.
2. **Atribusi MIT** untuk 271 baris data dari 9Router — ditunda user, tapi wajib
   sebelum push publik.
3. **Gateway Hermes tidak jalan** → cron tidak fire otomatis. Keputusan user:
   dijalankan manual saat dibutuhkan (`hermes gateway`), bukan dipasang sebagai service.

