package main

import (
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
