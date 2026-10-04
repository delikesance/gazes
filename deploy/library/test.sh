#!/usr/bin/env bash
# Checks the host deployment scripts: shellcheck (if installed), bash -n, a dry run and label validation.
set -u
cd "$(dirname "$0")" || exit 1
fail=0
ko() { echo "FAIL: $*"; fail=1; }

for f in install-host.sh mount-disk.sh label-disk.sh test.sh; do
  [ -f "$f" ] || { ko "$f is missing"; continue; }
  bash -n "$f" || ko "bash -n $f"
done

if command -v shellcheck >/dev/null 2>&1; then
  shellcheck ./*.sh || ko "shellcheck"
else
  echo "skip: shellcheck not installed"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
if out=$(bash install-host.sh --dry-run --root "$tmp" 2>&1); then
  for want in "/mnt/gazes/internal" ".gazes-library" \
    "gazes-library-mount@.service" "99-gazes-library.rules" "mount-disk.sh"; do
    grep -qF -- "$want" <<<"$out" || ko "dry run does not mention $want"
  done
  [ -z "$(find "$tmp" -mindepth 1 -print -quit)" ] || ko "dry run wrote files under the root"
else
  ko "install-host.sh --dry-run failed: $out"
fi

out=$(bash label-disk.sh /dev/null OTHER 2>&1) && ko "label-disk.sh accepted OTHER"
grep -qF "label must start with GAZES" <<<"$out" || ko "label-disk.sh message: $out"

for bad in OTHER "GAZES/x" "GAZES..1" "GAZES 1"; do
  bash mount-disk.sh "$bad" >/dev/null 2>&1 && ko "mount-disk.sh accepted '$bad'"
done

[ ! -e gazes-library-shared.service ] || ko "self-bind unit must not exist"
grep -q 'ACTION=="remove"' 99-gazes-library.rules || ko "udev remove rule missing"

[ "$fail" -eq 0 ] && echo "OK"
exit "$fail"
