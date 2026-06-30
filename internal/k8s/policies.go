package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	coreac "k8s.io/client-go/applyconfigurations/core/v1"
	metaac "k8s.io/client-go/applyconfigurations/meta/v1"
	netac "k8s.io/client-go/applyconfigurations/networking/v1"

	"github.com/ebnsina/uran-api/internal/instance"
)

// Per-namespace (per-org) isolation policy. These are platform-wide defaults,
// not per-deployment configuration.
const (
	quotaPods       = "20"
	quotaCPUReq     = "4"    // total requested CPU cores
	quotaMemReq     = "8Gi"  // total requested memory
	quotaCPULimit   = "8"    // total CPU limit
	quotaMemLimit   = "16Gi" // total memory limit
	containerCPUreq = "50m"
	containerMemReq = "64Mi"
	containerCPULim = "500m"
	containerMemLim = "256Mi"
)

// kubeSystemNamespace is where Traefik runs; its ingress must be allowed.
const kubeSystemNamespace = "kube-system"

// applyNamespacePolicies installs the ResourceQuota, LimitRange, and
// NetworkPolicy that isolate an org's namespace from other tenants.
func (r *Reconciler) applyNamespacePolicies(ctx context.Context, ns string) error {
	if err := r.applyResourceQuota(ctx, ns); err != nil {
		return err
	}
	if err := r.applyLimitRange(ctx, ns); err != nil {
		return err
	}
	return r.applyNetworkPolicy(ctx, ns)
}

func (r *Reconciler) applyResourceQuota(ctx context.Context, ns string) error {
	ac := coreac.ResourceQuota("uran-quota", ns).
		WithSpec(coreac.ResourceQuotaSpec().WithHard(corev1.ResourceList{
			corev1.ResourcePods:           resource.MustParse(quotaPods),
			corev1.ResourceRequestsCPU:    resource.MustParse(quotaCPUReq),
			corev1.ResourceRequestsMemory: resource.MustParse(quotaMemReq),
			corev1.ResourceLimitsCPU:      resource.MustParse(quotaCPULimit),
			corev1.ResourceLimitsMemory:   resource.MustParse(quotaMemLimit),
		}))
	if _, err := r.kube.CoreV1().ResourceQuotas(ns).Apply(ctx, ac, applyOpts()); err != nil {
		return fmt.Errorf("apply resourcequota %s: %w", ns, err)
	}
	return nil
}

func (r *Reconciler) applyLimitRange(ctx context.Context, ns string) error {
	ac := coreac.LimitRange("uran-limits", ns).
		WithSpec(coreac.LimitRangeSpec().WithLimits(
			coreac.LimitRangeItem().
				WithType(corev1.LimitTypeContainer).
				WithDefault(corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(containerCPULim),
					corev1.ResourceMemory: resource.MustParse(containerMemLim),
				}).
				WithDefaultRequest(corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(containerCPUreq),
					corev1.ResourceMemory: resource.MustParse(containerMemReq),
				}),
		))
	if _, err := r.kube.CoreV1().LimitRanges(ns).Apply(ctx, ac, applyOpts()); err != nil {
		return fmt.Errorf("apply limitrange %s: %w", ns, err)
	}
	return nil
}

// applyNetworkPolicy denies all ingress to the namespace except from pods in
// the same namespace and from the Traefik namespace (so external routing still
// works). Selecting all pods with a restrictive ingress rule is deny-by-default.
func (r *Reconciler) applyNetworkPolicy(ctx context.Context, ns string) error {
	ac := netac.NetworkPolicy("uran-isolation", ns).
		WithSpec(netac.NetworkPolicySpec().
			WithPodSelector(metaac.LabelSelector()). // all pods
			WithPolicyTypes(networkingv1.PolicyTypeIngress).
			WithIngress(netac.NetworkPolicyIngressRule().WithFrom(
				netac.NetworkPolicyPeer().WithPodSelector(metaac.LabelSelector()),
				netac.NetworkPolicyPeer().WithNamespaceSelector(
					metaac.LabelSelector().WithMatchLabels(map[string]string{
						"kubernetes.io/metadata.name": kubeSystemNamespace,
					}),
				),
			)),
		)
	if _, err := r.kube.NetworkingV1().NetworkPolicies(ns).Apply(ctx, ac, applyOpts()); err != nil {
		return fmt.Errorf("apply networkpolicy %s: %w", ns, err)
	}
	return nil
}

// containerResources returns the per-container requests/limits for an instance
// size, satisfying the namespace ResourceQuota.
func containerResources(size string) *coreac.ResourceRequirementsApplyConfiguration {
	r := instance.Get(size)
	return coreac.ResourceRequirements().
		WithRequests(corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(r.CPURequest),
			corev1.ResourceMemory: resource.MustParse(r.MemRequest),
		}).
		WithLimits(corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(r.CPULimit),
			corev1.ResourceMemory: resource.MustParse(r.MemLimit),
		})
}
