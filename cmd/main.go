package main

import (
	"context"
	"flag"
	"os"
	"log"
	"runtime"

	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	k8sutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"k8s.io/helm/pkg/kube"
	helmVersion "k8s.io/helm/pkg/version"

	"github.com/operator-framework/helm-app-operator-kit/helm-app-operator/pkg/helm"
	stub "github.com/operator-framework/helm-app-operator-kit/helm-app-operator/pkg/stub"
)

const (
	apiVersionEnvVar = "API_VERSION"
	kindEnvVar       = "KIND"
	helmChartEnvVar  = "HELM_CHART"

	// historyMaxEnvVar is the name of the env var for setting max history.
	historyMaxEnvVar = "TILLER_HISTORY_MAX"
	storageMemory    = "memory"
	storageConfigMap = "configmap"
	storageSecret    = "secret"
	// defaultMaxHistory sets the maximum number of releases to 0: unlimited
	defaultMaxHistory = 0
)

var (
	store      = flag.String("storage", storageConfigMap, "storage driver to use. One of 'configmap', 'memory', or 'secret'")
	maxHistory = flag.Int("history-max", historyMaxFromEnv(), "maximum number of releases kept in release history, with 0 meaning no limit")
	logger *log.Logger
)

func newLogger(prefix string) *log.Logger {
	if len(prefix) > 0 {
		prefix = fmt.Sprintf("[%s] ", prefix)
	}
	return log.New(os.Stderr, prefix, log.Flags())
}

func printVersion() {
	logger.Infof("Go Version: %s", runtime.Version())
	logger.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logger.Infof("operator-sdk Version: %v", sdkVersion.Version)
	logger.Infof("Helm/Tiller Version: %v", helmVersion.GetVersion())
}

func main() {
	logger = newLogger("main")

	printVersion()

	resource := os.Getenv(apiVersionEnvVar)
	kind := os.Getenv(kindEnvVar)
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		logger.Fatalf("Failed to get watch namespace: %v", err)
	}
	resyncPeriod := 5

	tillerKubeClient := kube.New(nil)
	chartDir := os.Getenv(helmChartEnvVar)

	clientset, err := tillerKubeClient.ClientSet()
	if err != nil {
		logger.Fatalf("Cannot initialize Kubernetes connection: %s", err)
	}

	storageBackend *storage.Storage
	switch *store {
	case storageMemory:
		storageBackend = storage.Init(driver.NewMemory())
	case storageConfigMap:
		cfgmaps := driver.NewConfigMaps(clientset.Core().ConfigMaps(namespace))
		cfgmaps.Log = newLogger("storage/driver").Printf
		storageBackend = storage.Init(driver.NewConfigMaps(cfgmaps))
		storageBackend.Log = newLogger("storage").Printf
	case storageSecret:
		secrets := driver.NewSecrets(clientset.Core().Secrets(namespace))
		secrets.Log = newLogger("storage/driver").Printf

		storageBackend = storage.Init(secrets)
		storageBackend.Log = newLogger("storage").Printf
	}

	if *maxHistory > 0 {
		storageBackend.MaxHistory = *maxHistory
	}

	controller := helm.NewInstaller(storageBackend, tillerKubeClient, chartDir)

	logger.Infof("Watching %s, %s, %s, %d", resource, kind, namespace, resyncPeriod)

	sdk.Watch(resource, kind, namespace, resyncPeriod)
	sdk.Handle(stub.NewHandler(controller))
	sdk.Run(context.TODO())
}
