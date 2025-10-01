# k8s-learning
https://kubernetes.io/zh-cn/docs/tutorials/hello-minikube/

---
# operator
kubebuilder 
官方背书：由 Kubernetes SIGs 维护，与 K8s 核心代码风格一致，兼容性最佳；
标准化：生成的项目结构、代码规范符合社区最佳实践，便于团队协作；
功能完备：内置 CRD 生成、客户端代码生成、Webhook 支持等，无需手动拼接工具链；
学习成本低：文档完善，且与 controller-runtime 深度集成，学会后可无缝迁移到其他工具。

---
# 🦙 Ollama - 本地运行大语言模型

## 安装 Ollama
```bash
# Linux/WSL 一键安装
curl -fsSL https://ollama.com/install.sh | sh

# 启动 Ollama 服务
ollama serve

# 或后台运行
nohup ollama serve > /tmp/ollama.log 2>&1 &
```

### ⚠️ 常见警告说明
安装时可能出现以下警告，**这些都是正常的**：

1. **`WARNING: systemd is not running`**
   - 原因：容器/WSL 环境通常不运行 systemd
   - 影响：无法自动启动服务，需要手动运行 `ollama serve`
   - 解决：无需处理，手动启动即可

2. **`WARNING: Unable to detect NVIDIA/AMD GPU`**
   - 原因：没有检测到 GPU 或缺少 lspci/lshw 工具
   - 影响：将使用 CPU 运行（仍然可用，只是速度较慢）
   - 解决：如有 GPU，安装检测工具：`sudo apt install -y pciutils lshw`

3. **`Warning: could not connect to a running Ollama instance`**
   - 原因：Ollama 服务未启动
   - 解决：运行 `ollama serve`（后台运行或单独终端）

### ✅ 验证安装
```bash
# 检查服务是否运行
ps aux | grep ollama

# 测试 API（默认端口 11434）
curl http://localhost:11434/api/tags

# 查看模型列表
ollama list
```

## 常用命令
```bash
# 查看版本
ollama --version

# 列出已安装的模型
ollama list

# 拉取模型（常用模型）
ollama pull llama3.2:1b     # Meta Llama 3.2 1B（轻量级，1.3GB）
ollama pull llama3.2:3b     # Meta Llama 3.2 3B（平衡性能，2GB）
ollama pull qwen2.5:7b      # 阿里通义千问 2.5 (7B)
ollama pull mistral         # Mistral 7B
ollama pull codellama       # Code Llama (代码模型)
ollama pull gemma2:2b       # Google Gemma 2 (2B)

# 运行模型（交互式）
ollama run llama3.2

# 删除模型
ollama rm llama3.2

# 查看模型信息
ollama show llama3.2
```

## 常用模型推荐
| 模型 | 大小 | 适用场景 |
|------|------|----------|
| `llama3.2:1b` | 1.3GB | 轻量级、快速响应 |
| `llama3.2:3b` | 2GB | 平衡性能与速度 |
| `qwen2.5:7b` | 4.7GB | 中文友好、通用任务 |
| `codellama:7b` | 3.8GB | 代码生成与理解 |
| `gemma2:2b` | 1.6GB | 轻量级、多语言 |

## API 使用
```bash
# 通过 API 调用（默认端口 11434）
curl http://localhost:11434/api/generate -d '{
  "model": "llama3.2",
  "prompt": "为什么天空是蓝色的？",
  "stream": false
}'

# 聊天 API
curl http://localhost:11434/api/chat -d '{
  "model": "llama3.2",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ]
}'
```

## Python 集成
```bash
# 安装 Python 客户端
pip install ollama

# 使用示例
python3 << 'EOF'
import ollama

response = ollama.chat(model='llama3.2', messages=[
  {'role': 'user', 'content': 'Why is the sky blue?'}
])
print(response['message']['content'])
EOF
```

## 模型管理技巧
```bash
# 查看模型存储位置
ls -lh ~/.ollama/models/

# 清理所有模型（释放空间）
ollama rm $(ollama list | awk 'NR>1 {print $1}')

# 检查 Ollama 服务状态
curl http://localhost:11434/api/tags
```

