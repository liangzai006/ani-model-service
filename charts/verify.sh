#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT
public_url=${HIGRESS_PUBLIC_BASE_URL:-https://inference.example.test}

helm lint "$repo_root/charts/ani-model"
helm lint "$repo_root/charts/ani-inference" \
  --set higress.publicBaseURL="$public_url"
helm template ani-model "$repo_root/charts/ani-model" \
  --namespace ani-model \
  -f "$repo_root/charts/ani-model/values-new-environment.yaml" \
  >"$tmp_dir/model.yaml"
helm template ani-inference "$repo_root/charts/ani-inference" \
  --namespace ani-inference \
  -f "$repo_root/charts/ani-inference/values-new-environment.yaml" \
  --set higress.publicBaseURL="$public_url" \
  >"$tmp_dir/inference.yaml"
test -s "$tmp_dir/model.yaml"
test -s "$tmp_dir/inference.yaml"
echo "rendered model and inference manifests"
