# EMTA CLI

CLI for Estonian Tax and Customs Board (EMTA) e-services. Authenticates via Smart-ID QR code and provides access to TSD (Income and Social Tax Return) declarations plus KMD drafts via the official XML format.

> [!IMPORTANT]
> **Unofficial, open source & experimental.** This is a community-driven project, part of [Estonia AI Kit](https://github.com/stefanoamorelli/estonia-ai-kit). It is not affiliated with, endorsed by, or connected to the Estonian Tax and Customs Board (EMTA) or any Estonian government institution. It relies on undocumented web interfaces (HTML parsing, internal redirect chains) that **can break at any time** without notice. Sessions expire after ~30 minutes. Use at your own risk. The authors accept no liability for any issues arising from its use.

> [!WARNING]
> **Security considerations for agentic use.** If you use this CLI as a skill for an AI agent (e.g., via shell access), be aware that you are granting the agent access to an authenticated government session containing sensitive tax data. Always review agent actions before approving them, and understand the risks of prompt injection and tool misuse in agentic workflows. Read more about agentic AI security:
>
> - [OWASP Top 10 for LLM & Generative AI](https://owasp.org/www-project-top-10-for-large-language-model-applications/)
> - [OWASP Agentic AI Threats and Mitigations](https://genai.owasp.org/resource/agentic-ai-threats-and-mitigations/)
> - [MCP Security: Tool Poisoning Attacks (Invariant Labs)](https://invariantlabs.ai/blog/mcp-security-notification-tool-poisoning-attacks)

## Prerequisites

- [Go 1.21+](https://go.dev/dl/)
- Smart-ID account linked to your Estonian ID

## Install

```sh
go install github.com/stefanoamorelli/estonia-ai-kit/cli/emta@latest
```

Or build from source:

```sh
cd cli/emta
go build -o emta-cli .
```

## Usage

### Login

Authenticate via Smart-ID QR code. You'll be prompted to select a principal (company/person) after scanning.

```sh
./emta-cli login
```

Session is stored in the OS keychain (macOS Keychain, GNOME Keyring, Windows Credential Manager) and expires after ~30 minutes server-side.

To clear the saved session:

```sh
./emta-cli logout
```

### List TSD Declarations

```sh
./emta-cli tsd list              # current year
./emta-cli tsd list --year 2025  # specific year
```

### Show TSD Summary

```sh
./emta-cli tsd show <declaration-id>
```

Get the declaration ID from `tsd list`.

### TSD XML Export

```sh
./emta-cli tsd xml export --declaration-id <id> --output tsd.xml
```

### TSD XML Import Into New Draft

```sh
./emta-cli tsd xml import --year 2026 --month 3 --input tsd.xml
```

Optionally include appendices and INF1 together with tax sums:

```sh
./emta-cli tsd xml import --year 2026 --month 3 --input tsd.xml --with-sums
```

### Submit TSD Draft

```sh
./emta-cli tsd submit --declaration-id <id> --confirm
```

### KMD

KMD is written only through EMTA's official XML format (`vatdeclaration.xsd`, schema copy in `testdata/kmd/`). One declaration JSON is the source for the upload and for the live check:

```sh
./emta-cli kmd list
./emta-cli kmd create --year 2026 --month 9          # empty draft, prints declaration_id
./emta-cli kmd xml build --input decl.json            # offline: validate + print XML
./emta-cli kmd xml import --declaration-id <id> --input decl.json
./emta-cli kmd verify --declaration-id <id> --input decl.json
```

Example input: `./examples/kmd-declaration.json`. Main keys are KMD line numbers (`line_1`, `line_1_1`, `line_1_2`, `line_2`, `line_2_1`, `line_2_2`, `line_3`, `line_3_1`, `line_3_1_1`, `line_3_2`, `line_3_2_1`, `line_5`, `line_5_1`…`line_5_4`, `line_5_3_cars`, `line_5_4_cars`, `line_6`, `line_6_1`, `line_7`, `line_7_1`, `line_8`, `line_9`, `line_10`, `line_11`); unknown keys are rejected. `noSales`/`noPurchases` are derived from `inf_a`/`inf_b` (only rows at or above the INF threshold belong there).

`kmd xml import` fails (non-zero exit) when:

- the draft belongs to another taxpayer or period than the JSON says;
- EMTA lists the uploaded file under "Failide laadimisel tekkinud vead";
- the live main form, INF A or INF B read back from EMTA differs from the JSON.

Reads (`kmd main read`, `kmd inf-a read`, `kmd inf-b read`, `kmd verify`) always come live from the e-MTA form; there is no local cache. `kmd upload-errors --declaration-id <id>` lists EMTA's upload error table.

Submitting is deliberately hard: `kmd submit --declaration-id <id> --input decl.json --confirm` only runs when the operator has exported `EMTA_ALLOW_SUBMIT=1`, and only after a passing live verify. `tsd submit` uses the same `EMTA_ALLOW_SUBMIT=1` guard.

## How It Works

1. **Authentication**: Smart-ID QR code login via TARA (Estonia's national authentication service), following the full OIDC redirect chain through `tara.ria.ee` back to `maasikas.emta.ee`.

2. **Session persistence**: Access token and HTTP cookies are stored in the OS keychain (encrypted by default). This avoids writing sensitive credentials to plain text files on disk. Subsequent commands retrieve the session from the keychain without re-authenticating.

3. **TSD data**: The TSD application is server-rendered HTML. Data is extracted by parsing the HTML response since no JSON API is available.

4. **Principal selection**: When accessing TSD, the EMTA portal requires selecting a represented person/company via the WFM (Workflow Manager) subsystem.

## License

AGPL-3.0. See [LICENSE](./LICENSE).

Copyright (c) 2025 Stefano Amorelli
