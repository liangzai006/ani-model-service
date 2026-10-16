# Task 1 test gap fixes

## Changes

- Added a catalog fake dedicated to `GetModelVersion` metadata tests that records the tenant and model ID passed to `GetModel`.
- Added regression coverage that requires the principal tenant and owning `v.ModelID` to reach the catalog.
- Added catalog error mapping coverage for `pgx.ErrNoRows` (`MODEL_NOT_FOUND`/404) and generic failures (`MODEL_STORE_ERROR`/500).

## TDD and verification

- Red phase: `go test ./internal/service -run 'TestGetModelVersion(PassesPrincipalTenantAndOwningModelIDToCatalog|MapsCatalogErrors)' -count=1` failed because the fake did not record arguments or inject errors.
- Focused green phase: `go test ./internal/service -run 'TestGetModelVersion(PassesPrincipalTenantAndOwningModelIDToCatalog|MapsCatalogErrors|ReturnsModelMetadata)' -count=1` passed.
- Package regression: `go test ./internal/service -count=1` passed.
- Repository verification: `make verify` passed.
- `git diff --check` passed.
