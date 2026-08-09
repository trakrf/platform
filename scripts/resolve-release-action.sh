#!/usr/bin/env bash
# TRA-1126 — decide what the release job should do for this commit.
#
# Pure git; no registry, no network. The registry half is unconditionally
# idempotent (`imagetools create -t :vX.Y.Z :sha-<short>` from the same digest
# is a no-op), so only the git tag needs a decision, and this is it.
#
# Usage: resolve-release-action.sh <bare-version> <commit-sha>
#   none    VERSION is not a clean release version — ordinary development merge
#   create  no tag for this version yet — mint it at <commit-sha>, then publish
#           the :vX.Y.Z image tag
#   skip    the tag already exists AT THIS COMMIT — a re-run. Do not touch the
#           git tag; DO republish the image tag, which is how a run that minted
#           the tag and then died before publishing repairs itself
#   stale   the tag exists at a DIFFERENT commit — mint nothing, publish nothing
#
# `stale` is the window between the release merge and the bump-back landing:
# VERSION on main is still clean, so every ordinary merge in it reaches this
# script with the release tag already sitting on an ancestor. That is normal and
# must not fail the build — main would go red for everyone until the bump-back
# merged. It must equally not republish :vX.Y.Z, which would re-point a released
# image tag at a commit that is not the release. Doing nothing is both.
#
# An operator editing VERSION back to an already-shipped number is topologically
# indistinguishable from that window, and lands in `stale` too. Also harmless:
# nothing is minted and nothing moves. The caller warns.
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
    echo "::warning::${tag} was already released at ${tagged}; this build is ${commit}."
    echo "Minting nothing and republishing nothing. Either the bump-back to the next"
    echo "-dev has not landed yet — normal, and harmless — or VERSION was edited back"
    echo "to an already-shipped number, which is a mistake to fix by bumping forward."
} >&2
echo "stale"