---
# 🔧 jq - 命令行 JSON 处理工具

## 什么是 jq？
`jq` 是一个轻量级且灵活的命令行 JSON 处理器，被称为"JSON 的 sed/awk"。它可以：
- 格式化和美化 JSON 输出
- 提取和过滤 JSON 数据
- 转换 JSON 结构
- 与管道命令完美集成

## 安装 jq
```bash
# Ubuntu/Debian
sudo apt install jq -y

# macOS
brew install jq

# 验证安装
jq --version
```

## 常用命令示例

### 1. 基础用法
```bash
# 美化 JSON（格式化输出）
echo '{"name":"test","value":123}' | jq .

# 提取字段
echo '{"name":"k8s","version":"1.27"}' | jq '.name'
# 输出: "k8s"

# 提取纯文本（去掉引号）
echo '{"name":"k8s"}' | jq -r '.name'
# 输出: k8s
```

### 2. 数组操作
```bash
# 提取数组第一个元素
echo '[{"name":"a"},{"name":"b"}]' | jq '.[0]'

# 遍历数组
echo '[{"name":"a"},{"name":"b"}]' | jq '.[] | .name'

# 过滤数组
echo '[{"name":"a","v":1},{"name":"b","v":2}]' | jq '.[] | select(.v > 1)'
```

### 3. Kubernetes 实际应用
```bash
# 获取所有 Pod 名称
kubectl get pods -o json | jq -r '.items[].metadata.name'

# 获取 Pod 的状态
kubectl get pods -o json | jq -r '.items[] | "\(.metadata.name): \(.status.phase)"'

# 获取容器镜像
kubectl get pods -o json | jq -r '.items[].spec.containers[].image' | sort -u

# 获取 Service 的 ClusterIP
kubectl get svc -o json | jq -r '.items[] | "\(.metadata.name): \(.spec.clusterIP)"'
```

### 4. Ollama API 解析
```bash
# 提取 AI 回复内容
curl -s http://localhost:11434/api/generate -d '{
  "model": "qwen2.5:0.5b",
  "prompt": "Hello",
  "stream": false
}' | jq -r '.response'

# 获取模型列表
curl -s http://localhost:11434/api/tags | jq -r '.models[].name'
```

### 5. 复杂操作
```bash
# 构造新的 JSON
echo '{"a":1,"b":2}' | jq '{name: .a, value: .b}'

# 合并多个字段
echo '{"first":"John","last":"Doe"}' | jq -r '.first + " " + .last'

# 计算数组长度
echo '{"items":[1,2,3]}' | jq '.items | length'

# 映射转换
echo '[1,2,3]' | jq 'map(. * 2)'
# 输出: [2,4,6]
```

## 常用参数
| 参数 | 说明 | 示例 |
|------|------|------|
| `-r` | 输出原始字符串（去引号） | `jq -r '.name'` |
| `-c` | 紧凑输出（单行） | `jq -c .` |
| `-S` | 按键排序 | `jq -S .` |
| `-e` | 设置退出码（用于脚本） | `jq -e '.error'` |
| `-n` | 不读取输入 | `jq -n '{a:1}'` |

## 实用技巧
```bash
# 从文件读取并处理
jq '.name' data.json

# 多个过滤器
echo '{"a":{"b":1}}' | jq '.a | .b'

# 条件判断
echo '{"age":25}' | jq 'if .age >= 18 then "adult" else "minor" end'

# 错误处理（字段不存在时返回 null）
echo '{"name":"test"}' | jq '.missing // "default"'
```

## jq 速查表
```bash
# 基本选择
.foo                    # 获取 foo 字段
.foo.bar               # 嵌套字段
.[0]                   # 数组第一个元素
.[]                    # 遍历数组
.[].name               # 每个元素的 name 字段

# 过滤和转换
select(.age > 18)      # 过滤
map(.name)             # 映射
group_by(.type)        # 分组
sort_by(.age)          # 排序
unique                 # 去重
length                 # 长度

# 组合
.a, .b                 # 多个输出
.a + .b                # 拼接
{name, age}            # 构造对象
```
ollama rm $(ollama list | awk 'NR>1 {print $1}')

