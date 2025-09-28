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