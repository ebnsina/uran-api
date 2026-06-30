package k8s

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
)

func TestHost(t *testing.T) {
	r := &Reconciler{baseDomain: "uran.local"}
	spec := ServiceSpec{Name: "sample-web"}
	if got := r.Host(spec); got != "sample-web.uran.local" {
		t.Errorf("Host() = %q, want sample-web.uran.local", got)
	}
}

func TestRolloutComplete(t *testing.T) {
	replicas := int32(1)
	ready := &appsv1.Deployment{}
	ready.Generation = 3
	ready.Spec.Replicas = &replicas
	ready.Status.ObservedGeneration = 3
	ready.Status.UpdatedReplicas = 1
	ready.Status.AvailableReplicas = 1
	if !rolloutComplete(ready) {
		t.Error("expected a fully-available deployment to be complete")
	}

	notReady := *ready
	notReady.Status.AvailableReplicas = 0
	if rolloutComplete(&notReady) {
		t.Error("expected a deployment with 0 available replicas to be incomplete")
	}

	stale := *ready
	stale.Status.ObservedGeneration = 2 // controller hasn't observed latest spec
	if rolloutComplete(&stale) {
		t.Error("expected a deployment with stale observedGeneration to be incomplete")
	}
}
