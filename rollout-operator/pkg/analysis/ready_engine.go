package analysis

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ReadyEngine struct {
	Client             client.Client
	DepName, Namespace string
}

func (e *ReadyEngine) Evaluate(ctx context.Context, s Spec, labels map[string]string) (Result, error) {
	lg := log.FromContext(ctx)
	lg.Info("ReadyEngine Evaluate called")
	// Allow dynamic override via labels; fall back to engine fields for backward compatibility
	depName := e.DepName
	if v, ok := labels["deployment"]; ok && v != "" {
		depName = v
	}
	namespace := e.Namespace
	if v, ok := labels["namespace"]; ok && v != "" {
		namespace = v
	}

	if depName == "" || namespace == "" {
		lg.Info("ReadyEngine missing inputs", "deployment", depName, "namespace", namespace)
		return Result{Passed: false, Reason: "missing deployment or namespace for readiness check"}, nil
	}

	var dep appsv1.Deployment
	if err := e.Client.Get(ctx, client.ObjectKey{Name: depName, Namespace: namespace}, &dep); err != nil {
		lg.Info("ReadyEngine get failed", "deployment", depName, "namespace", namespace, "err", err.Error())
		return Result{Passed: false, Reason: err.Error()}, nil
	}
	ready := dep.Status.ReadyReplicas
	desired := int32(0)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	passed := ready == desired && desired > 0
	reason := "waiting for readiness"
	if passed {
		reason = "deployment ready"
	}
	lg.Info("ReadyEngine evaluated", "deployment", depName, "namespace", namespace, "ready", ready, "desired", desired, "passed", passed)
	return Result{Passed: passed, Reason: reason}, nil
}
