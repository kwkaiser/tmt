#!/usr/bin/env sh
set -eu

update() {
  attr=$1
  file=$2
  key=$3
  if out=$(nix build --no-link ".#$attr" 2>&1); then
    echo "$key in $file is up to date"
    return
  fi
  got=$(printf '%s\n' "$out" | sed -n 's/.*got: *\(sha256-[A-Za-z0-9+/=]*\).*/\1/p' | head -n1)
  if [ -z "$got" ]; then
    printf '%s\n' "$out" >&2
    exit 1
  fi
  sed -i.bak "s|$key = \".*\";|$key = \"$got\";|" "$file"
  rm "$file.bak"
  nix build --no-link ".#$attr"
  echo "$key in $file updated to $got"
}

update tmt.goModules nix/tmt.nix vendorHash
