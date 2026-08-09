#!/usr/bin/env bash
# TRA-1126 Finding 4 — bind a clean release label to the commit its tag names.
#
# Under a declared VERSION the label is no longer self-validating. Between the
# release merge and the bump-back landing, VERSION on main is still a clean
# `1.5.0`, so EVERY ordinary merge in that window produces an image labelled
# v1.5.0 that passes assert-release-version.sh — including a plain
# promote-with-empty-source, which resolves to whatever main currently is.
#
# The git tag is minted once, at one commit, by one job. Requiring the promoted
# image to be that exact commit restores the exact-commit binding the tag used
# to provide, and makes the bump-back purely cosmetic rather than load-bearing.
#
# Usage: assert-release-commit.sh <version> <image-tag>
#   <version>    the id.trakrf.app-version label read off the registry (vX.Y.Z)
#   <image-tag>  the resolved source image tag, i.e. sha-<7hex>
#
# Takes the version v-PREFIXED, as it appears in the label, because that is the
# string its caller already holds. resolve-release-action.sh takes the bare
# form for the same reason — it reads the VERSION file.
#
# Requires a checkout with full history and tags (promote-prod uses fetch-depth: 0).
set -euo pipefail

version="${1-}"
image_tag="${2-}"

if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "::error::assert-release-commit.sh expects a clean vX.Y.Z, got '${version}'." >&2
    exit 1
fi

if [[ ! "${image_tag}" =~ ^sha-([0-9a-f]{7,40})$ ]]; then
    {
        echo "::error::Cannot bind '${image_tag}' to a commit."
        echo "A clean release version can only be promoted from an image tag that names"
        echo "a commit (sha-<short>). Re-run the promote and name the release tag"
        echo "(source = ${version}) so the source resolves to a commit."
    } >&2
    exit 1
fi
image_commit_ref="${BASH_REMATCH[1]}"

if ! image_commit=$(git rev-parse --verify --quiet "${image_commit_ref}^{commit}"); then
    echo "::error::Cannot resolve the image's commit '${image_commit_ref}' in this checkout." >&2
    exit 1
fi

if ! tag_commit=$(git rev-parse --verify --quiet "refs/tags/${version}^{commit}"); then
    {
        echo "::error::The image claims version ${version}, but there is no git tag ${version}."
        echo "CI mints the git tag and the :${version} image tag together on the release"
        echo "merge. An image labelled ${version} with no such tag was built inside the"
        echo "window where VERSION was clean but the release commit was a different one."
    } >&2
    exit 1
fi

if [ "${image_commit}" != "${tag_commit}" ]; then
    {
        echo "::error::Refusing to promote ${image_tag}: it is not the ${version} release commit."
        echo "  ${version} -> ${tag_commit}"
        echo "  ${image_tag} -> ${image_commit}"
        echo
        echo "VERSION stays clean on main between the release merge and the bump-back,"
        echo "so ordinary merges in that window also carry a clean ${version} label."
        echo "Promote the release tag itself:"
        echo "  gh workflow run promote-prod.yml -f source=${version}"
    } >&2
    exit 1
fi

echo "${image_tag} is the ${version} release commit (${tag_commit})"
