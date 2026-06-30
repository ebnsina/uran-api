package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	batchac "k8s.io/client-go/applyconfigurations/batch/v1"
	coreac "k8s.io/client-go/applyconfigurations/core/v1"
)

// applyCronJob reconciles a CronJob that runs the built image on a schedule.
// It shares the namespace env Secret and resource defaults with other workloads
// but has no Service or route.
func (r *Reconciler) applyCronJob(ctx context.Context, spec ServiceSpec) error {
	labels := selectorLabels(spec.Name)
	ac := batchac.CronJob(spec.Name, spec.Namespace).
		WithLabels(labels).
		WithSpec(batchac.CronJobSpec().
			WithSchedule(spec.Schedule).
			WithJobTemplate(batchac.JobTemplateSpec().
				WithSpec(batchac.JobSpec().
					WithTemplate(coreac.PodTemplateSpec().
						WithLabels(labels).
						WithSpec(coreac.PodSpec().
							WithRestartPolicy(corev1.RestartPolicyOnFailure).
							WithContainers(coreac.Container().
								WithName("app").
								WithImage(spec.Image).
								WithResources(containerResources(spec.InstanceSize)).
								WithEnvFrom(coreac.EnvFromSource().WithSecretRef(
									coreac.SecretEnvSource().WithName(envSecretName(spec.Name)),
								)),
							),
						),
					),
				),
			),
		)
	if _, err := r.kube.BatchV1().CronJobs(spec.Namespace).Apply(ctx, ac, applyOpts()); err != nil {
		return fmt.Errorf("apply cronjob %s: %w", spec.Name, err)
	}
	return nil
}
