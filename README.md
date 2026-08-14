# atlassian-cli

A command-line interface for Jira and Confluence. `atl` gives both a single,
scriptable terminal front-end: curated commands for everyday work plus a
generic `atl api` escape hatch that can invoke any operation in Atlassian's
published REST APIs by name.

- **Secure credentials.** The API token lives in the OS keychain (macOS
  Keychain, Windows Credential Manager, Linux Secret Service), never in a
  plaintext config file — only a warned, `0600` fallback when no keychain
  exists.
- **Safe partial page edits.** Confluence's API only accepts full-body
  replacement, so `confluence page patch` does it client-side: read the body
  and version, rewrite just the region you target (a heading, anchor, or
  regex), write back with the next version number.
- **Scriptable.** JSON output, meaningful exit codes, non-interactive flags,
  pagination, and shell completion.
- **Free and open source** (MIT) — no per-seat license, no closed binary
  handling your tokens.

## Installation

```sh
brew install branow/tap/atl                              # macOS / Linux
scoop bucket add branow https://github.com/branow/scoop-bucket && scoop install atl   # Windows
go install github.com/branow/atlassian-cli@latest        # Go 1.25+ (installs as "atlassian-cli")
```

Or grab a binary (macOS/Linux/Windows, amd64/arm64) or a `deb`/`rpm`/`apk`
from the [latest release](https://github.com/branow/atlassian-cli/releases/latest),
or `curl -fsSL https://raw.githubusercontent.com/branow/atlassian-cli/main/scripts/install.sh | sh`.
On macOS, a browser-downloaded archive is quarantined — clear it with
`xattr -d com.apple.quarantine ./atl` (Homebrew and the install script aren't
affected).

## Quick start

```sh
atl auth login                    # prompts for site + email + API token, verifies, stores in keychain
atl jira issue get PROJ-123
atl confluence page patch 12345 --heading "Status" --content status.xml
```

Create an API token at
<https://id.atlassian.com/manage-profile/security/api-tokens>; one token
serves both Jira and Confluence on a site.

## Usage

Commands follow a `atl <group> <noun> <verb>` shape, like `gh`.

### Auth and config

```sh
atl auth login | status | switch --profile sandbox | logout
atl config list                   # all profiles (* marks active)
atl config set output json        # per-profile defaults
```

Each profile stores a site, email, and output format; the token stays in the
keychain. Settings resolve flags > env (`ATL_PROFILE`, `ATL_SITE`,
`ATL_EMAIL`, `ATL_OUTPUT`) > config file (`~/.config/atl/config.yml`) >
defaults.

### Jira

```sh
atl jira issue get PROJ-123
atl jira issue list 'assignee = currentUser() AND resolution = Unresolved'
atl jira issue create --project PROJ --type Task --summary "Do the thing"
atl jira issue edit PROJ-123 -f priority=High
atl jira issue transition PROJ-123 --to "In Progress"
atl jira issue comment PROJ-123 --body "on it"
atl jira issue comment PROJ-123 --markdown --body "shipped in [PR](https://x/42), @[Jane Doe] please verify"
atl jira issue prs PROJ-123       # linked Bitbucket pull requests
atl jira project ls
atl jira board ls --project PROJ
atl jira sprint ls --board <boardId>
```

### Confluence

```sh
atl confluence space ls
atl confluence page get 12345
atl confluence page create --space DS --title "Notes" --body-file notes.xml
atl confluence page patch 12345 --anchor release-notes --content notes.xml
atl confluence page delete 12345          # to trash; --purge deletes permanently
atl confluence search 'space = DS and type = page'
atl confluence attach 12345 ./diagram.png
atl confluence comment add 12345 --inline --select "the API" --body "which one?"
```

`page patch` takes exactly one selector — `--heading`, `--anchor`, or
`--regex` (with `--replacement`) — and `--dry-run` prints the result instead
of saving.

## Scripting

Add `-o json` to any command and rely on exit codes. List commands take
`--limit` and `--paginate` (first page only unless `--paginate`). Log in
non-interactively by reading the token from stdin:

```sh
echo "$ATL_API_TOKEN" | atl auth login --site your-org.atlassian.net --email you@example.com --token-stdin
```

### Calling any API operation

Beyond the curated commands, `atl api` invokes any catalogued operation by its
operationId — the equivalent of `gh api`:

```sh
atl api --list                          # every operationId
atl api getIssue --describe             # product, method, path, fields (no network)
atl api getIssue -f issueIdOrKey=PROJ-1
atl jira api createIssue --input issue.json
atl confluence api getPageById -f id=12345
```

`-f key=value` fields become query params for GET/DELETE and the JSON body
otherwise; JSON-looking values are typed. `--input file.json` (or `-` for
stdin) sends a full nested body. An operationId in more than one product
(e.g. `getIssue`) is scoped by the namespaced `atl jira api` / `atl confluence
api` or pinned with `--product`.

Exit codes: `0` success · `1` generic / API business error (including 5xx that
is not throttling, e.g. a suspended-site 503) · `2` cancelled · `3` validation
(bad flags, unknown op) · `4` auth failure (401/403) · `5` not found (404) ·
`6` rate-limited (429, or a 503 carrying a `Retry-After`).

## Developing

```sh
make catalog     # rebuild the embedded catalog from specs/*.json
go build ./... && go vet ./... && go test ./...
```

Design and conventions live in [`docs/DESIGN.md`](docs/DESIGN.md). The embedded
[`atlas-catalog.json`](internal/atlapi/catalog/atlas-catalog.json) is distilled
from Atlassian's OpenAPI specs by `scripts/build-catalog` — a build artifact,
not something to edit by hand.

## License

[MIT](LICENSE)
