# EMTA XML Workflow Design

## Context

The current `cli/emta` tool has two different maturity levels:

- `tsd` supports listing declarations and reading the summary view (`110-119` lines).
- `kmd` supports browser-form-style read/update/submit flows implemented against Wicket/HTML endpoints.

For upstreamable long-term maintenance, we do not want to keep building declaration editing around form-field automation. The desired direction is XML-centric:

- download the declaration XML from EMTA
- edit the XML outside the UI
- create or update a draft through XML upload
- submit only after explicit confirmation

The UI/browser flow should be used only as transport glue for locating, downloading, uploading, and submitting declaration files.

## Goal

Introduce a shared XML import/export workflow in `cli/emta`, implement it first for TSD, and later reuse the same engine for KMD.

Phase 1 scope:

- export existing TSD declaration XML
- create a new TSD draft from XML
- upload/import TSD XML into EMTA
- submit a TSD draft

Phase 2 scope:

- migrate KMD onto the same XML workflow primitives

## Non-goals

Not part of the first TSD XML branch:

- full KMD XML migration
- payroll/accounting interpretation of declaration rows
- generic XML schema validation beyond what EMTA itself enforces
- silent or automatic submit flows
- deleting declarations or bulk declaration lifecycle management

## User-facing CLI design

### TSD

New commands under `tsd xml`:

- `emta-cli tsd xml export --declaration-id <id> --output <file>`
- `emta-cli tsd xml import --year <yyyy> --month <m> --input <file>`
- `emta-cli tsd xml import --declaration-id <id> --input <file>`
- `emta-cli tsd submit --declaration-id <id> --confirm`

Behavior:

- `export` reads an existing declaration and saves the exact XML file payload.
- `import --year/--month` creates a new draft for the target period, uploads XML, then returns declaration metadata and EMTA messages.
- `import --declaration-id` uploads XML into an existing draft if the EMTA flow allows it.
- `submit` remains explicit and confirmation-gated.

### KMD

No user-facing KMD XML commands in the first implementation branch.

However, the new shared XML workflow must be shaped so that later KMD commands can look like:

- `emta-cli kmd xml export ...`
- `emta-cli kmd xml import ...`

## Architecture

### 1. Shared XML workflow layer

Add a new API layer responsible for file-based declaration transport, independent of TSD/KMD business semantics.

Responsibilities:

- open declaration page or draft-creation flow
- discover file download/upload actions from Wicket/HTML
- perform file download
- perform multipart upload/import
- parse redirect targets and success/error messages
- return normalized metadata plus raw response context

This layer should not know TSD tax codes or KMD form fields.

### 2. Declaration-specific adapters

Add declaration adapters on top of the XML workflow:

- `TSDAdapter`
- later `KMDAdapter`

Responsibilities:

- declaration-specific page navigation
- naming and resolving create/import/export actions
- declaration-specific result parsing
- stable CLI-facing response types

### 3. CLI command layer

Extend `cmd/tsd.go` and later `cmd/kmd.go` to expose the file workflow in stable commands.

The CLI should remain thin:

- parse flags
- load EMTA client/session
- call adapter methods
- print normalized JSON

## Proposed code structure

Inside `cli/emta`:

- `api/xml_workflow.go`
  Shared file download/upload/submit helpers and normalized result types.

- `api/tsd_xml.go`
  TSD-specific navigation, page parsing, and XML command implementation.

- `api/tsd_xml_test.go`
  TSD XML parser and workflow tests from saved fixtures.

- `cmd/tsd.go`
  New `tsd xml ...` subcommands and integration with existing `tsd submit`.

Likely fixture area:

- `cli/emta/testdata/tsd/...`
  Saved HTML/Wicket fragments and sample XML files.

Later, when KMD is migrated:

- `api/kmd_xml.go`
- `api/kmd_xml_test.go`

## Request/response model

Shared workflow result shape should include:

- declaration id, when known
- page URL used
- action URL used
- file name, when exported
- messages extracted from EMTA
- raw redirect target, when relevant

TSD-specific methods can wrap this into clearer responses, but the shared transport result should preserve enough detail for debugging brittle Wicket flows.

## Workflow details

### Export existing TSD XML

Flow:

1. ensure TSD session/principal is active
2. open declaration by stable declaration id
3. locate XML export/download action from the declaration page
4. download bytes with the authenticated session
5. return file metadata and optionally save to disk in CLI layer

Key requirement:

- keep the downloaded bytes exact; do not normalize formatting on export

### Import XML into a new TSD draft

Flow:

1. navigate to TSD declarations list for year
2. create a new draft for target month/year
3. land on draft page
4. locate XML import/upload form action
5. upload XML as file payload
6. parse resulting page for errors, warnings, and stable declaration id

Key requirement:

- if EMTA rejects the XML, the CLI must return structured error messages instead of only raw HTML

### Import XML into an existing draft

Flow:

1. open draft by declaration id
2. locate the XML upload/import form
3. upload XML
4. parse and return validation messages

### Submit draft

Flow:

1. open declaration page
2. locate submit action/button
3. require explicit `--confirm`
4. submit and parse final status/messages

## Error handling

Hard failures:

- missing session
- missing declaration id
- upload/download form not found
- EMTA returns redirect loop or invalid Wicket response
- multipart upload rejected
- submit attempted without `--confirm`

Structured user-visible errors should preserve:

- operation name
- declaration id or period
- action URL if available
- extracted EMTA messages

Avoid generic “parse failed” messages when the HTML can yield a concrete failure reason.

## Testing strategy

### Unit tests

Add parser-level tests for:

- locating TSD XML export links/buttons
- locating TSD XML import form/actions
- parsing EMTA success/error messages
- parsing draft/declaration ids from resulting pages

### Fixture tests

Store representative HTML and Wicket responses captured from a real session and test against them offline.

Priority fixtures:

- TSD declarations list page
- TSD declaration detail page with XML export action
- TSD draft page with XML import form
- successful import result page
- validation error result page

### Manual smoke test

With a real EMTA session:

1. export one existing TSD XML
2. create a new draft from the same XML for a safe test period
3. confirm import messages
4. submit only when explicitly intended

## Security and safety

- never auto-submit without explicit confirmation
- do not log XML contents by default
- do not write sensitive XML to temp files unless explicitly requested
- preserve current keychain-backed session model

## Branching and upstreaming

Implementation should start from the existing `add-kmd` branch state, not from `main`.

This TSD/XML work should live on its own branch so it can become a separate upstream PR. After that, local downstream work can merge it with company-specific bookkeeping automation independently.

## Open questions resolved for this design

- Separate branch: yes
- Base branch: current `add-kmd`
- Editing method: XML files, not UI field automation
- Shared vs separate implementation: shared XML transport foundation, TSD first, KMD later
- TSD first-scope: export existing XML, import XML into draft/new draft, submit

## Recommended implementation order

1. add shared XML transport primitives
2. add TSD XML export
3. add TSD XML import into new draft
4. add TSD XML import into existing draft if supported by the same mechanism
5. add TSD submit via draft id
6. only after TSD is stable, refactor KMD onto the same file workflow
