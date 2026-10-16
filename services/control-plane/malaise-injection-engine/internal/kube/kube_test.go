package kube

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/cloud-native-encabulator/encabulator/services/control-plane/malaise-injection-engine/internal/malaise"
)

const (
	namespace = "encabulator"
	obsNS     = "encabulator-observability"
)

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newActuator builds an actuator over a fake clientset seeded with objects. The
// shuffle is the identity permutation so pod selection is deterministic.
func newActuator(t *testing.T, objects ...runtime.Object) (*Actuator, *fake.Clientset) {
	t.Helper()
	client := fake.NewClientset(objects...)
	a, err := New(Config{
		Client:                 client,
		Namespace:              namespace,
		ObservabilityNamespace: obsNS,
		Logger:                 discardLogger(),
		Shuffle:                func(int, func(i, j int)) {},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, client
}

func pod(name, app string, ready bool, target bool) *corev1.Pod {
	labels := map[string]string{NameLabel: app}
	if target {
		labels[TargetLabel] = "true"
	}
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
		},
	}
}

func deployment(name string, replicas int32, target bool) *appsv1.Deployment {
	labels := map[string]string{NameLabel: name}
	if target {
		labels[TargetLabel] = "true"
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	}
}

func TestNewValidatesItsConfig(t *testing.T) {
	if _, err := New(Config{Namespace: namespace}); err == nil {
		t.Error("a client is required")
	}
	if _, err := New(Config{Client: fake.NewClientset()}); err == nil {
		t.Error("a namespace is required")
	}
	a, err := New(Config{Client: fake.NewClientset(), Namespace: namespace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.observabilityNamespace != obsNS {
		t.Errorf("observability namespace = %q, want the default %q", a.observabilityNamespace, obsNS)
	}
	if a.Namespace() != namespace {
		t.Errorf("namespace = %q", a.Namespace())
	}
}

func TestValidateTarget(t *testing.T) {
	a, _ := newActuator(t,
		pod("gateway-1", "dingle-arm-gateway", true, true),
		pod("untagged-1", "demo-scenario-orchestrator", true, false),
		deployment("spurving-bearing-quoter", 2, true),
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
			Name: "valkey", Namespace: namespace,
			Labels: map[string]string{NameLabel: "valkey", TargetLabel: "true"},
		}},
	)
	ctx := context.Background()

	for _, target := range []string{"dingle-arm-gateway", "spurving-bearing-quoter", "valkey"} {
		if err := a.ValidateTarget(ctx, target); err != nil {
			t.Errorf("%s should be a valid target: %v", target, err)
		}
	}
	for _, target := range []string{"demo-scenario-orchestrator", "nothing-at-all"} {
		err := a.ValidateTarget(ctx, target)
		if !errors.Is(err, ErrTargetUnknown) {
			t.Errorf("%s: err = %v, want ErrTargetUnknown", target, err)
		}
	}
}

