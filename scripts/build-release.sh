#!/usr/bin/env bash
set -euo pipefail

release_tag="${1:-}"
output_dir="${2:-dist}"
if [[ ! "${release_tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$ ]]; then
  echo "Usage: $0 vMAJOR.MINOR.PATCH [output-directory]" >&2
  exit 2
fi
if [[ "${output_dir}" == "/" || "${output_dir}" == "." || "${output_dir}" == ".." ]]; then
  echo "Refusing unsafe output directory: ${output_dir}" >&2
  exit 5
fi
release_version="${release_tag#v}"
mkdir -p "${output_dir}"
temporary_root="$(mktemp -d)"
trap 'rm -rf "${temporary_root}"' EXIT

platforms=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
)

for platform in "${platforms[@]}"; do
  read -r target_os target_arch <<< "${platform}"
  staging="${temporary_root}/${target_os}-${target_arch}"
  mkdir -p "${staging}"
  binary="${staging}/agent-config-inspector"
  CGO_ENABLED=0 GOOS="${target_os}" GOARCH="${target_arch}" go build \
    -trimpath \
    -ldflags "-s -w -X github.com/east-true/agent-config-inspector/pkg/agentconfig.Version=${release_version}" \
    -o "${binary}" \
    ./cmd/agent-config-inspector
  cp LICENSE "${staging}/LICENSE"
  asset="agent-config-inspector_${release_version}_${target_os}_${target_arch}.tar.gz"
  tar -C "${staging}" -czf "${output_dir}/${asset}" agent-config-inspector LICENSE
done

if command -v sha256sum >/dev/null 2>&1; then
  (
    cd "${output_dir}"
    sha256sum agent-config-inspector_*.tar.gz > checksums.txt
  )
else
  (
    cd "${output_dir}"
    shasum -a 256 agent-config-inspector_*.tar.gz > checksums.txt
  )
fi