# 检查 Ollama 服务状态
curl http://localhost:11434/api/tags
```

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

- helm version  
    version.BuildInfo{Version:"v3.19.0", GitCommit:"3d8990f0836691f0229297773f3524598f46bda6", GitTreeState:"clean", GoVersion:"go1.24.7"}

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

## 网络测试
curl -I https://www.google.com

ping -c 4 www.google.com

## 查看kubelet配置
 kubectl describe nodes my-first-cluster-control-plane<节点名称> | grep -A 20 "System Info"

---


# 环境层级关系

┌─────────────────────────────────────┐
│  宿主机 (Host)                      │
│  ┌───────────────────────────────┐  │
│  │  Dev Container (你当前所在)   │  │
│  │  Ubuntu 24.04.2 LTS          │  │
│  │  ┌─────────────────────────┐  │  │
│  │  │  kind Container         │  │  │
│  │  │  Kubernetes 集群        │  │  │
│  │  │  kubelet 在这里运行     │  │  │
│  │  └─────────────────────────┘  │  │
│  └───────────────────────────────┘  │
└─────────────────────────────────────┘

# 进入 kind 集群容器
docker exec -it my-first-cluster-control-plane bash

# 在容器内查看 kubelet
systemctl status kubelet
ps aux | grep kubelet
```
👤 进程所有者：运行 kubelet 的用户
🆔 进程 ID (PID)：系统分配的进程标识符
💾 内存使用：CPU 和内存占用百分比
⏰ 启动时间：进程启动的具体时间
🚀 启动命令：完整的启动命令行参数
```

journalctl -u kubelet --no-pager -l # 显示 kubelet 的完整系统日志



## 职责对比表

| 组件 | 主要职责 | 类比 | 工作内容 |
|------|----------|------|----------|
| **kube-scheduler** | 调度决策 | 🧠 大脑 - 指挥官 | 决定 Pod 去哪个节点 |
| **kubelet** | 执行操作 | 💪 手脚 - 执行者 | 在节点上实际创建和管理 Pod |
| **API Server** | 协调通信 | 📡 通讯员 | 传递调度决策和状态更新 |

## 创建集群
```bash
kind create cluster --name my-first-cluster
```

## 切换集群 && 查看所有集群
```bash
# 查看当前 Context
kubectl config current-context

# 查看所有 Context
kubectl config get-contexts



## 进入容器内部 redis
```bash
kubectl -n work exec -it deployment/saythx-redis -- redis-cli
```

---
# ⎈ Helm - Kubernetes 包管理工具

## 什么是 Helm？
Helm 是 Kubernetes 的包管理器，就像：
- **apt/yum** 之于 Linux
- **npm** 之于 Node.js
- **pip** 之于 Python

它可以简化 Kubernetes 应用的部署和管理。

## 安装 Helm
```bash
# 方法1：一键安装脚本（推荐）
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# 方法2：二进制安装
wget https://get.helm.sh/helm-v3.19.0-linux-amd64.tar.gz
tar -zxvf helm-v3.19.0-linux-amd64.tar.gz
sudo mv linux-amd64/helm /usr/local/bin/helm

# 验证安装
helm version
```

## 核心概念

| 概念 | 说明 | 类比 |
|------|------|------|
| **Chart** | Helm 包，包含 K8s 应用的所有资源定义 | Docker image |
| **Repository** | Chart 仓库，存储和分享 Chart | Docker Hub |
| **Release** | Chart 的运行实例 | Docker container |
| **Values** | Chart 的配置参数 | 环境变量 |

## 常用命令

### 1. 仓库管理
```bash
# 添加常用仓库
helm repo add stable https://charts.helm.sh/stable
helm repo add bitnami https://charts.bitnami.com/bitnami

# 更新仓库
helm repo update

# 列出所有仓库
helm repo list

# 搜索 Chart
helm search repo nginx
helm search repo mysql

# 删除仓库
helm repo remove stable
```