func TestPing(t *testing.T) {
	a, _ := newActuator(t)
	if err := a.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestKillPodsDeletesReadyPodsOnly(t *testing.T) {
	a, client := newActuator(t,
		pod("line-1", "logarithmic-casing-line", true, true),
		pod("line-2", "logarithmic-casing-line", true, true),
		pod("line-3", "logarithmic-casing-line", false, true),  // not ready
		pod("quoter-1", "spurving-bearing-quoter", true, true), // not the target
	)
	m := build(t, malaise.Request{Target: "logarithmic-casing-line", Kind: malaise.KindPodKill,
		Params: malaise.Params{Count: 2}})

	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if m.Detail == nil || len(m.Detail.KilledPods) != 2 {
		t.Fatalf("detail = %+v, want two killed pods", m.Detail)
	}

	remaining := listPods(t, client)
	if _, ok := remaining["line-3"]; !ok {
		t.Error("the unready pod should have been left alone")
	}
	if _, ok := remaining["quoter-1"]; !ok {
		t.Error("another service's pod was killed")
	}
	if len(remaining) != 2 {
		t.Errorf("pods remaining = %v, want line-3 and quoter-1", remaining)
	}
}

func TestKillPodsStopsAtWhatIsAvailable(t *testing.T) {
	a, _ := newActuator(t, pod("line-1", "logarithmic-casing-line", true, true))
	m := build(t, malaise.Request{Target: "logarithmic-casing-line", Kind: malaise.KindPodKill,
		Params: malaise.Params{Count: 5}})

	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(m.Detail.KilledPods) != 1 {
		t.Errorf("killed %v, want just the one that existed", m.Detail.KilledPods)
	}
}

func TestKillPodsWithNothingReady(t *testing.T) {
	a, _ := newActuator(t, pod("line-1", "logarithmic-casing-line", false, true))
	m := build(t, malaise.Request{Target: "logarithmic-casing-line", Kind: malaise.KindPodKill})
	err := a.Apply(context.Background(), &m)
	if !errors.Is(err, ErrTargetUnknown) {
		t.Fatalf("err = %v, want ErrTargetUnknown", err)
	}
}

// Reverting a pod-kill is a no-op: the ReplicaSet makes the replacement.
func TestRevertPodKillDoesNothing(t *testing.T) {
	a, client := newActuator(t, pod("line-1", "logarithmic-casing-line", true, true))
	m := build(t, malaise.Request{Target: "logarithmic-casing-line", Kind: malaise.KindPodKill})
	if err := a.Revert(context.Background(), &m); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if len(listPods(t, client)) != 1 {
		t.Error("Revert should not have touched any pods")
	}
}

func TestPartitionFromNamedWorkloads(t *testing.T) {
	a, client := newActuator(t, pod("datastore-1", "hydrocoptic-datastore", true, true))
	m := build(t, malaise.Request{Target: "hydrocoptic-datastore", Kind: malaise.KindPartition,
		Params: malaise.Params{From: malaise.StringList{"panametric-fam-processor", "dingle-arm-gateway"}}})

	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if m.Detail == nil || m.Detail.NetworkPolicy == "" {
		t.Fatalf("detail = %+v, want a policy name", m.Detail)
	}

	policy := getPolicy(t, client, m.Detail.NetworkPolicy)
	if got := policy.Spec.PodSelector.MatchLabels[NameLabel]; got != "hydrocoptic-datastore" {
		t.Errorf("policy selects %q, want the target", got)
	}
	if len(policy.Spec.PolicyTypes) != 1 || policy.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress {
		t.Errorf("policy types = %v, want Ingress only", policy.Spec.PolicyTypes)
	}
	if len(policy.Spec.Ingress) != 1 {
		t.Fatalf("ingress rules = %d, want 1", len(policy.Spec.Ingress))
	}

	peers := policy.Spec.Ingress[0].From
	if len(peers) != 2 {
		t.Fatalf("peers = %d, want the observability namespace plus the NotIn selector", len(peers))
	}

	// Observability is always allowed, so Prometheus keeps scraping.
	if peers[0].NamespaceSelector == nil ||
		peers[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != obsNS {
		t.Errorf("first peer = %+v, want the observability namespace", peers[0])
	}

	// Everything except the named workloads, expressed as NotIn.
	sel := peers[1].PodSelector
	if sel == nil || len(sel.MatchExpressions) != 1 {
		t.Fatalf("second peer = %+v, want one match expression", peers[1])
	}
	expr := sel.MatchExpressions[0]
	if expr.Key != NameLabel {
		t.Errorf("expression key = %q, want %q", expr.Key, NameLabel)
	}
	if expr.Operator != metav1.LabelSelectorOpNotIn {
		t.Errorf("operator = %q, want NotIn", expr.Operator)
	}
	if len(expr.Values) != 2 || expr.Values[0] != "panametric-fam-processor" {
		t.Errorf("values = %v", expr.Values)
	}

	// The cluster remembers whose malaise this is and when it ends.
	if policy.Labels[MalaiseIDLabel] != m.ID {
		t.Errorf("malaise id label = %q, want %q", policy.Labels[MalaiseIDLabel], m.ID)
	}
	if policy.Labels[ManagedByLabel] != managedBy {
		t.Errorf("managed-by = %q", policy.Labels[ManagedByLabel])
	}
	if got := policy.Annotations[ExpiresAtAnnotation]; got != m.ExpiresAt.UTC().Format(time.RFC3339) {
		t.Errorf("expires-at = %q, want %q", got, m.ExpiresAt.UTC().Format(time.RFC3339))
	}
}

func TestPartitionWildcardAllowsOnlyObservability(t *testing.T) {
	a, client := newActuator(t, pod("quoter-1", "spurving-bearing-quoter", true, true))
	m := build(t, malaise.Request{Target: "spurving-bearing-quoter", Kind: malaise.KindPartition,
		Params: malaise.Params{From: malaise.StringList{"*"}}})

	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	peers := getPolicy(t, client, m.Detail.NetworkPolicy).Spec.Ingress[0].From
	if len(peers) != 1 {
		t.Fatalf("peers = %d, want only the observability namespace", len(peers))
	}
	if peers[0].NamespaceSelector == nil ||
		peers[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != obsNS {
		t.Errorf("peer = %+v, want the observability namespace", peers[0])
	}
	if peers[0].PodSelector != nil {
		t.Error("the wildcard must not allow any pod peer")
	}
}

func TestRevertPartitionDeletesThePolicy(t *testing.T) {
	a, client := newActuator(t, pod("quoter-1", "spurving-bearing-quoter", true, true))
	m := build(t, malaise.Request{Target: "spurving-bearing-quoter", Kind: malaise.KindPartition})
	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if err := a.Revert(context.Background(), &m); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if _, err := client.NetworkingV1().NetworkPolicies(namespace).
		Get(context.Background(), m.Detail.NetworkPolicy, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("policy still exists: %v", err)
	}

	// Reverting twice is not an error.
	if err := a.Revert(context.Background(), &m); err != nil {
		t.Fatalf("second Revert: %v", err)
	}
}

func TestPartitionToleratesAnExistingPolicy(t *testing.T) {
	a, _ := newActuator(t, pod("quoter-1", "spurving-bearing-quoter", true, true))
	m := build(t, malaise.Request{Target: "spurving-bearing-quoter", Kind: malaise.KindPartition})
	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Applying the same malaise again (a retry) must not fail.
	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
}

func TestPolicyNameIsRFC1123(t *testing.T) {
	name := policyName("hydrocoptic-datastore", "mal_0199c7f0-1234-7890-abcd-ef0123456789")
	if len(name) == 0 || len(name) > 253 {
		t.Fatalf("name %q has a bad length", name)
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' && r != '.' {
			t.Fatalf("name %q contains %q", name, r)
		}
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		t.Fatalf("name %q starts or ends with a hyphen", name)
	}
	// Two malaises against the same target must not collide.
	other := policyName("hydrocoptic-datastore", "mal_0199c7f0-1234-7890-abcd-ef0123450000")
	if name == other {
		t.Fatalf("names collided: %q", name)
	}
}

func TestScaleRemembersTheOriginalReplicas(t *testing.T) {
	a, client := newActuator(t, deployment("spurving-bearing-quoter", 3, true))
	m := build(t, malaise.Request{Target: "spurving-bearing-quoter", Kind: malaise.KindScale,
		Params: malaise.Params{Replicas: i32(1)}})

	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if m.Detail == nil || m.Detail.OriginalReplicas == nil || *m.Detail.OriginalReplicas != 3 {
		t.Fatalf("detail = %+v, want the original 3", m.Detail)
	}
	if *m.Detail.AppliedReplicas != 1 {
		t.Errorf("applied = %d, want 1", *m.Detail.AppliedReplicas)
	}

	dep := getDeployment(t, client, "spurving-bearing-quoter")
	if *dep.Spec.Replicas != 1 {
		t.Errorf("replicas = %d, want 1", *dep.Spec.Replicas)
	}
	if dep.Annotations[OriginalReplicasAnnotation] != "3" {
		t.Errorf("original-replicas annotation = %q, want 3", dep.Annotations[OriginalReplicasAnnotation])
	}
	if dep.Labels[MalaiseIDLabel] != m.ID {
		t.Errorf("malaise id label = %q", dep.Labels[MalaiseIDLabel])
	}
	if dep.Annotations[ExpiresAtAnnotation] == "" {
		t.Error("expires-at annotation is missing")
	}
	// The existing labels must survive the patch.
	if dep.Labels[NameLabel] != "spurving-bearing-quoter" {
		t.Errorf("the patch clobbered the name label: %v", dep.Labels)
	}
}

func TestRevertScaleRestoresAndCleansUp(t *testing.T) {
	a, client := newActuator(t, deployment("spurving-bearing-quoter", 3, true))
	m := build(t, malaise.Request{Target: "spurving-bearing-quoter", Kind: malaise.KindScale,
		Params: malaise.Params{Replicas: i32(0)}})
	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if err := a.Revert(context.Background(), &m); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	dep := getDeployment(t, client, "spurving-bearing-quoter")
	if *dep.Spec.Replicas != 3 {
		t.Errorf("replicas = %d, want the original 3", *dep.Spec.Replicas)
	}
	if _, ok := dep.Annotations[OriginalReplicasAnnotation]; ok {
		t.Error("the original-replicas annotation should be gone")
	}
	if _, ok := dep.Annotations[ExpiresAtAnnotation]; ok {
		t.Error("the expires-at annotation should be gone")
	}
	if _, ok := dep.Labels[MalaiseIDLabel]; ok {
		t.Error("the malaise-id label should be gone")
	}
	if dep.Labels[NameLabel] != "spurving-bearing-quoter" {
		t.Errorf("the restore clobbered the name label: %v", dep.Labels)
	}

	// Reverting twice is not an error and does not change anything.
	if err := a.Revert(context.Background(), &m); err != nil {
		t.Fatalf("second Revert: %v", err)
	}
	if *getDeployment(t, client, "spurving-bearing-quoter").Spec.Replicas != 3 {
		t.Error("the second revert changed the replica count")
	}
}

// A second scale malaise must not record the already-broken count as "original".
func TestScaleTwiceKeepsTheTrueOriginal(t *testing.T) {
	a, client := newActuator(t, deployment("spurving-bearing-quoter", 4, true))
	ctx := context.Background()

	first := build(t, malaise.Request{Target: "spurving-bearing-quoter", Kind: malaise.KindScale,
		Params: malaise.Params{Replicas: i32(2)}})
	if err := a.Apply(ctx, &first); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	second := build(t, malaise.Request{Target: "spurving-bearing-quoter", Kind: malaise.KindScale,
		Params: malaise.Params{Replicas: i32(1)}})
	if err := a.Apply(ctx, &second); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if *second.Detail.OriginalReplicas != 4 {
		t.Errorf("original = %d, want the true original 4", *second.Detail.OriginalReplicas)
	}

	if err := a.Revert(ctx, &second); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if got := *getDeployment(t, client, "spurving-bearing-quoter").Spec.Replicas; got != 4 {
		t.Errorf("replicas = %d, want 4", got)
	}
}

func TestScaleResolvesTargetByLabelWhenTheNameDiffers(t *testing.T) {
	dep := deployment("quoter-release-name", 2, true)
	dep.Labels[NameLabel] = "spurving-bearing-quoter"
	a, client := newActuator(t, dep)

	m := build(t, malaise.Request{Target: "spurving-bearing-quoter", Kind: malaise.KindScale,
		Params: malaise.Params{Replicas: i32(0)}})
	if err := a.Apply(context.Background(), &m); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := *getDeployment(t, client, "quoter-release-name").Spec.Replicas; got != 0 {
		t.Errorf("replicas = %d, want 0", got)
	}
}

func TestScaleMissingDeployment(t *testing.T) {
	a, _ := newActuator(t)
	m := build(t, malaise.Request{Target: "nowhere", Kind: malaise.KindScale,
		Params: malaise.Params{Replicas: i32(1)}})
	if err := a.Apply(context.Background(), &m); !errors.Is(err, ErrNoDeployment) {
		t.Fatalf("err = %v, want ErrNoDeployment", err)
	}
	// Reverting something that was never applied is fine.
	if err := a.Revert(context.Background(), &m); err != nil {
		t.Fatalf("Revert: %v", err)
	}
}

func TestApplyRejectsAppLevelKinds(t *testing.T) {
	a, _ := newActuator(t)
	m := build(t, malaise.Request{Target: "dingle-arm-gateway", Kind: malaise.KindLatency,
		Params: malaise.Params{Ms: 100}})
	if err := a.Apply(context.Background(), &m); err == nil {
		t.Fatal("expected an error for an app-level kind")
	}
	if err := a.Revert(context.Background(), &m); err != nil {
		t.Fatalf("Revert of an app-level kind: %v", err)
	}
}

// Sweep is what makes a restart safe: it works from the cluster alone.
func TestSweepRevertsStrandedChanges(t *testing.T) {
	stale := deployment("hydrocoptic-datastore", 1, true)
	stale.Labels[MalaiseIDLabel] = "mal_gone"
	stale.Annotations = map[string]string{
		ExpiresAtAnnotation:        now.Add(-time.Minute).Format(time.RFC3339),
		OriginalReplicasAnnotation: "2",
	}
	fresh := deployment("spurving-bearing-quoter", 1, true)
	fresh.Labels[MalaiseIDLabel] = "mal_current"
	fresh.Annotations = map[string]string{
		ExpiresAtAnnotation:        now.Add(time.Minute).Format(time.RFC3339),
		OriginalReplicasAnnotation: "3",
	}
	untouched := deployment("nofer-trunnion-dispatch", 2, true)

	stalePolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "malaise-partition-old", Namespace: namespace,
		Labels:      map[string]string{ManagedByLabel: managedBy, MalaiseIDLabel: "mal_old"},
		Annotations: map[string]string{ExpiresAtAnnotation: now.Add(-time.Second).Format(time.RFC3339)},
	}}
	freshPolicy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "malaise-partition-new", Namespace: namespace,
		Labels:      map[string]string{ManagedByLabel: managedBy, MalaiseIDLabel: "mal_new"},
		Annotations: map[string]string{ExpiresAtAnnotation: now.Add(time.Hour).Format(time.RFC3339)},
	}}
	// Somebody else's policy: not ours to delete.
	foreign := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "default-deny", Namespace: namespace,
	}}

	a, client := newActuator(t, stale, fresh, untouched, stalePolicy, freshPolicy, foreign)

	reverted, err := a.Sweep(context.Background(), now)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if reverted != 2 {
		t.Errorf("reverted %d, want 2 (one policy, one deployment)", reverted)
	}

	if got := *getDeployment(t, client, "hydrocoptic-datastore").Spec.Replicas; got != 2 {
		t.Errorf("stranded deployment replicas = %d, want the restored 2", got)
	}
	if got := *getDeployment(t, client, "spurving-bearing-quoter").Spec.Replicas; got != 1 {
		t.Errorf("live malaise was reverted early: replicas = %d, want 1", got)
	}
	if got := *getDeployment(t, client, "nofer-trunnion-dispatch").Spec.Replicas; got != 2 {
		t.Errorf("untouched deployment changed: replicas = %d", got)
	}

	policies := listPolicies(t, client)
	if _, ok := policies["malaise-partition-old"]; ok {
		t.Error("the expired policy should have been deleted")
	}
	if _, ok := policies["malaise-partition-new"]; !ok {
		t.Error("the live policy should have survived")
	}
	if _, ok := policies["default-deny"]; !ok {
		t.Error("a policy we do not manage was deleted")
	}
}

