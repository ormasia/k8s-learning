# k8s-learning
https://kubernetes.io/zh-cn/docs/tutorials/hello-minikube/

---
# operator
kubebuilder 
官方背书：由 Kubernetes SIGs 维护，与 K8s 核心代码风格一致，兼容性最佳；
标准化：生成的项目结构、代码规范符合社区最佳实践，便于团队协作；
功能完备：内置 CRD 生成、客户端代码生成、Webhook 支持等，无需手动拼接工具链；
学习成本低：文档完善，且与 controller-runtime 深度集成，学会后可无缝迁移到其他工具。

# version details
- go version go1.24.5 linux/amd64

-   kubectl version  
    Client Version: v1.34.1  
    Kustomize Version: v5.7.1  
    Server Version: v1.27.3  
    Warning: version difference between client (1.34) and server (1.27) exceeds the supported minor version skew of +/-1

- kubebuilder version  
    Version: main.version{KubeBuilderVersion:"3.14.0", KubernetesVendor:"1.27.1",   GitCommit:"11053630918ac421cb6eb6f0a3225e2a2ad49535", BuildDate:"2024-01-30T09:29:27Z",     GoOs:"linux", GoArch:"amd64"}

- kind version  
    kind v0.20.0 go1.20.4 linux/amd64

- docker version  
    Client:  
    Version:           28.3.1-1  
    API version:       1.51  
    Go version:        go1.23.8  
    Git commit:        38b7060a218775811da953650d8df7d492653f8f  
    Built:             Tue Jun 24 15:37:19 UTC 2025  
    OS/Arch:           linux/amd64  
    Context:           default  

    Server:  
    Engine:  
    Version:          28.3.1-1  
    API version:      1.51 (minimum version 1.24)  
    Go version:       go1.23.8  
    Git commit:       5beb93de84f02bf7e167cbb87ce81355ddd8f560  
    Built:            Wed Jul  2 13:31:29 2025  
    OS/Arch:          linux/amd64  
    Experimental:     false  
    containerd:  
    Version:          1.7.27-1  
    GitCommit:        05044ec0a9a75232cad458027ca83437aae3f4da  
    runc:  
    Version:          1.1.15-1  
    GitCommit:        bc20cb4497af9af01bea4a8044f1678ffca2745c  
    docker-init:  
    Version:          0.19.0  
    GitCommit:        de40ad0  

# 命令

## 查看svc
要查看集群里的“所有 Service”：

- 全部命名空间
  ```bash
  kubectl get svc -A
  ```
- 全部命名空间（更多列）
  ```bash
  kubectl get services --all-namespaces -o wide
  ```
- 显示服务的标签
  ```bash
  kubectl get svc -A --show-labels
  ```
- 仅某个命名空间
  ```bash
  kubectl get svc -n default
  ```
- 按标签筛选
  ```bash
  kubectl get svc -A -l app=demo
  ```
- 持续观察
  ```bash
  watch -n1 'kubectl get svc -A -o wide'
  ```

补充：想看每个 Service 的后端端点
- 传统 Endpoints
  ```bash
  kubectl get endpoints -A
  ```
- EndpointSlice（新资源）
  ```bash
  kubectl get endpointslices.discovery.k8s.io -A
  ```

  ## 通用命令
- 查看所有资源
  ```bash
  kubectl get all -A
  ```
- 查看所有资源（更多列）
  ```bash
  kubectl get all -A -o wide
  ```

## 清理命令

### 清理 rollout-operator
- 完全卸载 operator（包含 CRD、RBAC、服务等）
  ```bash
  cd /workspaces/k8s-learning/rollout-operator && make undeploy
  ```
- 清理特定应用的资源
  ```bash
  kubectl delete rollout,deploy,svc,ingress -l app=demo
  ```
- 强制删除命名空间（如果卡住）
  ```bash
  kubectl delete namespace rollout-operator-system --force --grace-period=0
  ```

