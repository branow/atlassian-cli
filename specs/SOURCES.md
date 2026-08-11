# OpenAPI specs — sources & refresh

These are Atlassian's published specs. They are the input to codegen (`make gen`);
the generated Go clients are derived artifacts and are never hand-edited.

| File | Product | Download URL |
|------|---------|--------------|
| `jira-cloud.v3.json` | Jira Cloud platform REST v3 | https://developer.atlassian.com/cloud/jira/platform/swagger-v3.v3.json |
| `jira-software.v3.json` | Jira Software (agile/sprints/boards) | https://developer.atlassian.com/cloud/jira/software/swagger.v3.json |
| `confluence-cloud.v1.json` | Confluence Cloud REST v1 | https://developer.atlassian.com/cloud/confluence/swagger.v3.json |
| `confluence-cloud.v2.json` | Confluence Cloud REST v2 (preferred) | https://developer.atlassian.com/cloud/confluence/openapi-v2.v3.json |
| `bitbucket-cloud.json` | Bitbucket Cloud REST | https://api.bitbucket.org/swagger.json |

Refresh: re-run `make specs` (see repo Makefile) to re-pull all of the above.

## What the specs do NOT cover
The value features live at endpoints that are absent from these specs and must be
hand-written against the internal API the web editor uses, behind `--experimental`:
- inline comments with editor highlight metadata (serializedHighlights / matchIndex)
- anything the Confluence/Jira SPA does that never got a public REST surface
