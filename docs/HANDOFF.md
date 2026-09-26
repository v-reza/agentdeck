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
| 4 | Halaman Provider | **SELESAI** |
| 5 | Form agent: provider jadi dropdown | **SELESAI** |
| 6 | Buang `agents.base_url` + constraint `agents_base_url_chk` | belum |

**Fase 5 selesai.** Form register cuma memilih provider + model: `provider` dan
`base_url` **diturunkan** dari `providers.protocol` / `providers.base_url` di server
(`applyProvider`), bukan diketik operator, dan model wajib ada di `models_json`
provider itu (AC10 — dilewati kalau daftar provider masih kosong, karena fase 3
menariknya lazily). `agents.provider_id` mengalir di jalur tulis **dan** di lima
pembaca (`GetAgent`, `ListAgents`, `ArchiveAgent`, `UnarchiveAgent`, `CreateAgent`) —
tanpa itu PATCH yang cuma ganti nama bakal menghapus provider-nya.

Satu temuan dari pengukuran, bukan penalaran: di ruang kerja yang **belum punya
provider**, form lama mustahil dipakai. `POST /agents` yang mengirim `{name, model}`
tanpa provider dan tanpa base_url dijawab **400 `invalid input`** — dan pesan itu
dipetakan frontend ke field **Nama**, jadi operator mengulang nama padahal masalahnya
di tempat lain. `ProviderSection` sekarang punya state kosong yang menyebut sebabnya
dan menautkan ke `/settings/providers`, dan submit menolak lebih awal dengan pesan di
section yang bisa memperbaikinya. Konsekuensinya `CredentialSection` jadi pernyataan,
bukan input: karena provider wajib, cabang field key per-agent (US-AD86) tak
terjangkau secara konstruksi. US-AD86 tetap berlaku di layar **detail** agent, yang di
luar scope fase 5.

Fase 2 dikerjakan sebagai **5 endpoint, bukan 7**; fase 3 melengkapi dua sisanya.
`POST /providers/{id}/verify` (AC3) menembak `POST {base_url}/chat/completions` dengan
`max_tokens: 1` — bukan `GET /models`, karena ada gateway yang balas `200` di `/models`
tanpa kredensial. `POST /providers/{id}/models` (AC7) menarik ulang daftar model.

**Fase 4 tidak pakai nav baru**, walau tabel di atas semula menulis begitu. Rail punya
6 slot tetap yang dipatok angka di `shell.md`; menambah slot ke-7 berarti mengubah shell
dan meregenerasi 51 layar yang sudah match — biaya besar untuk nol manfaat produk.
Kredensial juga settings-scoped, sama seperti API key dan webhook. Jadi route-nya
`/settings/providers`.

**AC7 sudah lengkap.** Refresh otomatis >24 jam jalan lewat `ModelRefresher`
(`internal/providerreg/refresher.go`), tick 15 menit, batch 25 provider per pass.

**AC3: probe mencoba sampai 3 model, berhenti di 2xx pertama.** Sebelumnya cuma
`Models[0]` satu kali, dan itu bikin false negative yang terukur: di provider
`keystore.edumai.tech`, `Models[0]` balas `502` sementara 5 model lain balas `200`
dengan key yang sama — badge "gagal" untuk kredensial sehat. Yang menuduh
kredensial hanya `401`/`403`; status lain berarti model/upstream-nya yang
bermasalah, jadi model berikutnya dicoba. Plafon 3 percobaan = ~3 token per klik.
**Tidak ada fan-out seluruh daftar** — provider dengan 699 model akan mengubah satu
klik jadi 699 request, melanggar aturan "probe hanya saat tombol ditekan" di
DECISIONS §6A.J. Detail + batas yang diketahui: `DECISIONS.md` §6A.J sub-bagian
"Probe: satu model tidak cukup".

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

