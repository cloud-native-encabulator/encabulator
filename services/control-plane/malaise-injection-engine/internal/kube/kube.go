// Package kube applies the Kubernetes-level malaises and, just as importantly,
// takes them back off again.
//
// Every object the actuator creates or mutates carries the malaise id as a label
// and an expiry as an annotation, so the cluster itself remembers what is going
// on. A restart of this service therefore cannot strand a fault: Sweep finds the
// evidence and reverts it.
package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/malaise-injection-engine/internal/malaise"
)

// Labels and annotations this service owns.
const (
	// NameLabel selects a workload's pods.
	NameLabel = "app.kubernetes.io/name"
	// ManagedByLabel marks the objects this service creates.
	ManagedByLabel = "app.kubernetes.io/managed-by"
	// PartOfLabel ties objects to the rest of the works.
	PartOfLabel = "app.kubernetes.io/part-of"
	// TargetLabel marks a workload as fair game for Kubernetes-level malaise.
	TargetLabel = "encabulator.io/malaise-target"
	// MalaiseIDLabel records which malaise touched an object.
	MalaiseIDLabel = "encabulator.io/malaise-id"
	// ExpiresAtAnnotation records when the change must be reverted.
	ExpiresAtAnnotation = "encabulator.io/expires-at"
	// OriginalReplicasAnnotation remembers a Deployment's replica count.
	OriginalReplicasAnnotation = "encabulator.io/original-replicas"

	managedBy = "malaise-injection-engine"
	partOf    = "encabulator"
)

// ErrTargetUnknown means no workload in the namespace is labelled for malaise
// under that name.
var ErrTargetUnknown = errors.New("no malaise target with that name")

// ErrNoDeployment means a scale malaise named something that is not a Deployment.
var ErrNoDeployment = errors.New("target is not a Deployment")

// Actuator talks to the Kubernetes API for one namespace.
type Actuator struct {
	client    kubernetes.Interface
	namespace string
	// observabilityNamespace is always allowed to reach a partitioned target, so
	// that Prometheus keeps scraping the very pods being broken.
	observabilityNamespace string
	log                    *slog.Logger
	shuffle                func(n int, swap func(i, j int))
}

// Config configures an Actuator.
type Config struct {
	Client                 kubernetes.Interface
	Namespace              string
	ObservabilityNamespace string
	Logger                 *slog.Logger
	// Shuffle may be nil; tests supply a deterministic permutation.
	Shuffle func(n int, swap func(i, j int))
}

// New builds an Actuator over an existing client.
func New(cfg Config) (*Actuator, error) {
	if cfg.Client == nil {
		return nil, errors.New("kube: client is required")
	}
	if cfg.Namespace == "" {
		return nil, errors.New("kube: namespace is required")
	}
	if cfg.ObservabilityNamespace == "" {
		cfg.ObservabilityNamespace = "encabulator-observability"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Shuffle == nil {
		cfg.Shuffle = rand.Shuffle
	}
	return &Actuator{
		client:                 cfg.Client,
		namespace:              cfg.Namespace,
		observabilityNamespace: cfg.ObservabilityNamespace,
		log:                    cfg.Logger,
		shuffle:                cfg.Shuffle,
	}, nil
}

// NewInCluster builds an Actuator from the in-cluster service account.
func NewInCluster(cfg Config) (*Actuator, error) {
	restCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}
	cfg.Client = client
	return New(cfg)
}

// Namespace returns the namespace the actuator operates in.
func (a *Actuator) Namespace() string { return a.namespace }

// Ping checks that the API server answers, for the readiness probe.
func (a *Actuator) Ping(ctx context.Context) error {
	_, err := a.client.CoreV1().Pods(a.namespace).List(ctx, metav1.ListOptions{Limit: 1})
	return err
}

