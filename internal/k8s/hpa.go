package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	autoscalingac "k8s.io/client-go/applyconfigurations/autoscaling/v2"
)

// hpaCPUTarget is the average CPU utilization (%) the autoscaler maintains.
const hpaCPUTarget = 70

// reconcileAutoscaler creates or removes the HorizontalPodAutoscaler for a spec
// so the cluster matches the desired autoscaling mode.
func (r *Reconciler) reconcileAutoscaler(ctx context.Context, spec ServiceSpec) error {
	if !spec.autoscaled() {
		return r.deleteAutoscaler(ctx, spec.Namespace, spec.Name)
	}
	ac := autoscalingac.HorizontalPodAutoscaler(spec.Name, spec.Namespace).
		WithLabels(selectorLabels(spec.Name)).
		WithSpec(autoscalingac.HorizontalPodAutoscalerSpec().
			WithScaleTargetRef(autoscalingac.CrossVersionObjectReference().
				WithAPIVersion("apps/v1").
				WithKind("Deployment").
				WithName(spec.Name)).
			WithMinReplicas(spec.MinReplicas).
			WithMaxReplicas(spec.MaxReplicas).
			WithMetrics(autoscalingac.MetricSpec().
				WithType(autoscalingv2.ResourceMetricSourceType).
				WithResource(autoscalingac.ResourceMetricSource().
					WithName(corev1.ResourceCPU).
					WithTarget(autoscalingac.MetricTarget().
						WithType(autoscalingv2.UtilizationMetricType).
						WithAverageUtilization(hpaCPUTarget)),
				),
			),
		)
	if _, err := r.kube.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Apply(ctx, ac, applyOpts()); err != nil {
		return fmt.Errorf("apply hpa %s: %w", spec.Name, err)
	}
	return nil
}

func (r *Reconciler) deleteAutoscaler(ctx context.Context, namespace, name string) error {
	err := r.kube.AutoscalingV2().HorizontalPodAutoscalers(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if ignoreNotFound(err) != nil {
		return fmt.Errorf("delete hpa %s: %w", name, err)
	}
	return nil
}
