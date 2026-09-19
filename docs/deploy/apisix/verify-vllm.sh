#!/usr/bin/env bash
set -euo pipefail

: "${APISIX_NODE_IP:?set APISIX_NODE_IP to a Kubernetes node address}"

route_status="$(kubectl -n ani-model-inference-e2e-retry-20260916 get httproute smollm2-vllm -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status} {end}')"
grep -q 'Accepted=True' <<<"$route_status"
grep -q 'ResolvedRefs=True' <<<"$route_status"

response="$(curl --fail-with-body --silent --show-error --max-time 20 \
  -H 'Host: smollm2.vllm.test' \
  "http://${APISIX_NODE_IP}:30090/v1/models")"
grep -Eq '"data"|"object"|"model"' <<<"$response"
printf '%s\n' "$response"
