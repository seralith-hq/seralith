# Self-hosting Seralith Control

Seralith Control is a single static binary with an embedded SQLite database. It
runs in a container, a VM, or an air-gapped enclave with no outbound network
access.

## Requirements

| Resource | Minimum | 1,000 developers |
| --- | --- | --- |
| CPU | 1 vCPU | 2 vCPU |
| Memory | 256 MiB | 1 GiB |
| Disk | 5 GiB | 100 GiB (about 50 KiB per attested commit) |

## Quick start with Docker Compose

```sh
export SERALITH_CONTROL_BOOTSTRAP_TOKEN=$(openssl rand -hex 24)
docker compose -f deploy/compose/docker-compose.yml up -d

curl -sS http://localhost:8080/v1/bootstrap \
  -H "X-Bootstrap-Token: $SERALITH_CONTROL_BOOTSTRAP_TOKEN" \
  -d '{"org":"Acme Defense","slug":"acme","email":"owner@acme.dev"}'
```

Save the returned token, then remove the bootstrap variable and restart.

## Kubernetes

```sh
kubectl apply -f deploy/kubernetes/seralith-control.yaml
kubectl -n seralith create secret generic seralith-control \
  --from-literal=bootstrap-token="$(openssl rand -hex 24)"
```

Terminate TLS at your ingress, or pass `--tls-cert` and `--tls-key`.

## Configuration

| Flag | Environment | Default |
| --- | --- | --- |
| `--addr` | `SERALITH_CONTROL_ADDR` | `:8080` |
| `--db` | `SERALITH_CONTROL_DB` | `seralith-control.db` |
| `--tls-cert`, `--tls-key` | `SERALITH_CONTROL_TLS_CERT`, `SERALITH_CONTROL_TLS_KEY` | unset |
| `--json-logs` | `SERALITH_CONTROL_JSON_LOGS=1` | off |
| | `SERALITH_CONTROL_BOOTSTRAP_TOKEN` | unset (bootstrap disabled) |

## Backups

The database runs in WAL mode. Back it up online with:

```sh
sqlite3 /var/lib/seralith/control.db ".backup '/backups/control-$(date +%F).db'"
```

After restoring, run `GET /v1/audit/verify` to confirm the audit chain.

## Monitoring

| Endpoint | Purpose |
| --- | --- |
| `/healthz` | Liveness |
| `/readyz` | Readiness, including the database |
| `/metrics` | Prometheus: request counts and latency by route, attestations ingested |

## Air-gapped installation

1. Transfer the `seralith-control` archive and `checksums.txt` through your
   cross-domain process and verify the SHA-256.
2. Run with `--tls-cert` and `--tls-key` from your enclave PKI.
3. Distribute the `seralith` CLI the same way. The workbench UI is embedded and
   makes no external requests.
