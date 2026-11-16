#!/usr/bin/env bash
# Démo bout-en-bout : réseau -> VM -> stop/start -> suppression
# Prérequis : API lancée, jq installé. Usage : API_TOKEN=xxx ./scripts/demo.sh
set -euo pipefail
API="${API:-http://localhost:8080/api/v1}"
H=(-H "Authorization: Bearer ${API_TOKEN:-}" -H "Content-Type: application/json")

FLAVOR=$(curl -s "${H[@]}" "$API/flavors" | jq -r '.[] | select(.name=="m1.tiny") | .id')
IMAGE=$(curl -s "${H[@]}" "$API/images" | jq -r '[.[] | select(.name | test("cirros"))][0].id')
echo "flavor=$FLAVOR image=$IMAGE"

NET=$(curl -s "${H[@]}" -X POST "$API/networks" -d '{"name":"demo-net","cidr":"10.42.0.0/24"}' | jq -r .id)
echo "network=$NET"

VM=$(curl -s "${H[@]}" -X POST "$API/servers" \
  -d "{\"name\":\"demo-vm\",\"flavor_id\":\"$FLAVOR\",\"image_id\":\"$IMAGE\",\"network_id\":\"$NET\"}" | jq -r .id)
echo "server=$VM"

for _ in $(seq 1 30); do
  ST=$(curl -s "${H[@]}" "$API/servers/$VM" | jq -r .status); echo "status=$ST"
  [[ "$ST" == "ACTIVE" ]] && break; sleep 5
done

curl -s "${H[@]}" -X POST "$API/servers/$VM/stop" | jq .
sleep 10
curl -s "${H[@]}" -X POST "$API/servers/$VM/start" | jq .

read -rp "Supprimer la VM et le réseau ? [y/N] " a
if [[ "$a" == "y" ]]; then
  curl -s "${H[@]}" -X DELETE "$API/servers/$VM" -o /dev/null -w "delete server: %{http_code}\n"
  sleep 15
  curl -s "${H[@]}" -X DELETE "$API/networks/$NET" -o /dev/null -w "delete network: %{http_code}\n"
fi
