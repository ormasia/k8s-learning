package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nginxv1alpha1 "github.com/ormasia/nginx-operator/api/v1alpha1"
)

// NginxReconciler 实现控制器的核心逻辑
type NginxReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=nginx.example.com,resources=nginxes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nginx.example.com,resources=nginxes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nginx.example.com,resources=nginxes/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile 是控制器的调谐循环（核心方法）
func (r *NginxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. 获取当前的 Nginx CR 实例
	nginx := &nginxv1alpha1.Nginx{}
	if err := r.Get(ctx, req.NamespacedName, nginx); err != nil {
		if errors.IsNotFound(err) {
			// CR 已被删除，无需处理
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Nginx CR")
		return ctrl.Result{}, err
	}

	// 2. 处理默认值（如果用户未配置 Spec 字段）
	replicas := int32(1)
	if nginx.Spec.Replicas != nil {
		replicas = *nginx.Spec.Replicas
	}
	image := "nginx:1.23"
	if nginx.Spec.Image != "" {
		image = nginx.Spec.Image
	}
	servicePort := int32(80)
	if nginx.Spec.ServicePort != 0 {
		servicePort = nginx.Spec.ServicePort
	}

	// 3. 同步创建/更新 Deployment（部署 Nginx Pod）
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nginx.Name,      // Deployment 名与 CR 名一致
			Namespace: nginx.Namespace, // 与 CR 在同一命名空间
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
					"cr":  nginx.Name, // 关联 CR 的标签（确保只管理当前 CR 的 Pod）
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "nginx",
						"cr":  nginx.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: image,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 80}, // Nginx 容器内部端口（固定 80）
							},
						},
					},
				},
			},
		},
	}

	// 关联 Deployment 与 CR（便于垃圾回收：CR 删除时自动删除 Deployment）
	if err := ctrl.SetControllerReference(nginx, deployment, r.Scheme); err != nil {
		log.Error(err, "Failed to set owner reference for Deployment")
		return ctrl.Result{}, err
	}

	// 检查 Deployment 是否存在：不存在则创建，存在则更新
	existingDeployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, existingDeployment); err != nil {
		if errors.IsNotFound(err) {
			// 创建 Deployment
			if err := r.Create(ctx, deployment); err != nil {
				log.Error(err, "Failed to create Deployment")
				return ctrl.Result{}, err
			}
			log.Info("Deployment created successfully")
		} else {
			log.Error(err, "Failed to get existing Deployment")
			return ctrl.Result{}, err
		}
	} else {
		// 更新 Deployment（仅当 Spec 与现有不一致时）
		if *existingDeployment.Spec.Replicas != replicas || existingDeployment.Spec.Template.Spec.Containers[0].Image != image {
			existingDeployment.Spec.Replicas = &replicas
			existingDeployment.Spec.Template.Spec.Containers[0].Image = image
			if err := r.Update(ctx, existingDeployment); err != nil {
				log.Error(err, "Failed to update Deployment")
				return ctrl.Result{}, err
			}
			log.Info("Deployment updated successfully")
		}
	}

	// 4. 同步创建/更新 Service（暴露 Nginx 访问）
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nginx.Name, // Service 名与 CR 名一致
			Namespace: nginx.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{ // 匹配 Nginx Pod 的标签
				"app": "nginx",
				"cr":  nginx.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       servicePort,        // 服务暴露的端口（用户配置）
					TargetPort: intstr.FromInt(80), // 指向 Pod 内部的 80 端口
				},
			},
			Type: corev1.ServiceTypeClusterIP, // 集群内部可访问（新手友好，无需外部负载均衡）
		},
	}

	// 关联 Service 与 CR
	if err := ctrl.SetControllerReference(nginx, service, r.Scheme); err != nil {
		log.Error(err, "Failed to set owner reference for Service")
		return ctrl.Result{}, err
	}

	// 检查 Service 是否存在：不存在则创建，存在则更新
	existingService := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, existingService); err != nil {
		if errors.IsNotFound(err) {
			// 创建 Service
			if err := r.Create(ctx, service); err != nil {
				log.Error(err, "Failed to create Service")
				return ctrl.Result{}, err
			}
			log.Info("Service created successfully")
		} else {
			log.Error(err, "Failed to get existing Service")
			return ctrl.Result{}, err
		}
	} else {
		// 更新 Service（仅当端口不一致时）
		if existingService.Spec.Ports[0].Port != servicePort {
			existingService.Spec.Ports[0].Port = servicePort
			if err := r.Update(ctx, existingService); err != nil {
				log.Error(err, "Failed to update Service")
				return ctrl.Result{}, err
			}
			log.Info("Service updated successfully")
		}
	}

	// 5. 更新 Nginx CR 的 Status 字段（反馈实际状态）
	// 获取最新的 Deployment 状态（就绪副本数）
	if err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, existingDeployment); err != nil {
		log.Error(err, "Failed to get Deployment status for updating Nginx Status")
		return ctrl.Result{}, err
	}
	// 获取最新的 Service 地址
	if err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, existingService); err != nil {
		log.Error(err, "Failed to get Service address for updating Nginx Status")
		return ctrl.Result{}, err
	}
	serviceAddress := fmt.Sprintf("%s.%s:%d", existingService.Name, existingService.Namespace, existingService.Spec.Ports[0].Port)

	// 更新 Status（仅当状态变化时）
	if nginx.Status.ReadyReplicas != existingDeployment.Status.ReadyReplicas || nginx.Status.ServiceAddress != serviceAddress {
		nginx.Status.ReadyReplicas = existingDeployment.Status.ReadyReplicas
		nginx.Status.ServiceAddress = serviceAddress
		if err := r.Status().Update(ctx, nginx); err != nil {
			log.Error(err, "Failed to update Nginx Status")
			return ctrl.Result{}, err
		}
		log.Info("Nginx Status updated successfully", "ReadyReplicas", nginx.Status.ReadyReplicas, "ServiceAddress", nginx.Status.ServiceAddress)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager 将控制器注册到 Manager（启动时加载）
func (r *NginxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nginxv1alpha1.Nginx{}). // 监听 Nginx CR 的变化
		Owns(&appsv1.Deployment{}).  // 监听关联的 Deployment 变化
		Owns(&corev1.Service{}).     // 监听关联的 Service 变化
		Complete(r)
}
