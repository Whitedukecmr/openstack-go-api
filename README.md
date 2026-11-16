# openstack-go-api

[![ci](https://github.com/Whitedukecmr/openstack-go-api/actions/workflows/ci.yml/badge.svg)](https://github.com/Whitedukecmr/openstack-go-api/actions/workflows/ci.yml)

API REST en **Go** pour piloter un cloud **OpenStack** (Nova, Neutron, Glance, Cinder), testée sur un lab **DevStack** all-in-one Ubuntu 22.04.

- Go 1.24, `net/http` standard (routage Go 1.22+), [gophercloud v2](https://github.com/gophercloud/gophercloud)
- Auth Keystone via variables `OS_*` (openrc), API protégée par Bearer token
- Logs JSON (`log/slog`), arrêt propre (SIGTERM), image Docker distroless non-root
- Couche `cloud.Cloud` (interface), donc les handlers sont testés sans OpenStack
- CI GitHub Actions : vet, gofmt, tests `-race`, golangci-lint, push d'image sur GHCR

```
client ──HTTP/JSON──▶ osapi (Go) ──gophercloud──▶ Keystone ─▶ Nova / Neutron / Glance / Cinder
                                                       (DevStack Ubuntu 22.04)
```

## 1. Lab DevStack

**VM recommandée** : Ubuntu 22.04 Server, 4 vCPU, **8 Go RAM minimum** (16 Go confortable), 60 Go disque, virtualisation imbriquée activée (sinon Nova bascule en QEMU, plus lent mais fonctionnel).

| RAM | Profil |
|-----|--------|
| 8 Go | `local.conf` tel quel : sans Horizon, 1 worker API, swap 4 Go |
| 16 Go+ | décommenter `enable_service horizon` et `API_WORKERS=2` |

```bash
git clone https://github.com/Whitedukecmr/openstack-go-api.git && cd openstack-go-api
sudo ./devstack/install.sh        # ~30-45 min
```

> DevStack `master` ne teste plus Ubuntu 22.04 : le script fixe `stable/2025.2`. Pour une autre branche, lance `sudo DEVSTACK_BRANCH=... ./devstack/install.sh`.

Vérification :

```bash
sudo -iu stack
source /opt/stack/devstack/openrc admin admin
openstack service list && openstack image list && openstack flavor list
```

## 2. Lancer l'API

```bash
source /opt/stack/devstack/openrc admin admin
export API_TOKEN=$(openssl rand -hex 16)
make run                           # écoute sur :8080
```

Avec Docker :

```bash
docker build -t osapi .
docker run --rm -p 8080:8080 --network host \
  -e API_TOKEN -e OS_AUTH_URL -e OS_USERNAME -e OS_PASSWORD -e OS_PROJECT_NAME \
  -e OS_USER_DOMAIN_NAME -e OS_PROJECT_DOMAIN_NAME -e OS_REGION_NAME osapi
```

## 3. Endpoints

Toutes les routes `/api/v1/*` exigent `Authorization: Bearer $API_TOKEN` si `API_TOKEN` est défini.

| Méthode | Route | Description |
|---|---|---|
| GET | `/healthz` | Liveness (sans auth) |
| GET | `/api/v1/servers` | Lister les VM |
| POST | `/api/v1/servers` | Créer une VM `{name, flavor_id, image_id, network_id, key_name?}` |
| GET | `/api/v1/servers/{id}` | Détail |
| DELETE | `/api/v1/servers/{id}` | Supprimer |
| POST | `/api/v1/servers/{id}/{start\|stop\|reboot}` | Actions |
| GET | `/api/v1/flavors` | Gabarits |
| GET | `/api/v1/images` | Images Glance |
| GET / POST | `/api/v1/networks` | Lister / créer réseau + sous-réseau `{name, cidr}` |
| DELETE | `/api/v1/networks/{id}` | Supprimer |
| GET / POST | `/api/v1/volumes` | Lister / créer `{name, size_gb}` |
| DELETE | `/api/v1/volumes/{id}` | Supprimer |

Exemple :

```bash
H="Authorization: Bearer $API_TOKEN"
curl -s -H "$H" localhost:8080/api/v1/flavors | jq
curl -s -H "$H" -X POST localhost:8080/api/v1/networks -d '{"name":"web-net","cidr":"10.10.0.0/24"}' | jq
```

Démo bout-en-bout (réseau, VM, stop/start, suppression) : `./scripts/demo.sh`

Codes de retour : `200/201/202/204` OK · `400` validation · `401` token · `404` ressource absente · `502` erreur OpenStack.

## 4. Dev

```bash
make test     # tests unitaires avec fake Cloud, -race
make lint
```

Arborescence :

```
cmd/osapi/            point d'entrée (config, graceful shutdown)
internal/cloud/       modèle + interface Cloud + implémentation gophercloud
internal/api/         handlers HTTP, middleware auth/logs, tests
devstack/             local.conf + script d'installation
scripts/demo.sh       scénario curl complet
```

## Roadmap

- [ ] Spec OpenAPI 3 + Swagger UI
- [ ] Floating IPs et security groups
- [ ] Tests d'intégration contre DevStack dans la CI (self-hosted runner)
- [ ] Métriques Prometheus `/metrics`

## Licence

MIT