// ValidateTarget reports whether a workload in the namespace is labelled
// encabulator.io/malaise-target=true under this name. Pods, Deployments and
// StatefulSets all count, so backing stores are targetable too.
func (a *Actuator) ValidateTarget(ctx context.Context, target string) error {
	selector := NameLabel + "=" + target + "," + TargetLabel + "=true"
	opts := metav1.ListOptions{LabelSelector: selector, Limit: 1}

	pods, err := a.client.CoreV1().Pods(a.namespace).List(ctx, opts)
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}
	if len(pods.Items) > 0 {
		return nil
	}
	deployments, err := a.client.AppsV1().Deployments(a.namespace).List(ctx, opts)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}
	if len(deployments.Items) > 0 {
		return nil
	}
	sets, err := a.client.AppsV1().StatefulSets(a.namespace).List(ctx, opts)
	if err != nil {
		return fmt.Errorf("list statefulsets: %w", err)
	}
	if len(sets.Items) > 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrTargetUnknown, target)
}

// Apply performs the Kubernetes action for m and fills in m.Detail.
func (a *Actuator) Apply(ctx context.Context, m *malaise.Malaise) error {
	switch m.Kind {
	case malaise.KindPodKill:
		return a.killPods(ctx, m)
	case malaise.KindPartition:
		return a.partition(ctx, m)
	case malaise.KindScale:
		return a.scale(ctx, m)
	default:
		return fmt.Errorf("kube: %q is not a Kubernetes-level kind", m.Kind)
	}
}

// Revert undoes whatever Apply did. It is idempotent: reverting twice, or
// reverting something that was never applied, is not an error.
func (a *Actuator) Revert(ctx context.Context, m *malaise.Malaise) error {
	switch m.Kind {
	case malaise.KindPodKill:
		return nil // a killed pod is not coming back; its owner makes a new one
	case malaise.KindPartition:
		return a.deletePolicies(ctx, MalaiseIDLabel+"="+m.ID)
	case malaise.KindScale:
		return a.restoreScale(ctx, m.Target)
	default:
		return nil
	}
}

func (a *Actuator) killPods(ctx context.Context, m *malaise.Malaise) error {
	pods, err := a.client.CoreV1().Pods(a.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: NameLabel + "=" + m.Target,
	})
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	ready := make([]string, 0, len(pods.Items))
	for i := range pods.Items {
		if podReady(&pods.Items[i]) {
			ready = append(ready, pods.Items[i].Name)
		}
	}
	if len(ready) == 0 {
		return fmt.Errorf("%w: no ready pods for %s", ErrTargetUnknown, m.Target)
	}
	a.shuffle(len(ready), func(i, j int) { ready[i], ready[j] = ready[j], ready[i] })

	killed := make([]string, 0, m.Params.Count)
	for _, name := range ready {
		if len(killed) >= m.Params.Count {
			break
		}
		err := a.client.CoreV1().Pods(a.namespace).Delete(ctx, name, metav1.DeleteOptions{})
		switch {
		case err == nil:
			killed = append(killed, name)
		case apierrors.IsNotFound(err):
			// It beat us to it. Fine.
		default:
			return fmt.Errorf("delete pod %s: %w", name, err)
		}
	}
	if len(killed) == 0 {
		return fmt.Errorf("%w: nothing left to kill for %s", ErrTargetUnknown, m.Target)
	}

	a.log.Info("induced malaise by pod deletion", "target", m.Target, "pods", killed)
	m.Detail = &malaise.Detail{KilledPods: killed}
	return nil
}

