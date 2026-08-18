# atl — a unified Atlassian CLI

One binary for Jira + Confluence (+ Bitbucket later). Built because every existing
option fails on one of three axes: secure credential storage, safe partial edits, or
being free/open. This doc is the design of record; it captures the decisions behind the
tool.

Repo: `atlassian-cli`. Module path: `github.com/branow/atlassian-cli`. Binary: `atl`.

## Why build instead of adopt

None of the field clears the bar:

- **pchuri/confluence-cli** — stores the API token in plaintext
  (`~/.config/confluence-cli/config.json`, file-perms only), full-body page overwrite,
  weak inline comments.
- **atlassian-cli (atlassiancli.com)** — the only one that nails keychain storage, but
  **proprietary and paid** ($9–15/mo/seat), a closed binary handling our tokens, and
  still REST-bound so it inherits the limits below.
- **chinmaymk/acli, confluence-poster, official Atlassian acli** — public-REST +
  full-overwrite, same ceiling; acli in particular is heavily limited on the Jira side.

Two pains are **platform limits, not tool bugs**, so switching tools can't fix them —
only a smarter client can work around them:

1. **No partial page update.** The Confluence Cloud API only accepts full-body
   replacement (open Atlassian bugs: atlassian-mcp-server#106, #210, ROVO-366). "Change
   one section" only exists **client-side**: GET storage body → patch target region →
   PUT with the current version number.
2. **Inline comments need editor metadata** the public v1 API won't give
   (`matchIndex`, `serializedHighlights`). v2's `/inline-comments` takes just
   `textSelection` + match index and covers most cases; the rest needs internal editor
   endpoints.

Decision: **build our own**. Do not bind ourselves to the public REST API — where a
capability exists only on an internal/undocumented endpoint, use it behind
`--experimental` with a printed warning rather than refusing.

## How the API surface is obtained: a catalog distilled from the specs

**Atlassian publishes OpenAPI specs** (`specs/`, see `SOURCES.md`), so a build script
distills operationId → {method, path, params, product} straight from them into an
embedded catalog. That catalog backs a generic `atl api <operation> -f k=v` dispatcher,
the same escape-hatch shape as `gh api`.

- Catalog build: `scripts/build-catalog` (a Go program) reads `specs/*.json` and emits
  `internal/atlapi/catalog/atlas-catalog.json` (embedded via `go:embed`).
- Generic escape hatch: `atl api <operationId> -f k=v` (also namespaced views like
  `atl jira api` / `atl confluence api`), the equivalent of `gh api`.
- Refresh: `make specs` re-pulls specs; `make catalog` rebuilds the embedded catalog.

Spec sizes: Jira ~420 paths, Jira Software ~78, Confluence v1 ~89 / v2 ~151, Bitbucket
~193. The distilled catalog currently carries ~1,000 operations across jira,
jira-software, confluence-v1, and confluence-v2.

## The split (the whole point)

The catalog gives the boring 90% (every operation, reachable generically). It does
**not** give the value, and the features we care about aren't in the specs at all. So
on top of the generic layer we hand-write curated commands:

- keychain-backed auth (one-time token in, OS keychain out)
- the client-side page-**patch engine** (heading / anchor / regex, version-aware)
- inline comments (v2 public first, internal editor endpoints behind `--experimental`)
- curated UX: a focused set of good commands, not 1,000 generic ones

## Repo shape

```
atl/
  main.go                          thin entry: cmd.Execute() -> exit code
  cmd/                             cobra tree + Factory DI (root, auth_*, config, api, version, completion)
  internal/
    cmdutil/                       Factory, typed errors, exit codes
    config/                        non-secret profiles (site, output); flag>env>file>default
    credentials/                   Store iface: keyring + plaintext fallback (never in config)
    iostreams/                     color / tty / no-input
    output/                        json + table renderers
    atlapi/                        HTTP client + retry + response parsing
      catalog/                     embedded catalog (atlas-catalog.json) + Lookup
    confluence/                    curated commands + value logic (patch engine, comments)
    jira/                          curated commands
  specs/                           Atlassian OpenAPI specs (catalog input) + SOURCES.md
  scripts/                         build-catalog (Go), install.sh
  .goreleaser.yml                  multi-platform release (brew/scoop/deb/rpm)
  Makefile                         specs · catalog · build · test
```

Module path `github.com/branow/atlassian-cli`; the binary built from `main.go` is `atl`.

Core packages: `cmdutil` (Factory pattern), `config`, `credentials` (keyring +
fallback), `iostreams`, `output`, the cobra/root wiring, and the generic `api` command.
Deps: `spf13/cobra`, `zalando/go-keyring`, `golang.org/x/term`, `gopkg.in/yaml.v3`.

## Auth model

`atl auth login` prompts once for **site + email + API token** and stores the secret in
the OS keychain (`zalando/go-keyring`: macOS Keychain / libsecret / Windows Credential
Manager), with a warned plaintext fallback (0600) only when no keychain backend exists.
**No plaintext token in the config file.**

Credentials are `{Site, Email, APIToken}`. Auth is HTTP Basic `email:APIToken` per
Atlassian Cloud. The same token serves both Jira and Confluence on a site, so one login
covers both product trees. Bitbucket uses its own auth (OAuth / app password) — handled
later.

Config (non-secret, `internal/config`) holds named profiles: `site`, `email`, default
`output`, active profile. Precedence flag > `ATL_*` env > file > default. The config
file lives at `~/.config/atl/config.yml` (`%AppData%\atl\config.yml` on Windows),
following the `gh`-style `~/.config` convention.

## Roadmap

1. **Skeleton** — package structure; `make specs`/`make catalog`/`build`,
   keychain auth, http client, output, generic `atl api`.
2. **Confluence** (proving ground) — `space ls`, `page get/create/delete`, the
   **patch engine** (`page patch --heading|--anchor|--regex`, version-aware),
   `search` (CQL), `attach`, `comment add --inline`.
3. **Jira** — mostly curating commands on the same catalog + core; add Jira Software
   (agile) and some `--experimental`.
4. **Bitbucket** — later; its own auth.

## v1 command set

```
atl auth login|logout|status|switch
atl config get|set|list
atl api <operationId> -f k=v            # generic escape hatch (gh api style)
atl confluence space ls
atl confluence page get|create|patch|delete
atl confluence search <cql>
atl confluence attach <pageId> <file>
atl confluence comment add <pageId> [--inline --select <text>]
atl jira issue get|list|create|edit|transition|comment|attach
atl jira project ls
atl jira board ls
atl jira sprint ls|get
```
