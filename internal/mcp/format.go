package mcp

import (
	"fmt"
	"strings"

	k8s "github.com/pananagnostou/k8s-lens-mcp/internal/k8s"
)

func formatClusterOverview(o *k8s.ClusterOverview) string {
	var b strings.Builder
	b.WriteString("## Cluster Overview\n\n")

	b.WriteString("### Nodes\n")
	for _, n := range o.Nodes {
		b.WriteString(fmt.Sprintf("- **%s** [%s] roles=%s age=%s kubelet=%s\n",
			n.Name, n.Status, strings.Join(n.Roles, ","), n.Age, n.KubeVersion))
	}

	b.WriteString("\n### Namespaces\n")
	b.WriteString(strings.Join(o.Namespaces, ", ") + "\n")

	b.WriteString("\n### Pod Summary\n")
	b.WriteString(fmt.Sprintf("Total: %d | Running: %d | Pending: %d | Failed: %d | Succeeded: %d | Unknown: %d\n",
		o.PodCounts.Total, o.PodCounts.Running, o.PodCounts.Pending,
		o.PodCounts.Failed, o.PodCounts.Succeeded, o.PodCounts.Unknown))

	return b.String()
}

func formatNamespaceSummary(s *k8s.NamespaceSummary) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Namespace: %s\n\n", s.Namespace))

	b.WriteString("### Deployments\n")
	if len(s.Deployments) == 0 {
		b.WriteString("(none)\n")
	}
	for _, d := range s.Deployments {
		b.WriteString(fmt.Sprintf("- **%s** ready=%s upToDate=%d available=%d age=%s\n",
			d.Name, d.Ready, d.UpToDate, d.Available, d.Age))
	}

	b.WriteString("\n### Pods\n")
	for _, p := range s.Pods {
		restartNote := ""
		if p.Restarts > 0 {
			restartNote = fmt.Sprintf(" ⚠ restarts=%d", p.Restarts)
		}
		b.WriteString(fmt.Sprintf("- **%s** [%s] node=%s age=%s%s\n",
			p.Name, p.Status, p.Node, p.Age, restartNote))
	}

	b.WriteString("\n### Services\n")
	b.WriteString(strings.Join(s.Services, ", ") + "\n")

	b.WriteString(fmt.Sprintf("\n### Resources\nConfigMaps: %d | Secrets: %d\n", s.ConfigMaps, s.Secrets))

	return b.String()
}

func formatEvents(events []k8s.EventEntry) string {
	if len(events) == 0 {
		return "No events found matching the given filters.\n"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Events (%d)\n\n", len(events)))
	for _, e := range events {
		icon := "ℹ"
		if e.Severity == "Warning" {
			icon = "⚠"
		}
		b.WriteString(fmt.Sprintf("%s [%s] %s | %s | ns=%s | count=%d\n   %s\n",
			icon, e.Time, e.Object, e.Reason, e.Namespace, e.Count, e.Message))
	}
	return b.String()
}

func formatPodDiagnosis(d *k8s.PodDiagnosis) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Pod Diagnosis: %s/%s\n\n", d.Namespace, d.PodName))
	b.WriteString(fmt.Sprintf("Phase: **%s** | Node: %s | Total Restarts: **%d**\n\n", d.Phase, d.Node, d.TotalRestarts))

	b.WriteString("### Containers\n")
	for _, c := range d.Containers {
		readyIcon := "✓"
		if !c.Ready {
			readyIcon = "✗"
		}
		stateStr := c.State
		if c.StateReason != "" {
			stateStr += " (" + c.StateReason + ")"
		}
		b.WriteString(fmt.Sprintf("- %s **%s** [%s] restarts=%d image=%s\n",
			readyIcon, c.Name, stateStr, c.Restarts, c.Image))
	}

	if len(d.RecentEvents) > 0 {
		b.WriteString("\n### Recent Events\n")
		for _, e := range d.RecentEvents {
			icon := "ℹ"
			if e.Severity == "Warning" {
				icon = "⚠"
			}
			b.WriteString(fmt.Sprintf("%s [%s] %s: %s\n", icon, e.Time, e.Reason, e.Message))
		}
	}

	if len(d.RecentLogs) > 0 {
		b.WriteString("\n### Recent Logs (tail 50)\n")
		for name, logs := range d.RecentLogs {
			b.WriteString(fmt.Sprintf("\n**Container: %s**\n```\n%s\n```\n", name, trimLogs(logs, 50)))
		}
	}

	return b.String()
}

func formatCrashTrace(traces []k8s.CrashTrace) string {
	var b strings.Builder
	b.WriteString("## Crash Trace\n\n")
	for _, t := range traces {
		b.WriteString(fmt.Sprintf("### Container: %s\n", t.Container))
		b.WriteString(fmt.Sprintf("Pod: %s/%s\n", t.Namespace, t.PodName))
		b.WriteString(fmt.Sprintf("Crash Reason: **%s** | Exit Code: **%d** | Restarts: **%d**\n",
			t.CrashReason, t.ExitCode, t.RestartCount))
		b.WriteString(fmt.Sprintf("Last Started: %s | Last Finished: %s\n\n", t.LastStarted, t.LastFinished))

		if t.LastLogs != "" {
			b.WriteString("**Last Logs (tail 100)**\n```\n")
			b.WriteString(trimLogs(t.LastLogs, 100))
			b.WriteString("\n```\n\n")
		}

		if len(t.Events) > 0 {
			b.WriteString("**Related Events**\n")
			for _, e := range t.Events {
				icon := "ℹ"
				if e.Severity == "Warning" {
					icon = "⚠"
				}
				b.WriteString(fmt.Sprintf("%s [%s] %s: %s\n", icon, e.Time, e.Reason, e.Message))
			}
		}
	}
	return b.String()
}

