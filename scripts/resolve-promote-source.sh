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
# ancestry-checked against origin/main. An already-formed sha-/latest image tag
# is passed through untouched: it is what the old contract accepted, and it
# cannot be ancestry-checked because it names an image, not a commit.
#
# Usage: resolve-promote-source.sh [ref]
#   (no argument, or empty)  -> origin/main HEAD
#   sha-<hex> | latest       -> passed through unchanged
#   anything else            -> resolved as a git ref -> sha-<first 7 of its sha>
set -euo pipefail

ref="${1-}"

# Already an image tag? Preserve the existing input contract exactly.
if [[ "${ref}" =~ ^(sha-[0-9a-f]{7,40}|latest)$ ]]; then
    echo "${ref}"
    exit 0
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

if ! git merge-base --is-ancestor "${sha}" origin/main; then
    {
        echo "::error::Refusing to promote '${ref}' (${sha}): it is not an ancestor of origin/main."
        echo "Only main-derived images may be promoted to :prod. The preview branch in"
        echo "particular is a force-rewritten PR composition and must never reach prod."
    } >&2
    exit 1
fi

# 7 characters, matching docker/metadata-action's format=short. NOT
# `git rev-parse --short`, which returns 8 and is the bug this script exists for.
echo "sha-${sha:0:7}"
