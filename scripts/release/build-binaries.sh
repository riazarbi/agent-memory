#!/usr/bin/env bash
# Cross-compile static memory binaries for each release target into OUT_DIR as
# memory_<tag>_<os>_<arch>.tar.gz, plus a checksums.txt. Each archive holds
# memory_<tag>_<os>_<arch>/memory and the repo's agent/ folder, as it is, at
# memory_<tag>_<os>_<arch>/agent/, so an install works from a download alone.
# Run it from the repo root.
#
#   scripts/release/build-binaries.sh 20261002 dist
set -euo pipefail

# macOS tar would otherwise add AppleDouble ._* entries for files carrying
# extended attributes; GNU tar ignores the variable.
export COPYFILE_DISABLE=1

tag=${1:?usage: $0 TAG OUT_DIR}
out=${2:?usage: $0 TAG OUT_DIR}
targets=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64)

rm -rf "$out"
mkdir -p "$out"
for target in "${targets[@]}"; do
    os=${target%/*}
    arch=${target#*/}
    name="memory_${tag}_${os}_${arch}"
    mkdir -p "$out/$name"
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
        go build -trimpath -ldflags='-s -w' -o "$out/$name/memory" ./cmd/memory
    cp -R agent "$out/$name/agent"
    tar -C "$out" -czf "$out/$name.tar.gz" "$name"
    rm -rf "${out:?}/$name"
done

(cd "$out" && shasum -a 256 ./*.tar.gz | sed 's| \./| |' > checksums.txt)
