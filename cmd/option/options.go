package option

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"k8s.io/helm/pkg/storage"
	"k8s.io/helm/pkg/storage/driver"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	helmVersion "k8s.io/helm/pkg/version"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/helm/pkg/kube"
)

const (
	storageMemory    = "memory"
	storageConfigMap = "configmap"
	storageSecret    = "secret"
)

var (
	logger *log.Logger
	//OptionOperatorName --name option
	OptionOperatorName string
	//OptionKubeConfig --kubeconfig option
	OptionKubeConfig string
	//OptionCRD --crd option
	OptionCRD string
	//OptionCRDName crd name
	OptionCRDName string
	//OptionCRDGroup crd group
	OptionCRDGroup string
	//OptionCRDVersion crd version
	OptionCRDVersion string
	//OptionCRDKind crd kind
	OptionCRDKind string
	//OptionCRDPlural crd plural
	OptionCRDPlural string
	//OptionCRDSingular crd singular
	OptionCRDSingular string
	//OptionAPIVersion crd ApiVersion: <group>/<version>
	OptionAPIVersion string
	//OptionInit --init option
	OptionInit bool
	//OptionInstallResource install <resource_name> option
	OptionInstallResource string
	//OptionInstallOnce install --once option
	OptionInstallOnce bool
	//OptionUninstallResource --uninstall=<resource_name> option
	OptionUninstallResource string
	//OptionInstallOptions install --option options
	OptionInstallOptions []string
	//OptionChart --chart option
	OptionChart string
	//OptionForce --force option
	OptionForce bool
	//OptionNamespace --namespace option
	OptionNamespace string
	//OptionAllNamespace --all-namespace option
	OptionAllNamespace bool
	//OptionTillerNamespace --tiller-namespace option
	OptionTillerNamespace string
	//OptionValueFiles --values/-f option
	OptionValueFiles []string
	//OptionStore --tiller-storage option
	OptionStore string
	//OptionMaxHistory --till-history-max option
	OptionMaxHistory int
	//OptionResyncPeriod --resync option
	OptionResyncPeriod int
	//OptionHooks --hooks option
	OptionHooks    bool
	optionContinue bool
)

