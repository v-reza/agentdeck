package artifact

// Tes aturan service artifact — 12.3, N22, 3.15.
//
// Yang diuji di sini adalah keputusan yang tidak kelihatan dari HTTP:
//
//  1. Digest-nya BENAR-BENAR dihitung ulang dari isi objek. Klien yang
//     mengirim sha256 milik orang lain harus ditolak — kalau tidak,
//     "memverifikasi SHA-256" (12.3 langkah 6) tidak memverifikasi apa pun.
//  2. `size` didaftarkan harus sama dengan yang dilaporkan storage. Tanpa itu
//     kuota N22 bisa dilewati dengan mendaftarkan ukuran kecil.
//  3. Kuota 100 MB per task dihitung dari yang sudah terdaftar.
//  4. `storage_key` harus di bawah prefix org sendiri — tanpa itu, satu
//     worker bisa mendaftarkan objek milik tenant lain hanya dengan menyebut
//     key-nya, dan `artifacts` jadi jembatan lintas tenant.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"agentdeck/internal/storage"
)

// ---- fake -----------------------------------------------------------------

type fakeRepo struct {
	rows  []Artifact
	byID  map[string]Artifact
	next  int
	failS int64
}

func newFakeRepo() *fakeRepo { return &fakeRepo{byID: map[string]Artifact{}} }

func (r *fakeRepo) ListByTask(_ context.Context, orgID, taskID string) ([]Artifact, error) {
	out := []Artifact{}
	for _, a := range r.rows {
		if a.OrgID == orgID && a.TaskID == taskID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *fakeRepo) Get(_ context.Context, orgID, id string) (Artifact, error) {
	a, ok := r.byID[id]
	if !ok || a.OrgID != orgID {
		return Artifact{}, ErrNotFound
	}
	return a, nil
}

func (r *fakeRepo) Create(_ context.Context, a Artifact) (Artifact, error) {
	r.next++
	r.rows = append(r.rows, a)
	r.byID[a.ID] = a
	return a, nil
}

func (r *fakeRepo) SumTaskBytes(_ context.Context, orgID, taskID string) (int64, error) {
	var total int64
	for _, a := range r.rows {
		if a.OrgID == orgID && a.TaskID == taskID {
			total += a.Size
		}
	}
	return total + r.failS, nil
}

type fakeObjects struct {
	objects map[string][]byte
	// headOnly: objek ada menurut HEAD tapi isinya tidak bisa dibaca.
	headOnly map[string]int64
	puts     []string
}

func newFakeObjects() *fakeObjects {
	return &fakeObjects{objects: map[string][]byte{}, headOnly: map[string]int64{}}
}

func (o *fakeObjects) PresignPut(key, _ string, _ time.Duration) (string, error) {
	o.puts = append(o.puts, key)
	return "http://storage.test/" + key + "?X-Amz-Signature=abc", nil
}

func (o *fakeObjects) PresignGet(key string, _ time.Duration) (string, error) {
	return "http://storage.test/" + key + "?X-Amz-Signature=abc", nil
}

func (o *fakeObjects) Head(_ context.Context, key string) (storage.ObjectInfo, error) {
	if size, ok := o.headOnly[key]; ok {
		return storage.ObjectInfo{Size: size}, nil
	}
	if body, ok := o.objects[key]; ok {
		return storage.ObjectInfo{Size: int64(len(body))}, nil
	}
	return storage.ObjectInfo{}, storage.ErrNotFound
}

func (o *fakeObjects) Fetch(_ context.Context, key string, limit int64) ([]byte, error) {
	body, ok := o.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("objek melebihi batas")
	}
	return body, nil
}

type fakeTasks struct {
	known map[string]bool
}

func (t *fakeTasks) TaskExists(_ context.Context, orgID, taskID string) (bool, error) {
	return t.known[orgID+"/"+taskID], nil
}

func (t *fakeTasks) RunExists(_ context.Context, orgID, runID string) (bool, error) {
	return t.known[orgID+"/"+runID], nil
}

// ---- fixture --------------------------------------------------------------

type fixture struct {
	svc   *Service
	repo  *fakeRepo
	store *fakeObjects
	ids   []string
}

const (
	orgA  = "01ORGAAAAAAAAAAAAAAAAAAAA"
	orgB  = "01ORGBBBBBBBBBBBBBBBBBBBB"
	taskA = "01TASKAAAAAAAAAAAAAAAAAAA"
	runA  = "01RUNAAAAAAAAAAAAAAAAAAAA"
	runB  = "01RUNBBBBBBBBBBBBBBBBBBBB"
)

