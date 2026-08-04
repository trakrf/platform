#!/usr/bin/env bash
# TRA-1085 — pull the id.trakrf.app-version label out of
# `docker buildx imagetools inspect --format '{{ json .Image }}'` output.
#
# The shape of that output is not obvious and differs by image:
#
#   single-platform: {"created":..., "config": {"Labels": {...}}, ...}
#   multi-platform:  {"linux/amd64": {"config": {"Labels": {...}}, ...},
#                     "linux/arm64": {...}}
#
# Our published images are always the second form — docker-build.yml builds
# amd64 and arm64 separately and merges them into one manifest list (TRA-909).
# A naive `{{ index .Image.Config.Labels "..." }}` template works only on the
# first form and errors on ours, which is why this is a script with tests
# rather than a template string in a YAML run: block.
#
# Reads the JSON on stdin. Prints the label value, or nothing at all when:
#   - no platform carries the label, or
#   - the platforms disagree about it (a merged manifest whose arches came from
#     different builds — the version is then not a fact about the image).
# Callers pass the result to assert-release-version.sh, which fails closed on
# an empty string.
set -euo pipefail

jq -r '
  # Normalise both shapes to a list of per-platform image configs.
  (if has("config") then [.] else [.[]] end)
  # A platform missing the label maps to "" rather than being dropped — an
  # image with one arch labelled and one not is a disagreement, not a match.
  | map(.config.Labels["id.trakrf.app-version"] // "")
  | unique
  # Exactly one distinct, non-empty value across every platform, or nothing.
  | if length == 1 and .[0] != "" then .[0] else empty end
'
