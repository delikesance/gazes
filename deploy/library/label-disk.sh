#!/usr/bin/env bash
# Usage: label-disk.sh DEV LABEL
# Sets the filesystem label of DEV to LABEL (must start with GAZES) without formatting.
set -euo pipefail

dev=${1:-}
label=${2:-}
[ -n "$dev" ] && [ -n "$label" ] || { echo "usage: label-disk.sh DEV LABEL" >&2; exit 2; }
case "$label" in
  GAZES*) ;;
  *) echo "label-disk: label must start with GAZES" >&2; exit 2 ;;
esac
if [[ ! "$label" =~ ^[A-Za-z0-9_-]+$ ]]; then
  echo "label-disk: label may only contain letters, digits, '_' and '-'" >&2
  exit 2
fi
[ -b "$dev" ] || { echo "label-disk: $dev is not a block device" >&2; exit 2; }

fstype=$(blkid -o value -s TYPE "$dev" || true)
case "$fstype" in
  ext2 | ext3 | ext4) e2label "$dev" "$label" ;;
  exfat) exfatlabel "$dev" "$label" ;;
  ntfs) ntfslabel "$dev" "$label" ;;
  "")
    echo "$dev has no filesystem."
    printf 'To format it as ext4 (ERASES %s), type: FORMAT %s\n> ' "$dev" "$dev"
    read -r answer
    [ "$answer" = "FORMAT $dev" ] || { echo "aborted"; exit 1; }
    mkfs.ext4 -L "$label" "$dev"
    ;;
  *) echo "label-disk: unsupported filesystem '$fstype' (use e2label/exfatlabel/ntfslabel manually)" >&2; exit 1 ;;
esac
echo "$dev is now labelled $label"
