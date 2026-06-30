package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreac "k8s.io/client-go/applyconfigurations/core/v1"
)

// diskVolumeName is the pod volume name for a service's persistent disk.
const diskVolumeName = "data"

// pvcName is the PersistentVolumeClaim backing a service's disk.
func pvcName(name string) string { return name + "-data" }

// reconcileDisk ensures the PVC exists when a disk is attached, and removes it
// when detached. Removing the claim deletes the underlying data.
func (r *Reconciler) reconcileDisk(ctx context.Context, spec ServiceSpec) error {
	if !spec.hasDisk() {
		return r.deletePVC(ctx, spec.Namespace, pvcName(spec.Name))
	}
	ac := coreac.PersistentVolumeClaim(pvcName(spec.Name), spec.Namespace).
		WithLabels(selectorLabels(spec.Name)).
		WithSpec(coreac.PersistentVolumeClaimSpec().
			WithAccessModes(corev1.ReadWriteOnce).
			WithResources(coreac.VolumeResourceRequirements().
				WithRequests(corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(spec.DiskSize),
				})))
	// A PVC's spec is immutable except for growth, so don't force-overwrite.
	if _, err := r.kube.CoreV1().PersistentVolumeClaims(spec.Namespace).Apply(ctx, ac,
		metav1.ApplyOptions{FieldManager: fieldManager}); err != nil {
		return fmt.Errorf("apply pvc %s: %w", spec.Name, err)
	}
	return nil
}

func (r *Reconciler) deletePVC(ctx context.Context, namespace, name string) error {
	err := r.kube.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if ignoreNotFound(err) != nil {
		return fmt.Errorf("delete pvc %s: %w", name, err)
	}
	return nil
}
