#!/usr/bin/env bash
set -euo pipefail

modules=(errors observability security http postgres messaging)
for module in "${modules[@]}"; do
  echo "==> $module"
  (
    cd "$module"
    unformatted="$(find . -name '*.go' -type f -print0 | xargs -0 gofmt -l)"
    if [[ -n "$unformatted" ]]; then
      echo "gofmt required:"
      echo "$unformatted"
      exit 1
    fi
    go vet ./...
    go test ./...
    go build ./...
  )
done
