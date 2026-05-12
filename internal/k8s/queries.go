package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ─── Cluster Overview ────────────────────────────────────────────────────────

type ClusterOverview struct {
	Nodes      []NodeSummary
	Namespaces []string
	PodCounts  PodCounts
}

type NodeSummary struct {
	Name       string
	Status     string
	Roles      []string
	Age        string
	KubeVersion string
}

type PodCounts struct {
	Running   int
	Pending   int
	Failed    int
	Succeeded int
	Unknown   int
	Total     int
}

func (c *Client) ClusterOverview(ctx context.Context) (*ClusterOverview, error) {
	nodes, err := c.Kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	nsList, err := c.Kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces: %w", err)
	}

	pods, err := c.Kube.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	var nodeSummaries []NodeSummary
	for _, n := range nodes.Items {
		status := "NotReady"
		for _, cond := range n.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				status = "Ready"
			}
		}
		var roles []string
		for k := range n.Labels {
			if strings.HasPrefix(k, "node-role.kubernetes.io/") {
				roles = append(roles, strings.TrimPrefix(k, "node-role.kubernetes.io/"))
			}
		}
		if len(roles) == 0 {
			roles = []string{"worker"}
		}
		nodeSummaries = append(nodeSummaries, NodeSummary{
			Name:        n.Name,
			Status:      status,
			Roles:       roles,
			Age:         age(n.CreationTimestamp.Time),
			KubeVersion: n.Status.NodeInfo.KubeletVersion,
		})
	}

	var namespaces []string
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}

	var counts PodCounts
	for _, p := range pods.Items {
		counts.Total++
		switch p.Status.Phase {
		case corev1.PodRunning:
			counts.Running++
		case corev1.PodPending:
			counts.Pending++
		case corev1.PodFailed:
			counts.Failed++
		case corev1.PodSucceeded:
			counts.Succeeded++
		default:
			counts.Unknown++
		}
	}

	return &ClusterOverview{
		Nodes:      nodeSummaries,
		Namespaces: namespaces,
		PodCounts:  counts,
	}, nil
}

// ─── Namespace Summary ────────────────────────────────────────────────────────

type NamespaceSummary struct {
	Namespace    string
	Pods         []PodBrief
	Deployments  []DeploymentBrief
	Services     []string
	ConfigMaps   int
	Secrets      int
}

type PodBrief struct {
	Name     string
	Status   string
	Restarts int32
	Age      string
	Node     string
}

type DeploymentBrief struct {
	Name      string
	Ready     string
	UpToDate  int32
	Available int32
	Age       string
}

func (c *Client) NamespaceSummary(ctx context.Context, namespace string) (*NamespaceSummary, error) {
	pods, err := c.Kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	deps, err := c.Kube.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	svcs, err := c.Kube.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	cms, err := c.Kube.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing configmaps: %w", err)
	}

	secrets, err := c.Kube.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	var podBriefs []PodBrief
	for _, p := range pods.Items {
		restarts := int32(0)
		for _, cs := range p.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}
		podBriefs = append(podBriefs, PodBrief{
			Name:     p.Name,
			Status:   string(p.Status.Phase),
			Restarts: restarts,
			Age:      age(p.CreationTimestamp.Time),
			Node:     p.Spec.NodeName,
		})
	}

	var depBriefs []DeploymentBrief
	for _, d := range deps.Items {
		ready := fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, d.Status.Replicas)
		depBriefs = append(depBriefs, DeploymentBrief{
			Name:      d.Name,
			Ready:     ready,
			UpToDate:  d.Status.UpdatedReplicas,
			Available: d.Status.AvailableReplicas,
			Age:       age(d.CreationTimestamp.Time),
		})
	}

	var svcNames []string
	for _, s := range svcs.Items {
		svcNames = append(svcNames, s.Name)
	}

	return &NamespaceSummary{
		Namespace:   namespace,
		Pods:        podBriefs,
		Deployments: depBriefs,
		Services:    svcNames,
		ConfigMaps:  len(cms.Items),
		Secrets:     len(secrets.Items),
	}, nil
}

// ─── Events List ─────────────────────────────────────────────────────────────

type EventEntry struct {
	Time      string
	Severity  string
	Reason    string
	Object    string
	Namespace string
	Message   string
	Count     int32
}

