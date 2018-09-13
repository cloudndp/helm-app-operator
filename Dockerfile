FROM golang:1.10 as build
ADD . /go/src/github.com/xiaopal/helm-app-operator
WORKDIR  /go/src/github.com/xiaopal/helm-app-operator
RUN CGO_ENABLED=0 GOOS=linux go build -o /helm-app-operator -ldflags '-s -w' cmd/*.go
RUN chmod +x /helm-app-operator

FROM alpine:3.7

# https://kubernetes-charts.storage.googleapis.com
ARG HELM_STABLE_REPO_URL=https://npc.nos-eastchina1.126.net/charts/stable

RUN apk add --no-cache bash coreutils curl openssh-client openssl git ansible findutils py-netaddr rsync && \
	mkdir -p ~/.ssh && chmod 700 ~/.ssh && \
	echo -e 'StrictHostKeyChecking no\nUserKnownHostsFile /dev/null' >~/.ssh/config && \
	curl -sSL "http://npc.nos-eastchina1.126.net/dl/dumb-init_1.2.0_amd64.tar.gz" | tar -zx -C /usr/bin && \
    curl -sSL 'https://npc.nos-eastchina1.126.net/dl/kubernetes-client-v1.9.3-linux-amd64.tar.gz' | tar -zx -C /usr/ && \
    ln -s /usr/kubernetes/client/bin/kubectl /usr/bin/ && \
	mkdir -p /usr/helm && \
	curl -sSL 'https://npc.nos-eastchina1.126.net/dl/helm-v2.9.1-linux-amd64.tar.gz' | tar -zx -C /usr/helm && \
	ln -s /usr/helm/linux-amd64/helm /usr/bin/ && \
	ansible-galaxy install -p /etc/ansible/roles xiaopal.npc_setup && \
	ansible-playbook /etc/ansible/roles/xiaopal.npc_setup/tests/noop.yml && \
    helm init --client-only --stable-repo-url "$HELM_STABLE_REPO_URL"

COPY --from=build /helm-app-operator /helm-app-operator
RUN ln -s /helm-app-operator /usr/bin/helm-app-operator

ENV OPERATOR_NAME='helm-app-operator' \
    CRD_RESOURCE='HelmApp,helmapps.xiaopal.github.com/v1beta1' \
    HELM_CHART='' \
	FETCH_CHART_EXEC='[ ! -z "$FETCH_CHART" ] && rm -fr "$FETCH_CHART_TO" && FETCH_TMP="$(mktemp -d)" && trap "rm -fr $FETCH_TMP" EXIT && mkdir -p "$(dirname "$FETCH_CHART_TO")" && helm fetch -d "$FETCH_TMP" --untar "$FETCH_CHART_FROM" && mv "$FETCH_TMP/"* "$FETCH_CHART_TO"'

ENTRYPOINT [ "/usr/bin/dumb-init", "/helm-app-operator" ]
CMD [ "--tiller-storage=secret", "--all-namespaces" ]