func newFixture() *fixture {
	f := &fixture{repo: newFakeRepo(), store: newFakeObjects()}
	f.ids = []string{"01ART00000000000000000001", "01ART00000000000000000002", "01ART00000000000000000003"}
	n := 0
	f.svc = New(Options{
		Repo: f.repo,
		Tasks: &fakeTasks{known: map[string]bool{
			orgA + "/" + taskA: true,
			orgA + "/" + runA:  true,
			orgB + "/" + runB:  true,
		}},
		Store: f.store,
		IDs: func() string {
			id := f.ids[n%len(f.ids)]
			n++
			return id
		},
		Now: func() time.Time { return time.Unix(1700000000, 0).UTC() },
	})
	return f
}

// upload menaruh objek di storage dan mengembalikan key + digest-nya.
func (f *fixture) upload(key string, body []byte) (string, string) {
	f.store.objects[key] = body
	return key, storage.SHA256Hex(body)
}

func validKey(id, name string) string {
	return storage.ArtifactKey(orgA, taskA, runA, id, name)
}

// ---- tes ------------------------------------------------------------------

func TestRegisterVerifiesSHA256AgainstTheObject(t *testing.T) {
	f := newFixture()
	key := validKey("01ART00000000000000000001", "laporan.txt")
	f.upload(key, []byte("isi sebenarnya"))

	// Digest milik isi LAIN: klien salah, dan itu harus ketahuan.
	wrong := storage.SHA256Hex([]byte("isi yang lain"))
	_, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename:   "laporan.txt",
		Size:       14,
		SHA256:     wrong,
		StorageKey: key,
		RunID:      runA,
	})
	if !errors.Is(err, ErrSHA256Match) {
		t.Fatalf("Register dengan digest salah = %v, want ErrSHA256Match", err)
	}
}

func TestRegisterAcceptsTheCorrectDigest(t *testing.T) {
	f := newFixture()
	body := []byte("isi sebenarnya")
	key := validKey("01ART00000000000000000001", "laporan.txt")
	_, digest := f.upload(key, body)

	got, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename:   "laporan.txt",
		Size:       int64(len(body)),
		SHA256:     strings.ToUpper(digest),
		StorageKey: key,
		RunID:      runA,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Digest dinormalkan ke huruf kecil: kolomnya CHECK(char_length = 64) dan
	// dua penulisan berbeda huruf untuk isi yang sama akan memecah pencarian.
	if got.SHA256 != digest {
		t.Fatalf("sha256 = %q, want %q", got.SHA256, digest)
	}
	if got.Size != int64(len(body)) {
		t.Fatalf("size = %d, want %d", got.Size, len(body))
	}
}

func TestRegisterRejectsSizeThatStorageDisagreesWith(t *testing.T) {
	f := newFixture()
	key := validKey("01ART00000000000000000001", "bohong.txt")
	f.upload(key, []byte("cuma 8 byte"))

	// Mendaftarkan 1 MB untuk objek 8 byte: kuota N22 bisa dilewati dengan
	// melaporkan ukuran besar; sebaliknya, mendaftar kecil lalu menyimpan besar
	// juga. Keduanya ditolak karena ukurannya dibandingkan dengan HEAD.
	_, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename:   "bohong.txt",
		Size:       1024 * 1024,
		SHA256:     storage.SHA256Hex([]byte("cuma 8 byte")),
		StorageKey: key,
		RunID:      runA,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register dengan size tidak cocok = %v, want ErrInvalidInput", err)
	}
}

func TestRegisterRefusesAnObjectThatIsNotThere(t *testing.T) {
	f := newFixture()
	key := validKey("01ART00000000000000000001", "hantu.txt")

	_, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename:   "hantu.txt",
		Size:       10,
		SHA256:     strings.Repeat("a", 64),
		StorageKey: key,
		RunID:      runA,
	})
	if !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("Register objek hilang = %v, want ErrObjectMissing", err)
	}
}

func TestRegisterRefusesAKeyFromAnotherOrg(t *testing.T) {
	f := newFixture()
	// Key milik org B, TAPI dengan segmen run yang benar untuk permintaan ini.
	//
	// Bentuk ini yang penting: kalau segmen run-nya salah, pemeriksaan
	// konsistensi run yang menangkapnya dan pemeriksaan prefix org tidak pernah
	// diuji — persis bagaimana mutant "prefix tidak diperiksa" bisa lolos.
	key := storage.ArtifactKey(orgB, taskA, runA, "01ART00000000000000000009", "milik-b.txt")
	f.upload(key, []byte("rahasia org B"))

	_, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename:   "milik-b.txt",
		Size:       int64(len("rahasia org B")),
		SHA256:     storage.SHA256Hex([]byte("rahasia org B")),
		StorageKey: key,
		RunID:      runA,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register key tenant lain = %v, want ErrInvalidInput", err)
	}

	// Kasus kedua: key dengan org benar tapi task lain. Prefix-nya harus
	// mencakup task juga, bukan hanya org.
	otherTask := storage.ArtifactKey(orgA, "01TASKLAINXXXXXXXXXXXXXXX", runA, "01ART00000000000000000010", "lain.txt")
	f.upload(otherTask, []byte("task lain"))
	if _, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename: "lain.txt", Size: int64(len("task lain")),
		SHA256: storage.SHA256Hex([]byte("task lain")), StorageKey: otherTask, RunID: runA,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register key task lain = %v, want ErrInvalidInput", err)
	}
}

