# 对外暴露集群的方式对比

## 🔌 集群对外暴露的真实架构

### 1. 📋 各种暴露方式对比

| 方式 | 是否需要CCM | 工作层次 | 适用场景 |
|------|-------------|----------|----------|
| **NodePort** | ❌ 不需要 | L4传输层 | 开发测试环境 |
| **LoadBalancer + CCM** | ✅ 需要 | L4传输层 | 生产环境，自动LB |
| **Ingress Controller** | ❌ 不需要CCM | L7应用层 | HTTP/HTTPS服务 |
| **ExternalName** | ❌ 不需要 | DNS层 | 服务重定向 |

### 2. 🏗️ 真实云环境架构流程

#### AWS EKS + CCM 完整流程：
```
Internet
    ↓
AWS Application Load Balancer (由CCM创建)
    ↓
AWS Target Groups (CCM管理)
    ↓
EC2 Instances:NodePort (Kubernetes节点)
    ↓
kube-proxy (iptables规则)
    ↓
Pod Network (CNI)
    ↓
应用Pod
```

#### 详细步骤：
```bash
# 1. 用户创建LoadBalancer Service
kubectl apply -f service.yaml

# 2. kube-controller-manager处理Service对象
Service Controller → 创建Service → 分配ClusterIP

# 3. CCM Service Controller被触发
aws-cloud-controller-manager → 检测到LoadBalancer类型

# 4. CCM调用AWS API
CreateLoadBalancer → 创建ALB/NLB
CreateTargetGroup → 创建目标组
RegisterTargets → 注册EC2实例

# 5. 更新Service状态
Service.status.loadBalancer.ingress[0].hostname = "xxx.elb.amazonaws.com"

# 6. 流量路径建立
外部请求 → AWS LB → EC2:NodePort → Pod
```

### 3. 🔍 CCM vs Ingress Controller 对比

#### Cloud Controller Manager:
```yaml
作用域: 云基础设施集成
层次: L4 负载均衡
管理: 云负载均衡器生命周期
示例:
  LoadBalancer Service → CCM → AWS ELB
  节点故障 → CCM → 自动清理云资源
```

#### Ingress Controller:
```yaml
作用域: 应用层路由
层次: L7 HTTP/HTTPS
管理: 域名路由和SSL终结
示例:
  Ingress规则 → nginx-controller → 域名路由
  TLS证书 → cert-manager → 自动续期
```

### 4. 🎯 实际生产环境组合使用

#### 典型架构：
```
Internet
    ↓
Route 53 (DNS)
    ↓
AWS ALB (CCM创建的LoadBalancer)
    ↓  
Ingress Controller (nginx/traefik)
    ↓
各种Service (ClusterIP)
    ↓
应用Pod
```

#### 配置示例：
```yaml
# 1. CCM管理的LoadBalancer - 入口负载均衡器
apiVersion: v1
kind: Service
metadata:
  name: ingress-nginx-controller
  namespace: ingress-nginx
spec:
  type: LoadBalancer  # CCM会创建AWS ALB
  ports:
  - port: 80
    targetPort: 80
  - port: 443 
    targetPort: 443
  selector:
    app.kubernetes.io/name: ingress-nginx

---
# 2. Ingress规则 - 应用层路由
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app-ingress
spec:
  rules:
  - host: api.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: api-service
            port:
              number: 80
  - host: web.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: web-service  
            port:
              number: 80
```

### 5. 📊 总结

#### CCM的真实价值：
- 🔗 **自动化云集成**：无需手动创建LB
- 🛡️ **故障自愈**：节点故障自动清理
- 💰 **成本优化**：资源自动回收
- 🔄 **状态同步**：云资源与K8s状态一致

#### 对外暴露的完整链路：
```
CCM ≠ 直接对外暴露
CCM = 云基础设施的自动化管理员

真正的对外暴露链路：
外部请求 → DNS → 云LB(CCM管理) → Ingress(应用路由) → Service → Pod
```