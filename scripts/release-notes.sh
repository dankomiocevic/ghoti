#!/usr/bin/env bash
#
# Prints the CHANGELOG.md section for one version, to be used as the body of
# the GitHub release. It exits with an error when the section does not exist,
# so a tag cannot be released without notes.
#
# Usage: scripts/release-notes.sh v0.2.0 > notes.md

set -euo pipefail

if [ $# -ne 1 ]; then
	echo "usage: $0 <tag>" >&2
	exit 2
fi

version="${1#v}"
changelog="$(dirname "$0")/../CHANGELOG.md"

notes="$(awk -v version="$version" '
	/^## \[/ {
		if (found) exit
		found = ($0 ~ "^## \\[" version "\\]")
		next
	}
	found { print }
' "$changelog")"

if [ -z "${notes//[[:space:]]/}" ]; then
	echo "$0: no section for version ${version} in ${changelog}" >&2
	exit 1
fi

printf '%s\n' "$notes"