func TestRegisterRefusesMalformedDigest(t *testing.T) {
	f := newFixture()
	key := validKey("01ART00000000000000000001", "x.txt")
	f.upload(key, []byte("x"))

	for _, bad := range []string{"", "abc", strings.Repeat("z", 64), strings.Repeat("a", 63)} {
		_, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
			Filename:   "x.txt",
			Size:       1,
			SHA256:     bad,
			StorageKey: key,
			RunID:      runA,
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("sha256 %q = %v, want ErrInvalidInput", bad, err)
		}
	}
}

func TestUploadURLRefusesAFileOver25MB(t *testing.T) {
	f := newFixture()
	_, err := f.svc.UploadURL(context.Background(), orgA, taskA, runA, "besar.bin", "", MaxFileSize+1)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("UploadURL > 25 MB = %v, want ErrTooLarge", err)
	}
	// Tepat di batas harus diterima — batasnya inklusif, sama dengan
	// artifacts_size_chk (<= 26214400).
	if _, err := f.svc.UploadURL(context.Background(), orgA, taskA, runA, "pas.bin", "", MaxFileSize); err != nil {
		t.Fatalf("UploadURL tepat 25 MB = %v, want sukses", err)
	}
}

func TestUploadURLRefusesWhenTaskQuotaWouldBeExceeded(t *testing.T) {
	f := newFixture()
	// Sudah terpakai 95 MB; menambah 10 MB melewati 100 MB.
	f.repo.rows = append(f.repo.rows, Artifact{
		ID: "01ART00000000000000000099", OrgID: orgA, TaskID: taskA,
		Size: 95 * 1024 * 1024, StorageKey: "x", SHA256: strings.Repeat("a", 64),
	})
	_, err := f.svc.UploadURL(context.Background(), orgA, taskA, runA, "lagi.bin", "", 10*1024*1024)
	if !errors.Is(err, ErrTaskQuota) {
		t.Fatalf("UploadURL melewati kuota = %v, want ErrTaskQuota", err)
	}
}

func TestUploadURLPutsTheObjectUnderTheOrgPrefix(t *testing.T) {
	f := newFixture()
	// Nilainya tepat di bawah batas per file supaya lulus pemeriksaan ukuran;
	// yang diuji di sini bentuk key-nya, bukan ukurannya.
	ticket, err := f.svc.UploadURL(context.Background(), orgA, taskA, runA, "report.pdf", "application/pdf", 1024)
	if err != nil {
		t.Fatalf("UploadURL: %v", err)
	}
	// Bentuk key dari 3.15, dan prefix org wajib — itu yang bikin pemeriksaan
	// lintas tenant di Register punya arti.
	want := "artifacts/" + orgA + "/" + taskA + "/"
	if !strings.HasPrefix(ticket.StorageKey, want) {
		t.Fatalf("storage_key = %q, want berawalan %q", ticket.StorageKey, want)
	}
	if !strings.HasSuffix(ticket.StorageKey, "-report.pdf") {
		t.Fatalf("storage_key = %q, want berakhiran nama file", ticket.StorageKey)
	}
	if ticket.MaxFileSize != MaxFileSize {
		t.Fatalf("max_file_size = %d, want %d", ticket.MaxFileSize, MaxFileSize)
	}
	if ticket.ExpiresAt.Sub(f.svc.now()) != UploadURLTTL {
		t.Fatalf("TTL = %v, want %v", ticket.ExpiresAt.Sub(f.svc.now()), UploadURLTTL)
	}
}

func TestUploadURLRefusesAnUnknownTask(t *testing.T) {
	f := newFixture()
	_, err := f.svc.UploadURL(context.Background(), orgA, "01TASKTIDAKADAXXXXXXXXXXX", runA, "x.bin", "", 10)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("UploadURL task tidak ada = %v, want ErrTaskNotFound", err)
	}
}

