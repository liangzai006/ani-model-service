#!/usr/bin/env bash
set -euo pipefail

# Keep Proto generation behind one deterministic entrypoint. Generated Go
# files are written beside their source Proto files under api/.
cd "$(dirname "$0")/.."
export GOWORK=off

BUF=${BUF:-buf}
action=${1:-generate}
case "$action" in
  lint|generate) ;;
  *)
    echo "usage: $0 [lint|generate]" >&2
    exit 2
    ;;
esac

test "$("$BUF" --version)" = 1.60.0
go version -m "$(command -v "$BUF")" \
  | grep -Eq 'mod[[:space:]]+github.com/bufbuild/buf[[:space:]]+v1.60.0'

"$BUF" lint
if [[ "$action" == generate ]]; then
  "$BUF" build
  "$BUF" generate --template buf.gen.yaml
fi
