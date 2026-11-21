#!/bin/sh
# Origin: nodelogicau/intentions-cli scripts/build-bundle.sh @ 48738cf7453e (binary renamed)
# Assemble the Claude Desktop extension bundle (.mcpb) from built binaries.
#
#   scripts/build-bundle.sh <version> <dist-dir>
#
# Looks for a darwin universal binary (GoReleaser `universal_binaries`), else
# builds one with lipo when both darwin arches exist, else falls back to the
# host's darwin binary; and a windows amd64 binary. Exits 0 silently when the
# required binaries are not (yet) present, so it can run as a GoReleaser
# per-build post hook and simply do its work on the last build.
set -eu

VERSION="${1:?version}"; DIST="${2:?dist dir}"
VERSION="${VERSION#v}"
HERE="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$DIST"
# Absolute, so the zip step works from any working directory.
OUT="$(cd "$DIST" && pwd)/episodes-${VERSION}.mcpb"

find_bin() { # glob words -> first existing match, or empty (never fails)
  for c in "$@"; do
    if [ -e "$c" ]; then printf '%s' "$c"; return 0; fi
  done
  return 0
}

STAGE="$(mktemp -d)"; trap 'rm -rf "$STAGE"' EXIT

# GoReleaser writes its universal binary to dist/<id>_darwin_all/episodes
# (the id is prefixed, e.g. episodes-universal_darwin_all), so match any
# *_darwin_all directory first. `make cross` produces flat per-arch files, so
# fall back to building the universal binary here with lipo — into the stage,
# never dist, or a later run would pick up the stale result above.
DARWIN="$(find_bin "${DIST}"/*_darwin_all/episodes)"
if [ -z "$DARWIN" ]; then
  ARM="$(find_bin "${DIST}"/episodes_darwin_arm64*/episodes)"; [ -n "$ARM" ] || ARM="$(find_bin "${DIST}/episodes_darwin_arm64")"
  AMD="$(find_bin "${DIST}"/episodes_darwin_amd64*/episodes)"; [ -n "$AMD" ] || AMD="$(find_bin "${DIST}/episodes_darwin_amd64")"
  if [ -n "$ARM" ] && [ -n "$AMD" ]; then
    command -v lipo >/dev/null 2>&1 || {
      echo "build-bundle: both darwin arches present but lipo is unavailable; refusing to ship a single-architecture bundle" >&2
      exit 1
    }
    lipo -create -output "$STAGE/episodes-universal" "$ARM" "$AMD"
    DARWIN="$STAGE/episodes-universal"
  else
    DARWIN="${ARM:-$AMD}"
  fi
fi
WIN="$(find_bin "${DIST}"/episodes_windows_amd64*/episodes.exe)"; [ -n "$WIN" ] || WIN="$(find_bin "${DIST}/episodes_windows_amd64.exe")"

if [ -z "$DARWIN" ] || [ -z "$WIN" ]; then
  echo "build-bundle: waiting for binaries (darwin=${DARWIN:-none} win=${WIN:-none})" >&2
  exit 0
fi

# A macOS bundle that is not universal breaks Intel Macs; say so rather than ship it.
if command -v file >/dev/null 2>&1 && ! file "$DARWIN" | grep -q "universal binary"; then
  echo "build-bundle: ${DARWIN} is not a universal binary; the bundle would break on Intel Macs" >&2
  exit 1
fi

mkdir -p "$STAGE/server/darwin" "$STAGE/server/win32"
cp "$DARWIN" "$STAGE/server/darwin/episodes"; chmod 0755 "$STAGE/server/darwin/episodes"
cp "$WIN" "$STAGE/server/win32/episodes.exe"
sed "s/__VERSION__/${VERSION}/g" "$HERE/bundle/manifest.json.tmpl" > "$STAGE/manifest.json"
cp "$HERE/bundle/icon.png" "$STAGE/icon.png"

# Sanity: manifest is JSON and every referenced binary exists.
python3 - "$STAGE" <<'EOF'
import json, os, sys
stage = sys.argv[1]
m = json.load(open(os.path.join(stage, "manifest.json")))
assert m["manifest_version"] == "0.3" and m["server"]["type"] == "binary"
refs = [m["server"]["entry_point"]] + [o["command"].replace("${__dirname}/", "") for o in m["server"]["mcp_config"]["platform_overrides"].values()]
for r in refs:
    assert os.path.exists(os.path.join(stage, r)), r
assert m["user_config"]["workspace"]["type"] == "directory" and m["user_config"]["workspace"]["required"]
EOF

rm -f "$OUT"
( cd "$STAGE" && zip -q -r -X "$OUT" manifest.json icon.png server )
echo "build-bundle: wrote $OUT" >&2
