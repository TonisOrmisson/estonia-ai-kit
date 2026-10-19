# emta-cli

CLI for Estonian Tax and Customs Board (EMTA) e-services. Binary at `./emta-cli` (or build with `go build -o emta-cli .`).

## Using the CLI

### Login (interactive, requires user to scan QR code)

```sh
./emta-cli login
```

Session is stored in the OS keychain (encrypted). Expires after ~30 minutes.

### Logout

```sh
./emta-cli logout
```

### List TSD declarations

```sh
./emta-cli tsd list              # current year
./emta-cli tsd list --year 2025  # specific year
```

### Show TSD declaration summary (tax codes 110-119)

```sh
./emta-cli tsd show <declaration-id>
```

Get the declaration ID from `tsd list`.

### Export TSD XML

```sh
./emta-cli tsd xml export --declaration-id <id> --output tsd.xml
```

### Import TSD XML

```sh
./emta-cli tsd xml import --year 2026 --month 3 --input tsd.xml
./emta-cli tsd xml import --year 2026 --month 3 --input tsd.xml --with-sums
```

### Submit TSD draft

```sh
./emta-cli tsd submit --declaration-id <id> --confirm
```

## Notes

- Login is interactive (Smart-ID QR code) — ask the user to scan when running `login`
- If you get "session expired", the user needs to run `login` again
- Do not hardcode company/person names
- KMD is XML-only: `kmd xml build|import`, `kmd verify` (see README). Never upload CSV or hand-made files to KMD.
- A KMD draft counts as filled only when `kmd xml import` / `kmd verify` exits 0 — HTTP success is not proof
- TSD XML create/import uses the declarations page file-import workflow
- `kmd submit` / `tsd submit` require `EMTA_ALLOW_SUBMIT=1` set by the human operator; agents must not set it
