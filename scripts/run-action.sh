#!/usr/bin/env bash
set -euo pipefail

case "${ACI_COMMAND}" in
  scan|verify) ;;
  *)
    echo "Unsupported command: ${ACI_COMMAND}. Use scan or verify." >&2
    exit 2
    ;;
esac

case "${ACI_FAIL_ON}" in
  error|warning|never) ;;
  *)
    echo "Unsupported fail-on value: ${ACI_FAIL_ON}." >&2
    exit 2
    ;;
esac

case "${RUNNER_OS}" in
  Linux) asset_os="linux" ;;
  macOS) asset_os="darwin" ;;
  *)
    echo "Unsupported runner OS: ${RUNNER_OS}." >&2
    exit 4
    ;;
esac

case "${RUNNER_ARCH}" in
  X64) asset_arch="amd64" ;;
  ARM64) asset_arch="arm64" ;;
  *)
    echo "Unsupported runner architecture: ${RUNNER_ARCH}." >&2
    exit 4
    ;;
esac

if [[ ! "${ACI_ACTION_REPOSITORY}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  echo "Invalid action repository identity." >&2
  exit 5
fi

release_tag="${ACI_VERSION}"
if [[ "${release_tag}" == "latest" ]]; then
  release_url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${ACI_ACTION_REPOSITORY}/releases/latest")"
  release_tag="${release_url##*/}"
fi
if [[ ! "${release_tag}" =~ ^v[0-9][A-Za-z0-9._-]*$ ]]; then
  echo "Invalid release tag: ${release_tag}." >&2
  exit 5
fi

release_version="${release_tag#v}"
asset_name="agent-config-inspector_${release_version}_${asset_os}_${asset_arch}.tar.gz"
download_root="https://github.com/${ACI_ACTION_REPOSITORY}/releases/download/${release_tag}"
temporary_dir="$(mktemp -d "${RUNNER_TEMP}/agent-config-inspector.XXXXXX")"
trap 'rm -rf "${temporary_dir}"' EXIT

curl -fsSL "${download_root}/${asset_name}" -o "${temporary_dir}/${asset_name}"
curl -fsSL "${download_root}/checksums.txt" -o "${temporary_dir}/checksums.txt"

expected_checksum="$(awk -v asset="${asset_name}" '$2 == asset { print $1 }' "${temporary_dir}/checksums.txt")"
if [[ ! "${expected_checksum}" =~ ^[0-9a-f]{64}$ ]]; then
  echo "Release checksum is missing or malformed for ${asset_name}." >&2
  exit 5
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum="$(sha256sum "${temporary_dir}/${asset_name}" | awk '{ print $1 }')"
else
  actual_checksum="$(shasum -a 256 "${temporary_dir}/${asset_name}" | awk '{ print $1 }')"
fi
if [[ "${actual_checksum}" != "${expected_checksum}" ]]; then
  echo "Checksum verification failed for ${asset_name}." >&2
  exit 5
fi

archive_listing="$(tar -tzf "${temporary_dir}/${asset_name}")"
expected_listing=$'agent-config-inspector\nLICENSE'
if [[ "${archive_listing}" != "${expected_listing}" ]]; then
  echo "Verified release archive must contain only agent-config-inspector and LICENSE." >&2
  exit 5
fi

tar -C "${temporary_dir}" -xzf "${temporary_dir}/${asset_name}"
binary="${temporary_dir}/agent-config-inspector"
if [[ ! -x "${binary}" ]]; then
  echo "Verified release archive does not contain the expected executable." >&2
  exit 5
fi

sarif_file="${RUNNER_TEMP}/agent-config-inspector-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}.sarif"
echo "sarif-file=${sarif_file}" >> "${GITHUB_OUTPUT}"

set +e
if [[ "${ACI_COMMAND}" == "verify" ]]; then
  "${binary}" verify "${ACI_WORKSPACE}" --snapshot "${ACI_SNAPSHOT}" --format sarif --fail-on "${ACI_FAIL_ON}" > "${sarif_file}"
else
  "${binary}" scan "${ACI_WORKSPACE}" --format sarif --fail-on "${ACI_FAIL_ON}" > "${sarif_file}"
fi
status=$?
set -e

if [[ ! -s "${sarif_file}" ]]; then
  echo "Agent Config Inspector did not produce a SARIF result." >&2
  exit "${status}"
fi
exit "${status}"
