# atlassian-cli

One command-line interface for Atlassian Cloud — Jira and Confluence today,
Bitbucket later.

`atl` gives Jira and Confluence a single, scriptable terminal front-end:
curated commands for the things you do every day (issues, pages, spaces,
search, comments) plus a generic `atl api` escape hatch that can invoke any
operation in Atlassian's published REST APIs by name.

- Credentials live in the OS keychain (macOS Keychain, Windows Credential
  Manager, Linux Secret Service), never in a plaintext config file.
- `page patch` edits one section of a Confluence page safely — it reads the
  current body and version, rewrites just the region you target, and writes
  back with the next version number, instead of overwriting the whole page.
- JSON output, meaningful exit codes, and non-interactive flags make it
  scriptable; shell completion covers commands and operation names.
- Free and open source (MIT).

## Why

Existing options each fall short on one of three axes — secure credential
storage, safe partial edits, or being free and open:

- **Secure credentials.** The token is stored in the OS keychain via a
  keyring, with a warned, file-permission-locked plaintext fallback only when
  no keychain backend exists. It never lands in the config file.
- **Safe partial page edits.** The Confluence Cloud API only accepts a
  full-body replacement, so a "change one section" edit has to be done
  client-side. `atl confluence page patch` does exactly that — version-aware,
  targeting a heading, a named anchor, or a regex — so concurrent edits and
  the rest of the page survive.
- **Free and open.** No per-seat license, no closed binary handling your
  tokens.

## Installation

**Homebrew** (macOS and Linux):

```sh
brew install branow/tap/atl
```

**Scoop** (Windows):

```powershell
scoop bucket add branow https://github.com/branow/scoop-bucket
scoop install atl
```

