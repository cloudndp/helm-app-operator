FROM golang:1.10 as build
ADD . /go/src/github.com/xiaopal/helm-app-operator
WORKDIR  /go/src/github.com/xiaopal/helm-app-operator
RUN CGO_ENABLED=0 GOOS=linux go build -o /helm-app-operator -ldflags '-s -w' cmd/*.go
RUN chmod +x /helm-app-operator

FROM alpine:3.7

# https://kubernetes-charts.storage.googleapis.com
ARG HELM_STABLE_REPO_URL=https://npc.nos-eastchina1.126.net/charts/stable

RUN apk add --no-cache bash coreutils curl openssh-client openssl git findutils && \
	curl -sSL 'http://npc.nos-eastchina1.126.net/dl/jq_1.5_linux_amd64.tar.gz' | tar -zx -C /usr/local/bin && \
	curl -sSL 'https://npc.nos-eastchina1.126.net/dl/kubernetes-client-v1.9.3-linux-amd64.tar.gz' | tar -zx -C /usr/local && \
	ln -s /usr/local/kubernetes/client/bin/kubectl /usr/local/bin/kubectl && \
	mkdir -p /usr/local/helm && \
	curl -sSL 'https://npc.nos-eastchina1.126.net/dl/helm-v2.9.1-linux-amd64.tar.gz' | tar -zx -C /usr/local/helm && \
	ln -s /usr/local/helm/linux-amd64/helm /usr/local/bin/helm && \
    helm init --client-only --stable-repo-url "$HELM_STABLE_REPO_URL"

COPY --from=build /helm-app-operator /helm-app-operator
RUN ln -s /helm-app-operator /usr/local/bin/helm-app-operator

ENV OPERATOR_NAME='helm-app-operator' \
    CRD_RESOURCE='HelmApp,helmapps.xiaopal.github.com/v1beta1' \
    HELM_CHART='' \
	FETCH_CHART_EXEC='[ ! -z "$FETCH_CHART" ] && rm -fr "$FETCH_CHART_TO" && FETCH_TMP="$(mktemp -d)" && trap "rm -fr $FETCH_TMP" EXIT && mkdir -p "$(dirname "$FETCH_CHART_TO")" && helm fetch -d "$FETCH_TMP" --untar "$FETCH_CHART_FROM" && mv "$FETCH_TMP/"* "$FETCH_CHART_TO"'

ENTRYPOINT [ "/helm-app-operator" ]
CMD [ "--tiller-storage=secret", "--all-namespaces" ]
