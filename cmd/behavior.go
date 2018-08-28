package main

import (
	"fmt"
	"os"
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
	chart, translated := helmext.ReleaseOption(r, helmext.OptionChart, ""), helmext.TranslateChartPath(r, chartPath)
	if chart == "" {
		return translated, nil
	}
	if strings.HasPrefix(chart, "http://") || strings.HasPrefix(chart, "https://") {
		return fetchChart(r, chart)
	}
	if _, err := os.Stat(translated); !os.IsNotExist(err) {
		return translated, nil
	}
	return fetchChart(r, chart)
}

func fetchChart(r *v1alpha1.HelmApp, chart string) (string, error) {
	name := chart
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = strings.TrimSuffix(name[index+1:], ".tgz")
	}
	chartDir := fmt.Sprintf("%s/%s", os.ExpandEnv("$HOME/.charts"), name)
	fetchCmd := fmt.Sprintf("declare CHART='%s' CHART_DIR='%s'; %s", chart, chartDir, `
		[ -d "$CHART_DIR" ] || {
			echo "fetching '$CHART' to $CHART_DIR..." 
			mkdir -p "${CHART_DIR%/*}"
			helm fetch --untar --destination "${CHART_DIR%/*}" "$CHART"
		}`)
	if err := execEvent(r, "chart", fetchCmd); err != nil {
		return "", err
	}
	return chartDir, nil
}