func (c *Client) EventsList(ctx context.Context, namespace, podName, severity string, since time.Duration, limit int64) ([]EventEntry, error) {
	ns := namespace
	if ns == "" {
		ns = ""
	}

	opts := metav1.ListOptions{}
	if limit > 0 {
		opts.Limit = limit
	}

	events, err := c.Kube.CoreV1().Events(ns).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	cutoff := time.Now().Add(-since)
	var result []EventEntry
	for _, e := range events.Items {
		t := e.LastTimestamp.Time
		if t.IsZero() {
			t = e.EventTime.Time
		}
		if since > 0 && t.Before(cutoff) {
			continue
		}
		if severity != "" && !strings.EqualFold(e.Type, severity) {
			continue
		}
		if podName != "" && e.InvolvedObject.Name != podName {
			continue
		}
		result = append(result, EventEntry{
			Time:      t.Format(time.RFC3339),
			Severity:  e.Type,
			Reason:    e.Reason,
			Object:    fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
			Namespace: e.Namespace,
			Message:   e.Message,
			Count:     e.Count,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time > result[j].Time
	})

	return result, nil
}

// ─── Pod Diagnose ─────────────────────────────────────────────────────────────

type PodDiagnosis struct {
	PodName       string
	Namespace     string
	Phase         string
	Node          string
	TotalRestarts int32
	Containers    []ContainerDiagnosis
	RecentEvents  []EventEntry
	RecentLogs    map[string]string
}

type ContainerDiagnosis struct {
	Name         string
	Ready        bool
	Restarts     int32
	State        string
	StateReason  string
	Image        string
}

func (c *Client) PodDiagnose(ctx context.Context, namespace, podName string) (*PodDiagnosis, error) {
	pod, err := c.Kube.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod: %w", err)
	}

	var containers []ContainerDiagnosis
	totalRestarts := int32(0)
	for _, cs := range pod.Status.ContainerStatuses {
		totalRestarts += cs.RestartCount
		state, reason := containerStateStr(cs.State)
		containers = append(containers, ContainerDiagnosis{
			Name:        cs.Name,
			Ready:       cs.Ready,
			Restarts:    cs.RestartCount,
			State:       state,
			StateReason: reason,
			Image:       cs.Image,
		})
	}

	events, _ := c.EventsList(ctx, namespace, podName, "", time.Hour, 20)

	logs := make(map[string]string)
	for _, cs := range pod.Spec.Containers {
		logBytes, err := c.Kube.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
			Container: cs.Name,
			TailLines: int64ptr(50),
		}).DoRaw(ctx)
		if err == nil {
			logs[cs.Name] = string(logBytes)
		}
	}

	return &PodDiagnosis{
		PodName:       podName,
		Namespace:     namespace,
		Phase:         string(pod.Status.Phase),
		Node:          pod.Spec.NodeName,
		TotalRestarts: totalRestarts,
		Containers:    containers,
		RecentEvents:  events,
		RecentLogs:    logs,
	}, nil
}

// ─── Crash Trace ─────────────────────────────────────────────────────────────

type CrashTrace struct {
	PodName       string
	Namespace     string
	Container     string
	CrashReason   string
	RestartCount  int32
	LastStarted   string
	LastFinished  string
	ExitCode      int32
	LastLogs      string
	Events        []EventEntry
}

func (c *Client) CrashTrace(ctx context.Context, namespace, podName string) ([]CrashTrace, error) {
	pod, err := c.Kube.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod: %w", err)
	}

	events, _ := c.EventsList(ctx, namespace, podName, "", 24*time.Hour, 30)

	var traces []CrashTrace
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.LastTerminationState.Terminated == nil && cs.State.Terminated == nil {
			continue
		}

		term := cs.State.Terminated
		if term == nil {
			term = cs.LastTerminationState.Terminated
		}

		logBytes, _ := c.Kube.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
			Container: cs.Name,
			TailLines: int64ptr(100),
			Previous:  cs.State.Terminated == nil,
		}).DoRaw(ctx)

		traces = append(traces, CrashTrace{
			PodName:      podName,
			Namespace:    namespace,
			Container:    cs.Name,
			CrashReason:  term.Reason,
			RestartCount: cs.RestartCount,
			LastStarted:  term.StartedAt.Format(time.RFC3339),
			LastFinished: term.FinishedAt.Format(time.RFC3339),
			ExitCode:     term.ExitCode,
			LastLogs:     string(logBytes),
			Events:       events,
		})
	}

	if len(traces) == 0 {
		return nil, fmt.Errorf("no crash data found for pod %s (not in crash state)", podName)
	}

	return traces, nil
}

// ─── Deployment Timeline ──────────────────────────────────────────────────────

type DeploymentTimelineEntry struct {
	Revision    int64
	ChangedAt   string
	Image       string
	Replicas    int32
	Cause       string
}

