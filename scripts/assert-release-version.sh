#!/usr/bin/env bash
# TRA-1085 item 1 — refuse to promote an image that was not built from a
# release tag.
#
# APP_VERSION comes from `git describe --tags` at BUILD time, and promote-prod
# is a pure manifest re-tag with no rebuild. So the natural order — merge, then
# tag the release — bakes a dev-shaped version (v1.2.0-559-gaa9822bb) into the
# image permanently, and /health would report it for the entire life of the
# release. Tagging afterwards changes nothing, because nothing rebuilds.
#
# That happened on v1.3.0 and was caught by luck. This is the check that makes
# it unshippable rather than folklore.
#
# Usage: assert-release-version.sh <version>
# Exits 0 and echoes <version> if it is a clean vX.Y.Z; exits 1 otherwise.
set -euo pipefail

version="${1-}"

if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    {
        echo "::error::Refusing to promote an image whose version is '${version}'."
        echo "Expected a clean release version matching ^v[0-9]+\\.[0-9]+\\.[0-9]+\$ (e.g. v1.3.0)."
        echo
        echo "This almost always means the image was built before the release tag existed."
        echo "APP_VERSION is resolved by \`git describe\` at build time and promote-prod"
        echo "never rebuilds, so tagging after the build cannot fix an image already pushed."
        echo
        echo "Fix: tag the release, let the tag build finish, then promote the tag:"
        echo "  git tag vX.Y.Z && git push origin vX.Y.Z"
        echo "  # wait for Docker Build and Push to complete on refs/tags/vX.Y.Z"
        echo "  # then re-run this workflow with source = vX.Y.Z"
        echo
        echo "An empty version means the image predates the id.trakrf.app-version label"
        echo "(TRA-1085); rebuild it from a tagged commit."
    } >&2
    exit 1
fi

echo "${version}"
