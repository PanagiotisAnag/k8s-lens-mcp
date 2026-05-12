package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	k8sclient "github.com/pananagnostou/k8s-lens-mcp/internal/k8s"
)

func NewServer(k8s *k8sclient.Client) *server.MCPServer {
	s := server.NewMCPServer(
		"k8s-lens-mcp",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	registerTools(s, k8s)
	return s
}

func registerTools(s *server.MCPServer, k8s *k8sclient.Client) {
	// k8s_cluster_overview
	s.AddTool(mcp.NewTool("k8s_cluster_overview",
		mcp.WithDescription("General cluster snapshot: nodes, namespaces, running/failed pod counts."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		overview, err := k8s.ClusterOverview(ctx)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatClusterOverview(overview)), nil
	})

	// k8s_namespace_summary
	s.AddTool(mcp.NewTool("k8s_namespace_summary",
		mcp.WithDescription("What is running in a specific namespace: pods, deployments, services, configmaps, secrets."),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Target namespace")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, err := req.RequireString("namespace")
		if err != nil {
			return toolError(err), nil
		}
		summary, err := k8s.NamespaceSummary(ctx, ns)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatNamespaceSummary(summary)), nil
	})

	// k8s_events_list
	s.AddTool(mcp.NewTool("k8s_events_list",
		mcp.WithDescription("List Kubernetes events with optional filters for namespace, pod, severity, and time window."),
		mcp.WithString("namespace", mcp.Description("Filter by namespace (empty = all namespaces)")),
		mcp.WithString("pod_name", mcp.Description("Filter events for a specific pod")),
		mcp.WithString("severity", mcp.Description("Filter by event type: Warning or Normal")),
		mcp.WithString("since", mcp.Description("Time window, e.g. 1h, 30m, 24h (default: 1h)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of events to return (default: 50)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns := req.GetString("namespace", "")
		podName := req.GetString("pod_name", "")
		severity := req.GetString("severity", "")
		sinceStr := req.GetString("since", "1h")
		limit := int64(req.GetFloat("limit", 50))

		since, err := parseDuration(sinceStr)
		if err != nil {
			return toolError(fmt.Errorf("invalid since value %q: %w", sinceStr, err)), nil
		}

		events, err := k8s.EventsList(ctx, ns, podName, severity, since, limit)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatEvents(events)), nil
	})

	// k8s_pod_diagnose
	s.AddTool(mcp.NewTool("k8s_pod_diagnose",
		mcp.WithDescription("Correlate events, logs, and restart count for a pod to diagnose issues."),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Pod namespace")),
		mcp.WithString("pod_name", mcp.Required(), mcp.Description("Pod name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, err := req.RequireString("namespace")
		if err != nil {
			return toolError(err), nil
		}
		pod, err := req.RequireString("pod_name")
		if err != nil {
			return toolError(err), nil
		}
		diagnosis, err := k8s.PodDiagnose(ctx, ns, pod)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatPodDiagnosis(diagnosis)), nil
	})

	// k8s_crash_trace
	s.AddTool(mcp.NewTool("k8s_crash_trace",
		mcp.WithDescription("For CrashLoopBackOff pods: what crashed, when, exit code, and last logs."),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Pod namespace")),
		mcp.WithString("pod_name", mcp.Required(), mcp.Description("Pod name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, err := req.RequireString("namespace")
		if err != nil {
			return toolError(err), nil
		}
		pod, err := req.RequireString("pod_name")
		if err != nil {
			return toolError(err), nil
		}
		traces, err := k8s.CrashTrace(ctx, ns, pod)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatCrashTrace(traces)), nil
	})

	// k8s_deployment_timeline
	s.AddTool(mcp.NewTool("k8s_deployment_timeline",
		mcp.WithDescription("Timeline of rollout revisions for a deployment: image changes, replica counts, causes."),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Deployment namespace")),
		mcp.WithString("deployment", mcp.Required(), mcp.Description("Deployment name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, err := req.RequireString("namespace")
		if err != nil {
			return toolError(err), nil
		}
		dep, err := req.RequireString("deployment")
		if err != nil {
			return toolError(err), nil
		}
		entries, err := k8s.DeploymentTimeline(ctx, ns, dep)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatDeploymentTimeline(entries, dep)), nil
	})

	// k8s_diff_rollout
	s.AddTool(mcp.NewTool("k8s_diff_rollout",
		mcp.WithDescription("Compare two rollout revisions of a deployment: image and environment variable differences."),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Deployment namespace")),
		mcp.WithString("deployment", mcp.Required(), mcp.Description("Deployment name")),
		mcp.WithNumber("revision_from", mcp.Required(), mcp.Description("Source revision number")),
		mcp.WithNumber("revision_to", mcp.Required(), mcp.Description("Target revision number")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, err := req.RequireString("namespace")
		if err != nil {
			return toolError(err), nil
		}
		dep, err := req.RequireString("deployment")
		if err != nil {
			return toolError(err), nil
		}
		revA, err := req.RequireFloat("revision_from")
		if err != nil {
			return toolError(err), nil
		}
		revB, err := req.RequireFloat("revision_to")
		if err != nil {
			return toolError(err), nil
		}
		diff, err := k8s.DiffRollout(ctx, ns, dep, int64(revA), int64(revB))
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatRolloutDiff(diff)), nil
	})

	// k8s_resource_forecast
	s.AddTool(mcp.NewTool("k8s_resource_forecast",
		mcp.WithDescription("Based on current CPU/memory usage vs limits, estimate resource headroom. Requires metrics-server."),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Target namespace")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, err := req.RequireString("namespace")
		if err != nil {
			return toolError(err), nil
		}
		forecast, err := k8s.ResourceForecast(ctx, ns)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatResourceForecast(forecast)), nil
	})

	// k8s_misconfiguration_scan
	s.AddTool(mcp.NewTool("k8s_misconfiguration_scan",
		mcp.WithDescription("Scan pods for common misconfigurations: missing resource limits, missing probes, security issues."),
		mcp.WithString("namespace", mcp.Description("Namespace to scan (empty = all namespaces)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns := req.GetString("namespace", "")
		issues, err := k8s.MisconfigurationScan(ctx, ns)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatMisconfigIssues(issues, namespaceLabel(ns))), nil
	})

	// k8s_restart_history
	s.AddTool(mcp.NewTool("k8s_restart_history",
		mcp.WithDescription("Restart pattern over time for a pod: per-container restart counts, last exit codes and reasons."),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Pod namespace")),
		mcp.WithString("pod_name", mcp.Required(), mcp.Description("Pod name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns, err := req.RequireString("namespace")
		if err != nil {
			return toolError(err), nil
		}
		pod, err := req.RequireString("pod_name")
		if err != nil {
			return toolError(err), nil
		}
		history, err := k8s.RestartHistory(ctx, ns, pod)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatRestartHistory(history)), nil
	})

	// k8s_node_pressure
	s.AddTool(mcp.NewTool("k8s_node_pressure",
		mcp.WithDescription("Resource pressure per node: memory/CPU/disk pressure conditions, allocatable vs capacity, pod counts."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pressure, err := k8s.NodePressure(ctx)
		if err != nil {
			return toolError(err), nil
		}
		return toolText(formatNodePressure(pressure)), nil
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func toolText(text string) *mcp.CallToolResult {
	return mcp.NewToolResultText(text)
}

func toolError(err error) *mcp.CallToolResult {
	return mcp.NewToolResultError(err.Error())
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return time.Hour, nil
	}
	return time.ParseDuration(s)
}

func namespaceLabel(ns string) string {
	if ns == "" {
		return "all namespaces"
	}
	return ns
}