func (c *Client) DeploymentTimeline(ctx context.Context, namespace, deploymentName string) ([]DeploymentTimelineEntry, error) {
	rsList, err := c.Kube.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing replicasets: %w", err)
	}

	dep, err := c.Kube.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting deployment: %w", err)
	}

	depSelector := labels.Set(dep.Spec.Selector.MatchLabels)

	var entries []DeploymentTimelineEntry
	for _, rs := range rsList.Items {
		if !depSelector.AsSelector().Matches(labels.Set(rs.Labels)) {
			continue
		}

		revStr := rs.Annotations["deployment.kubernetes.io/revision"]
		rev := int64(0)
		fmt.Sscanf(revStr, "%d", &rev)

		images := []string{}
		for _, c := range rs.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}

		cause := rs.Annotations["kubernetes.io/change-cause"]
		if cause == "" {
			cause = "—"
		}

		entries = append(entries, DeploymentTimelineEntry{
			Revision:  rev,
			ChangedAt: age(rs.CreationTimestamp.Time),
			Image:     strings.Join(images, ", "),
			Replicas:  *rs.Spec.Replicas,
			Cause:     cause,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Revision > entries[j].Revision
	})

	return entries, nil
}

// ─── Diff Rollout ─────────────────────────────────────────────────────────────

type RolloutDiff struct {
	Deployment string
	Namespace  string
	From       RolloutRevision
	To         RolloutRevision
	ImageDiffs []string
	EnvDiffs   []string
}

type RolloutRevision struct {
	Revision int64
	Image    string
	Replicas int32
}

func (c *Client) DiffRollout(ctx context.Context, namespace, deploymentName string, revA, revB int64) (*RolloutDiff, error) {
	rsList, err := c.Kube.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing replicasets: %w", err)
	}

	dep, err := c.Kube.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting deployment: %w", err)
	}

	depSelector := labels.Set(dep.Spec.Selector.MatchLabels)

	revMap := map[int64]*corev1.PodSpec{}
	repMap := map[int64]int32{}
	for _, rs := range rsList.Items {
		if !depSelector.AsSelector().Matches(labels.Set(rs.Labels)) {
			continue
		}
		revStr := rs.Annotations["deployment.kubernetes.io/revision"]
		rev := int64(0)
		fmt.Sscanf(revStr, "%d", &rev)
		tpl := rs.Spec.Template.DeepCopy()
		revMap[rev] = &tpl.Spec
		repMap[rev] = *rs.Spec.Replicas
	}

	tplA, ok := revMap[revA]
	if !ok {
		return nil, fmt.Errorf("revision %d not found", revA)
	}
	tplB, ok := revMap[revB]
	if !ok {
		return nil, fmt.Errorf("revision %d not found", revB)
	}

	imagesA := containerImages(tplA)
	imagesB := containerImages(tplB)

	var imageDiffs []string
	for name, imgA := range imagesA {
		imgB := imagesB[name]
		if imgA != imgB {
			imageDiffs = append(imageDiffs, fmt.Sprintf("%s: %s → %s", name, imgA, imgB))
		}
	}

	var envDiffs []string
	envA := containerEnv(tplA)
	envB := containerEnv(tplB)
	for k, vA := range envA {
		if vB, ok := envB[k]; ok && vA != vB {
			envDiffs = append(envDiffs, fmt.Sprintf("%s: %q → %q", k, vA, vB))
		} else if !ok {
			envDiffs = append(envDiffs, fmt.Sprintf("%s: %q → (removed)", k, vA))
		}
	}
	for k, vB := range envB {
		if _, ok := envA[k]; !ok {
			envDiffs = append(envDiffs, fmt.Sprintf("%s: (added) → %q", k, vB))
		}
	}

	return &RolloutDiff{
		Deployment: deploymentName,
		Namespace:  namespace,
		From:       RolloutRevision{Revision: revA, Image: strings.Join(imagesList(imagesA), ", "), Replicas: repMap[revA]},
		To:         RolloutRevision{Revision: revB, Image: strings.Join(imagesList(imagesB), ", "), Replicas: repMap[revB]},
		ImageDiffs: imageDiffs,
		EnvDiffs:   envDiffs,
	}, nil
}

// ─── Resource Forecast ────────────────────────────────────────────────────────

type ResourceForecast struct {
	Namespace      string
	CPU            ResourceForecastItem
	Memory         ResourceForecastItem
}

type ResourceForecastItem struct {
	CurrentUsage   string
	Limit          string
	UsagePercent   float64
	EstimatedExhaust string
}