func podReady(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// policyName derives an RFC 1123 name from the malaise id.
func policyName(target, id string) string {
	suffix := strings.ToLower(strings.TrimPrefix(id, malaise.IDPrefix))
	suffix = strings.ReplaceAll(suffix, "_", "-")
	if len(suffix) > 12 {
		suffix = suffix[len(suffix)-12:]
	}
	name := "malaise-partition-" + target + "-" + suffix
	if len(name) > 253 {
		name = name[:253]
	}
	return strings.Trim(name, "-")
}

func (a *Actuator) partition(ctx context.Context, m *malaise.Malaise) error {
	// The observability namespace is always allowed through, so a partitioned
	// service keeps being scraped and the demo can show the damage.
	peers := []networkingv1.NetworkPolicyPeer{{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": a.observabilityNamespace},
		},
	}}
	if !m.Params.From.Wildcard() {
		// Everything except the named workloads. NotIn also matches pods that do
		// not carry the label at all, which is what "everything except" means.
		peers = append(peers, networkingv1.NetworkPolicyPeer{
			PodSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      NameLabel,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   append([]string(nil), m.Params.From...),
				}},
			},
		})
	}

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName(m.Target, m.ID),
			Namespace: a.namespace,
			Labels: map[string]string{
				ManagedByLabel: managedBy,
				PartOfLabel:    partOf,
				MalaiseIDLabel: m.ID,
			},
			Annotations: map[string]string{
				ExpiresAtAnnotation: m.ExpiresAt.UTC().Format(time.RFC3339),
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{NameLabel: m.Target}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{{From: peers}},
		},
	}

	created, err := a.client.NetworkingV1().NetworkPolicies(a.namespace).Create(ctx, policy, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		created, err = a.client.NetworkingV1().NetworkPolicies(a.namespace).Get(ctx, policy.Name, metav1.GetOptions{})
	}
	if err != nil {
		return fmt.Errorf("create network policy: %w", err)
	}

	a.log.Info("induced malaise by network partition",
		"target", m.Target, "policy", created.Name, "from", []string(m.Params.From))
	m.Detail = &malaise.Detail{NetworkPolicy: created.Name}
	return nil
}

func (a *Actuator) deletePolicies(ctx context.Context, selector string) error {
	list, err := a.client.NetworkingV1().NetworkPolicies(a.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return fmt.Errorf("list network policies: %w", err)
	}
	for i := range list.Items {
		name := list.Items[i].Name
		err := a.client.NetworkingV1().NetworkPolicies(a.namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete network policy %s: %w", name, err)
		}
		a.log.Info("lifted network partition", "policy", name)
	}
	return nil
}

