对于新手学习 K8s Operator 开发，最友好的项目是 **“基于 Kubebuilder 开发一个 Nginx Operator”**——功能聚焦、依赖简单、覆盖 Operator 核心流程（CRD 定义、控制器逻辑、状态同步），且能直观看到效果（创建 CR 后自动部署 Nginx 实例）。以下是项目的 **完整学习路径、代码实现、测试步骤**，帮你从 0 掌握 Operator 开发核心逻辑。


## 一、项目定位与学习目标
### 1. 项目核心功能
开发一个名为 `NginxOperator` 的控制器，实现：
- 通过自定义资源（CR）`Nginx` 定义 Nginx 实例的配置（如副本数、镜像版本、服务端口）；
- 控制器自动监听 `Nginx` CR 的变化，同步创建/更新/删除对应的 K8s 资源（`Deployment` 部署 Nginx、`Service` 暴露访问入口）；
- 实时更新 `Nginx` CR 的 `Status` 字段（如当前运行的副本数、服务访问地址），便于查看实例状态。

### 2. 新手友好点
- **功能简单**：仅依赖 K8s 核心 API（Deployment、Service），无外部服务（如数据库、监控）依赖；
- **流程完整**：覆盖 Operator 开发全生命周期（环境搭建→CRD 定义→控制器编写→部署测试）；
- **直观验证**：CR 创建后可通过 `kubectl` 直接查看 Nginx Pod、Service，快速验证效果；
- **技术栈主流**：基于 Kubebuilder（K8s 官方推荐的 Operator 开发框架），Go 语言编写，贴合工业界实践。


