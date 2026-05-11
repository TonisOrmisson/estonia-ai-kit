# PR Summary

## Title

Add EMTA KMD draft tooling and harden EMTA/LHV session persistence

## Summary

This PR adds KMD support to `emta-cli` so agents can list declarations, read draft data, update KMD main form fields, read/update `INF A` and `INF B`, delete annex rows, and use a separate guarded submit command.  
It also improves EMTA login/session reuse for `customer-kmd2` and adds file-based fallback session storage for both EMTA and LHV so the tools work reliably in WSL/Linux environments without a working secrets service.

## Main Changes

- added `emta-cli kmd list`
- added `emta-cli kmd main read|create|update`
- added `emta-cli kmd inf-a read|update|delete`
- added `emta-cli kmd inf-b read|update|delete`
- added guarded `emta-cli kmd submit --confirm`
- implemented KMD HTML/Wicket parsing and request handling in `cli/emta/api/kmd.go`
- added JSON examples for `kmd main`, `inf-a`, and `inf-b`
- improved EMTA auth flow for current `govsso` login path
- improved EMTA session handling for `customer-kmd2`, WFM principal selection, and tab-specific KMD reads
- added file-session fallback for EMTA and LHV when keyring/secrets is unavailable
- added tests for auth/session handling and KMD parser/merge behavior
- sanitized tests/examples so no real private data is stored in the repo
- added repo instruction file forbidding real private data in tracked files/history

## Validation

- `go test ./...` passes in `cli/emta`
- `go test ./internal/config` passes in `cli/lhv`
- live-tested against EMTA draft workflow for KMD main form and annexes
- live-tested EMTA submit flow up to successful declaration submission

## Notes

- this PR is intentionally agent-oriented and uses JSON-first command IO by default
- submit remains a separate explicit command because users may want to review saved drafts before final submission
