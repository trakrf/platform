#!/usr/bin/env bash
# TRA-1085 item 1 / TRA-1126 — refuse to promote an image that is not a release
# build.
#
# The platform version is declared in the root VERSION file and baked into the
# image at build time; promote-prod is a pure manifest re-tag with no rebuild.
# During development VERSION carries a `-dev` prerelease, so no ordinary main
# build and no preview composition can produce a clean vX.Y.Z — this check is
# what makes that unshippable rather than folklore.
#
# It answers "is this a release build". It deliberately does NOT answer "is this
# THE release build" — assert-release-commit.sh does that, by binding the
# version to the commit its git tag names. Before TRA-1126 the two questions
# were one, because a clean version required a real tag at that commit.
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
        echo "The version is declared in the root VERSION file. A clean vX.Y.Z is produced"
        echo "only by a release PR that flips VERSION to X.Y.Z; every other build carries"
        echo "the -dev prerelease and is deliberately unpromotable."
        echo
        echo "Fix: promote a release tag."
        echo "  gh workflow run promote-prod.yml -f source=vX.Y.Z"
        echo
        echo "An empty version means the image predates the id.trakrf.app-version label"
        echo "(TRA-1085); rebuild it from a commit that declares a release VERSION."
    } >&2
    exit 1
fi

echo "${version}"
