package traffic

import (
	"context"
	"fmt"
	"strconv"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NginxProvider struct {
	Client    client.Client
	Namespace string
}

// SetWeight 设置金丝雀流量权重
func (p *NginxProvider) SetWeight(ctx context.Context, host, stable, canary string, weight int32) error {
	lg := log.FromContext(ctx)
	lg.Info("Setting nginx ingress weight", "host", host, "stable", stable, "canary", canary, "weight", weight)

	// 确保主 ingress 存在（指向 stable service）
	if err := p.ensureStableIngress(ctx, host, stable); err != nil {
		return fmt.Errorf("failed to ensure stable ingress: %w", err)
	}

	// 创建或更新 canary ingress
	return p.updateCanaryIngress(ctx, host, stable, canary, weight)
}

// Promote 将流量完全切换到 canary
func (p *NginxProvider) Promote(ctx context.Context, host, stable, canary string) error {
	lg := log.FromContext(ctx)
	lg.Info("Promoting canary to stable", "host", host, "stable", stable, "canary", canary)

	// 更新主 ingress 指向 canary service
	if err := p.promoteToStable(ctx, host, stable, canary); err != nil {
		return fmt.Errorf("failed to promote canary: %w", err)
	}

	// 删除 canary ingress
	return p.deleteCanaryIngress(ctx, host)
}

// Reset 重置流量到 stable，删除 canary ingress
func (p *NginxProvider) Reset(ctx context.Context, host, stable, canary string) error {
	lg := log.FromContext(ctx)
	lg.Info("Resetting traffic to stable", "host", host, "stable", stable, "canary", canary)

	// 确保主 ingress 指向 stable service
	if err := p.ensureStableIngress(ctx, host, stable); err != nil {
		return fmt.Errorf("failed to ensure stable ingress: %w", err)
	}

	// 删除 canary ingress
	return p.deleteCanaryIngress(ctx, host)
}

// ensureStableIngress 确保主 ingress 存在并指向 stable service
func (p *NginxProvider) ensureStableIngress(ctx context.Context, host, stableService string) error {
	ingressName := p.getStableIngressName(host)

	var ingress networkingv1.Ingress
	err := p.Client.Get(ctx, client.ObjectKey{Name: ingressName, Namespace: p.Namespace}, &ingress)

	if apierrors.IsNotFound(err) {
		// 创建新的主 ingress
		newIngress := p.createStableIngressSpec(ingressName, host, stableService)
		return p.Client.Create(ctx, newIngress)
	} else if err != nil {
		return err
	}

	// 检查是否需要更新 service
	if p.needsServiceUpdate(&ingress, stableService) {
		p.updateIngressService(&ingress, stableService)
		return p.Client.Update(ctx, &ingress)
	}

	return nil
}

// updateCanaryIngress 创建或更新 canary ingress
func (p *NginxProvider) updateCanaryIngress(ctx context.Context, host, stable, canary string, weight int32) error {
	ingressName := p.getCanaryIngressName(host)

	var ingress networkingv1.Ingress
	err := p.Client.Get(ctx, client.ObjectKey{Name: ingressName, Namespace: p.Namespace}, &ingress)

	if apierrors.IsNotFound(err) {
		// 创建新的 canary ingress
		newIngress := p.createCanaryIngressSpec(ingressName, host, canary, weight)
		return p.Client.Create(ctx, newIngress)
	} else if err != nil {
		return err
	}

	// 更新现有的 canary ingress
	p.updateCanaryAnnotations(&ingress, weight)
	p.updateIngressService(&ingress, canary)
	return p.Client.Update(ctx, &ingress)
}

// promoteToStable 将主 ingress 切换到 canary service
func (p *NginxProvider) promoteToStable(ctx context.Context, host, stable, canary string) error {
	ingressName := p.getStableIngressName(host)

	var ingress networkingv1.Ingress
	if err := p.Client.Get(ctx, client.ObjectKey{Name: ingressName, Namespace: p.Namespace}, &ingress); err != nil {
		return err
	}

	p.updateIngressService(&ingress, canary)
	return p.Client.Update(ctx, &ingress)
}

// deleteCanaryIngress 删除 canary ingress
func (p *NginxProvider) deleteCanaryIngress(ctx context.Context, host string) error {
	ingressName := p.getCanaryIngressName(host)

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: p.Namespace,
		},
	}

	err := p.Client.Delete(ctx, ingress)
	if apierrors.IsNotFound(err) {
		return nil // 已经不存在，认为成功
	}
	return err
}

// 辅助方法
func (p *NginxProvider) getStableIngressName(host string) string {
	return fmt.Sprintf("%s-stable", host)
}

func (p *NginxProvider) getCanaryIngressName(host string) string {
	return fmt.Sprintf("%s-canary", host)
}

func (p *NginxProvider) createStableIngressSpec(name, host, service string) *networkingv1.Ingress {
	pathTypePrefix := networkingv1.PathTypePrefix

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.Namespace,
			Labels: map[string]string{
				"app":   "rollout-stable",
				"track": "stable",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathTypePrefix,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: service,
											Port: networkingv1.ServiceBackendPort{
												Number: 8080,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (p *NginxProvider) createCanaryIngressSpec(name, host, service string, weight int32) *networkingv1.Ingress {
	pathTypePrefix := networkingv1.PathTypePrefix

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.Namespace,
			Labels: map[string]string{
				"app":   "rollout-canary",
				"track": "canary",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/canary":        "true",
				"nginx.ingress.kubernetes.io/canary-weight": strconv.Itoa(int(weight)),
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathTypePrefix,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: service,
											Port: networkingv1.ServiceBackendPort{
												Number: 8080,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (p *NginxProvider) needsServiceUpdate(ingress *networkingv1.Ingress, targetService string) bool {
	if len(ingress.Spec.Rules) == 0 || ingress.Spec.Rules[0].HTTP == nil {
		return true
	}

	if len(ingress.Spec.Rules[0].HTTP.Paths) == 0 {
		return true
	}

	backend := ingress.Spec.Rules[0].HTTP.Paths[0].Backend
	return backend.Service == nil || backend.Service.Name != targetService
}

func (p *NginxProvider) updateIngressService(ingress *networkingv1.Ingress, service string) {
	if len(ingress.Spec.Rules) > 0 && ingress.Spec.Rules[0].HTTP != nil {
		if len(ingress.Spec.Rules[0].HTTP.Paths) > 0 {
			if ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service != nil {
				ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name = service
			}
		}
	}
}

func (p *NginxProvider) updateCanaryAnnotations(ingress *networkingv1.Ingress, weight int32) {
	if ingress.Annotations == nil {
		ingress.Annotations = make(map[string]string)
	}
	ingress.Annotations["nginx.ingress.kubernetes.io/canary"] = "true"
	ingress.Annotations["nginx.ingress.kubernetes.io/canary-weight"] = strconv.Itoa(int(weight))
}

func stringPtr(s string) *string {
	return &s
}
