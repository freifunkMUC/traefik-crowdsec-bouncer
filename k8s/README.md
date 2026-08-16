# Kubernetes example

Example manifests for running the bouncer alongside Traefik's [Kubernetes CRD provider](https://doc.traefik.io/traefik/reference/routing-configuration/kubernetes/crd/http/middleware/). These are a starting point, not a ready-to-apply chart — adjust namespaces, `CROWDSEC_AGENT_HOST`, image tag, and the label selectors in `networkpolicy.yaml`/`middleware.yaml` to your actual cluster before applying.

Read the main [README](../README.md#security)'s Security section first — the `NetworkPolicy` here is what actually keeps the bouncer from being bypassable, not the forwardAuth secret.

## Files

- `secret.example.yaml` — template for the CrowdSec bouncer API key and the forwardAuth shared secret. Fill in real values and apply as your own Secret; don't commit the filled-in version.
- `deployment.yaml` — the bouncer itself, stream mode enabled by default, with readiness/liveness probes wired to `/api/v1/healthz` and `/api/v1/ping` respectively.
- `service.yaml` — ClusterIP service for the bouncer.
- `networkpolicy.yaml` — restricts inbound traffic to the bouncer to Traefik's pods only. **Requires a CNI that enforces NetworkPolicy** (e.g. Calico, Cilium) — verify enforcement on your cluster, some CNIs accept the resource without enforcing it.
- `middleware.yaml` — Traefik `Middleware` CRD wiring the bouncer in via forwardAuth. Reference it from your `IngressRoute`'s `spec.routes[].middlewares`.

## Order

1. `kubectl apply -f secret.example.yaml` (after filling in real values)
2. `kubectl apply -f deployment.yaml -f service.yaml -f networkpolicy.yaml -f middleware.yaml`
3. Add `crowdsec-bouncer` to the `middlewares` list of the `IngressRoute`(s) you want protected.