func formatDeploymentTimeline(entries []k8s.DeploymentTimelineEntry, deployment string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Deployment Timeline: %s\n\n", deployment))
	if len(entries) == 0 {
		b.WriteString("No rollout history found.\n")
		return b.String()
	}
	b.WriteString("Rev | Age        | Replicas | Image(s)                          | Cause\n")
	b.WriteString("----|------------|----------|-----------------------------------|---------\n")
	for _, e := range entries {
		b.WriteString(fmt.Sprintf("%-4d| %-10s | %-8d | %-33s | %s\n",
			e.Revision, e.ChangedAt, e.Replicas, truncate(e.Image, 33), e.Cause))
	}
	return b.String()
}

func formatRolloutDiff(d *k8s.RolloutDiff) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Rollout Diff: %s/%s  (rev %d → rev %d)\n\n",
		d.Namespace, d.Deployment, d.From.Revision, d.To.Revision))

	b.WriteString("### Image Changes\n")
	if len(d.ImageDiffs) == 0 {
		b.WriteString("No image changes.\n")
	}
	for _, diff := range d.ImageDiffs {
		b.WriteString("- " + diff + "\n")
	}

	b.WriteString("\n### Environment Changes\n")
	if len(d.EnvDiffs) == 0 {
		b.WriteString("No environment changes.\n")
	}
	for _, diff := range d.EnvDiffs {
		b.WriteString("- " + diff + "\n")
	}

	b.WriteString(fmt.Sprintf("\nFrom rev %d: %s (%d replicas)\n", d.From.Revision, d.From.Image, d.From.Replicas))
	b.WriteString(fmt.Sprintf("To   rev %d: %s (%d replicas)\n", d.To.Revision, d.To.Image, d.To.Replicas))

	return b.String()
}

func formatResourceForecast(f *k8s.ResourceForecast) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Resource Forecast: namespace/%s\n\n", f.Namespace))

	b.WriteString("### CPU\n")
	b.WriteString(fmt.Sprintf("Usage: %s / %s (%.1f%%)\n", f.CPU.CurrentUsage, f.CPU.Limit, f.CPU.UsagePercent))
	b.WriteString(fmt.Sprintf("Status: %s\n\n", f.CPU.EstimatedExhaust))

	b.WriteString("### Memory\n")
	b.WriteString(fmt.Sprintf("Usage: %s / %s (%.1f%%)\n", f.Memory.CurrentUsage, f.Memory.Limit, f.Memory.UsagePercent))
	b.WriteString(fmt.Sprintf("Status: %s\n", f.Memory.EstimatedExhaust))

	return b.String()
}

func formatMisconfigIssues(issues []k8s.MisconfigIssue, namespace string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Misconfiguration Scan: %s\n\n", namespace))
	if len(issues) == 0 {
		b.WriteString("✓ No misconfigurations found.\n")
		return b.String()
	}
	b.WriteString(fmt.Sprintf("Found %d issue(s):\n\n", len(issues)))
	for _, i := range issues {
		icon := "ℹ"
		if i.Severity == "WARNING" {
			icon = "⚠"
		} else if i.Severity == "CRITICAL" {
			icon = "✗"
		}
		b.WriteString(fmt.Sprintf("%s [%s] %s — %s\n", icon, i.Severity, i.Resource, i.Issue))
	}
	return b.String()
}

func formatRestartHistory(h *k8s.RestartHistory) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Restart History: %s/%s\n\n", h.Namespace, h.PodName))
	for _, c := range h.Containers {
		b.WriteString(fmt.Sprintf("### Container: %s\n", c.Name))
		b.WriteString(fmt.Sprintf("Total Restarts: **%d**\n", c.RestartCount))
		if c.LastState != "" {
			b.WriteString(fmt.Sprintf("Last Exit: reason=%s code=%d at=%s\n",
				c.LastState, c.LastExitCode, c.LastFinished))
		} else {
			b.WriteString("No recorded previous termination.\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func formatNodePressure(nodes []k8s.NodePressure) string {
	var b strings.Builder
	b.WriteString("## Node Pressure Report\n\n")
	for _, n := range nodes {
		readyStr := "Ready"
		if !n.Ready {
			readyStr = "NOT READY"
		}
		b.WriteString(fmt.Sprintf("### %s [%s]\n", n.Name, readyStr))

		pressures := []string{}
		if n.MemoryPressure {
			pressures = append(pressures, "MemoryPressure")
		}
		if n.DiskPressure {
			pressures = append(pressures, "DiskPressure")
		}
		if n.PIDPressure {
			pressures = append(pressures, "PIDPressure")
		}
		if n.Unschedulable {
			pressures = append(pressures, "Unschedulable")
		}
		if len(pressures) > 0 {
			b.WriteString(fmt.Sprintf("⚠ Active Pressures: %s\n", strings.Join(pressures, ", ")))
		} else {
			b.WriteString("✓ No active pressures\n")
		}

		b.WriteString(fmt.Sprintf("CPU: %s allocatable / %s capacity\n",
			n.Allocatable.CPU, n.Capacity.CPU))
		b.WriteString(fmt.Sprintf("Memory: %s allocatable / %s capacity\n",
			n.Allocatable.Memory, n.Capacity.Memory))
		b.WriteString(fmt.Sprintf("Pods: %d running / %d max\n\n", n.PodCount, n.MaxPods))
	}
	return b.String()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func trimLogs(logs string, maxLines int) string {
	lines := strings.Split(logs, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
