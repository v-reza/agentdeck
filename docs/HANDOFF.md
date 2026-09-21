# Handoff — status AgentDeck per 2026-09-21

Buat session baru. Baca `.hermes.md` + `docs/DECISIONS.md` dulu, terus file ini.

---

## Kerjaan yang sedang jalan: Provider Registry

Konsepnya disetujui 2026-09-21. Tujuh fase, detail di `docs/CONCEPT-PROVIDER-REGISTRY.md`.

| Fase | Isi | Status |
|---|---|---|
| 0 | Kontrak: PRD + DECISIONS + ARCHITECTURE | **SELESAI** |
| 0.5 | Selaraskan §18 + gate backend · arsip dokumen mati | **SELESAI** |
| 1 | Migrasi `0010`: tabel `providers`, `agents.provider_id`, backfill | **SELESAI** |
| 2 | Backend CRUD provider + RBAC (5 endpoint) | **SELESAI** |
| 3 | Probe protocol-aware (openai_compatible dulu) | **SELESAI** |
| 4 | Halaman Provider (nav baru) | belum |
| 5 | Form agent: provider jadi dropdown | belum |
| 6 | Buang `agents.base_url` + constraint `agents_base_url_chk` | belum |

Fase 2 dikerjakan sebagai **5 endpoint, bukan 7**; fase 3 melengkapi dua sisanya.
`POST /providers/{id}/verify` (AC3) menembak `POST {base_url}/chat/completions` dengan
`max_tokens: 1` — bukan `GET /models`, karena ada gateway yang balas `200` di `/models`
tanpa kredensial. `POST /providers/{id}/models` (AC7) menarik ulang daftar model.

**AC7 sudah lengkap.** Refresh otomatis >24 jam jalan lewat `ModelRefresher`
(`internal/providerreg/refresher.go`), tick 15 menit, batch 25 provider per pass.

Kenapa **background ticker, bukan refresh-saat-dibaca**: floor `GET /providers` itu
Viewer, dan refresh memanggil upstream pakai kredensial ruang kerja. Viewer yang buka
halaman nggak boleh bisa memicu panggilan keluar — itu otoritas yang sama yang bikin
`POST /providers/{id}/models` di-floor Admin. Refresher jalan sebagai "nobody", jadi
nggak butuh role apa pun. Ada test yang mengunci ini: `TestRefresherIsNotTriggeredByARead`.

Ticker-nya 15 menit sementara staleness-nya 24 jam — sengaja. Tick-nya cuma SELECT
terindeks yang biasanya kosong; yang mahal adalah fetch yang dia mungkin picu. Tick
24 jam juga berarti provider yang didaftarkan sesaat setelah tick menunggu sehari
penuh untuk daftar pertamanya.

`models_fetched_at IS NULL` dibaca **stale**, bukan fresh: "belum pernah tarik" justru
state yang perlu diambil. Query-nya `ListStaleProviderModels` — **satu-satunya query
provider yang sengaja tidak org-scoped**, karena refresher nggak punya tenant di
tangan. Dikunci test Postgres lintas-org; kode yang melayani request nggak boleh
memanggilnya.

`ponytail:` refresher-nya single-process. Dua replika API bakal dobel-tick dan
dobel-fetch. Kalau itu jadi masalah, fix-nya `pg_try_advisory_lock` per batch, bukan
queue.
Aturan mengikat: `DECISIONS.md` §6A.J. Endpoint: `ARCHITECTURE.md` §6.2.8 (total 123).

---

## Di mana kita sekarang

AgentDeck = board orkestrasi AI-agent fleet. Go + PostgreSQL 16 backend,
React 19 + Vite frontend. Repo publik: `https://github.com/v-reza/agentdeck`.

**Fase**: masih development, ngejar **fitur jalan + design match**, bukan
kelengkapan test. Rigor test diturunin (lihat `.hermes.md` bagian gate berlapis).

## Keputusan proses (2026-09-21)

1. **Gate berlapis.** Layout = screenshot elemen + ukur DOM, nol unit test.
   Logic = test. Security = test + mutation. Full suite 1× sehari, bukan tiap
   iterasi. Alasan: satu hari kepakai buat verifikasi layout pakai 92 e2e test.