1. **Design match — ini yang paling gede.** Angkanya **dihasilkan**
   `tools/design_audit.py`, bukan dihitung tangan: `docs/DESIGN-INVENTORY.md`
   sekarang ditulis ulang script itu tiap dijalankan, dan `--check` jadi gate
   (exit 1 kalau jargon bocor atau `<select>` bawaan hidup).

   Tiga koreksi yang alat itu temukan di inventory lama — jangan pakai angka lama:

   | Klaim lama | Yang terukur |
   |---|---|
   | mockup pakai "satu set konsisten Lucide" | **22 dari 51** file mockup pakai ligature Material Symbols |
   | 55 `animate-pulse` = skeleton | 55 itu mayoritas dot status; blok skeleton asli **323 di design, 0 di impl** |
   | "warna status 10 dari 10 beda" | benar, tapi parser lama cuma menemukan **3 dari 10** (bullet di-wrap + `archived` deskriptornya dua kata) |

   Sisa kelas masalah, urut dari paling murah:

   | # | Masalah | design | impl | Biaya fix |
   |---|---|---|---|---|
   | 1 | **Warna status beda 10/10** | `#16a34a` (done) | `#15803d` | 10 baris `index.css` |
   | 2 | Icon Lucide | 319 `<svg>` | 3 | 1 dependency, 319 titik |
   | 3 | Grafik batang / sparkline | 119 `<rect>` | 3 | ikut nomor 2 |
   | 4 | Skeleton loading | 323 blok | ~~**0**~~ 20 komponen di 17 layar | SELESAI |
   | 5 | State archived | 6 `line-through` | 0 | per layar |
   | 6 | Token `--spacing-*` mati | 6 token | **0 dipakai** | hapus atau pakai |
   | 7 | `<select>` bawaan | — | ~~**4 file, 7 titik**~~ **0** | SELESAI (fase D) |

   **Akar masalahnya**: nol gate yang ngecek `index.css` terhadap `DESIGN.md`.
   `verify_suite.py` cuma `designmd lint` DESIGN.md sendiri. Persis pola §18 —
   klaim tanpa gate = drift diam-diam. Gate baru untuk ini: `tools/design_audit.py
   --check`.
2. **Layar agent registry belum nemu "titik enaknya".** Satu hari lebih ngotak-atik
   implementasi, tapi pertanyaan "informasi apa yang harus ada di layar ini" belum
   dijawab. **Perlu sesi khusus nggak ngoding.**
3. ~~`<select>` bawaan masih hidup di 4 file~~ **Selesai (fase D, commit
   `4af38e1`)**: nol `<select>` bawaan, semuanya lewat `Combobox`.
   Skeleton loading juga selesai (fase E): komponen `Skeleton`/`SkeletonRows`/
   `SkeletonText` di `components/ui/skeleton.tsx`, dipakai 17 layar. Bentuknya
   ikut DESIGN.md (`skeleton` token: `surface-sunken`, radius 4px) — mockup 43
   memakai `#e8ecea` + `rounded-[4px]` yang memang token itu. Mockup TIDAK
   berdenyut (`animate-pulse` nol di file itu), jadi skeleton ini juga tidak.
   Sisa fase: F (warna status 10/10, state archived, icon Lucide, jargon, token
   spacing mati).
4. **Kolom Provider sekarang menampilkan NAMA provider, bukan protokol.** Bug yang
   dilaporkan: agent di provider `9router` tampil sebagai `openai_compatible`,
   karena `agents.provider` itu protokol yang diturunkan server (US-AD109 AC6).
   Registry + hero detail me-resolve nama dari `GET /providers` yang sudah ada di
   cache. **Jangan** tambah kolom `provider_name` di `agents`: itu denormalisasi
   yang fase 6 baru saja buang.
5. **Kartu "Estimated rate" sekarang misahin tiga sebab**: `loading` (catalog belum
   sampai), `noProvider` (agent nggak punya provider), `unpriced` (model memang
   nggak ada di tabel). Sebelumnya tiga-tiganya satu kalimat.
4. Pesan backend 409/403 masih Inggris, UI default Indonesia.
4b. ~~**Harga model BYO = nol, dan tidak ada tempat mengisinya.**~~ **SELESAI** — tingkat 1
   ada sejak migrasi `0012` + `internal/modelprice` + tiga endpoint `GET`/`PUT`/`DELETE
   /api/v1/model-prices` (baca Viewer, tulis Admin). `GET /agent-catalog` sekarang mengisi
   parameter `override` yang dulu selalu `nil`, dan mengoreksi dirinya sendiri: model yang
   punya override dilaporkan `price_source: manual` dengan harga override itu — bukan harga
   katalog — plus model yang cuma punya override **muncul** di daftar padahal tabel bersama
   tidak mengenalnya. Terukur lewat API nyata: `my-own-llama-70b` → `in=$0.50 out=$1.50`/1M.
   **Yang masih terbuka:** penulis `ledger_entries`. Runtime LLM belum ada, jadi mekanismenya
   siap tapi belum ada yang memakainya — dan gate biaya US-AD32 tetap tidak akan menyala untuk
   agent BYO sampai executor M4 menulis baris ledger. UI pengisian harga **belum ada** dan
   nol mockup menggambarkannya; tiga endpointnya sengaja tidak dikarang ke layar.
   Detail + angka di DECISIONS §6A.C.1.
