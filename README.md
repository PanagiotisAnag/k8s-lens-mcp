# k8s-lens-mcp

MCP server that wraps the Kubernetes API and gives AI agents structured access for debugging and monitoring Kubernetes clusters.

Authenticates via the local `~/.kube/config` — no extra credentials needed.

## Prerequisites

- Go 1.22+
- A running Kubernetes cluster accessible via `kubectl`
- (Optional) [metrics-server](https://github.com/kubernetes-sigs/metrics-server) for `k8s_resource_forecast`

## Build & Install

```bash
cd k8s-lens-mcp
go mod tidy
go build -o k8s-lens-mcp ./cmd/k8s-lens-mcp
```

## Claude Desktop Integration

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "k8s-lens": {
      "command": "/absolute/path/to/k8s-lens-mcp"
    }
  }
}
```

## Tools

### Context

| Tool | Description |
|------|-------------|
| `k8s_cluster_overview` | Nodes, namespaces, pod phase counts |
| `k8s_namespace_summary` | Deployments, pods, services, configmaps, secrets in a namespace |

### Reactive Debugging

| Tool | Description |
|------|-------------|
| `k8s_events_list` | Events with filters: namespace, pod_name, severity, since, limit |
| `k8s_pod_diagnose` | Events + logs + restart count correlated for one pod |
| `k8s_crash_trace` | CrashLoopBackOff root-cause: exit code, reason, last logs |
| `k8s_deployment_timeline` | Revision history of a deployment with image and replica changes |
| `k8s_diff_rollout` | Side-by-side diff of two deployment revisions |

### Proactive / Monitoring

| Tool | Description |
|------|-------------|
| `k8s_resource_forecast` | CPU/memory headroom based on current usage vs limits |
| `k8s_misconfiguration_scan` | Missing resource limits, missing probes, security context issues |
| `k8s_restart_history` | Per-container restart counts and last termination reasons |
| `k8s_node_pressure` | Memory/CPU/disk pressure conditions, allocatable vs capacity |

## Tool Parameters

### `k8s_events_list`
- `namespace` — filter by namespace (omit for all)
- `pod_name` — filter for a specific pod
- `severity` — `Warning` or `Normal`
- `since` — duration string: `1h`, `30m`, `24h` (default `1h`)
- `limit` — max events returned (default `50`)

### `k8s_diff_rollout`
- `namespace`, `deployment` — target deployment
- `revision_from`, `revision_to` — revision numbers to compare

### `k8s_resource_forecast`
Requires metrics-server. Returns current CPU/memory usage vs configured limits and a headroom status.

## Output Format

All tools return structured, human-readable Markdown that AI agents can reason over directly — not raw kubectl dumps.