2. **Session per milestone.** Session lama (`20260919_004317_ed3ea9`) 114.336 pesan /
   91MB — 2× lipat session terbesar kedua. Tiap turn bawa konteks yang udah
   di-compress, jadi detail ilang diam-diam. Pindah session per milestone.
3. **Design inventory sebelum bilang layar kelar.** Lihat `docs/DESIGN-INVENTORY.md`.

## Kerjaan yang BELUM di-commit

53 file kotor. Semua gate hijau (e2e 92/92, vitest 58, Go suite,
`verify_web` 0 FAIL, `verify_suite` 0 FAIL). **Nol commit** — user belum minta.

Termasuk fase 0 + 0.5: kontrak provider registry (`00-PRD.md` US-AD109,
`DECISIONS.md` §6A.J, `ARCHITECTURE.md` §3 + §6.2.8 + §6.2.20 + §18/§18.3),
gate baru `check_structure()` + cek kolom `Status`, dan arsip/hapus dokumen mati.

**Urutan commit yang diusulkan ada di `docs/COMMIT-PLAN.md`** — 4 commit, dipisah
per tema (BYO flow / kontrak / restructure dokumen / env). Belum dijalankan.

Yang berubah:

- **Backend**: `cmd/api/provider_models.go` (endpoint probe stateless baru),
  `internal/board/service.go` (`validateBaseURL` + `ValidateAddressOnly`),
  `internal/provider/provider.go` (pisah cek alamat vs cek DNS),
  `cmd/api/boards.go`, `internal/board/pgx.go`, `internal/store/queries*`
  (`base_url` + `reasoning_effort` masuk `CreateAgent`)
- **Frontend**: `components/ui/combobox.tsx` (BARU — semua select pakai ini),
  `routes/dashboard/agents/CreateAgentFields.tsx` (BARU), `lib/field-error.ts`,
  `AgentRegistry.tsx` (`table-fixed` + `<colgroup>`), `CreateAgentForm.tsx`,
  `StatusFilter.tsx`, `components/ui/input.tsx`, `components/ui/modal.tsx`,
  `lib/i18n.ts`, `store/api/agents.ts`
- **Test**: `e2e/combobox.ts`, `e2e/agent-form.spec.ts`, `e2e/agents.spec.ts`,
  `agents_baseurl_test.go`, `agents_model_gate_test.go`, `provider_models_test.go`,
  `queries_columns_test.go`, `agents.test.ts`

## Keputusan final yang mengikat

- **`provider_id` NULL itu sah dan permanen** (diputuskan di fase 1). Bukan cuma
  transient backfill seperti yang sempat ditulis di DDL. Artinya sama dengan
  `provider_api_key_enc IS NULL`: alamatnya implisit lewat env default §16.
  963 agent `openai` + 10 `anthropic` + 2 `google` + 1 `deepseek` memang tanpa
  base URL, jadi mereka **tidak** dapat provider — mengarang base URL buat mereka
  ditabrak §6A.J ("provider bawaan: kosong total, nggak ada template siap pakai").
- **Backfill grup per `(org_id, base_url, api_key_enc)`**, bukan per org. Per org
  bakal melebur dua agent satu org dengan alamat beda jadi satu provider dan
  diam-diam memindahkan alamat salah satunya.
- **ID provider = `min(agent.id)`** dari grupnya, bukan ULID baru. Generator ULID
  di SQL berarti dua implementasi ULID (Go + plpgsql) — persis pola dual source of
  truth yang repo ini perangi. Nyilih id agent: valid 26 char, unik, urut waktu,
  deterministik, nol objek baru.
- **`models_json` diisi model yang lagi dipakai** grup itu; `models_fetched_at`
  tetap NULL. `'[]'` bikin fase 5 punya state rusak (model agent nggak ada di
  allowlist providernya). **Fase 3 wajib baca NULL sebagai "belum pernah tarik →
  tarik"**, bukan "nggak perlu refresh".
- **Nol FK `provider_id → providers(id)`** — DDL §3 nggak nyantumin, dan FK nggak
  bisa negasin kecocokan org. AC5 tetap query app-level karena wajib nyebut agent
  mana yang memakai.