func (c *Client) ResourceForecast(ctx context.Context, namespace string) (*ResourceForecast, error) {
	if c.Metrics == nil {
		return nil, fmt.Errorf("metrics-server not available in this cluster")
	}

	podMetrics, err := c.Metrics.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod metrics: %w", err)
	}

	pods, err := c.Kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	totalCPUUsage := int64(0)
	totalMemUsage := int64(0)
	for _, pm := range podMetrics.Items {
		for _, c := range pm.Containers {
			totalCPUUsage += c.Usage.Cpu().MilliValue()
			totalMemUsage += c.Usage.Memory().Value()
		}
	}

	totalCPULimit := int64(0)
	totalMemLimit := int64(0)
	for _, p := range pods.Items {
		for _, c := range p.Spec.Containers {
			if !c.Resources.Limits.Cpu().IsZero() {
				totalCPULimit += c.Resources.Limits.Cpu().MilliValue()
			}
			if !c.Resources.Limits.Memory().IsZero() {
				totalMemLimit += c.Resources.Limits.Memory().Value()
			}
		}
	}

	cpuPct := 0.0
	if totalCPULimit > 0 {
		cpuPct = float64(totalCPUUsage) / float64(totalCPULimit) * 100
	}
	memPct := 0.0
	if totalMemLimit > 0 {
		memPct = float64(totalMemUsage) / float64(totalMemLimit) * 100
	}

	return &ResourceForecast{
		Namespace: namespace,
		CPU: ResourceForecastItem{
			CurrentUsage:    fmt.Sprintf("%dm", totalCPUUsage),
			Limit:           fmt.Sprintf("%dm", totalCPULimit),
			UsagePercent:    cpuPct,
			EstimatedExhaust: exhaustETA(cpuPct),
		},
		Memory: ResourceForecastItem{
			CurrentUsage:    fmt.Sprintf("%dMi", totalMemUsage/1024/1024),
			Limit:           fmt.Sprintf("%dMi", totalMemLimit/1024/1024),
			UsagePercent:    memPct,
			EstimatedExhaust: exhaustETA(memPct),
		},
	}, nil
}

// ─── Misconfiguration Scan ────────────────────────────────────────────────────

type MisconfigIssue struct {
	Severity  string
	Resource  string
	Namespace string
	Issue     string
}

func (c *Client) MisconfigurationScan(ctx context.Context, namespace string) ([]MisconfigIssue, error) {
	pods, err := c.Kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	var issues []MisconfigIssue
	for _, p := range pods.Items {
		for _, ct := range p.Spec.Containers {
			ref := fmt.Sprintf("pod/%s container/%s", p.Name, ct.Name)
			ns := p.Namespace

			if ct.Resources.Requests == nil || ct.Resources.Requests.Cpu().IsZero() {
				issues = append(issues, MisconfigIssue{"WARNING", ref, ns, "Missing CPU request"})
			}
			if ct.Resources.Requests == nil || ct.Resources.Requests.Memory().IsZero() {
				issues = append(issues, MisconfigIssue{"WARNING", ref, ns, "Missing memory request"})
			}
			if ct.Resources.Limits == nil || ct.Resources.Limits.Cpu().IsZero() {
				issues = append(issues, MisconfigIssue{"WARNING", ref, ns, "Missing CPU limit"})
			}
			if ct.Resources.Limits == nil || ct.Resources.Limits.Memory().IsZero() {
				issues = append(issues, MisconfigIssue{"WARNING", ref, ns, "Missing memory limit"})
			}
			if ct.LivenessProbe == nil {
				issues = append(issues, MisconfigIssue{"INFO", ref, ns, "Missing liveness probe"})
			}
			if ct.ReadinessProbe == nil {
				issues = append(issues, MisconfigIssue{"INFO", ref, ns, "Missing readiness probe"})
			}
			if ct.SecurityContext == nil || ct.SecurityContext.RunAsNonRoot == nil || !*ct.SecurityContext.RunAsNonRoot {
				issues = append(issues, MisconfigIssue{"WARNING", ref, ns, "Container may run as root (runAsNonRoot not set)"})
			}
			if ct.SecurityContext == nil || ct.SecurityContext.ReadOnlyRootFilesystem == nil || !*ct.SecurityContext.ReadOnlyRootFilesystem {
				issues = append(issues, MisconfigIssue{"INFO", ref, ns, "Root filesystem is writable"})
			}
		}
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity < issues[j].Severity
		}
		return issues[i].Resource < issues[j].Resource
	})

	return issues, nil
}

// ─── Restart History ──────────────────────────────────────────────────────────