4c. **Rujukan silang `§6A.G` / `§6A.H` sempat salah tunjuk.** Skill library dulu `§6A.G`;
   saat §6A.J ditambahkan, judulnya ikut berubah jadi `§6A.H` dan `§6A.G` hilang, sementara
   `ARCHITECTURE.md` + `cmd/api/agent_skills.go` masih menunjuk `§6A.G`. Sudah dibalikin ke
   G, dan `tools/verify_suite.py` sekarang memeriksa penomoran 6A. (label kembar / hilang).
4d. **Nol test untuk perpindahan kolom board, dan itu menutupi bug yang bikin seluruh
   drag-and-drop mati.** `UpdateTaskStatusParams` punya dua field (`Status`, `Status_2`)
   hasil generate sqlc dari `SET status = $4 WHERE ... AND status = $3`, jadi `Status`
   adalah **guard** dan `Status_2` nilai barunya — terbalik dari kesan namanya.
   `internal/board/pgx.go` mengisinya dengan urutan intuitif, sehingga `WHERE status =
   <tujuan>` tidak pernah cocok: **setiap** `POST /tasks/{id}/move` balas 409 `task is
   not in the expected state`, tanpa error, tanpa log. Fix: `Status: string(from)`.
   Sekarang dijaga `internal/board/task_status_test.go` lawan Postgres nyata (mutasi
   dikembalikan ke urutan lama → MERAH, dengan pesan yang menunjuk penyebabnya).
   Pelajarannya bukan soal satu swap: suite punya 0 test `UpdateTaskStatus`, jadi bug
   ini tidak bisa dilihat gate mana pun. Fon tulis status task cuma satu, jadi blast
   radius-nya seluruh board.
4e. **`tasks.archived_at` nol penulis.** Ada di DDL dan di `taskResponse`, tapi tidak
   ada satu statement pun yang menulisnya — arsip cuma mengubah `status`. Semua read
   path menyaring `status != 'archived'`, jadi tidak ada bug hari ini; `archived_at`
   cuma selalu NULL. Jangan pakai kolom itu untuk memutuskan apa pun sampai ada penulis.
4f. **US-AD59 bocor 3 AC sekaligus, semuanya di jalur arsip task.** (a) tidak ada
   penegakan "hanya dari status terminal" — `done`/`cancelled`/`archived` saja yang
   boleh; (b) route `move` di-floor Member, padahal AC4 bilang arsip butuh owner/admin;
   (c) arsip kedua balas 409, padahal AC3 bilang idempoten. Ketiganya sekarang dijaga
   `cmd/api/tasks_archive_test.go` + probe API nyata. Kontrak yang bertabrakan: baris
   ARCHITECTURE untuk `/tasks/{id}/move` menulis floor `Member` tanpa menyebut pengecualian
   arsip — PRD AC4 yang dipakai, dan barisnya dikoreksi.
4g. **Toolbar board tidak pernah ada, dan itu bukan cuma soal tampilan.**
   `CreateTaskForm.tsx` ada sejak dulu tapi **tidak diimpor di mana pun**, jadi
   dari UI tidak ada cara membuat task sama sekali — walau `POST
   /boards/{id}/tasks` sudah ✅ dan mockup 21 menggambar modalnya lengkap.
   Field `body` (US-AD11 AC1) juga tidak bisa diisi dari mana pun.
   Sekarang: `BoardToolbar` satu komponen dipakai kanban + tabel (design
   menggambar bar-nya sekali; search box-nya sudah drift sebelumnya), modal
   "Buat Task Baru" ikut `AppShell` lewat pola `TaskCreateFormHost`.
   Pelajarannya: **komponen ada ≠ fitur ada.** Gate mana pun tidak akan melihat
   file yang tidak diimpor; yang melihat hanya orang yang mencoba memakai
   layarnya.
4h. **Kontrak §6.2.16 mengklaim filter `status, assignee, search` di
   `GET /boards/{id}/tasks` dengan status ✅, tapi handler-nya nol query
   param.** Filternya hanya ada di client (`TableView`), jadi `curl` dan app
   tidak sepakat soal board yang sama. Sekarang tiga-tiganya di SQL lewat
   `sqlc.narg`, dan `status` menerima pengulangan parameter karena chip
   filter-nya multi-pilih — satu nilai saja akan diam-diam membuang chip kedua.
   Dijaga `internal/board/task_filter_test.go` + `cmd/api/tasks_filter_test.go`
   lawan Postgres nyata.
