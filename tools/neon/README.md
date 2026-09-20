# Neon policy for AgentDeck

`neon.ts` declares this project's branch policy (preview branches auto-expire
after 7 days; the default branch inherits project defaults).

## Why this lives in `tools/neon/`

The repo root is a Go module. `neon config init` scaffolds a `package.json`,
`node_modules/`, and `neon.ts` — none of which belong next to `go.mod`. Keeping
them here keeps the root clean.

## Running it

`neon deploy` walks **up** from the cwd looking for `neon.ts` and stops at the
first directory containing `.git`. It does not search subdirectories, so from
the repo root the config is invisible. Pass it explicitly:

```sh
neon deploy --config tools/neon/neon.ts --no-env-pull
neon config plan --config tools/neon/neon.ts
```

`--no-env-pull` matters: by default `neon deploy` writes `DATABASE_URL` into
`.env` as the **owner** role, which breaks the least-privilege split documented
in `../../.env.example` (runtime = restricted role). Re-apply the split by hand
after any pull, or just use `--no-env-pull`.

Running from inside this directory also works without `--config`, but the cwd
walk still stops at `.git` above it, so `--config` is the reliable form.
