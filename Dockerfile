FROM golang:1.10 as build
ADD . /go/src/github.com/xiaopal/helm-app-operator
WORKDIR  /go/src/github.com/xiaopal/helm-app-operator
RUN CGO_ENABLED=0 GOOS=linux go build -o /helm-app-operator -ldflags '-s -w' cmd/main.go
RUN chmod +x /helm-app-operator

FROM xiaopal/helm-client:latest
COPY --from=build /helm-app-operator /usr/bin/helm-app-operator
ENV OPERATOR_NAME='helm-app-operator' \
    CRD_RESOURCE='HelmApp,helmapps.xiaopal.github.com/v1beta1' \
    HELM_CHART='/chart'

ENTRYPOINT [ "/usr/local/entrypoint.sh", "helm-app-operator" ]
CMD [ "--tiller-storage=secret --all-namespaces" ]
