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

# A mountpoint left behind by an unplugged disk (device gone, or now a different device) is detached first.
if mountpoint -q "$target"; then
  current=$(findmnt -no SOURCE "$target" || true)
  if [ -e "$dev" ] && [ -n "$current" ] && [ "$(readlink -f "$current")" = "$(readlink -f "$dev")" ]; then
    exit 0
  fi
  umount -l "$target"
fi
[ -e "$dev" ] || { echo "mount-disk: $dev not found" >&2; exit 1; }

mkdir -p "$target"

fstype=$(blkid -o value -s TYPE "$dev" || true)
opts=nofail,noatime
case "$fstype" in
  exfat | ntfs | vfat) opts="$opts,uid=$UID_GID,gid=$UID_GID" ;;
esac
if ! mount -o "$opts" "$dev" "$target"; then
  rmdir "$target" 2>/dev/null || true
  exit 1
fi

case "$fstype" in
  ext2 | ext3 | ext4 | xfs | btrfs) chown "$UID_GID:$UID_GID" "$target" ;;
esac

marker=$target/.gazes-library
if [ ! -e "$marker" ]; then
  printf '{"disk_id":"%s"}' "$(cat /proc/sys/kernel/random/uuid)" >"$marker"
  chown "$UID_GID:$UID_GID" "$marker" 2>/dev/null || true
fi