func parseOptions() error {
	cmd := &cobra.Command{
		Use: os.Args[0],
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if len(OptionCRD) == 0 {
				return fmt.Errorf("--crd required")
			}
			crd := strings.Split(OptionCRD, ",")
			if len(crd) < 2 {
				return fmt.Errorf(" illegal --crd Option")
			}
			OptionCRDKind, OptionCRDSingular = crd[0], strings.ToLower(crd[0])
			if len(crd) > 2 {
				OptionCRDSingular = crd[2]
			}
			crd = strings.Split(crd[1], "/")
			if len(crd) < 2 {
				return fmt.Errorf(" illegal --crd Option")
			}
			OptionCRDName, OptionCRDVersion = crd[0], crd[1]
			crd = strings.Split(OptionCRDName, ".")
			if len(crd) < 2 {
				return fmt.Errorf(" illegal --crd Option")
			}
			OptionCRDPlural, OptionCRDGroup = crd[0], strings.Join(crd[1:], ".")
			OptionAPIVersion = fmt.Sprintf("%s/%s", OptionCRDGroup, OptionCRDVersion)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if OptionAllNamespace {
				OptionNamespace = metav1.NamespaceAll
			}
			if len(OptionChart) == 0 {
				return fmt.Errorf("--chart required")
			}
			optionContinue = true
			return nil
		},
	}
	cmdInit := &cobra.Command{
		Use: "init",
		RunE: func(cmd *cobra.Command, args []string) error {
			OptionInit = true
			optionContinue = true
			return nil
		},
	}
	cmdInstall := &cobra.Command{
		Use: "install [flags] RESOURCE_NAME",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("command 'install' requires a resource name")
			}
			OptionInstallResource = args[0]
			if len(OptionNamespace) == 0 {
				return fmt.Errorf("--namespace required")
			}
			optionContinue = true
			return nil
		},
	}
	cmdUninstall := &cobra.Command{
		Use: "uninstall [flags] RESOURCE_NAME",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("command 'uninstall' requires a resource name")
			}
			OptionUninstallResource = args[0]
			if len(OptionNamespace) == 0 {
				return fmt.Errorf("--namespace required")
			}
			optionContinue = true
			return nil
		},
	}
	cmd.AddCommand(cmdInit, cmdInstall, cmdUninstall)
	flagsPersistent, flagsOperator, _ /*flagsInit*/, flagsInstall, flagsUninstall :=
		cmd.PersistentFlags(), cmd.Flags(), cmdInit.Flags(), cmdInstall.Flags(), cmdUninstall.Flags()
	flagsPersistent.StringVar(&OptionKubeConfig, "kubeconfig", kubeconfigFromEnv(), "kubeconfig path, default to in-cluster config")
	flagsPersistent.StringVar(&OptionCRD, "crd", os.Getenv("CRD_RESOURCE"), "CRD resource of form '<Kind>,<plural>.<group>/<api-version>[,<singular>]', eg. CustomApp,custom-apps.xiaopal.github.com/v1beta1,custom-app")

	flagsOperator.StringVarP(&OptionOperatorName, "name", "n", os.Getenv(k8sutil.OperatorNameEnvVar), "operator name, default to helm-app-operator")
	flagsOperator.StringVarP(&OptionChart, "chart", "c", os.Getenv("HELM_CHART"), "chart dir")
	flagsOperator.BoolVar(&OptionAllNamespace, "all-namespaces", false, "watch all namespace")
	flagsOperator.StringVar(&OptionNamespace, "namespace", watchNamespaceFromEnv(), "watch namespace. defaults to current namespace.")
	flagsOperator.BoolVar(&OptionForce, "force", false, "upgrade with force option")
	flagsOperator.StringSliceVarP(&OptionValueFiles, "values", "f", nil, "specify values in a YAML file(can specify multiple)")
	flagsOperator.BoolVar(&OptionHooks, "hooks", true, "enable hooks")
	flagsOperator.StringVar(&OptionTillerNamespace, "tiller-namespace", tillerNamespaceFromEnv(), "tiller namespace. defaults to current namespace.")
	flagsOperator.StringVar(&OptionStore, "tiller-storage", storageConfigMap, "storage driver to use. One of 'configmap', 'memory', or 'secret'")
	flagsOperator.IntVar(&OptionMaxHistory, "tiller-history-max", historyMaxFromEnv(), "maximum number of releases kept in release history, with 0 meaning no limit")
	flagsOperator.IntVar(&OptionResyncPeriod, "resync", 0, "resync period, default 0")

	flagsInstall.StringVar(&OptionNamespace, "namespace", watchNamespaceFromEnv(), "install to namespace. defaults to current namespace.")
	flagsInstall.BoolVar(&OptionInstallOnce, "once", false, "install crd resource if not exists")
	flagsInstall.StringArrayVarP(&OptionInstallOptions, "option", "o", nil, "option annotation, eg. -o pre-install='echo $EVENT_TYPE'")
	flagsInstall.StringSliceVarP(&OptionValueFiles, "values", "f", nil, "specify values in a YAML file(can specify multiple)")

	flagsUninstall.StringVar(&OptionNamespace, "namespace", watchNamespaceFromEnv(), "uninstall from namespace. defaults to current namespace.")
	return cmd.Execute()
}

//NewLogger 配置 logger
func NewLogger(prefix string) *log.Logger {
	if len(prefix) > 0 {
		prefix = fmt.Sprintf("[%s] ", prefix)
	}
	return log.New(os.Stderr, prefix, log.Flags())
}

func init() {
	logger = NewLogger("option")
	logger.Printf("Go Version: %s", runtime.Version())
	logger.Printf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logger.Printf("operator-sdk Version: %v", sdkVersion.Version)
	logger.Printf("Helm/Tiller Version: %v", helmVersion.GetVersion())

	if err := parseOptions(); err != nil {
		logger.Fatalf("faild to parse options: %v", err)
	} else if !optionContinue {
		os.Exit(0)
	}

	//设置环境变量兼容 helm-app-operator-kit 类型注册
	os.Setenv("KIND", OptionCRDKind)
	os.Setenv("API_VERSION", OptionAPIVersion)

	os.Setenv(k8sutil.WatchNamespaceEnvVar, OptionNamespace)
	if len(OptionKubeConfig) > 0 {
		os.Setenv(k8sutil.KubeConfigEnvVar, OptionKubeConfig)
		os.Setenv("KUBECONFIG", OptionKubeConfig)
	}
	if len(OptionOperatorName) == 0 {
		OptionOperatorName = "helm-app-operator"
	}
	os.Setenv(k8sutil.OperatorNameEnvVar, OptionOperatorName)
	os.Setenv("HELM_CHART", OptionChart)
}

