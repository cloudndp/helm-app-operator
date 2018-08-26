# helm-app-operator

# example: redis-operator
```
$ kubectl -n kube-system create -f- <<\EOF
# deploy/redis-operator.yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: redis-operator

---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: redis-operator
subjects:
- kind: ServiceAccount
  name: redis-operator
roleRef:
  kind: ClusterRole
  name: admin
  apiGroup: rbac.authorization.k8s.io

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis-operator
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis-operator
  template:
    metadata:
      labels:
        app: redis-operator
    spec:
      # hostNetwork: true
      serviceAccountName: redis-operator
      containers:
        - name: operator
          image: xiaopal/helm-app-operator
          command: 
            - /bin/bash
            - -c
            - helm-app-operator init && 
              helm-app-operator install --once redis &&
              helm fetch stable/redis --untar && 
              exec helm-app-operator --chart=/redis
          env:
            - name: CRD_RESOURCE
              value: RedisApp,redisapps.xiaopal.github.com/v1beta1
            - name: OPERATOR_NAME
              valueFrom: {fieldRef: {fieldPath: "metadata.labels['app']"}}
EOF
```


# build/test
```
CGO_ENABLED=0 GOOS=linux go build -o bin/helm-app-operator -ldflags '-s -w' cmd/*.go                  

bin/helm-app-operator --crd Test,tests.xiaopal.github.com/v1 --kubeconfig ~/.kube/config init

bin/helm-app-operator --crd Test,tests.xiaopal.github.com/v1 --kubeconfig ~/.kube/config install test

bin/helm-app-operator --crd Test,tests.xiaopal.github.com/v1 --kubeconfig ~/.kube/config --chart=$PWD

bin/helm-app-operator --crd Test,tests.xiaopal.github.com/v1 --kubeconfig ~/.kube/config uninstall test

```
