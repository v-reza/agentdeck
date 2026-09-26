@echo off
REM Gate ringan buat /goal overnight. Exit 0 = hijau.
REM
REM Kenapa BUKAN tools/gate.cmd: file itu milik sesi lain (untracked) dan isinya
REM suite penuh yang terukur 670 detik — di atas plafon gate 300s
REM (hermes_cli/goals.py DEFAULT_GATE_TIMEOUT_SECONDS, tidak ada config key-nya).
REM Gate yang lewat plafon mati di tengah dan dilaporkan sebagai gagal.
REM
REM Yang sengaja TIDAK ada di sini, dan alasannya:
REM   - go test ./... : 490 detik. Dijalankan per fase oleh agent, bukan gate.
REM   - go vet ./...  : 83 detik cache kosong. Ikut sesi go test per fase.
REM   - prettier --check . : merah karena frontend/e2e/demo/measure-aksi.mjs
REM     (file sesi lain, bukan milik run ini). Scope dipersempit ke src/.
REM
REM PATH di-set eksplisit: cmd.exe mewarisi environment Hermes, dan go/node
REM tidak ada di sana. Tanpa ini gate gagal dengan "'go' is not recognized"
REM dan gagal selamanya.
setlocal
set "PATH=C:\Program Files\Go\bin;C:\Program Files\nodejs;%PATH%"
cd /d "%~dp0.."

for /f "delims=" %%i in ('gofmt -l .') do (
  echo GAGAL: gofmt belum bersih: %%i
  exit /b 1
)
go build ./... || exit /b 1

cd frontend || exit /b 1
node node_modules/typescript/bin/tsc -b || exit /b 1
node node_modules/prettier/bin/prettier.cjs --check src/ || exit /b 1
node node_modules/vitest/vitest.mjs run || exit /b 1

exit /b 0
