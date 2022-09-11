## 安装前准备

```shell
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.9.1/cert-manager.yaml
```

## 安装 Firefly

```shell
(⎈ |ik8s01:default)➜  ~ kubectl create ns firefly-system
namespace/firefly-system created
(⎈ |ik8s01:default)➜  ~ kubectl apply -f https://raw.githubusercontent.com/carlory/firefly/main/deploy/install.firefly.io_karmadas.yaml
customresourcedefinition.apiextensions.k8s.io/karmadas.install.firefly.io created
(⎈ |ik8s01:default)➜  ~ kubectl apply -f https://raw.githubusercontent.com/carlory/firefly/main/deploy/install.yaml
deployment.apps/firefly-controller-manager created
serviceaccount/firefly-controller-manager created
clusterrolebinding.rbac.authorization.k8s.io/firefly-controller-manager created
clusterrole.rbac.authorization.k8s.io/firefly-aggregate-to-admin created
(⎈ |ik8s01:default)➜  ~ kubens firefly-system
Context "ik8s01" modified.
Active namespace is "firefly-system".
(⎈ |ik8s01:firefly-system)➜  ~ kubectl -n firefly-system get po
NAME                                          READY   STATUS    RESTARTS   AGE
firefly-controller-manager-56db99fb85-fx4xh   1/1     Running   0          40s
```

## 创建 Karmada

部署 Karmada

```shell
(⎈ |ik8s01:firefly-system)➜  ~ kubectl apply -f https://raw.githubusercontent.com/carlory/firefly/main/example/demo.yaml
karmada.install.firefly.io/demo created

(⎈ |ik8s01:firefly-system)➜  ~ kubectl -n firefly-system get po,svc,secret
NAME                                                         READY   STATUS    RESTARTS   AGE
pod/demo-etcd-0                                             1/1     Running   0          21s
pod/demo-firefly-karmada-manager-5678598d95-dcrf5           1/1     Running   0          8s
pod/demo-karmada-aggregated-apiserver-65c77fbf4d-vdr9k      1/1     Running   0          11s
pod/demo-karmada-apiserver-8495c66b47-kllwn                 1/1     Running   0          21s
pod/demo-karmada-controller-manager-6fbd48544b-jdd7f        1/1     Running   0          8s
pod/demo-karmada-kube-controller-manager-57f9fd76f6-z9whl   1/1     Running   0          8s
pod/demo-karmada-scheduler-64b65b45d8-sxw8q                 1/1     Running   0          8s
pod/demo-karmada-webhook-5488959847-n7lzk                   1/1     Running   0          8s
pod/firefly-controller-manager-7fd49597b6-4s4zz              1/1     Running   0          69s

NAME                                         TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)             AGE
service/demo-etcd                           ClusterIP   None             <none>        2379/TCP,2380/TCP   21s
service/demo-karmada-aggregated-apiserver   ClusterIP   10.111.178.231   <none>        443/TCP             11s
service/demo-karmada-apiserver              ClusterIP   10.111.66.86     <none>        5443/TCP            21s
service/demo-karmada-webhook                ClusterIP   10.100.216.76    <none>        443/TCP             8s

NAME                                               TYPE                                  DATA   AGE
secret/default-token-4chv4                         kubernetes.io/service-account-token   3      74m
secret/demo-etcd-cert                             Opaque                                4      21s
secret/demo-firefly-karmada-manager-token-fpptr   kubernetes.io/service-account-token   3      42m
secret/demo-karmada-cert                          Opaque                                16     21s
secret/demo-karmada-webhook-cert                  Opaque                                2      21s
secret/demo-kubeconfig                            Opaque                                1      21s
secret/firefly-controller-manager-token-w6xzh      kubernetes.io/service-account-token   3      69s
```

登陆集群主机, 接入成员集群

```shell
[root@ik8s01 firefly]# vi /etc/hosts
10.111.66.86    demo-karmada-apiserver.firefly-system.svc.cluster.local

[root@ik8s01 firefly]# kubectl get  -n firefly-system secret demo-kubeconfig -ojsonpath='{.data.kubeconfig}' | base64 -d > config
[root@ik8s01 firefly]# karmadactl join ik8s --kubeconfig config --cluster-kubeconfig ../clusters/ik8s.config --cluster-context  kubernetes-admin@cluster.local 
cluster(ik8s) is joined successfully
```

查看自动部署的调度器估算器

```shell
(⎈ |ik8s01:firefly-system)➜  ~ kubectl -n firefly-system get po,svc,rolebinding
NAME                                                         READY   STATUS    RESTARTS   AGE
pod/demo-etcd-0                                             1/1     Running   0          2m22s
pod/demo-firefly-karmada-manager-5678598d95-dcrf5           1/1     Running   0          2m9s
pod/demo-karmada-aggregated-apiserver-65c77fbf4d-vdr9k      1/1     Running   0          2m12s
pod/demo-karmada-apiserver-8495c66b47-kllwn                 1/1     Running   0          2m22s
pod/demo-karmada-controller-manager-6fbd48544b-jdd7f        1/1     Running   0          2m9s
pod/demo-karmada-kube-controller-manager-57f9fd76f6-z9whl   1/1     Running   0          2m9s
pod/demo-karmada-scheduler-64b65b45d8-sxw8q                 1/1     Running   0          2m9s
pod/demo-karmada-webhook-5488959847-n7lzk                   1/1     Running   0          2m9s
pod/firefly-controller-manager-7fd49597b6-4s4zz              1/1     Running   0          3m10s
pod/karmada-scheduler-estimator-ik8s-69c8656785-r7lzq        1/1     Running   0          17s

NAME                                         TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)             AGE
service/demo-etcd                           ClusterIP   None             <none>        2379/TCP,2380/TCP   2m23s
service/demo-karmada-aggregated-apiserver   ClusterIP   10.111.178.231   <none>        443/TCP             2m13s
service/demo-karmada-apiserver              ClusterIP   10.111.66.86     <none>        5443/TCP            2m23s
service/demo-karmada-webhook                ClusterIP   10.100.216.76    <none>        443/TCP             2m10s
service/karmada-scheduler-estimator-ik8s     ClusterIP   10.103.181.159   <none>        10352/TCP           18s

NAME                                                                  ROLE                AGE
rolebinding.rbac.authorization.k8s.io/demo-firefly-karmada-manager   ClusterRole/admin   2m10s
```

## 删除 Karmada

```shell
(⎈ |ik8s01:firefly-system)➜  ~ kubectl -n firefly-system get po
NAME                                          READY   STATUS        RESTARTS   AGE
demo-karmada-apiserver-8495c66b47-kllwn      1/1     Terminating   0          3m16s
firefly-controller-manager-7fd49597b6-4s4zz   1/1     Running       0          4m4s
(⎈ |ik8s01:firefly-system)➜  ~ kubectl -n firefly-system get svc,secret,rolebinding
NAME                                               TYPE                                  DATA   AGE
secret/default-token-4chv4                         kubernetes.io/service-account-token   3      77m
secret/demo-firefly-karmada-manager-token-fpptr   kubernetes.io/service-account-token   3      45m
secret/firefly-controller-manager-token-w6xzh      kubernetes.io/service-account-token   3      4m15s
```
