#!/usr/bin/env bash
# TRA-1126 — derive the platform version string from the declared VERSION file.
#
# VERSION holds BARE semver (1.5.0-dev). The `v` prefix is owned by this
# derivation layer, so the git tag, the id.trakrf.app-version OCI label,
# /health and the UI all stay byte-compatible with the pre-TRA-1126 shape
# without VERSION itself having to carry a redundant character.
#
# Usage: build-version.sh [suffix]
#   build-version.sh                     -> v1.5.0-dev
#   build-version.sh -preview+419+420    -> v1.5.0-dev-preview+419+420
#
# The Dockerfile's build-meta stage mirrors this rule against the COPY'd file
# and refuses a build whose APP_VERSION build-arg disagrees, so a caller cannot
# inject a version the commit does not declare.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
suffix="${1-}"

if [ ! -f "${repo_root}/VERSION" ]; then
    echo "::error::No VERSION file at ${repo_root}/VERSION." >&2
    exit 1
fi

version=$(tr -d '[:space:]' < "${repo_root}/VERSION")

# Bare semver, optional prerelease. Deliberately no leading v: see above.
if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    {
        echo "::error::VERSION is '${version}', which is not bare semver."
        echo "Expected MAJOR.MINOR.PATCH with an optional prerelease, e.g. 1.5.0-dev or 1.5.0."
        echo "The 'v' prefix is added by this script — do not put it in the file."
    } >&2
    exit 1
fi

echo "v${version}${suffix}"