4i. **Gate `verify_suite.py` cuma satu arah soal endpoint.** Arah "doc menjanjikan
   ✅ tapi route tidak ada" dan "route ada tapi ditandai ⬜" sudah dijaga; arah
   ketiga — **route ada tapi tidak tercatat sama sekali** — tidak. Itu bukan
   celah teoretis: `GET /boards/{id}/assignable-agents` ditambahkan dengan nol
   baris dokumen dan gate-nya hijau. Arah ketiga sekarang FAIL, dan langsung
   menemukan `GET /users/{id}` yang sudah jalan sejak US-AD89 tanpa pernah masuk
   tabel. Jumlah endpoint: **129** (dari 127).
4j. **Modal `21-task-create` baru sekarang match design-nya.** Empat gap nyata,
   dua di antaranya bukan kosmetik: (a) prioritas digambar sebagai **empat level
   berlabel** `P0 — Blocker` … `P3 — Low`, implementasinya input angka mentah
   `−10..10` — baris board bisa berbunyi "7"; sekarang Combobox 4 pilihan yang
   memetakan ke 0..3. (b) **footer modal tidak ada sama sekali**: design menggambar
   band `#f6f7f6` full-bleed berisi "Target: <board>" + Batal + tombol utama
   berikon. Band itu pola modal design (juga di `22-column-editor` dan
   `31-cost-export`), dan `Modal` tidak bisa menghasilkannya karena header-nya
   sibling, bukan anak — jadi `footer=` sekarang membuat kartu ber-*section*
   (header 20×14 bergaris, body ber-padding, footer `--color-surface-page`
   20×12). Empat pemakai `footer=` lain tidak berubah: hanya modal kartu yang
   sectioned, panel `placement="right"` tetap seperti sebelumnya. Dijaga e2e
   yang **mengukur DOM** (lebar 560, radius 14, padding band, warna band, tinggi
   tombol 32), bukan screenshot full-page.
   (c) subjudul header + nama board di footer: dulu tidak ada konteks board sama
   sekali. (d) penanda wajib `*` di label — sekarang `Field` punya prop
   `required`; murni presentasional, `required` di input tetap yang memblokir.
   **Dua hal design sengaja TIDAK diikuti:** field "Status Awal" (Backlog/Ready) —
   `ready` itu yang diklaim dispatcher (DECISIONS §3) dan US-AD11 AC1/AC5 bilang
   task baru mulai `backlog`; form yang bisa bikin `ready` = nyerahin kerjaan ke
   runner yang belum pernah lihat task-nya. Dan kartu helper "AC5 …/AC1 …" —
   itu jargon spec (`US-AD`, `AC`) di UI yang dirender, dilarang `.hermes.md`;
   dua faktanya ditulis ulang pakai bahasa operator.
4k. **`ValidateProvider` mengukur `provider` dengan kosakata yang salah.** Tabel
   harga berkunci **vendor** (`openai`, `deepseek`, `qwen`, … 18 nama); schema
   berkunci **protokol** (`openai_compatible`/`anthropic`/`google`). Keduanya cuma
   kebetulan beririsan — `anthropic` dan `google` dua-duanya, `openai` cuma yang
   pertama. Karena validatornya membaca `pricing.Providers()`, `POST
   /projects/{id}/agents` dengan `provider:"deepseek"` tersimpan sebagai 201 —
   nilai yang nol layer lain terima, dan yang **sudah nggak ditawarkan form**
   sejak registry jadi pemilik endpoint+kredensial. Ini inkonsistensi yang
   `DECISIONS §6A.J` sendiri catat; sekarang validatornya baca
   `providerreg.AcceptableProtocol` dan `pricing.Providers()` (nggol pemanggil)
   dihapus. Terukur lawan container: `provider:"deepseek"` **201 → 400**, dan
   tiga jalur sah (cuma `provider_id` persis seperti UI, `provider_id`+`provider`,
   tanpa provider_id) **tetap apa adanya**.
   Dua hal yang bikin ini mahal ketemu, dan keduanya dicatat supaya nggak
   keulang: (a) **`TestPutProviderKeyAcceptsKnownProviders` adalah test yang
   menahan bug-nya** — dia assert `openai`/`deepseek`/`qwen` harus lolos, jadi
   memperbaiki validatornya kelihatan seperti regresi; test itu ditulis ulang ke
   protokol, dan sisi penolakannya pindah ke `provider_protocol_test.go`. (b) 38
   fixture test kirim `provider:"openai"`. Yang gate US-AD67 AC2 (model harus
   berharga) **tidak boleh** dipindah ke `openai_compatible`, karena protokol itu
   sengaja dikecualikan dari AC2 (US-AD106) ⇒ test itu bakal lolos vakum;
   dipindah ke `google`, yang vendor sekaligus protokol, jadi gate-nya tetap
   menggigit. Mutasi (validator nerima apa pun non-kosong) **CAUGHT** di 5 test
   lintas dua lapisan.
