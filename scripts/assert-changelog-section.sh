#!/usr/bin/env bash
# TRA-1085 item 4, built under TRA-1126 — a release cannot be cut without the
# matching CHANGELOG.md section.
#
# Before TRA-1126 there was nothing natural to hang this on: the release act was
# an untracked `git push origin vX.Y.Z` with no diff and no checks. Now the
# release act IS a diff — the one that flips VERSION to a clean X.Y.Z — so the
# gate is just a condition on that diff, and it runs inside the existing
# `lint-test` required check rather than adding a new required context that the
# main-branch-protection ruleset would have to be edited to know about.
#
# Inert whenever VERSION carries a prerelease, which is every ordinary PR.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

version=$(tr -d '[:space:]' < "${repo_root}/VERSION")

if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "VERSION is '${version}' (in development) — changelog gate not applicable."
    exit 0
fi

# -F, and the heading includes its closing bracket, so 1.5.0 is not satisfied
# by a `## [1.5.01]` section.
if grep -qF "## [${version}]" "${repo_root}/CHANGELOG.md"; then
    echo "CHANGELOG.md has the ## [${version}] section."
    exit 0
fi

{
    echo "::error::VERSION declares release ${version} but CHANGELOG.md has no '## [${version}]' section."
    echo
    echo "A release PR carries both: the VERSION flip and the changelog section it"
    echo "ships. Move everything under '## [Unreleased]' that is actually shipping"
    echo "into a new '## [${version}] - YYYY-MM-DD' section."
} >&2
exit 1
