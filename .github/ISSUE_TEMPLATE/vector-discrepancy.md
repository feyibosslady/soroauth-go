---
name: Vector discrepancy report
about: Report a discrepancy between generated golden vectors and the reference implementation
title: "[Vector Discrepancy] "
labels: 'vector-discrepancy, triage'
assignees: ''

---

**Please search existing issues first** to avoid duplicates. This template is for reporting disagreements between sorauth-generated vectors and the reference JS SDK vectors.

---

## Reported Discrepancy

### Vector Information
- **Vector name:** `<name from testdata/vectors/>`
- **Vector file:** `testdata/vectors/<name>.json`

### sorauth Version
- **Go version:** `go version <output of go version>`
- **soroauth version:** `<version from go.mod or "unstable/local"`>
- **Build:** `<commit SHA or "local build">`

### Reference SDK
- **JS SDK version:** `@stellar/stellar-sdk@<version>`
- **Reference implementation:** `@stellar/stellar-sdk@17.1.0` (exact pin in `testdata/gen/package.json`)

### The Discrepancy

**Expected (from JS reference):**
- `preimage_xdr`: `<base64>` _(copy from vector JSON)_
- `payload_hex`: `<hex>` _(copy from vector JSON)_
- `signed_entry_xdr`: `<base64>` _(copy from vector JSON)_

**Actual (from sorauth):**
- `preimage_xdr`: `<base64>` _(output of `soroauth explain-error` or manual inspection)_
- `payload_hex`: `<hex>` _(output of `soroauth explain-error` or manual inspection)_
- `signed_entry_xdr`: `<base64>` _(output of `soroauth explain-error` or manual inspection)_

### Command That Produced the Discrepancy
```
soroauth <command> [flags]
```
_Example: `soroauth sign --entry <b64> --valid-until <n> --network testnet --secret-env SEED`_

### Additional Context
- **Network:** testnet | public | rpc | futurenet
- **Any custom flags or options used:**
- **Error messages (if any):**
- **Reproduction steps:**

---

## Verification Steps

1. Regenerate the vector using the reference generator:
   ```bash
   npm ci && node testdata/gen/gen.mjs
   ```
2. Compare the output:
   ```bash
   git diff testdata/vectors/<name>.json
   ```
3. Run the golden test:
   ```bash
   go test -run Golden -v ./...
   ```

---

## Guidelines

- **Never edit a vector by hand.** Vectors must only be regenerated via `gen.mjs` with the exact `@stellar/stellar-sdk@17.1.0` pin.
- If a vector appears wrong, stop and report - do not modify the vector to make a test pass.
- Include both the `preimage_xdr`, `payload_hex`, and `signed_entry_xdr` fields in your report.
- Unrecognised or unknown error patterns will be triaged by maintainers.
- This template routes to maintainers via GitHub Security Advisories for critical signature-correctness bugs.

---

**Conflict check:** Do not create duplicate issues. Search existing issues first.