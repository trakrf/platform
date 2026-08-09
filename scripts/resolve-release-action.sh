#!/usr/bin/env bash
# TRA-1126 — decide what the release job should do for this commit.
#
# Pure git; no registry, no network. The registry half is unconditionally
# idempotent (`imagetools create -t :vX.Y.Z :sha-<short>` from the same digest
# is a no-op), so only the git tag needs a decision, and this is it.
#
# Usage: resolve-release-action.sh <bare-version> <commit-sha>
#   none    VERSION is not a clean release version — ordinary development merge
#   create  no tag for this version yet — mint it at <commit-sha>
#   skip    the tag already exists AT THIS COMMIT — a re-run, or a later merge
#           inside the stale-clean-VERSION window before the bump-back lands
#
# Exit 1 when the tag exists at a DIFFERENT commit: that is a version being
# reused, which would silently repoint a released artifact.
#
# Takes the version BARE, as it appears in the file, because that is the string
# its caller already holds. assert-release-commit.sh takes the v-prefixed form
# for the same reason — it reads the OCI label.
set -euo pipefail

version="${1-}"
commit="${2-}"

# Only a clean X.Y.Z is a release. Anything with a prerelease suffix (-dev) or
# any other shape is ordinary development and mints nothing.
if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "none"
    exit 0
fi

tag="v${version}"

if ! resolved=$(git rev-parse --verify --quiet "${commit}^{commit}"); then
    echo "::error::Cannot resolve '${commit}' to a commit." >&2
    exit 1
fi
commit="${resolved}"

if ! tagged=$(git rev-parse --verify --quiet "refs/tags/${tag}^{commit}"); then
    echo "create"
    exit 0
fi

if [ "${tagged}" = "${commit}" ]; then
    echo "skip"
    exit 0
fi

{
    echo "::error::Refusing to release ${tag}: the tag already exists at a different commit."
    echo "  tag ${tag} -> ${tagged}"
    echo "  this build -> ${commit}"
    echo
    echo "VERSION declares a version that has already been released. Either the"
    echo "bump-back to the next -dev never landed, or VERSION was edited back to"
    echo "an already-shipped number. Bump VERSION forward; never re-point a tag."
} >&2
exit 1
