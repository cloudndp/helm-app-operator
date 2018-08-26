package main

import (
	"context"
	"log"
	"os"

	"github.com/xiaopal/helm-app-operator/cmd/option"

	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/xiaopal/helm-app-operator/cmd/helmext"
)

var (
	logger *log.Logger
)

func main() {
	logger = option.NewLogger("main")

	if option.OptionInit {
		if err := initCRDResource(); err != nil {
			logger.Fatalf("Cannot initialize CRD resource: %v", err)
		}
		os.Exit(0)
	}

	if len(option.OptionInstallResource) > 0 {
		if err := installCRDResource(option.OptionInstallResource); err != nil {
			logger.Fatalf("Cannot install CRD resource: %v", err)
		}
		os.Exit(0)
	}

	if len(option.OptionUninstallResource) > 0 {
		if err := uninstallCRDResource(option.OptionUninstallResource); err != nil {
			logger.Fatalf("Cannot uninstall CRD resource: %s", err)
		}
		os.Exit(0)
	}

	storageBackend, err := option.GetStorageBackend()
	if err != nil {
		logger.Fatalf(err.Error())
	}

	clientset, err := internalclientset.NewForConfig(k8sclient.GetKubeConfig())
	if err != nil {
		logger.Fatalf("Cannot initialize Kubernetes connection: %v", err)
	}
	kubeClient, err := option.KubeClient()
	if err != nil {
		logger.Fatalf(err.Error())
	}
	logger.Printf("watching ApiVersion: %s, Kind: %s, Namespace: %s", option.OptionAPIVersion, option.OptionCRDKind, option.OptionNamespace)
	sdk.Watch(option.OptionAPIVersion, option.OptionCRDKind, option.OptionNamespace, option.OptionResyncPeriod)
	sdk.Handle(&handler{
		helmext.NewInstallerWithBehavior(storageBackend, kubeClient, option.OptionChart, installerBehavior{clientset}),
	})
	sdk.Run(context.TODO())
}
