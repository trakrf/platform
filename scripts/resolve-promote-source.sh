#!/usr/bin/env bash
# TRA-1085 item 2 — resolve promote-prod's source image tag from a git ref.
#
# `git rev-parse --short` returns 8 characters; docker/metadata-action publishes
# 7 (`type=sha,prefix=sha-,format=short`). The first v1.3.0 promotion failed on
# exactly that mismatch — `ghcr.io/trakrf/backend:sha-aa9822bb: not found`.
# The preflight caught it and refused rather than creating a broken :prod tag,
# which is the correct design and is kept. But nobody should have to know which
# of two short-sha conventions applies: name a git ref, and this resolves the
# image tag.
#
# Accepting arbitrary refs would lose the guarantee the old input regex gave —
# that only main-derived images can reach :prod — so every ref resolved here is
# ancestry-checked.
#
# TRA-1127 item 2: that check is *promotable = an ancestor of `origin/main`, or
# of a `release/X.Y.x` hotfix line*. The rule exists to keep the force-rewritten
# `preview` composition out of prod, and the wider form still does that. A patch
# on a shipped release branches from its tag rather than from main, so its
# commits are never main's ancestors and the narrower form refused them outright.
#
# Only the `.x` maintenance lines widen it, NOT `release/` as a prefix.
# `release/X.Y.Z` is the ordinary release PR branch (docs/releasing.md step 1) —
# an unmerged proposal whose head declares a clean VERSION, which is exactly the
# shape Finding 3 below closed off.
#
# TRA-1126 Finding 3: `sha-<hex>` used to be passed through unchecked, on the
# stated belief that it "cannot be ancestry-checked because it names an image,
# not a commit". That was simply wrong — sha-<hex> IS the git short sha, so it
# resolves with `git rev-parse` and is checked like anything else. It mattered
# little while every preview version was describe-shaped; with a declared
# VERSION (TRA-1126) an OPEN release PR puts a clean `1.5.0` into the preview
# composition, and that preview image's sha- tag was directly promotable.
#
# `latest` is still passed through: it names an image and no commit. It is
# refused downstream by assert-release-commit.sh, which cannot bind it to the
# release tag's commit.
#
# Usage: resolve-promote-source.sh [ref]
#   (no argument, or empty)  -> origin/main HEAD
#   latest                   -> passed through unchanged
#   sha-<hex>                -> resolved as a commit, ancestry-checked, renormalised
#   anything else            -> resolved as a git ref -> sha-<first 7 of its sha>
#
# TRA-1085 item 2 / TRA-1127 item 2.
set -euo pipefail

ref="${1-}"

# Names an image and no commit; preserved for the old input contract.
if [ "${ref}" = "latest" ]; then
    echo "latest"
    exit 0
fi

# An image tag that DOES name a commit: strip the prefix and resolve it as a
# ref, so it gets the same ancestry check as everything else.
if [[ "${ref}" =~ ^sha-([0-9a-f]{7,40})$ ]]; then
    ref="${BASH_REMATCH[1]}"
fi

# No ref given: promote whatever main currently points at.
if [ -z "${ref}" ]; then
    ref="origin/main"
fi

if ! sha=$(git rev-parse --verify --quiet "${ref}^{commit}"); then
    {
        echo "::error::Cannot resolve '${ref}' to a commit."
        echo "Pass a release tag (v1.3.0), a branch, or a full sha — or leave it empty for current main."
    } >&2
    exit 1
fi

# Ancestor of main, or of any hotfix line. `for-each-ref` reads the remote refs
# because that is what promote-prod has: it runs on a CI checkout, where local
# branches other than the checked-out one do not exist.
promotable=false
if git merge-base --is-ancestor "${sha}" origin/main; then
    promotable=true
else
    while read -r line; do
        [ -n "${line}" ] || continue
        if git merge-base --is-ancestor "${sha}" "${line}"; then
            promotable=true
            break
        fi
    done < <(git for-each-ref --format='%(refname)' 'refs/remotes/origin/release/*.x')
fi

if [ "${promotable}" != true ]; then
    {
        echo "::error::Refusing to promote '${ref}' (${sha}): it is not an ancestor of"
        echo "origin/main, nor of any release/X.Y.x hotfix line."
        echo "Only main-derived and hotfix-line images may be promoted to :prod. The"
        echo "preview branch in particular is a force-rewritten PR composition and must"
        echo "never reach prod, and an open release/X.Y.Z PR head is not a release yet."
    } >&2
    exit 1
fi

# 7 characters, matching docker/metadata-action's format=short. NOT
# `git rev-parse --short`, which returns 8 and is the bug this script exists for.
echo "sha-${sha:0:7}"
