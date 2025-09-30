可以！kind 完全支持创建多节点 Kubernetes 集群。让我为你演示如何创建和管理多节点集群。

## 🚀 创建多节点 kind 集群

### 方法1：使用配置文件（推荐）

```bash
# 创建多节点集群配置文件
cat > /tmp/kind-multi-node-config.yaml << 'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
- role: worker
EOF

# 使用配置创建集群
kind create cluster --name multi-node --config /tmp/kind-multi-node-config.yaml

# 验证集群节点
kubectl get nodes -o wide
```

### 方法2：创建高可用（HA）控制平面集群

```bash
# 创建 HA 集群配置（3 个 control-plane + 3 个 worker）
cat > /tmp/kind-ha-config.yaml << 'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: control-plane
- role: control-plane
- role: worker
- role: worker
- role: worker
EOF

# 创建 HA 集群
kind create cluster --name ha-cluster --config /tmp/kind-ha-config.yaml
```

## 📊 验证多节点集群

```bash
# 查看集群节点
kubectl get nodes -o wide

# 查看 Docker 容器（每个节点一个容器）
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# 查看节点详细信息
kubectl describe nodes

# 查看 Pod 分布在不同节点上
kubectl get pods -A -o wide
```

## 🎯 实际演示 - 创建测试集群

让我为你创建一个完整的多节点集群示例：

```bash
#!/bin/bash
echo "🚀 创建 kind 多节点集群"
echo "========================"

# 1. 创建配置文件
cat > /tmp/kind-test-cluster.yaml << 'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: test-multi-node
nodes:
- role: control-plane
  # 可选：自定义端口映射
  extraPortMappings:
  - containerPort: 30000
    hostPort: 30000
    protocol: TCP
- role: worker
  labels:
    tier: frontend
- role: worker
  labels:
    tier: backend
- role: worker
  labels:
    tier: database
EOF

# 2. 创建集群
kind create cluster --config /tmp/kind-test-cluster.yaml

# 3. 验证集群
echo ""
echo "📊 集群节点："
kubectl get nodes -o wide

echo ""
echo "🐳 Docker 容器："
docker ps --filter "name=test-multi-node" --format "table {{.Names}}\t{{.Status}}"

echo ""
echo "🏷️  节点标签："
kubectl get nodes --show-labels

# 4. 测试 Pod 调度到不同节点
echo ""
echo "🧪 测试 Pod 分布："
kubectl create deployment nginx --image=nginx --replicas=3

sleep 5

echo ""
echo "📍 Pod 分布在不同节点："
kubectl get pods -o wide
```

保存并执行：
```bash
chmod +x /tmp/create-multi-node.sh
/tmp/create-multi-node.sh
```

## 🔍 高级配置选项

### 配置网络和端口映射

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
- role: worker
- role: worker
```

### 配置节点资源限制

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
  # 使用不同的镜像
  image: kindest/node:v1.27.3
- role: worker
  # 自定义标签
  labels:
    disk: ssd
```

## 📋 多节点集群的优势

### 在你的学习环境中：

1. **测试调度策略**：
```bash
# 创建带节点亲和性的 Deployment
kubectl apply -f - << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
spec:
  replicas: 3
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: tier
                operator: In
                values:
                - frontend
      containers:
      - name: nginx
        image: nginx
EOF

kubectl get pods -o wide
```

2. **测试 Pod 驱逐和重新调度**：
```bash
# 标记节点为不可调度
kubectl cordon <worker-node-name>

# 驱逐节点上的 Pod
kubectl drain <worker-node-name> --ignore-daemonsets --delete-emptydir-data

# 观察 Pod 重新调度
kubectl get pods -o wide -w
```

3. **测试节点故障恢复**：
```bash
# 停止一个 worker 节点容器
docker stop <worker-container-name>

# 观察 Pod 状态和重新调度
kubectl get pods -A -o wide -w
```

## 🔧 管理多节点集群

```bash
# 查看所有 kind 集群
kind get clusters

# 切换集群上下文
kubectl config use-context kind-multi-node

# 查看集群信息
kubectl cluster-info --context kind-multi-node

# 删除特定集群
kind delete cluster --name multi-node

# 删除所有 kind 集群
kind delete clusters --all
```

## 🎯 验证脚本

创建一个完整的验证脚本：

```bash
#!/bin/bash
echo "🔍 多节点集群验证"
echo "=================="

echo "1️⃣ 集群节点状态："
kubectl get nodes -o custom-columns="NAME:.metadata.name,STATUS:.status.conditions[-1].type,ROLES:.metadata.labels.kubernetes\.io/role,VERSION:.status.nodeInfo.kubeletVersion"

echo ""
echo "2️⃣ Docker 容器映射："
docker ps --filter "name=kind" --format "table {{.Names}}\t{{.Image}}\t{{.Ports}}"

echo ""
echo "3️⃣ Pod 分布统计："
kubectl get pods -A -o wide | awk '{print $8}' | sort | uniq -c

echo ""
echo "4️⃣ 节点资源分配："
kubectl describe nodes | grep -A 5 "Allocated resources"

echo ""
echo "5️⃣ 创建测试 Deployment："
kubectl create deployment multi-node-test --image=nginx --replicas=6 --dry-run=client -o yaml | kubectl apply -f -

sleep 5

echo ""
echo "6️⃣ Pod 在各节点的分布："
kubectl get pods -l app=multi-node-test -o wide

echo ""
echo "✅ 验证完成！"
```

**总结**：kind 完全支持多节点集群，你可以创建任意数量的 control-plane 和 worker 节点，每个节点对应一个 Docker 容器，非常适合学习 Kubernetes 的调度、高可用、故障恢复等特性！🚀