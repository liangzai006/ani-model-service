#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

diff -rq "$repo_root/migrations" "$repo_root/charts/ani-model/migrations"

helm lint "$repo_root/charts/ani-model" \
  --set-string storage.direct.endpoint=s3.ani-system.svc.cluster.local:9000
helm template ani-model "$repo_root/charts/ani-model" \
  --namespace ani-system \
  --set-string storage.direct.endpoint=s3.ani-system.svc.cluster.local:9000 \
  >"$tmp_dir/model.yaml"
test -s "$tmp_dir/model.yaml"
grep -q 'schema_migrations' "$tmp_dir/model.yaml"
grep -q 'pre-install,pre-upgrade' "$tmp_dir/model.yaml"
grep -q 'CREATE TABLE public.models' "$tmp_dir/model.yaml"
grep -q 'ANI_MODEL_SERVER_CERT_FILE' "$tmp_dir/model.yaml"
grep -q 'ANI_MODEL_SERVER_CA_FILE' "$tmp_dir/model.yaml"
grep -q '^kind: Certificate$' "$tmp_dir/model.yaml"
if helm template ani-model "$repo_root/charts/ani-model" \
  --namespace wrong-ns \
  --set-string storage.direct.endpoint=s3.ani-system.svc.cluster.local:9000 \
  >"$tmp_dir/wrong-namespace.yaml" 2>/dev/null; then
  echo "chart must reject namespaces other than ani-system" >&2
  exit 1
fi
helm template ani-model "$repo_root/charts/ani-model" \
  --namespace ani-system \
  --set-string storage.mode=grpc \
  --set-string storage.grpc.address=storage.ani-system.svc.cluster.local:8443 \
  --set-string storage.grpc.serverName=storage.ani.internal \
  --set-string storage.grpc.caCertSecret=ani-storage-ca \
  >"$tmp_dir/model-grpc.yaml"
grep -q 'ANI_STORAGE_GRPC_SERVER_NAME' "$tmp_dir/model-grpc.yaml"
if helm template ani-model "$repo_root/charts/ani-model" \
  --namespace ani-system \
  --set-string storage.direct.endpoint=s3.ani-system.svc.cluster.local:9000 \
  --set-string inference.grpcAddress=inference.ani-system.svc.cluster.local:19090 \
  --set-string inference.serverName=ani-inference.ani.internal \
  --set-string inference.tls.existingSecret=ani-model-inference-mtls \
  >"$tmp_dir/model-inference.yaml" 2>/dev/null; then
  grep -q 'ANI_INFERENCE_CA_FILE' "$tmp_dir/model-inference.yaml"
  grep -q 'ANI_INFERENCE_CERT_FILE' "$tmp_dir/model-inference.yaml"
  grep -q 'ANI_INFERENCE_KEY_FILE' "$tmp_dir/model-inference.yaml"
else
  echo "inference mTLS configuration did not render" >&2
  exit 1
fi
if grep -q 'ANI_MODEL_REQUIRE_CLIENT_CERT' "$tmp_dir/model.yaml"; then
  echo "client certificate verification is mandatory and must not be a switch" >&2
  exit 1
fi
if grep -q 'ANI_INFERENCE_GRPC_ADDR' "$tmp_dir/model.yaml"; then
  echo "inference address must stay empty unless explicitly set" >&2
  exit 1
fi
echo "rendered model manifest"
