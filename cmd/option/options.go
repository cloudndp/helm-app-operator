package option

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	"k8s.io/helm/pkg/storage"
	"k8s.io/helm/pkg/storage/driver"

	"github.com/ghodss/yaml"

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
	//OptionInstallResource --install=<resource_name> option
	OptionInstallResource string
	//OptionInstallOnce --install-once=<resource_name> option
	OptionInstallOnce string
	//OptionUninstallResource --uninstall=<resource_name> option
	OptionUninstallResource string
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
	OptionValueFiles valueFiles
	//OptionStore --tiller-storage option
	OptionStore string
	//OptionMaxHistory --till-history-max option
	OptionMaxHistory int
	//OptionResyncPeriod --resync option
	OptionResyncPeriod int
)

func parseOptions() error {
	flag.StringVar(&OptionOperatorName, "name", os.Getenv(k8sutil.OperatorNameEnvVar), "operator name, default to helm-app-operator")
	flag.StringVar(&OptionKubeConfig, "kubeconfig", kubeconfigFromEnv(), "kubeconfig path, default to in-cluster config")
	flag.StringVar(&OptionCRD, "crd", os.Getenv("CRD_RESOURCE"), "CRD resource of form '<Kind>,<plural>.<group>/<api-version>[,<singular>]', eg. CustomApp,custom-apps.xiaopal.github.com/v1beta1,custom-app")
	flag.BoolVar(&OptionInit, "init", false, "init crd resource")
	flag.StringVar(&OptionInstallResource, "install", "", "install or update crd resource")
	flag.StringVar(&OptionInstallOnce, "install-once", "", "install crd resource if not exists")
	flag.StringVar(&OptionUninstallResource, "uninstall", "", "uninstall crd resource")
	flag.StringVar(&OptionChart, "chart", os.Getenv("HELM_CHART"), "chart dir")
	flag.BoolVar(&OptionForce, "force", false, "upgrade with force option")
	flag.BoolVar(&OptionAllNamespace, "all-namespaces", false, "watch all namespace")
	flag.StringVar(&OptionNamespace, "namespace", watchNamespaceFromEnv(), "watch namespace. defaults to current namespace.")
	flag.Var(&OptionValueFiles, "values", "specify values in a YAML file(can specify multiple)")
	flag.Var(&OptionValueFiles, "f", "alias for --values")
	flag.StringVar(&OptionTillerNamespace, "tiller-namespace", tillerNamespaceFromEnv(), "tiller namespace. defaults to current namespace.")
	flag.StringVar(&OptionStore, "tiller-storage", storageConfigMap, "storage driver to use. One of 'configmap', 'memory', or 'secret'")
	flag.IntVar(&OptionMaxHistory, "tiller-history-max", historyMaxFromEnv(), "maximum number of releases kept in release history, with 0 meaning no limit")
	flag.IntVar(&OptionResyncPeriod, "resync", 5, "resync period, default 5")
	flag.Parse()

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

	if OptionAllNamespace {
		OptionNamespace = metav1.NamespaceAll
	}

	if OptionInit {
		return nil
	}

	if len(OptionInstallResource) > 0 || len(OptionUninstallResource) > 0 || len(OptionInstallOnce) > 0 {
		if len(OptionInstallOnce) > 0 {
			OptionInstallResource = OptionInstallOnce
		}
		if len(OptionNamespace) == 0 {
			return fmt.Errorf("--namespace required")
		}
		return nil
	}

	if len(OptionChart) == 0 {
		return fmt.Errorf("--chart required")
	}
	return nil
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
		logger.Fatal(err)
		flag.Usage()
		os.Exit(1)
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
		log.Printf("Invalid max history %q. Defaulting to 1.", val)
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

type valueFiles []string

func (v *valueFiles) String() string {
	return fmt.Sprint(*v)
}

func (v *valueFiles) Set(value string) error {
	for _, filePath := range strings.Split(value, ",") {
		*v = append(*v, filePath)
	}
	return nil
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