func kubeconfigFromEnv() string {
	if cfg, found := os.LookupEnv(k8sutil.KubeConfigEnvVar); found {
		return cfg
	}
	return os.Getenv("KUBECONFIG")
}

func historyMaxFromEnv() int {
	val := os.Getenv("TILLER_HISTORY_MAX")
	if val == "" {
		return 1
	}
	ret, err := strconv.Atoi(val)
	if err != nil {
		logger.Printf("Invalid max history %q. Defaulting to 1.", val)
		return 1
	}
	return ret
}

func watchNamespaceFromEnv() string {
	if ns, found := os.LookupEnv(k8sutil.WatchNamespaceEnvVar); found {
		return ns
	}
	if ns, found := os.LookupEnv("POD_NAMESPACE"); found {
		return ns
	}
	if data, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns
		}
	}
	return "default"
}

func tillerNamespaceFromEnv() string {
	if ns, found := os.LookupEnv("TILLER_NAMESPACE"); found {
		return ns
	}
	if ns, found := os.LookupEnv("POD_NAMESPACE"); found {
		return ns
	}
	if data, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns
		}
	}
	return "kube-system"
}

//GetStorageBackend 获取 tiller 后端配置
func GetStorageBackend() (*storage.Storage, error) {
	clientset, err := internalclientset.NewForConfig(k8sclient.GetKubeConfig())
	if err != nil {
		return nil, fmt.Errorf("Cannot initialize Kubernetes connection: %s", err)
	}

	var storageBackend *storage.Storage
	switch OptionStore {
	case storageMemory:
		storageBackend = storage.Init(driver.NewMemory())
	case storageConfigMap:
		cfgmaps := driver.NewConfigMaps(clientset.Core().ConfigMaps(OptionTillerNamespace))
		storageBackend = storage.Init(cfgmaps)
	case storageSecret:
		secrets := driver.NewSecrets(clientset.Core().Secrets(OptionTillerNamespace))
		storageBackend = storage.Init(secrets)
	default:
		logger.Printf("unknown storage option %s, fallback to memory", OptionStore)
		storageBackend = storage.Init(driver.NewMemory())
	}

	if OptionMaxHistory > 0 {
		storageBackend.MaxHistory = OptionMaxHistory
	}
	storageBackend.Log = NewLogger("storage").Printf
	return storageBackend, nil
}

//DecorateValues 组装 values
func DecorateValues(specValues map[string]interface{}, valueYamls [][]byte) (map[string]interface{}, error) {
	base := map[string]interface{}{}
	for _, filePath := range OptionValueFiles {
		bytes, err := ioutil.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		currentMap := map[string]interface{}{}
		if err := yaml.Unmarshal(bytes, &currentMap); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %s", filePath, err)
		}
		// Merge with the previous map
		base = mergeValues(base, currentMap)
	}
	base = mergeValues(base, specValues)
	for _, valueYaml := range valueYamls {
		currentMap := map[string]interface{}{}

		if err := yaml.Unmarshal(valueYaml, &currentMap); err != nil {
			return nil, fmt.Errorf("failed to parse yaml: %s", err)
		}
		// Merge with the previous map
		base = mergeValues(base, currentMap)
	}
	return base, nil
}

// Merges source and destination map, preferring values from the source map
func mergeValues(dest map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		// If the key doesn't exist already, then just set the key to that value
		if _, exists := dest[k]; !exists {
			dest[k] = v
			continue
		}
		nextMap, ok := v.(map[string]interface{})
		// If it isn't another map, overwrite the value
		if !ok {
			dest[k] = v
			continue
		}
		// Edge case: If the key exists in the destination, but isn't a map
		destMap, isMap := dest[k].(map[string]interface{})
		// If the source map has a map for this key, prefer it
		if !isMap {
			dest[k] = v
			continue
		}
		// If we got to this point, it is a map in both, so merge them
		dest[k] = mergeValues(destMap, nextMap)
	}
	return dest
}

//KubeClient 获取 kube.Client
func KubeClient() (*kube.Client, error) {
	var clientConfig clientcmd.ClientConfig
	if len(OptionKubeConfig) > 0 {
		apiConfig, err := clientcmd.LoadFromFile(OptionKubeConfig)
		if err != nil {
			return nil, fmt.Errorf("Cannot load kubeconfig %s: %s", OptionKubeConfig, err)
		}
		clientConfig = clientcmd.NewDefaultClientConfig(*apiConfig, &clientcmd.ConfigOverrides{})
	}
	return kube.New(clientConfig), nil
}