### 验证清理结果
- 检查是否还有 rollout-operator 相关服务
  ```bash
  kubectl get services --all-namespaces -o wide
  ```
- 检查集群总体状态
  ```bash
  kubectl get all -A
  ```

## Webhook TLS 证书配置

### 方案1：安装 cert-manager（生产推荐）
cert-manager 是 Kubernetes 标准的证书管理工具，自动处理证书生成和轮换：

```bash
# 安装 cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# 等待 cert-manager 就绪
kubectl wait --for=condition=ready pod -l app=cert-manager -n cert-manager --timeout=60s
kubectl wait --for=condition=ready pod -l app=cainjector -n cert-manager --timeout=60s  
kubectl wait --for=condition=ready pod -l app=webhook -n cert-manager --timeout=60s

# 验证安装
kubectl get pods -n cert-manager
```

### 方案2：使用自签名证书（开发测试）
开发环境可以使用自签名证书：

```bash
# 生成自签名证书
openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -days 365 -nodes \
  -subj "/CN=rollout-operator-webhook-service.rollout-operator-system.svc"

# 创建 secret
kubectl create secret tls webhook-server-cert --cert=tls.crt --key=tls.key -n rollout-operator-system

# 获取 CA bundle（用于 webhook 配置）
cat tls.crt | base64 -w 0
```

## 📋 方案对比说明

**cert-manager 和自签名证书不冲突**，它们是两种不同的证书管理策略：

### cert-manager 方案
- **自动化**：自动生成、轮换证书
- **生产级**：支持 Let's Encrypt、内部CA等
- **一次配置**：配置好后无需手动干预
- **使用场景**：生产环境、长期项目

### 自签名证书方案  
- **手动管理**：需要手动生成和更新证书
- **简单快速**：适合快速测试和开发
- **临时使用**：证书有效期内有效（365天）
- **使用场景**：开发测试、一次性部署

### 当前状态
你现在两种方案都已准备好：
1. ✅ **cert-manager 已安装** - 可用于自动化证书管理
2. ✅ **自签名证书已生成** - 位于 `/tmp/tls.*`，可手动使用

### 方案3：禁用 webhook（最简单）
如果不需要 webhook 功能，可以禁用：

```bash
# 删除 CRD 中的 webhook 配置
kubectl patch crd rollouts.delivery.example.com --type='merge' -p='{"spec":{"conversion":{"strategy":"None"}}}'

# 或者删除并重新应用没有 webhook 的 CRD
cd /workspaces/k8s-learning/rollout-operator
kubectl delete -k config/crd
kubectl apply -k config/crd --dry-run=client  # 检查配置
kubectl apply -k config/crd
```

## 测试流程

### 使用本地 operator 测试（推荐）
```bash
# 关闭之前的rollout资源
kubectl delete rollout demo-rollout -n default

# 本地运行（禁用webhook）
cd /workspaces/k8s-learning/rollout-operator
ENABLE_WEBHOOKS=false go run cmd/main.go

# 创建新的rollout测试
kubectl apply -f rollout-sample.yaml

# 查看资源
kubectl get rollouts,deploy,svc,ingress -l app=demo
```

### 使用集群部署（需要证书）
```bash
# 重新部署operator（应用新的流量分割功能）
cd /workspaces/k8s-learning/rollout-operator && make deploy

# 如果遇到证书问题，检查webhook配置
kubectl get validatingwebhookconfigurations,mutatingwebhookconfigurations
```


方案 1：cert-manager（推荐）
优点：

自动化证书管理
证书自动轮换
生产环境标准
支持多种 CA（Let's Encrypt、自签名等）
安装演示：


kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

kubectl wait --for=condition=ready pod -l app=cert-manager -n cert-manager --timeout=60s

kubectl get pods -n cert-manager
太好了！cert-manager 已经安装成功。

方案 2：自签名证书（开发测试）

cd /tmp && openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -days 365 -nodes -subj "/CN=rollout-operator-webhook-service.rollout-operator-system.svc"

ls -la /tmp/tls.*