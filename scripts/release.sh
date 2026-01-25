#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: scripts/release.sh vX.Y.Z [--push] [--remote origin]

Updates cmd/paw/version_map.go, commits the change if needed, and tags the release.
EOF
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION=""
PUSH="false"
REMOTE="origin"

while [ "$#" -gt 0 ]; do
	case "$1" in
		-h|--help)
			usage
			exit 0
			;;
		--push)
			PUSH="true"
			;;
		--remote)
			shift
			REMOTE="${1:-}"
			;;
		v*)
			VERSION="$1"
			;;
		*)
			usage
			exit 1
			;;
	esac
	shift
done

if [ -z "$VERSION" ]; then
	usage
	exit 1
fi

if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
	echo "Version must look like vX.Y.Z (optional suffix), got: $VERSION" >&2
	exit 1
fi

if [ -z "$REMOTE" ]; then
	echo "--remote requires a value" >&2
	exit 1
fi

if [ -n "$(git -C "$ROOT_DIR" status --porcelain)" ]; then
	echo "Working tree is not clean. Commit or stash changes first." >&2
	exit 1
fi

if git -C "$ROOT_DIR" rev-parse --verify "refs/tags/$VERSION" >/dev/null 2>&1; then
	echo "Tag already exists: $VERSION" >&2
	exit 1
fi

"$ROOT_DIR/scripts/update-version-map.sh" "$VERSION"

VERSION_MAP_FILE="$ROOT_DIR/cmd/paw/version_map.go"
if ! git -C "$ROOT_DIR" diff --quiet -- "$VERSION_MAP_FILE"; then
	git -C "$ROOT_DIR" add "$VERSION_MAP_FILE"
	git -C "$ROOT_DIR" commit -m "chore: update version map for $VERSION"
fi

git -C "$ROOT_DIR" tag "$VERSION"

if [ "$PUSH" = "true" ]; then
	git -C "$ROOT_DIR" push "$REMOTE" HEAD
	git -C "$ROOT_DIR" push "$REMOTE" "$VERSION"
fi
