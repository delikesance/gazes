#!/usr/bin/env bash
# Usage: mount-disk.sh <LABEL>
# Mounts /dev/disk/by-label/<LABEL> read-write on /mnt/gazes/<LABEL> and makes it a library disk.
set -euo pipefail

POOL=/mnt/gazes
UID_GID=10001

label=${1:-}
case "$label" in
  GAZES*) ;;
  *) echo "mount-disk: label must start with GAZES" >&2; exit 2 ;;
esac
case "$label" in
  */* | *..*) echo "mount-disk: label must not contain '/' or '..'" >&2; exit 2 ;;
esac
if [[ ! "$label" =~ ^[A-Za-z0-9_-]+$ ]]; then
  echo "mount-disk: label may only contain letters, digits, '_' and '-'" >&2
  exit 2
fi

dev=/dev/disk/by-label/$label
target=$POOL/$label
[ -e "$dev" ] || { echo "mount-disk: $dev not found" >&2; exit 1; }

mkdir -p "$target"
if mountpoint -q "$target"; then
  exit 0
fi

fstype=$(blkid -o value -s TYPE "$dev" || true)
opts=nofail,noatime
case "$fstype" in
  exfat | ntfs | vfat) opts="$opts,uid=$UID_GID,gid=$UID_GID" ;;
esac
mount -o "$opts" "$dev" "$target"

case "$fstype" in
  ext4 | xfs | btrfs) chown "$UID_GID:$UID_GID" "$target" ;;
esac

marker=$target/.gazes-library
if [ ! -e "$marker" ]; then
  printf '{"disk_id":"%s"}' "$(cat /proc/sys/kernel/random/uuid)" >"$marker"
  chown "$UID_GID:$UID_GID" "$marker" 2>/dev/null || true
fi