// A policy with no readable expiry is stranded and must go.
func TestSweepRemovesPoliciesWithNoReadableExpiry(t *testing.T) {
	noAnnotation := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "malaise-partition-a", Namespace: namespace,
		Labels: map[string]string{ManagedByLabel: managedBy, MalaiseIDLabel: "mal_a"},
	}}
	garbled := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "malaise-partition-b", Namespace: namespace,
		Labels:      map[string]string{ManagedByLabel: managedBy, MalaiseIDLabel: "mal_b"},
		Annotations: map[string]string{ExpiresAtAnnotation: "the day after tomorrow"},
	}}
	a, client := newActuator(t, noAnnotation, garbled)

	if _, err := a.Sweep(context.Background(), now); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if got := len(listPolicies(t, client)); got != 0 {
		t.Errorf("%d policies left, want 0", got)
	}
}

func TestSweepReportsApiErrors(t *testing.T) {
	a, client := newActuator(t)
	client.PrependReactor("list", "networkpolicies",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("apiserver said no")
		})
	if _, err := a.Sweep(context.Background(), now); err == nil {
		t.Fatal("expected the list failure to be reported")
	}
}

func TestSweepIgnoresUnreadableOriginalReplicas(t *testing.T) {
	broken := deployment("hydrocoptic-datastore", 1, true)
	broken.Labels[MalaiseIDLabel] = "mal_x"
	broken.Annotations = map[string]string{
		ExpiresAtAnnotation:        now.Add(-time.Minute).Format(time.RFC3339),
		OriginalReplicasAnnotation: "several",
	}
	a, _ := newActuator(t, broken)
	if _, err := a.Sweep(context.Background(), now); err == nil {
		t.Fatal("expected the unreadable annotation to be reported")
	}
}

