#!/usr/bin/env bash
# Usage: sudo install-host.sh [--dry-run] [--root DIR]
# Installs the host side of the AV1 library: /mnt/gazes pool, udev rules and mount units.
# Idempotent. --dry-run prints the actions without writing; --root prefixes every written path.
set -euo pipefail

dry=0
root=
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) dry=1 ;;
    --root) root=${2:?--root needs a directory}; shift ;;
    *) echo "usage: install-host.sh [--dry-run] [--root DIR]" >&2; exit 2 ;;
  esac
  shift
done

here=$(cd "$(dirname "$0")" && pwd)
pool=$root/mnt/gazes
systemd_dir=$root/etc/systemd/system
udev_dir=$root/etc/udev/rules.d
lib_dir=$root/usr/local/lib/gazes-library

act() {
  if [ "$dry" -eq 1 ]; then echo "[dry-run] $*"; else "$@"; fi
}
say() { if [ "$dry" -eq 1 ]; then echo "[dry-run] $*"; else echo "$*"; fi; }
install_file() { # mode src dst
  say "install $2 -> $3"
  [ "$dry" -eq 1 ] || install -D -m "$1" "$2" "$3"
}

# The container binds /mnt/gazes with rshared: disks mounted below it later only show up if the mount that
# holds /mnt/gazes is itself shared (the default with systemd).
if [ -z "$root" ]; then
  probe=$pool
  while [ ! -d "$probe" ]; do probe=$(dirname "$probe"); done
  propagation=$(findmnt -no PROPAGATION -T "$probe" || true)
  if [ "$propagation" != "shared" ]; then
    echo "install-host: the mount holding /mnt/gazes has propagation '${propagation:-unknown}', expected 'shared'." >&2
    echo "Run 'mount --make-rshared /' (systemd does this by default) and retry." >&2
    exit 1
  fi
fi

say "create $pool/internal (owner 10001:10001)"
if [ "$dry" -eq 0 ]; then
  mkdir -p "$pool/internal"
  chown 10001:10001 "$pool/internal" || [ -n "$root" ]
fi

marker=$pool/internal/.gazes-library
if [ ! -e "$marker" ]; then
  say "write $marker with a new disk_id"
  if [ "$dry" -eq 0 ]; then
    printf '{"disk_id":"%s"}' "$(cat /proc/sys/kernel/random/uuid)" >"$marker"
    chown 10001:10001 "$marker" || [ -n "$root" ]
  fi
else
  say "keep existing $marker"
fi

install_file 0755 "$here/mount-disk.sh" "$lib_dir/mount-disk.sh"
install_file 0644 "$here/gazes-library-mount@.service" "$systemd_dir/gazes-library-mount@.service"
install_file 0644 "$here/99-gazes-library.rules" "$udev_dir/99-gazes-library.rules"

if [ -z "$root" ]; then
  act systemctl daemon-reload
    act udevadm control --reload
else
  say "skip systemctl/udevadm (custom --root)"
fi
