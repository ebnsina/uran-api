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

func TestHostsAndMatch(t *testing.T) {
	r := &Reconciler{baseDomain: "uran.local"}
	spec := ServiceSpec{Name: "web", Domains: []string{"example.com", "www.example.com"}}

	hosts := r.hosts(spec)
	want := []string{"web.uran.local", "example.com", "www.example.com"}
	if len(hosts) != len(want) {
		t.Fatalf("hosts() = %v", hosts)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Errorf("hosts()[%d] = %q, want %q", i, hosts[i], want[i])
		}
	}

	got := hostMatch(hosts)
	expect := "Host(`web.uran.local`) || Host(`example.com`) || Host(`www.example.com`)"
	if got != expect {
		t.Errorf("hostMatch() = %q", got)
	}
}

func TestTLSSecretName(t *testing.T) {
	if got := tlsSecretName("web"); got != "web-tls" {
		t.Errorf("tlsSecretName = %q", got)
	}
}

func TestSpecModes(t *testing.T) {
	// A disk pins to one replica and disables autoscaling.
	withDisk := ServiceSpec{MaxReplicas: 4, DiskSize: "1Gi", DiskPath: "/data"}
	if !withDisk.hasDisk() {
		t.Error("hasDisk should be true")
	}
	if withDisk.autoscaled() {
		t.Error("autoscaling must be disabled when a disk is attached")
	}

	// Without a disk, max>0 enables autoscaling.
	autoscaling := ServiceSpec{MaxReplicas: 4}
	if autoscaling.hasDisk() || !autoscaling.autoscaled() {
		t.Error("expected autoscaling, no disk")
	}

	// Incomplete disk config (size without path) is not a disk.
	if (ServiceSpec{DiskSize: "1Gi"}).hasDisk() {
		t.Error("size without path is not a disk")
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
