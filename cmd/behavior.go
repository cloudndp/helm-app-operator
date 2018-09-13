package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaopal/helm-app-operator/cmd/option"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"

	"github.com/operator-framework/helm-app-operator-kit/helm-app-operator/pkg/apis/app/v1alpha1"
	"github.com/xiaopal/helm-app-operator/cmd/helmext"
)

type installerBehavior struct {
	clientset internalclientset.Interface
}

func (c installerBehavior) ReleaseValues(raw *v1alpha1.HelmApp) (map[string]interface{}, error) {
	clientset := c.clientset
	namespace, name := raw.GetNamespace(), raw.GetName()
	valueYamls := [][]byte{}
	if cfgmap, err := clientset.Core().ConfigMaps(namespace).Get(name, metav1.GetOptions{}); err == nil {
		for _, key := range []string{"values.yaml", "values"} {
			if valueYaml, ok := cfgmap.Data[key]; ok {
				valueYamls = append(valueYamls, []byte(valueYaml))
			}
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	if secret, err := clientset.Core().Secrets(namespace).Get(name, metav1.GetOptions{}); err == nil {
		for _, key := range []string{"values.yaml", "values"} {
			if valueYaml, ok := secret.Data[key]; ok {
				valueYamls = append(valueYamls, valueYaml)
			}
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}

	return option.DecorateValues(raw.Spec, valueYamls)
}

func (c installerBehavior) OptionForce(r *v1alpha1.HelmApp) bool {
	return helmext.ReleaseOptionBool(r, helmext.OptionForce, option.OptionForce)
}

func (c installerBehavior) Logger(r *v1alpha1.HelmApp) func(string, ...interface{}) {
	return option.NewLogger("tiller").Printf
}

func (c installerBehavior) TranslateChartPath(r *v1alpha1.HelmApp, chartPath string) (string, error) {
	chart, chartSrc := helmext.ReleaseOption(r, helmext.OptionChart, ""), ""
	if chart != "" {
		if strings.HasPrefix(chart, "http://") || strings.HasPrefix(chart, "https://") {
			uri, err := url.Parse(chart)
			if err != nil {
				return "", err
			}
			name := filepath.Base(uri.Path)
			name = strings.TrimSuffix(name, ".tgz")
			name = strings.TrimSuffix(name, ".tar.gz")
			chartPath = filepath.Join(chartPath, name)
			chartSrc = chart
		} else if filepath.IsAbs(chart) {
			chartPath = chart
		} else {
			chartPath = filepath.Join(chartPath, chart)
			chartSrc = chart
		}
	}
	return fetchChart(r, chart, chartPath, chartSrc)
}

func fetchChart(r *v1alpha1.HelmApp, chart string, chartPath string, chartSrc string) (string, error) {
	fetchAlways := strings.ToLower(helmext.ReleaseOption(r, "fetch", "")) == "always"
	if _, err := os.Stat(chartPath); !os.IsNotExist(err) && !fetchAlways {
		return chartPath, nil
	}
	if option.OptionFetchExec == "" {
		return "", fmt.Errorf("chart %s not exists and --fetch-exec not present", chartPath)
	}
	if err := execEvent(r, "chart", option.OptionFetchExec,
		fmt.Sprintf("FETCH_CHART=%s", chart),
		fmt.Sprintf("FETCH_CHART_TO=%s", chartPath),
		fmt.Sprintf("FETCH_CHART_FROM=%s", chartSrc),
	); err != nil {
		return "", err
	}
	return chartPath, nil
}