4l. **M4 sebagian: siklus hidup run + ledger (10 endpoint, ⬜ → ✅).** Yang dibangun:
   migrasi `0013` (tabel `steps`, `approvals`, `artifacts` — tiga yang belum ada;
   `runs`/`events`/`ledger_entries` sudah sejak `0008`), `internal/board/runtime.go`,
   `cmd/api/runs.go`, dan `POST /tasks/{id}/claim` sebagai pintu masuk.
   `ledger_entries` akhirnya punya penulis, dan `tasks.consecutive_failures`
   akhirnya punya penulis juga — sebelumnya cuma **dibaca**, jadi plafon retry
   dibandingkan dengan angka yang tidak pernah bergerak = retry tanpa batas.
   Temuan yang cuma kelihatan lawan DB nyata: predikat klaim butuh
   `current_run_id IS NULL`, dan **nol kode** yang melepas ikatan itu — tanpa
   `ClearTaskCurrentRun` di `EndRun`, setiap task yang pernah disentuh satu run
   jadi tidak bisa diklaim selamanya (retry J4, reclaim J6, `review` → `ready`).
   **Kontrak yang bertabrakan:** ARCHITECTURE §5.3/§5.4 bilang run sukses → `done`;
   PRD US-AD22 AC2 + J2 bilang → `review`. PRD yang dipakai — kalau sukses langsung
   `done`, status `review` tidak pernah bisa dimasuki padahal dia ada di state
   machine dan di alur cerita.
   **Yang TIDAK dibangun, dan itu disengaja:** executor LLM. Nol kode di repo ini
   yang menentukan agent mana mengerjakan task mana lewat keputusan sendiri, dan
   US-AD11 menetapkan **satu** agent per task — jadi tidak bisa di-broadcast dan
   tidak bisa ditebak dari kontrak. Itu keputusan produk, bukan pekerjaan impl.
   Efeknya: kredensial ledger juga belum ada route-nya (kontrak tidak
   mendefinisikannya) — rollup biaya dibuktikan di `internal/board/runtime_test.go`
   lawan Postgres, bukan lewat HTTP.
   **Klaim "verbatim" di trace itu salah, dan probe yang menangkapnya:** `steps.payload_json`
   bertipe JSONB, jadi Postgres menormalkan urutan kunci + spasi sebelum handler
   melihatnya. Test handler lolos karena stub-nya mengembalikan struct in-memory,
   yang tidak punya perilaku JSONB — hanya probe lawan container yang menunjukkan
   `{"z":1,"a":[1,2]}` kembali sebagai `{"a": [1, 2], "z": 1}`. Klaim di doc, test,
   ARCHITECTURE, dan `DiffViewer` dikoreksi jadi "setia pada isi, bukan byte".
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
2. **Roadmap provider registry (fase 1–6) SELESAI SEMUA.** `agents.base_url` +
   constraint `agents_base_url_chk` sudah dibuang migrasi `0011`, dan
   `SyncAgentBaseURLForProvider` ikut hilang — agent menyimpan **rujukan**
   (`provider_id`), bukan salinan alamat. Layar detail agent ikut pindah ke dropdown
   registry. Nggak ada fase lanjutan yang tertulis di `CONCEPT-PROVIDER-REGISTRY.md`.
3. **Kalau nggak ada instruksi lain**, pilih dari backlog yang belum tersentuh:
   endpoint yang belum jalan (61 dari 123 — `ARCHITECTURE.md` §6.2 kolom `Status`,
   angkanya dijaga `tools/verify_suite.py`) atau warna status (10 baris `index.css`,
   `docs/DESIGN-INVENTORY.md` §2).
4. ~~Jangan kerjain fase 6 dulu~~ — **selesai**, jangan diulang.
5. **Jembatan `SyncAgentBaseURLForProvider` sudah dibuang.** Jangan tambah kolom
   salinan alamat baru di `agents`: satu alamat, satu tempat (`providers.base_url`),
   dan itulah yang bikin edit provider nggak bisa basi di agent (AC6).

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
