#!/usr/bin/env bash
# Checks a commit must pass before it is deployed (run by .forgejo/workflows/*).
# Expects go, node and pnpm on PATH and is run from the repository root.
#
# GO_TEST_ADVISORY=1 runs `go test` but only warns when it fails, so deploys are
# not blocked by the tests that were red when this gate was added. Set it to 0
# in the workflows once the Go suite is green to make the tests blocking.
set -euo pipefail

GO_TAGS=${GO_TAGS:-nosqlite}
GO_TEST_ADVISORY=${GO_TEST_ADVISORY:-0}

step() { printf '\n==> %s\n' "$*"; }

step "go build"
go build -tags="$GO_TAGS" ./...

step "go vet"
go vet -tags="$GO_TAGS" ./...

step "pnpm install"
pnpm --dir web install --frozen-lockfile

step "tsc --noEmit"
pnpm --dir web exec tsc --noEmit

step "web file-selection tests"
pnpm --dir web run test:files

step "go test"
if go test -tags="$GO_TAGS" ./...; then
	:
elif [ "$GO_TEST_ADVISORY" = 1 ]; then
	echo "::warning::go test failed; not blocking the deploy because GO_TEST_ADVISORY=1"
else
	exit 1
fi
