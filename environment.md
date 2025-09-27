以下是在 **Linux、macOS、Windows** 系统上安装 `kubectl` 和 `kind` 的常用命令，选择对应系统的步骤即可：


### 一、安装 `kubectl`（跨系统）
`kubectl` 是操作 K8s 集群的命令行工具，所有系统都可通过官方脚本或包管理器安装。

#### 1. Linux（以 Ubuntu/Debian 为例）
```bash
# 方法 1：通过官方脚本下载最新版
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
# 赋予执行权限
chmod +x ./kubectl
# 移动到 PATH 目录（需 sudo 权限）
sudo mv ./kubectl /usr/local/bin/kubectl
# 验证安装
kubectl version --client
```

#### 2. macOS
```bash
# 方法 1：使用 Homebrew（推荐）
brew install kubectl

# 方法 2：手动下载（适用于无 Homebrew 环境）
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/darwin/amd64/kubectl"
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/kubectl

# 验证安装
kubectl version --client
```


### 二、安装 `kind`（跨系统）
`kind` 用于在本地通过 Docker 创建 K8s 集群，依赖 Docker 环境（需先安装 Docker）。

#### 1. Linux
```bash
# 下载最新版（v0.20.0 为示例，可替换为最新版本）
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
# 赋予执行权限
chmod +x ./kind
# 移动到 PATH 目录
sudo mv ./kind /usr/local/bin/kind
# 验证安装
kind version
```

#### 2. macOS
```bash
# 方法 1：Homebrew（推荐）
brew install kind

# 方法 2：手动下载
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-darwin-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# 验证安装
kind version
```


### 三、验证安装与使用
1. **确认 Docker 已启动**（kind 依赖 Docker 运行集群）：
   ```bash
   # Linux/macOS 检查 Docker 状态
   docker --version
   ```

2. **用 kind 创建本地 K8s 集群**：
   ```bash
   kind create cluster --name my-first-cluster
   ```

3. **用 kubectl 操作集群**：
   ```bash
   # 查看集群节点
   kubectl get nodes
   # 输出类似：
   # NAME                         STATUS   ROLES           AGE   VERSION
   # my-first-cluster-control-plane   Ready    control-plane   1m    v1.27.3
   ```


### 说明
- `kubectl` 和 `kind` 的版本会持续更新，建议通过官方文档获取最新安装链接：
  - [kubectl 官方安装指南](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
  - [kind 官方安装指南](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- 若安装后命令无法识别，需检查文件是否已放入系统 `PATH` 目录（如 `/usr/local/bin`、`/usr/bin` 等）。


安装 Kubebuilder 需要根据操作系统选择对应命令，以下是 **Linux、macOS、Windows** 系统的安装步骤（基于最新稳定版，当前推荐 v3.14.0）：


### 一、依赖检查
Kubebuilder 依赖 **Go 1.19+** 环境，先确认 Go 已安装：
```bash
go version  # 输出类似：go version go1.20.xx linux/amd64
```
若未安装 Go，参考 [Go 官方安装指南](https://go.dev/doc/install)。


### 二、安装步骤

#### 1. Linux（amd64 架构）
```bash
# 下载最新稳定版（v3.14.0 为例，可替换为最新版本）
curl -L -o kubebuilder https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.14.0/kubebuilder_linux_amd64

# 赋予执行权限
chmod +x kubebuilder

# 移动到系统 PATH 目录（需 sudo 权限）
sudo mv kubebuilder /usr/local/bin/

# 验证安装
kubebuilder version  # 输出版本信息即成功
```


#### 2. macOS（Intel 芯片/amd64）
```bash
# 下载最新稳定版
curl -L -o kubebuilder https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.14.0/kubebuilder_darwin_amd64

# 赋予权限并移动
chmod +x kubebuilder
sudo mv kubebuilder /usr/local/bin/

# 验证
kubebuilder version
```

**M1/M2 芯片（arm64）**：
```bash
curl -L -o kubebuilder https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.14.0/kubebuilder_darwin_arm64
chmod +x kubebuilder
sudo mv kubebuilder /usr/local/bin/
kubebuilder version
```



### 三、获取最新版本
Kubebuilder 版本会持续更新，可通过以下方式获取最新版本：
1. 访问 [Kubebuilder  Releases 页面](https://github.com/kubernetes-sigs/kubebuilder/releases)，查看最新 tag（如 `v3.14.0`）；
2. 替换上述命令中的版本号（如将 `v3.14.0` 改为最新版本）。


### 四、验证安装
安装成功后，执行以下命令应输出版本信息：
```bash
kubebuilder version
# 示例输出：
# Version: main.version{Version:"v3.14.0", GitCommit:"...", BuildDate:"..."}
```


通过以上步骤，即可完成 Kubebuilder 的安装，后续可使用 `kubebuilder init` 等命令初始化 Operator 项目。