// helpers

func build(t *testing.T, req malaise.Request) malaise.Malaise {
	t.Helper()
	m, err := malaise.Build(req, now)
	if err != nil {
		t.Fatalf("malaise.Build: %v", err)
	}
	return m
}

func i32(v int32) *int32 { return &v }

func listPods(t *testing.T, client *fake.Clientset) map[string]bool {
	t.Helper()
	list, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	out := map[string]bool{}
	for _, p := range list.Items {
		out[p.Name] = true
	}
	return out
}

func listPolicies(t *testing.T, client *fake.Clientset) map[string]bool {
	t.Helper()
	list, err := client.NetworkingV1().NetworkPolicies(namespace).
		List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list network policies: %v", err)
	}
	out := map[string]bool{}
	for _, p := range list.Items {
		out[p.Name] = true
	}
	return out
}

func getPolicy(t *testing.T, client *fake.Clientset, name string) *networkingv1.NetworkPolicy {
	t.Helper()
	p, err := client.NetworkingV1().NetworkPolicies(namespace).
		Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get network policy %s: %v", name, err)
	}
	return p
}

func getDeployment(t *testing.T, client *fake.Clientset, name string) *appsv1.Deployment {
	t.Helper()
	d, err := client.AppsV1().Deployments(namespace).
		Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment %s: %v", name, err)
	}
	return d
}