- **Klaim §6A.J "15 agent menyimpan key yang sama persis" tidak bisa diverifikasi
  dari DB.** Yang terukur: 47 agent BYO, 15 punya key, **15 org berbeda**, dan
  **0 org punya >1 agent ber-key**. Ciphertext beda semua, tapi itu tidak
  membuktikan apa-apa (nonce AES-GCM acak per baris). Yang jelas: di dataset dev
  ini nol workspace menyimpan key dobel, jadi payoff dedupe-nya nol di sini.
  Klaim itu tetap jadi alasan desain, tapi jangan diulang sebagai fakta terukur.
- **Provider BYO-only di form register** — **DIBATALKAN** oleh `DECISIONS.md` §6A.J.
  Provider sekarang datang dari daftar milik org. `CREATE_PROVIDER_CHOICES` akan diganti
  daftar itu di fase 5; jangan dipertahankan sebagai keputusan.
- **SSRF**: `localhost` + `127.0.0.1` **string persis** boleh, boleh `http`.
  Alias loopback (`2130706433`, `0x7f000001`, `[::1]`) + RFC1918 + link-local tetap ditolak.
  Mengalahkan US-AD106 AC3 — konflik dicatat di `DECISIONS.md` §6A.F
  (SSRF-nya sendiri masih berlaku; yang digantikan §6A.J cuma soal lokasi kredensial).
- **Jalur tulis nggak boleh resolve DNS.** `ValidateAddressOnly` (teks + IP literal)
  buat create/update; `ValidateOperatorBaseURL` (resolve) buat jalur yang konek.
- **Endpoint probe** `POST /api/v1/provider/models`, floor Admin, stateless, nol DB.
  `400` user-error / `502` upstream.
- **Satu dropdown**: `Combobox` buat semua select.
- **Modal register 560px** (`size="lg"`), panel kredensial tetap 420px.
- **`PATCH /providers/{id}` itu partial, bukan full-replace.** Bentuk `UpdateAgent`
  (tulis semua kolom) akan mengosongkan tiap kolom yang nggak dikirim — dan tiga di
  antaranya (`models_json`, `models_fetched_at`, `last_verified_at`) milik fase 3.
  Ganti nama provider = model list + timestamp verifikasi hilang diam-diam.
  Implementasi: `COALESCE(sqlc.narg(...), kolom)`, dan tiga kolom fase 3 **tidak ada**
  di SET clause sama sekali.
- **`is_default` digerakkan dua statement: clear dulu, baru set.**
  `providers_org_default_key` itu partial unique index non-deferrable, jadi satu
  `UPDATE ... SET is_default = (id = $x)` masih bisa kena 23505 — Postgres cek index
  per baris dan bisa mengunjungi default baru lebih dulu. State nol-default sah (AC9),
  jadi ini bukan setengah-tulis.
- **`CreateInput.IsDefault` tri-state (`*bool`).** Create nggak punya nilai tersimpan
  buat fallback, jadi "nggak dikirim" dan "eksplisit false" bakal sama-sama jadi zero
  value. Provider pertama satu workspace **wajib** jadi default (AC9) sementara
  `false` eksplisit wajib dihormati — dua hal itu cuma bisa dibedakan kalau
  tri-state. `nil` = workspace yang mutusin (`count == 0`).
- **Edit provider menyinkronkan `agents.base_url`.** Sampai fase 6 buang kolomnya,
  `agents.base_url` masih yang dirender layar agent — tanpa sinkron, AC6 gagal secara
  observable. Query-nya dijaga `AND provider = 'openai_compatible'` biar nggak
  nabrak `agents_base_url_chk` (constraint itu baru dibuang di fase 6, bukan sekarang).
  Ini jembatan sementara, dibuang bareng kolomnya.
- **AC5 409 pakai JSON body** `{error, agents[], total}`, bukan `http.Error` teks
  polos — AC5 minta nyebut agent mana yang memakai. Daftar dibatasi 20 + `total`
  supaya pemotongan nggak dibaca sebagai kebenaran utuh.
- **Kredensial tetap tulis-saja di registry.** `has_key` di semua read; `masked_key`
  cuma di response request yang mengirim key, dan dihitung dari plaintext request itu
  (storage cuma punya ciphertext). Nol endpoint yang bisa menghapus kredensial — di
  luar 7 endpoint kontrak, jadi nggak dikarang.
- **`POST /providers` menerima tiga protokol**, bukan cuma `openai_compatible`.
  DDL nerima ketiganya dan nolak selain itu = API lebih ketat dari kontrak tanpa AC
  yang memintanya. Yang belum ada cuma probe-nya (fase 3).

