# colophon — common dev + release recipes. Run `just` (or `just --list`) to see them.

# release tags are clean semver vX.Y.Z; pushing one fires build-release.yml.
semver_re := "^v[0-9]+\\.[0-9]+\\.[0-9]+$"

# list recipes
default:
    @just --list

# build ./colophon with the version baked in (mirrors the CI ldflags)
build:
    #!/usr/bin/env bash
    set -euo pipefail
    go build -trimpath \
      -ldflags "-s -w -X github.com/jmylchreest/colophon/internal/cli.version=$(just get-version)" \
      -o colophon ./cmd/colophon
    echo "built ./colophon ($(./colophon --version))"

# run the Go test suite (main module + the search submodule), race-enabled
test:
    go test -race ./...
    cd search && go test ./...

# run the JavaScript reader's parity tests against the emitted fixture
test-js:
    cd search && go test ./... >/dev/null && node --test search.test.mjs

# vet + golangci-lint
lint:
    go vet ./...
    golangci-lint run ./...

# gofmt the tree
fmt:
    gofmt -w .

# --- versioning / release ------------------------------------------------------
# Release tags are clean semver vX.Y.Z (no prerelease). `bump*` creates the tag
# locally; `release` pushes it, which triggers .github/workflows/build-release.yml
# (cross-platform build + GitHub release). A push to main without a tag publishes a
# -SNAPSHOT prerelease.

# the latest release tag (vX.Y.Z), or empty if there are none
get-latest-release:
    @git tag --list 'v*' | grep -E '{{semver_re}}' | sort -V | tail -1

# the current version: the exact tag if HEAD is tagged, else a next-patch snapshot
get-version:
    #!/usr/bin/env bash
    set -euo pipefail
    latest=$(git tag --list 'v*' | grep -E '{{semver_re}}' | sort -V | tail -1 || true)
    if [ -z "$latest" ]; then echo "v0.0.0-dev"; exit 0; fi
    if [ "$(git rev-parse HEAD)" = "$(git rev-parse "$latest^{commit}")" ]; then
      echo "$latest"
    else
      next=$(echo "$latest" | sed 's/^v//' | awk -F. '{print "v"$1"."$2"."$3+1}')
      echo "${next}-$(git rev-parse --short HEAD)-SNAPSHOT"
    fi

# bump the patch version and tag it (alias for bump-patch)
bump force="": (bump-patch force)

# create the next patch/minor/major tag (pass force=1 to re-tag the current commit)
bump-patch force="": (_bump "patch" force)
bump-minor force="": (_bump "minor" force)
bump-major force="": (_bump "major" force)

_bump kind force="":
    #!/usr/bin/env bash
    set -euo pipefail
    latest=$(git tag --list 'v*' | grep -E '{{semver_re}}' | sort -V | tail -1 || true)
    if [ -z "$latest" ]; then
      case "{{kind}}" in patch) next=v0.0.1;; minor) next=v0.1.0;; major) next=v1.0.0;; esac
      echo "No release tags yet — creating $next"
    else
      if [ "$(git rev-parse HEAD)" = "$(git rev-parse "$latest^{commit}")" ] && [ -z "{{force}}" ]; then
        echo "HEAD is already tagged $latest — nothing to release (pass force=1 to re-tag)." >&2
        exit 1
      fi
      read -r ma mi pa < <(echo "$latest" | sed 's/^v//' | awk -F. '{print $1, $2, $3}')
      case "{{kind}}" in
        patch) next="v${ma}.${mi}.$((pa+1))";;
        minor) next="v${ma}.$((mi+1)).0";;
        major) next="v$((ma+1)).0.0";;
      esac
      echo "Bumping $latest -> $next"
    fi
    git tag -a "$next" -m "Release $next"
    echo "Tagged $next. Push it with: just release"

# push main and the latest release tag to origin → triggers the CI release
release:
    #!/usr/bin/env bash
    set -euo pipefail
    latest=$(git tag --list 'v*' | grep -E '{{semver_re}}' | sort -V | tail -1 || true)
    if [ -z "$latest" ]; then echo "No release tag to push — run 'just bump' first." >&2; exit 1; fi
    echo "Pushing main and $latest to origin…"
    git push origin main
    git push origin "$latest"