### 2. Chart 操作
```bash
# 搜索 Chart
helm search hub wordpress    # 在 Artifact Hub 搜索
helm search repo nginx        # 在已添加的仓库搜索

# 查看 Chart 信息
helm show chart bitnami/nginx
helm show values bitnami/nginx
helm show readme bitnami/nginx
helm show all bitnami/nginx

# 下载 Chart
helm pull bitnami/nginx
helm pull bitnami/nginx --untar  # 解压
```

### 3. 安装应用
```bash
# 基本安装
helm install my-nginx bitnami/nginx

# 指定命名空间
helm install my-nginx bitnami/nginx -n demo --create-namespace

# 自定义配置（使用 values 文件）
helm install my-nginx bitnami/nginx -f custom-values.yaml

# 命令行覆盖配置
helm install my-nginx bitnami/nginx --set replicaCount=3

# 试运行（不实际安装）
helm install my-nginx bitnami/nginx --dry-run --debug

# 生成 YAML 清单（不安装）
helm template my-nginx bitnami/nginx
```

### 4. 管理 Release
```bash
# 列出所有 Release
helm list
helm list -A            # 所有命名空间
helm list -n demo       # 指定命名空间

# 查看 Release 状态
helm status my-nginx
helm status my-nginx -n demo

# 获取 Release 的 values
helm get values my-nginx
helm get manifest my-nginx

# 查看历史版本
helm history my-nginx
```

### 5. 升级和回滚
```bash
# 升级 Release
helm upgrade my-nginx bitnami/nginx
helm upgrade my-nginx bitnami/nginx -f new-values.yaml
helm upgrade my-nginx bitnami/nginx --set replicaCount=5

# 升级或安装（不存在则安装）
helm upgrade --install my-nginx bitnami/nginx

# 回滚到上一个版本
helm rollback my-nginx

# 回滚到指定版本
helm rollback my-nginx 2

# 查看回滚差异
helm diff rollback my-nginx 1
```

### 6. 卸载应用
```bash
# 卸载 Release
helm uninstall my-nginx

# 卸载但保留历史
helm uninstall my-nginx --keep-history

# 批量卸载
helm list -q | xargs -L1 helm uninstall
```

## 实际应用示例

### 示例1：安装 Nginx Ingress Controller
```bash
# 添加仓库
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update

# 安装
helm install nginx-ingress ingress-nginx/ingress-nginx \
  --namespace ingress-nginx --create-namespace \
  --set controller.service.type=NodePort

# 检查状态
kubectl get pods -n ingress-nginx
helm status nginx-ingress -n ingress-nginx
```

### 示例2：安装 MySQL
```bash
# 创建自定义配置文件
cat > mysql-values.yaml << EOF
auth:
  rootPassword: "mypassword"
  database: "mydb"
  username: "myuser"
  password: "myuserpassword"
primary:
  persistence:
    size: 8Gi
EOF

# 安装
helm install my-mysql bitnami/mysql -f mysql-values.yaml

# 获取 MySQL 连接信息
kubectl get secret --namespace default my-mysql -o jsonpath="{.data.mysql-root-password}" | base64 -d
```

### 示例3：安装 Redis
```bash
# 安装
helm install my-redis bitnami/redis \
  --set auth.password=redis123 \
  --set master.persistence.size=4Gi

# 连接 Redis
export REDIS_PASSWORD=$(kubectl get secret --namespace default my-redis -o jsonpath="{.data.redis-password}" | base64 -d)
kubectl run --namespace default redis-client --rm --tty -i --restart='Never' \
  --env REDIS_PASSWORD=$REDIS_PASSWORD \
  --image docker.io/bitnami/redis:7.2.6-debian-12-r3 -- bash
redis-cli -h my-redis-master -a $REDIS_PASSWORD
```