## Divergensi struktur backend yang BELUM ditangani

Dua hal ini nyata dan terukur, tapi **sengaja ditunda** (keputusan user 2026-09-21:
fase 2 tetap 5 endpoint, divergensi dicatat, dikerjakan pas runtime mulai dibangun).

1. **`cmd/agentdeck` vs `cmd/api` — nol gate.** `ARCHITECTURE.md:66` (P5) nulis
   "`cmd/agentdeck` menerima flag `-role=api|dispatcher|worker|all`". Realita: cuma
   `cmd/api/`, dan `grep 'flag\.' cmd/api/main.go` → nol. §18 bahkan nggak punya §18.1
   (lompat §18 → §18.3 → §18.2) dan gate lolos. Ini klaim kontrak tanpa wujud yang
   **nggak ketangkep gate apa pun**.
   Nggak bisa dibangun sekarang: flag `-role` baru ada artinya kalau
   dispatcher/worker ada, dan §18.3 bilang semuanya "Belum". Bangun sekarang =
   scaffolding buat nanti.
2. **`internal/board/` paket 4 domain — 2325 LOC, 40+ method `Service`.** Project,
   Board, Agent, Task, TaskLink, Event numpuk di situ; lima modul §6.2.20 ada di
   dalamnya (Projects 5/5, Boards 7/7, Agents 15/15, Task Links 4/4, Tasks 7/11).
   `internal/auth/` juga multi-tanggung-jawab (3759 LOC: session, RBAC, API key,
   audit, profil, avatar, reset) tapi itu satu bounded context "identity", lebih
   bisa dibela.

   **Opini: jangan pecah sekarang.** Kontraknya sendiri nggak minta paket per domain
   — yang diminta P5 cuma *aturan arah import* (`internal/dispatcher` nggak boleh
   di-import handler HTTP), bukan jumlah paket. Restructure sekarang = ubah §18 +
   ubah gate + pindah 4 domain + tulis ulang test yang nyetir `board.Service`: diff
   gede, nol hasil yang keliatan user, sementara runtime 0% jadi. Kalau nanti mulai,
   tempat paling natural itu paket domain baru — `internal/providerreg/` (fase 2)
   udah jadi preseden split tanpa big-bang.

   **Yang murah dan pantes dibetulin sekarang**: divergensi P5. Betulin dokumennya
   biar cocok realita (`cmd/api/`, flag ditunda sampai role-nya ada).

## Masalah yang BELUM beres

1. **Design match — ini yang paling gede.** Detail + angka di `docs/DESIGN-INVENTORY.md`
   (50 layar diaudit, 36 ada impl, 14 nol impl). Lima kelas masalah, urut dari
   paling murah dibetulin:

   | # | Masalah | design | impl | Biaya fix |
   |---|---|---|---|---|
   | 1 | **Warna status beda 10/10** | `#16a34a` (done) | `#15803d` | 10 baris `index.css` |
   | 2 | Icon Lucide | 319 `<svg>` | 3 | 1 dependency, 319 titik |
   | 3 | Grafik batang / sparkline | 119 `<rect>` | 3 | ikut nomor 2 |
   | 4 | Skeleton loading | 55 `animate-pulse` | 4 | per layar |
   | 5 | State archived | 6 `line-through` | 0 | per layar |
   | 6 | Token `--spacing-*` mati | 6 token | **0 dipakai** | hapus atau pakai |

   **Akar masalahnya**: nol gate yang ngecek `index.css` terhadap `DESIGN.md`.
   `verify_suite.py` cuma `designmd lint` DESIGN.md sendiri. Persis pola §18 —
   klaim tanpa gate = drift diam-diam. Kalau bikin gate baru, taruh di sini.
2. **Layar agent registry belum nemu "titik enaknya".** Satu hari lebih ngotak-atik
   implementasi, tapi pertanyaan "informasi apa yang harus ada di layar ini" belum
   dijawab. **Perlu sesi khusus nggak ngoding.**
3. `statusFilter` + `AgentDetailForm` masih ada `<select>` native sisa. **Fase 5
   tidak menyentuh dua ini** — mereka bukan bagian provider registry, jadi jangan
   dihitung sebagai kerjaan fase itu.