func TestGetIsScopedToTheOrg(t *testing.T) {
	f := newFixture()
	key := validKey("01ART00000000000000000001", "a.txt")
	_, digest := f.upload(key, []byte("halo"))
	created, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename: "a.txt", Size: 4, SHA256: digest, StorageKey: key,
		RunID: runA,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := f.svc.Get(context.Background(), orgB, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get lintas org = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.Get(context.Background(), orgA, created.ID); err != nil {
		t.Fatalf("Get org sendiri = %v, want sukses", err)
	}
}

func TestDownloadURLUsesTheStoredKey(t *testing.T) {
	f := newFixture()
	key := validKey("01ART00000000000000000001", "a.txt")
	_, digest := f.upload(key, []byte("halo"))
	created, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename: "a.txt", Size: 4, SHA256: digest, StorageKey: key,
		RunID: runA,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	url, expires, err := f.svc.DownloadURL(context.Background(), orgA, created.ID)
	if err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}
	if !strings.Contains(url, key) {
		t.Fatalf("URL %q tidak menunjuk key %q", url, key)
	}
	if expires.Sub(f.svc.now()) != DownloadURLTTL {
		t.Fatalf("TTL unduh = %v, want %v", expires.Sub(f.svc.now()), DownloadURLTTL)
	}
}

func TestNilStorageMeansNotConfigured(t *testing.T) {
	svc := New(Options{Repo: newFakeRepo(), Tasks: &fakeTasks{known: map[string]bool{}}})
	if svc.Configured() {
		t.Fatal("Configured() = true padahal storage nil")
	}
	if _, err := svc.UploadURL(context.Background(), orgA, taskA, runA, "x", "", 1); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("UploadURL tanpa storage = %v, want ErrNotConfigured", err)
	}
}

func TestRegisterRefusesWhenTheKeyBelongsToAnotherRun(t *testing.T) {
	f := newFixture()
	// Key dibuat untuk run B, tapi didaftarkan sebagai run A. Barisnya akan
	// benar dan isinya salah — itu yang dicegah.
	key := storage.ArtifactKey(orgA, taskA, runB, "01ART00000000000000000001", "a.txt")
	_, digest := f.upload(key, []byte("halo"))
	_, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename: "a.txt", Size: 4, SHA256: digest, StorageKey: key, RunID: runA,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register key run lain = %v, want ErrInvalidInput", err)
	}
}

func TestRegisterRefusesAnUnknownRun(t *testing.T) {
	f := newFixture()
	key := storage.ArtifactKey(orgA, taskA, runA, "01ART00000000000000000001", "a.txt")
	_, digest := f.upload(key, []byte("halo"))
	_, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename: "a.txt", Size: 4, SHA256: digest, StorageKey: key,
		RunID: "01RUNTIDAKADAXXXXXXXXXXX",
	})
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("Register run tidak ada = %v, want ErrRunNotFound", err)
	}
	// run_id kosong juga ditolak, dengan pesan yang menyebutnya wajib.
	_, err = f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename: "a.txt", Size: 4, SHA256: digest, StorageKey: key,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register tanpa run_id = %v, want ErrInvalidInput", err)
	}
}

func TestUploadURLRequiresAKnownRun(t *testing.T) {
	f := newFixture()
	if _, err := f.svc.UploadURL(context.Background(), orgA, taskA, "01RUNTIDAKADAXXXXXXXXXXX", "x.bin", "", 10); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("UploadURL run tidak ada = %v, want ErrRunNotFound", err)
	}
	if _, err := f.svc.UploadURL(context.Background(), orgA, taskA, "", "x.bin", "", 10); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UploadURL tanpa run = %v, want ErrInvalidInput", err)
	}
}

func TestLargeObjectSkipsBodyVerificationButStillChecksSize(t *testing.T) {
	f := newFixture()
	// Objek di atas MaxVerifyBytes: HEAD melaporkan ukurannya, isinya tidak
	// dibaca ulang. Ukuran yang cocok tetap harus diterima.
	key := validKey("01ART00000000000000000001", "besar.bin")
	size := int64(MaxVerifyBytes + 1024)
	f.store.headOnly[key] = size

	got, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename: "besar.bin", Size: size, SHA256: strings.Repeat("a", 64), StorageKey: key,
		RunID: runA,
	})
	if err != nil {
		t.Fatalf("Register objek besar = %v, want sukses", err)
	}
	if got.Size != size {
		t.Fatalf("size = %d, want %d", got.Size, size)
	}

	// Ukuran yang tidak cocok tetap ditolak walau isinya tidak dibaca.
	if _, err := f.svc.Register(context.Background(), orgA, taskA, RegisterInput{
		Filename: "besar.bin", Size: size + 1, SHA256: strings.Repeat("a", 64), StorageKey: key,
		RunID: runA,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register ukuran tidak cocok = %v, want ErrInvalidInput", err)
	}
}

func TestListRefusesAnUnknownTask(t *testing.T) {
	f := newFixture()
	if _, err := f.svc.List(context.Background(), orgA, "01TASKTIDAKADAXXXXXXXXXXX"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("List task tidak ada = %v, want ErrTaskNotFound", err)
	}
}