// scalePatch is the merge patch that changes replicas and leaves the trail.
type scalePatch struct {
	Metadata struct {
		Labels      map[string]*string `json:"labels"`
		Annotations map[string]*string `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		Replicas *int32 `json:"replicas"`
	} `json:"spec"`
}

func (a *Actuator) scale(ctx context.Context, m *malaise.Malaise) error {
	deployments := a.client.AppsV1().Deployments(a.namespace)
	dep, err := a.findDeployment(ctx, m.Target)
	if err != nil {
		return err
	}

	original := int32(1)
	if dep.Spec.Replicas != nil {
		original = *dep.Spec.Replicas
	}
	// If a previous malaise already scaled this Deployment, the annotation holds
	// the true original. Never overwrite it with an already-broken value.
	if existing, ok := dep.Annotations[OriginalReplicasAnnotation]; ok {
		if parsed, perr := strconv.ParseInt(existing, 10, 32); perr == nil {
			original = int32(parsed)
		}
	}

	want := *m.Params.Replicas
	patch := scalePatch{}
	patch.Metadata.Labels = map[string]*string{MalaiseIDLabel: ptr(m.ID)}
	patch.Metadata.Annotations = map[string]*string{
		ExpiresAtAnnotation:        ptr(m.ExpiresAt.UTC().Format(time.RFC3339)),
		OriginalReplicasAnnotation: ptr(strconv.Itoa(int(original))),
	}
	patch.Spec.Replicas = &want

	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("encode scale patch: %w", err)
	}
	if _, err := deployments.Patch(ctx, dep.Name, types.MergePatchType, body, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("patch deployment %s: %w", dep.Name, err)
	}

	a.log.Info("induced malaise by scaling",
		"target", m.Target, "deployment", dep.Name, "from", original, "to", want)
	m.Detail = &malaise.Detail{OriginalReplicas: &original, AppliedReplicas: &want}
	return nil
}

// restoreScale returns a Deployment to its remembered replica count and removes
// the trail. A Deployment with no annotation is left alone.
func (a *Actuator) restoreScale(ctx context.Context, target string) error {
	dep, err := a.findDeployment(ctx, target)
	if err != nil {
		if errors.Is(err, ErrNoDeployment) || errors.Is(err, ErrTargetUnknown) {
			return nil // nothing to put back
		}
		return err
	}
	return a.restoreDeployment(ctx, dep)
}

func (a *Actuator) restoreDeployment(ctx context.Context, dep *appsv1.Deployment) error {
	raw, ok := dep.Annotations[OriginalReplicasAnnotation]
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return fmt.Errorf("deployment %s has an unreadable %s: %q",
			dep.Name, OriginalReplicasAnnotation, raw)
	}
	original := int32(parsed)

	patch := scalePatch{}
	patch.Metadata.Labels = map[string]*string{MalaiseIDLabel: nil}
	patch.Metadata.Annotations = map[string]*string{
		ExpiresAtAnnotation:        nil,
		OriginalReplicasAnnotation: nil,
	}
	patch.Spec.Replicas = &original

	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("encode restore patch: %w", err)
	}
	updated, err := a.client.AppsV1().Deployments(a.namespace).
		Patch(ctx, dep.Name, types.MergePatchType, body, metav1.PatchOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("patch deployment %s: %w", dep.Name, err)
	}
	a.log.Info("restored replicas", "deployment", updated.Name, "replicas", original)
	return nil
}

// findDeployment resolves a target name to a Deployment, by name first and then
// by the app.kubernetes.io/name label.
func (a *Actuator) findDeployment(ctx context.Context, target string) (*appsv1.Deployment, error) {
	deployments := a.client.AppsV1().Deployments(a.namespace)
	dep, err := deployments.Get(ctx, target, metav1.GetOptions{})
	if err == nil {
		return dep, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("get deployment %s: %w", target, err)
	}

	list, err := deployments.List(ctx, metav1.ListOptions{LabelSelector: NameLabel + "=" + target})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoDeployment, target)
	}
	return &list.Items[0], nil
}

// Sweep reverts every change whose expiry has passed, using only what the
// cluster itself records. It is how a restart avoids stranding a fault, and it
// runs even when the malaise store has lost the record.
func (a *Actuator) Sweep(ctx context.Context, now time.Time) (int, error) {
	reverted := 0
	var errs []error

	policies, err := a.client.NetworkingV1().NetworkPolicies(a.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: ManagedByLabel + "=" + managedBy,
	})
	if err != nil {
		errs = append(errs, fmt.Errorf("list network policies: %w", err))
	} else {
		for i := range policies.Items {
			p := &policies.Items[i]
			if !pastExpiry(p.Annotations, now) {
				continue
			}
			if derr := a.deletePolicies(ctx, MalaiseIDLabel+"="+p.Labels[MalaiseIDLabel]); derr != nil {
				errs = append(errs, derr)
				continue
			}
			reverted++
		}
	}

	deployments, err := a.client.AppsV1().Deployments(a.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: MalaiseIDLabel,
	})
	if err != nil {
		errs = append(errs, fmt.Errorf("list deployments: %w", err))
	} else {
		for i := range deployments.Items {
			dep := &deployments.Items[i]
			if !pastExpiry(dep.Annotations, now) {
				continue
			}
			if rerr := a.restoreDeployment(ctx, dep); rerr != nil {
				errs = append(errs, rerr)
				continue
			}
			reverted++
		}
	}
	return reverted, errors.Join(errs...)
}

// pastExpiry reports whether the expiry annotation is missing, unreadable, or
// in the past. A stranded object with no readable expiry is swept.
func pastExpiry(annotations map[string]string, now time.Time) bool {
	raw, ok := annotations[ExpiresAtAnnotation]
	if !ok {
		return true
	}
	expires, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true
	}
	return !now.Before(expires)
}

func ptr[T any](v T) *T { return &v }