4. Pesan backend 409/403 masih Inggris, UI default Indonesia.
5. Belum ada `LICENSE`/`NOTICE`/`THIRD_PARTY`. Konflik lisensi di design
   (`09b-github.html` Apache-2.0 vs `05-landing.html` MIT).
6. ~~Audit `livez`/`metrics` + tabel tanpa DDL~~ **SELESAI** — lihat
   "Kemajuan nyata" di bawah. Angkanya sekarang dijaga gate, bukan dihitung manual.
7. `US-AD03` TODO — `DELETE /api/v1/orgs/{id}` belum ada.

## Environment

- Docker hidup: `agentdeck-api` :8080, `agentdeck-db` :5432, `agentdeck-web` :5173.
- **Ubah kode Go → rebuild container**, kalau nggak route baru nggak ada (ini
  yang bikin 404 dan gw buang waktu nyari bug di test).
- Playwright baseURL `http://127.0.0.1:5173`. Cookie `Secure` → `page.request` 401,
  browser `fetch` 200. Test pakai `page.evaluate` + `credentials: 'include'`.
- Go: `/c/Program Files/Go/bin`. sqlc: `/c/Users/Reza/go/bin`. Node: `/c/Program Files/nodejs`.

## Wajib

**API key 9Router sudah dirotasi user — insiden ditutup.** Nilai lama jangan
pernah ditulis di file mana pun (repo ini publik).

## Mulai dari mana (buat session baru)

1. Baca `.hermes.md` (auto-load) → `docs/DECISIONS.md` §6A.J → file ini.
2. **Kalau nggak ada instruksi lain: fase 4** — halaman Provider (nav baru).
   Fase 1 (`0010`) + fase 2 (CRUD provider, 5 endpoint) + fase 3 (probe + refresh
   otomatis, 2 endpoint) **selesai**. Tujuh fase di tabel atas.
3. **Yang paling murah + paling kerasa kalau mau cepat**: warna status. 10 baris
   `index.css` — lihat `docs/DESIGN-INVENTORY.md` §2.
4. Yang **jangan** dikerjain dulu: buang `agents.base_url` (fase 6) sebelum form agent
   pindah ke dropdown provider (fase 5). Kolom itu masih yang dirender layar agent.
5. **Fase 5**: `agents.base_url` masih disinkronkan dari provider
   (`SyncAgentBaseURLForProvider`) — jembatan sementara, dibuang bareng kolomnya di
   fase 6. Jangan tambah pembaca baru untuk kolom itu.

## Kemajuan nyata (dihitung dari kode, bukan dari niat)

| | Jumlah |
|---|---|
| Endpoint terdefinisi di kontrak | 123 |
| **Endpoint jalan** (route terdaftar di `cmd/api`) | **62** |
| Endpoint belum | 61 |
| Tabel kontrak | 26 |
| Tabel ada migrasi + DB | 17 |

Angka tabel itu sempat **basi satu** sebelum fase 1: HANDOFF menulis 17 sementara
gate menghitung 16 dari `internal/migrate/*.up.sql`. Sekarang 17 dan cocok —
tapi jangan percaya angka tabel di dokumen tanpa menjalankan `verify_suite.py`.

Per modul (detail di `ARCHITECTURE.md` §6.2.20): Agents **15/15** · Boards **7/7** ·
Projects **5/5** · Task Links **4/4** · Providers **7/7** · Orgs **8/9** · Auth **7/11** ·
Tasks **7/11** · Health **2/4** · API Keys **0/5** · Runs **0/6** · Steps **0/3** ·
SSE **0/4** · Approvals **0/5** · Ledger **0/5** · Artifacts **0/5** · Comments **0/4** ·
Webhooks **0/7** · Audit **0/6**.

Providers **7/7** — `verify` (AC3) dan `models` (AC7) selesai di fase 3.

Modul runtime (`dispatcher`, `executor`, `sse`, `storage`, `webhook`) **nol kode**.
Runtime belum pernah memanggil LLM. Satu-satunya panggilan inference di repo adalah
probe kredensial fase 3 (`internal/provider.ProbeInference`), dan itu bukan runtime:
`max_tokens: 1`, dipicu tombol Uji, tidak menyimpan hasil. `grep chat/completions` →
satu, di probe itu.

Angka ini dijaga gate: kolom `Status` di §6.2 diperiksa `verify_suite.py` dua arah,
jadi ✅ palsu dan ⬜ palsu dua-duanya FAIL.