**Linux packages**: `deb`, `rpm`, and `apk` packages are attached to the
[latest release](https://github.com/branow/atlassian-cli/releases/latest), e.g.:

```sh
sudo dpkg -i atl_*_linux_amd64.deb
```

**Shell script** (Linux and macOS; installs to `/usr/local/bin`, or
`~/.local/bin` when that is not writable):

```sh
curl -fsSL https://raw.githubusercontent.com/branow/atlassian-cli/main/scripts/install.sh | sh
```

**Manual**: download the archive for your platform (macOS, Linux, Windows;
amd64 and arm64) from the
[latest release](https://github.com/branow/atlassian-cli/releases/latest),
unpack it, and put the `atl` binary on your `PATH`:

```sh
tar xzf atl_*_darwin_arm64.tar.gz   # .zip on Windows
sudo mv atl /usr/local/bin/
```

On macOS, archives downloaded with a browser are quarantined and Gatekeeper
will refuse to run the unsigned binary; clear it with
`xattr -d com.apple.quarantine /usr/local/bin/atl`. Homebrew and the shell
script are not affected.

**Go** 1.25+:

```sh
go install github.com/branow/atlassian-cli@latest   # installs as "atlassian-cli"
```

or build from a checkout with `go build -o atl .`

## Quick start

```sh
# Log in: prompts for site, email, and API token, then verifies the
# credentials against the API before storing the token in the OS keychain.
atl auth login

# Jira: fetch an issue, then search with JQL
atl jira issue get PROJ-123
atl jira issue list 'project = PROJ AND status = "In Progress"'

# Confluence: list spaces, create a page, then patch one section of it
atl confluence space ls
atl confluence page create --space DS --title "Notes" --body-file notes.xml
atl confluence page patch 12345 --heading "Status" --content status.xml
```

Create an API token at
<https://id.atlassian.com/manage-profile/security/api-tokens>. One token
serves both Jira and Confluence on a site, so a single login covers both.

## Usage

Commands follow a noun-verb shape (`atl <group> <noun> <verb>`), like
`gh`/`stripe`.

### Authentication and configuration

```sh
atl auth login                    # site + email + API token -> OS keychain
atl auth status                   # who am I, which profile
atl auth switch --profile sandbox # change the active profile
atl auth logout

atl config list                   # all configured profiles
atl config set output json        # per-profile defaults (output, site, email)
```

Each profile stores a site, email, and preferred output format; the token
stays in the keychain. Settings resolve as flags > environment variables
(`ATL_PROFILE`, `ATL_SITE`, `ATL_EMAIL`, `ATL_OUTPUT`) > config file
(`~/.config/atl/config.yml`; `%AppData%\atl\config.yml` on Windows) >
defaults.

### Jira

```sh
atl jira issue get PROJ-123
atl jira issue list 'assignee = currentUser() AND resolution = Unresolved'
atl jira issue create --project PROJ --type Task --summary "Do the thing"
atl jira issue edit PROJ-123 -f priority=High
atl jira issue transition PROJ-123 --to "In Progress"
atl jira issue comment PROJ-123 --body "on it"

atl jira project ls
atl jira board ls --project PROJ
atl jira sprint ls --board <boardId>
atl jira sprint get <sprintId>
```

### Confluence

```sh
atl confluence space ls
atl confluence page get 12345
atl confluence page create --space DS --title "Notes" --body-file notes.xml
atl confluence page patch 12345 --anchor release-notes --content notes.xml
atl confluence page delete 12345
atl confluence search 'space = DS and type = page'
atl confluence attach 12345 ./diagram.png
atl confluence comment add 12345 --body "looks good"
```

`page patch` takes exactly one selector — `--heading <text>`,
`--anchor <name>`, or `--regex <pattern>` (with `--replacement`) — and
`--dry-run` prints the resulting body instead of saving it.

## Scripting

Add `-o json` to any command for machine-readable output, and rely on exit
codes. List commands (`jira issue list`, `jira project ls`, `board ls`,
`sprint ls`, `confluence space ls`, `confluence search`) take `--limit` and
`--paginate` — by default only the first page is returned; `--paginate`
follows every page until exhausted or `--limit` is reached.

Log in non-interactively for CI by reading the token from stdin:

```sh
echo "$ATL_API_TOKEN" | atl auth login \
  --site your-org.atlassian.net --email you@example.com --token-stdin
```

### Calling any API operation

Beyond the curated commands, `atl api` invokes any operation in the embedded
catalog by its operationId — the equivalent of `gh api`:

```sh
atl api --list                         # every catalogued operationId
atl api getIssue --describe            # its product, method, path, and fields
atl api getIssue -f issueIdOrKey=PROJ-1
atl jira api createIssue --input issue.json
atl confluence api getPageById -f id=12345
```

`--describe` documents an operation without any credentials or network —
product, HTTP method, path, and its path/query/body/response fields, each
with its type and (for request fields) whether it is required, with nested
component types expanded inline. Add `-o json` for a machine-readable shape.

`-f/--field key=value` fields become query parameters for GET/DELETE
operations and the JSON request body otherwise; values that look like JSON
are typed (`42` a number, `true` a boolean, `[1,2]` an array), everything
else is a string. `--input file.json` (or `--input -` for stdin) sends a full
JSON object for nested bodies. Some operationIds exist in more than one
product (e.g. `getIssue` in both Jira and Jira Software); scope them with the
namespaced `atl jira api` / `atl confluence api`, or pin with `--product`.

| Exit code | Meaning |
|---|---|
| 0 | success |
| 1 | generic failure, including business errors reported by the API |
| 2 | cancelled by the user |
| 3 | validation error (bad flags, unknown/unsupported operation) |
| 4 | authentication failure (not logged in, HTTP 401/403) |
| 5 | not found (HTTP 404) |
| 6 | rate-limited or unavailable (HTTP 429, 5xx after retries) |

## Developing

```sh
make catalog     # rebuild the embedded API catalog from specs/*.json
go build ./...
go vet ./...
go test ./...
```

Architecture, command conventions, and design decisions are described in
[`docs/DESIGN.md`](docs/DESIGN.md). The embedded operation catalog
([`atlas-catalog.json`](internal/atlapi/catalog/atlas-catalog.json)) is
distilled from Atlassian's published OpenAPI specs by the `scripts/build-catalog`
program (`make catalog`; `make specs` re-pulls the specs listed in
[`specs/SOURCES.md`](specs/SOURCES.md)) — treat it as a build artifact, not
something to edit by hand.

## License

[MIT](LICENSE)
