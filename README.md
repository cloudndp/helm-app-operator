# helm-app-operator

# example: redis-operator
```
kubectl -n kube-system create -f deploy/redis-operator.yaml
```


# build
```
CGO_ENABLED=0 GOOS=linux go build -o bin/helm-app-operator -ldflags '-s -w' cmd/main.go                  

bin/helm-app-operator --crd Test,tests.xiaopal.github.com/v1 --kubeconfig ~/.kube/config --init

bin/helm-app-operator --crd Test,tests.xiaopal.github.com/v1 --kubeconfig ~/.kube/config --install-once=test

bin/helm-app-operator --crd Test,tests.xiaopal.github.com/v1 --kubeconfig ~/.kube/config --uninstall=test

bin/helm-app-operator --crd Test,tests.xiaopal.github.com/v1 --kubeconfig ~/.kube/config --chart=$PWD

```
