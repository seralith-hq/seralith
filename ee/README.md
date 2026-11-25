# Seralith Control

Seralith Control is the organization control plane for Seralith. It is available
with Team, Enterprise, and Sovereign subscriptions and is licensed under the
[Seralith Commercial License](LICENSE).

| Capability | Description |
| --- | --- |
| Key registry | One source of truth for which human and machine keys an organization trusts, with roles and revocation. CI pulls it as `trust.yaml`. |
| Policy distribution | Versioned, validated policies per project. Developers and CI pull the published version. |
| Attestation archive | Ingests DSSE envelopes, verifies them against the registry, and re-evaluates them under the current policy. |
| Analytics | Human vs. agent lines, usage by tool and declared model, and commits awaiting approval. |
| Audit trail | A hash-chained, verifiable record of every administrative action. |
| Operations | `/healthz`, `/readyz`, Prometheus `/metrics`, structured logs, and graceful shutdown. |

## Running

```sh
export SERALITH_CONTROL_BOOTSTRAP_TOKEN=$(openssl rand -hex 24)
seralith-control --addr :8080 --db /var/lib/seralith/control.db \
  --tls-cert /etc/seralith/tls.crt --tls-key /etc/seralith/tls.key --json-logs

curl -sS https://control.example.com/v1/bootstrap \
  -H "X-Bootstrap-Token: $SERALITH_CONTROL_BOOTSTRAP_TOKEN" \
  -d '{"org":"Acme Defense","slug":"acme","email":"owner@acme.dev"}'
```

Bootstrap works exactly once, and only with the operator-supplied secret.
Unset the variable afterward.

## Roles

| Role | Read | Upload attestations | Manage keys, members, policy | Read audit trail |
| --- | --- | --- | --- | --- |
| owner, admin | Yes | Yes | Yes | Yes |
| member | Yes | Yes | | |
| ci | Yes | Yes | | |
| auditor | Yes | | | Yes |

## API

| Method | Path | Role |
| --- | --- | --- |
| `POST` | `/v1/bootstrap` | bootstrap secret |
| `GET` | `/v1/me` | any |
| `GET` `POST` | `/v1/tokens`, `DELETE /v1/tokens/{id}` | any (own tokens) |
| `GET` `POST` | `/v1/members` | admin |
| `GET` / `POST` | `/v1/projects` | any / admin |
| `GET` / `POST` | `/v1/keys`, `POST /v1/keys/{id}/revoke` | any / admin |
| `GET` | `/v1/trust.yaml` | any |
| `GET` / `PUT` | `/v1/projects/{project}/policy`, `GET .../policy.yaml` | any / admin |
| `POST` | `/v1/projects/{project}/attestations` | writer |
| `GET` | `/v1/projects/{project}/attestations?commit=&type=&envelope=true` | any |
| `GET` | `/v1/projects/{project}/stats` | any |
| `GET` | `/v1/audit`, `/v1/audit/verify` | admin, auditor |