## 二、前置环境准备
确保本地已安装以下工具（版本建议）：
| 工具         | 作用                          | 版本要求       | 安装参考                          |
|--------------|-------------------------------|----------------|-----------------------------------|
| Go           | 开发语言（Operator 核心语言） | 1.20+          | [Go 官网](https://go.dev/dl/)     |
| Kubebuilder  | Operator 开发脚手架           | 3.0+           | [Kubebuilder 安装指南](https://book.kubebuilder.io/quick-start.html#installation) |
| Kind/Minikube| 本地 K8s 集群（用于测试）     | Kind 0.20+     | [Kind 官网](https://kind.sigs.k8s.io/) |
| Kubectl      | K8s 命令行工具                | 与集群版本匹配 | [Kubectl 安装](https://kubernetes.io/docs/tasks/tools/install-kubectl/) |
| Docker       | 构建 Operator 镜像            | 20.10+         | [Docker 官网](https://docs.docker.com/get-docker/) |


## 三、项目开发步骤（Step-by-Step）
### 步骤 1：初始化 Kubebuilder 项目
Kubebuilder 会自动生成 Operator 项目的目录结构、脚手架代码（如 CRD 生成模板、控制器框架）。

1. **创建项目目录**：
   ```bash
   mkdir nginx-operator && cd nginx-operator
   ```

2. **初始化项目**：
   ```bash
   # 初始化 Go 模块（替换为你的模块名，如 github.com/your-name/nginx-operator）
   go mod init github.com/your-name/nginx-operator
   # 初始化 Kubebuilder 项目（--domain 为自定义域名，如 example.com）
   kubebuilder init --domain example.com --repo github.com/your-name/nginx-operator
   ```
   执行后会生成以下核心目录：
   - `api/`：存放 CRD 的 API 定义（Go 结构体）；
   - `controllers/`：存放控制器逻辑（Reconcile 循环）；
   - `config/`：存放 CRD、RBAC 权限、部署配置等 YAML 文件；
   - `main.go`：Operator 入口文件（启动控制器）。


### 步骤 2：创建自定义资源（CRD）`Nginx`
通过 Kubebuilder 命令生成 `Nginx` CRD 的 API 定义和脚手架代码，定义 CR 的“期望状态（Spec）”和“实际状态（Status）”。

1. **创建 API 组/版本/资源**：
   ```bash
   # 创建 API 组（nginx）、版本（v1alpha1）、资源（Nginx）
   kubebuilder create api --group nginx --version v1alpha1 --kind Nginx
   ```
   执行时会提示：
   - `Create Resource [y/n]`：输入 `y`（生成 CRD 资源定义）；
   - `Create Controller [y/n]`：输入 `y`（生成控制器脚手架）。

2. **定义 `Nginx` CR 的 Spec 与 Status**：
   编辑 `api/v1alpha1/nginx_types.go` 文件，修改 `NginxSpec`（期望配置）和 `NginxStatus`（实际状态）结构体：
   ```go
   package v1alpha1

   import (
       metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
   )

   // NginxSpec 定义 Nginx 实例的期望配置
   type NginxSpec struct {
       // 副本数（默认 1）
       Replicas *int32 `json:"replicas,omitempty"`
       // Nginx 镜像版本（默认 nginx:1.23）
       Image string `json:"image,omitempty"`
       // 服务暴露端口（默认 80）
       ServicePort int32 `json:"servicePort,omitempty"`
   }

   // NginxStatus 定义 Nginx 实例的实际状态
   type NginxStatus struct {
       // 当前运行的副本数
       ReadyReplicas int32 `json:"readyReplicas,omitempty"`
       // 服务访问地址（格式：<service-name>.<namespace>:<port>）
       ServiceAddress string `json:"serviceAddress,omitempty"`
   }

   //+kubebuilder:object:root=true
   //+kubebuilder:subresource:status
   //+kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas"
   //+kubebuilder:printcolumn:name="Image",type="string",JSONPath=".spec.image"
   //+kubebuilder:printcolumn:name="ReadyReplicas",type="integer",JSONPath=".status.readyReplicas"

   // Nginx 是自定义资源的核心结构体
   type Nginx struct {
       metav1.TypeMeta   `json:",inline"`
       metav1.ObjectMeta `json:"metadata,omitempty"`

       Spec   NginxSpec   `json:"spec,omitempty"`
       Status NginxStatus `json:"status,omitempty"`
   }

   //+kubebuilder:object:root=true

   // NginxList 是 Nginx 资源的列表结构体
   type NginxList struct {
       metav1.TypeMeta `json:",inline"`
       metav1.ListMeta `json:"metadata,omitempty"`
       Items           []Nginx `json:"items"`
   }

   func init() {
       SchemeBuilder.Register(&Nginx{}, &NginxList{})
   }
   ```
   关键说明：
   - `//+kubebuilder:subresource:status`：允许 CR 更新 `Status` 字段；
   - `//+kubebuilder:printcolumn`：定义 `kubectl get nginx` 时显示的列（便于查看状态）；
   - `Spec` 字段：用户可配置的参数（副本数、镜像、端口），带 `omitempty` 表示可选；
   - `Status` 字段：控制器自动更新的状态（就绪副本数、服务地址）。

3. **生成 CRD 相关代码与 YAML**：
   ```bash
   # 生成 API 类型的客户端代码（用于控制器操作 CR）
   make generate
   # 生成 CRD 的 YAML 文件（存放在 config/crd/bases/）
   make manifests
   ```
   执行后会在 `config/crd/bases/` 下生成 `nginx.example.com_nginxes.yaml`（CRD 定义文件）。


### 步骤 3：编写控制器核心逻辑（Reconcile 循环）
控制器的核心是 `Reconcile` 方法，负责**同步“期望状态（Nginx CR Spec）”与“实际状态（K8s 资源）”**——即：
1. 当用户创建 `Nginx` CR 时，自动创建对应的 `Deployment` 和 `Service`；
2. 当用户修改 `Nginx` CR（如调整副本数）时，自动更新 `Deployment`；
3. 当 `Deployment`/`Service` 状态变化时（如副本就绪），自动更新 `Nginx` CR 的 `Status`。

编辑 `controllers/nginx_controller.go` 文件，替换 `Reconcile` 方法的逻辑：
```go
package controllers

import (
    "context"
    "fmt"
    "strconv"

    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    nginxv1alpha1 "github.com/your-name/nginx-operator/api/v1alpha1"
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
            Name:      nginx.Name,      // Service 名与 CR 名一致
            Namespace: nginx.Namespace,
        },
        Spec: corev1.ServiceSpec{
            Selector: map[string]string{ // 匹配 Nginx Pod 的标签
                "app": "nginx",
                "cr":  nginx.Name,
            },
            Ports: []corev1.ServicePort{
                {
                    Port:       servicePort, // 服务暴露的端口（用户配置）
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
        For(&nginxv1alpha1.Nginx{}).          // 监听 Nginx CR 的变化
        Owns(&appsv1.Deployment{}).           // 监听关联的 Deployment 变化
        Owns(&corev1.Service{}).              // 监听关联的 Service 变化
        Complete(r)
}
```

#### 关键逻辑解释
1. **CR 获取**：通过 `r.Get()` 拿到当前触发 Reconcile 的 `Nginx` CR 实例；
2. **默认值处理**：避免用户未配置字段导致错误（如副本数默认 1，镜像默认 `nginx:1.23`）；
3. **Deployment/Service 同步**：
   - 用 `SetControllerReference` 建立 CR 与子资源（Deployment/Service）的“.owner-reference”关系，实现“CR 删除时自动删除子资源”；
   - 先检查资源是否存在：不存在则创建，存在则对比 Spec 差异，差异时更新；
4. **Status 更新**：从 Deployment 中获取“就绪副本数”，从 Service 中获取“访问地址”，更新到 CR 的 `Status` 字段，便于用户查看。


### 步骤 4：本地测试（基于 Kind 集群）
#### 1. 启动 Kind 集群（本地测试环境）
```bash
# 创建 Kind 集群（单节点，足够测试）
kind create cluster --name nginx-operator-test
# 确认集群正常
kubectl cluster-info
```

#### 2. 部署 CRD 到集群
```bash
# 应用 CRD YAML（生成的文件在 config/crd/bases/）
kubectl apply -f config/crd/bases/nginx.example.com_nginxes.yaml
# 确认 CRD 已部署
kubectl get crd | grep nginx.example.com
```

#### 3. 运行 Operator 控制器（本地调试模式）
```bash
# 本地启动控制器（连接 Kind 集群，无需构建镜像）
make run ENABLE_WEBHOOKS=false
```
- `ENABLE_WEBHOOKS=false`：新手可暂时关闭 Webhook（复杂功能，后续再学），聚焦核心控制器逻辑；
- 启动成功后，控制台会输出日志，等待监听 `Nginx` CR 的变化。


#### 4. 创建 `Nginx` CR 实例，验证效果
打开新的终端窗口，创建 `nginx-sample.yaml` 文件（CR 实例）：
```yaml
apiVersion: nginx.example.com/v1alpha1
kind: Nginx
metadata:
  name: nginx-sample
  namespace: default
spec:
  replicas: 2          # 副本数 2
  image: nginx:1.24    # 镜像版本 1.24
  servicePort: 8080    # 服务暴露端口 8080
```

应用 CR 并验证：
```bash
# 应用 CR 实例
kubectl apply -f nginx-sample.yaml

# 1. 查看 Nginx CR 状态（Status 会自动更新）
kubectl get nginx nginx-sample -o yaml
# 预期输出：Status.ReadyReplicas=2，Status.ServiceAddress=nginx-sample.default:8080

# 2. 查看自动创建的 Deployment
kubectl get deployment nginx-sample
# 预期输出：READY 2/2（副本数 2，全部就绪）

# 3. 查看自动创建的 Service
kubectl get service nginx-sample
# 预期输出：TYPE=ClusterIP，PORT(S)=8080/TCP

# 4. 测试 Nginx 服务（集群内部访问）
kubectl run -it --rm --image=busybox:1.35 test-pod -- sh
# 在 test-pod 内部执行：
wget -O - http://nginx-sample.default:8080
# 预期输出：Nginx 的默认欢迎页面（证明服务正常）
```


#### 5. 验证更新逻辑（修改 CR 副本数）
```bash
# 修改 CR 的副本数为 3
kubectl patch nginx nginx-sample -p '{"spec":{"replicas":3}}' --type=merge

# 查看 Deployment 副本数是否更新
kubectl get deployment nginx-sample
# 预期输出：READY 3/3（副本数已变为 3）

# 查看 CR Status 是否更新
kubectl get nginx nginx-sample -o jsonpath='{.status.readyReplicas}'
# 预期输出：3
```


#### 6. 验证删除逻辑（删除 CR）
```bash
# 删除 CR 实例
kubectl delete nginx nginx-sample

# 查看 Deployment 和 Service 是否被自动删除
kubectl get deployment nginx-sample  # 预期：NotFound
kubectl get service nginx-sample     # 预期：NotFound
```


## 四、进阶学习方向（项目扩展）
完成基础版本后，可逐步添加功能，深化对 Operator 的理解：
1. **添加 Webhook**：实现 CR 创建/更新前的合法性校验（如禁止副本数小于 0、镜像版本必须指定标签）；
2. **支持滚动升级**：当 CR 的 `image` 字段修改时，实现 Deployment 的滚动升级（而非直接重启）；
3. **集成监控**：在控制器中添加 Prometheus 指标（如 `nginx_operator_reconcile_count` 统计 Reconcile 次数）；
4. **支持持久化存储**：在 CR Spec 中添加 `PersistentVolumeClaim` 配置，控制器自动创建 PVC 并挂载到 Nginx Pod；
5. **构建镜像部署到集群**：通过 `make docker-build docker-push` 构建 Operator 镜像，用 `config/manager/manager.yaml` 部署到 K8s 集群（而非本地运行）。


## 五、关键知识点总结
通过这个项目，你会掌握 Operator 开发的核心概念：
1. **CRD（自定义资源定义）**：如何定义用户可配置的 `Spec` 和控制器维护的 `Status`；
2. **Reconcile 循环**：理解“调谐（Tune）”逻辑——持续同步期望状态与实际状态；
3. **Owner Reference**：通过“所有者引用”实现 CR 与子资源的生命周期绑定；
4. **RBAC 权限**：控制器需要哪些权限才能操作 CR、Deployment、Service（通过 `//+kubebuilder:rbac` 注解自动生成）；
5. **Kubebuilder 工具链**：`make generate`/`make manifests` 等命令的作用，以及项目目录结构的含义。

这个项目的代码完全可复现，且每个步骤都有明确的验证点，非常适合新手入门。当你熟练掌握后，可尝试开发更复杂的 Operator（如管理数据库、消息队列的 Operator）。