type RestartHistory struct {
	PodName   string
	Namespace string
	Containers []ContainerRestartInfo
}

type ContainerRestartInfo struct {
	Name         string
	RestartCount int32
	LastState    string
	LastExitCode int32
	LastFinished string
}

func (c *Client) RestartHistory(ctx context.Context, namespace, podName string) (*RestartHistory, error) {
	pod, err := c.Kube.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod: %w", err)
	}

	var containers []ContainerRestartInfo
	for _, cs := range pod.Status.ContainerStatuses {
		info := ContainerRestartInfo{
			Name:         cs.Name,
			RestartCount: cs.RestartCount,
		}

		if cs.LastTerminationState.Terminated != nil {
			t := cs.LastTerminationState.Terminated
			info.LastState = t.Reason
			info.LastExitCode = t.ExitCode
			info.LastFinished = t.FinishedAt.Format(time.RFC3339)
		}

		containers = append(containers, info)
	}

	return &RestartHistory{
		PodName:    podName,
		Namespace:  namespace,
		Containers: containers,
	}, nil
}

// ─── Node Pressure ────────────────────────────────────────────────────────────

type NodePressure struct {
	Name           string
	Ready          bool
	MemoryPressure bool
	DiskPressure   bool
	PIDPressure    bool
	Unschedulable  bool
	Allocatable    ResourceInfo
	Capacity       ResourceInfo
	PodCount       int
	MaxPods        int64
}

type ResourceInfo struct {
	CPU    string
	Memory string
}

func (c *Client) NodePressure(ctx context.Context) ([]NodePressure, error) {
	nodes, err := c.Kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	pods, err := c.Kube.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	podsByNode := map[string]int{}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			podsByNode[p.Spec.NodeName]++
		}
	}

	var result []NodePressure
	for _, n := range nodes.Items {
		np := NodePressure{
			Name:          n.Name,
			Unschedulable: n.Spec.Unschedulable,
			Allocatable: ResourceInfo{
				CPU:    n.Status.Allocatable.Cpu().String(),
				Memory: fmt.Sprintf("%dMi", n.Status.Allocatable.Memory().Value()/1024/1024),
			},
			Capacity: ResourceInfo{
				CPU:    n.Status.Capacity.Cpu().String(),
				Memory: fmt.Sprintf("%dMi", n.Status.Capacity.Memory().Value()/1024/1024),
			},
			PodCount: podsByNode[n.Name],
		}
		if maxPods, ok := n.Status.Capacity.Pods().AsInt64(); ok {
			np.MaxPods = maxPods
		}

		for _, cond := range n.Status.Conditions {
			switch cond.Type {
			case corev1.NodeReady:
				np.Ready = cond.Status == corev1.ConditionTrue
			case corev1.NodeMemoryPressure:
				np.MemoryPressure = cond.Status == corev1.ConditionTrue
			case corev1.NodeDiskPressure:
				np.DiskPressure = cond.Status == corev1.ConditionTrue
			case corev1.NodePIDPressure:
				np.PIDPressure = cond.Status == corev1.ConditionTrue
			}
		}

		result = append(result, np)
	}

	return result, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func containerStateStr(s corev1.ContainerState) (string, string) {
	if s.Running != nil {
		return "Running", ""
	}
	if s.Waiting != nil {
		return "Waiting", s.Waiting.Reason
	}
	if s.Terminated != nil {
		return "Terminated", s.Terminated.Reason
	}
	return "Unknown", ""
}

func int64ptr(i int64) *int64 { return &i }

func exhaustETA(pct float64) string {
	remaining := 100 - pct
	if pct <= 0 {
		return "no usage data"
	}
	if remaining <= 0 {
		return "EXHAUSTED"
	}
	if pct >= 90 {
		return "CRITICAL — less than 10% headroom"
	}
	if pct >= 75 {
		return "WARNING — less than 25% headroom"
	}
	return fmt.Sprintf("%.1f%% used, %.1f%% headroom remaining", pct, remaining)
}

func containerImages(spec *corev1.PodSpec) map[string]string {
	m := map[string]string{}
	for _, c := range spec.Containers {
		m[c.Name] = c.Image
	}
	return m
}

func containerEnv(spec *corev1.PodSpec) map[string]string {
	m := map[string]string{}
	for _, c := range spec.Containers {
		for _, e := range c.Env {
			m[fmt.Sprintf("%s.%s", c.Name, e.Name)] = e.Value
		}
	}
	return m
}

func imagesList(m map[string]string) []string {
	var out []string
	for _, v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