### 示例4：安装 Prometheus + Grafana 监控栈
```bash
# 添加仓库
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# 安装 kube-prometheus-stack（包含 Prometheus、Grafana、Alertmanager）
helm install monitoring prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --set grafana.adminPassword=admin123

# 访问 Grafana
kubectl port-forward -n monitoring svc/monitoring-grafana 3000:80
# 浏览器访问: http://localhost:3000 (admin/admin123)
```

## 创建自己的 Chart

```bash
# 创建新 Chart
helm create mychart

# Chart 目录结构
mychart/
  Chart.yaml          # Chart 元数据
  values.yaml         # 默认配置值
  charts/             # 依赖的 Chart
  templates/          # Kubernetes 资源模板
    deployment.yaml
    service.yaml
    ingress.yaml
    _helpers.tpl      # 模板辅助函数

# 验证 Chart
helm lint mychart

# 打包 Chart
helm package mychart

# 本地安装测试
helm install test-release ./mychart --dry-run --debug
helm install test-release ./mychart
```

## 常用仓库

| 仓库 | 地址 | 说明 |
|------|------|------|
| **Bitnami** | https://charts.bitnami.com/bitnami | 最常用，应用最全 |
| **Stable** | https://charts.helm.sh/stable | 官方稳定版（已归档） |
| **Ingress-nginx** | https://kubernetes.github.io/ingress-nginx | Nginx Ingress |
| **Prometheus** | https://prometheus-community.github.io/helm-charts | 监控栈 |
| **Jetstack** | https://charts.jetstack.io | cert-manager |
| **Elastic** | https://helm.elastic.co | ELK 堆栈 |

## 实用技巧

### 1. 查看 Chart 的默认配置
```bash
helm show values bitnami/nginx > nginx-default-values.yaml
```

### 2. 使用多个 values 文件
```bash
helm install my-app ./mychart \
  -f values.yaml \
  -f values-prod.yaml \
  --set image.tag=v2.0.0
```

### 3. 查看将要部署的资源
```bash
helm template my-app ./mychart | kubectl diff -f -
```

### 4. 导出已部署的 Release 配置
```bash
helm get values my-nginx > current-values.yaml
```

### 5. 监控部署进度
```bash
helm upgrade my-app ./mychart --wait --timeout 5m
```

## Helm vs kubectl 对比

| 操作 | kubectl | Helm |
|------|---------|------|
| 部署应用 | 多个 yaml 文件 | 一个 Chart |
| 配置管理 | 手动修改 | values.yaml |
| 版本控制 | 无内置 | 自动版本管理 |
| 回滚 | 手动操作 | `helm rollback` |
| 模板化 | kustomize | 内置 Go template |
| 依赖管理 | 手动 | Chart 依赖 |

## 故障排查

```bash
# 查看部署日志
helm install my-app ./mychart --debug

# 查看实际渲染的 YAML
helm template my-app ./mychart

# 查看 Release 历史
helm history my-app

# 获取 Release 信息
helm get all my-app

# 测试 Chart
helm test my-app
```

## 最佳实践

1. **使用版本管理**：始终在 Chart.yaml 中指定版本号
2. **参数化配置**：将可配置项放入 values.yaml
3. **文档化**：在 Chart 中包含 README.md
4. **测试先行**：使用 `--dry-run` 验证
5. **命名规范**：Release 名称使用有意义的名字
6. **命名空间隔离**：生产环境使用独立命名空间
7. **备份 values**：保存自定义的 values 文件

## 快速参考

```bash
# 安装
helm install <release> <chart>

# 升级
helm upgrade <release> <chart>

# 回滚
helm rollback <release> <revision>

# 卸载
helm uninstall <release>

# 查看
helm list
helm status <release>
helm get values <release>

# 仓库
helm repo add <name> <url>
helm repo update
helm search repo <keyword>
```

# 获取集群列表
kind get clusters

# control-plane 可以理解为集群的主节点（master node）

# 运行aiops 调用ollama
./aiops-propose -n default -p bad-5b779fc7d5-hkqgj --ollama http://127.0.0.1:11434 --model qwen2.5:7b