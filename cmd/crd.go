package main

import (
	"strings"

	"github.com/xiaopal/helm-app-operator/cmd/helmext"
	"github.com/xiaopal/helm-app-operator/cmd/option"

	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/operator-framework/helm-app-operator-kit/helm-app-operator/pkg/apis/app/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
)

func initCRDResource() error {
	clientset, err := apiextclientset.NewForConfig(k8sclient.GetKubeConfig())
	if err != nil {
		return err
	}
	crd := &apiextv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: option.OptionCRDName},
		Spec: apiextv1beta1.CustomResourceDefinitionSpec{
			Group:   option.OptionCRDGroup,
			Version: option.OptionCRDVersion,
			Scope:   apiextv1beta1.NamespaceScoped,
			Names: apiextv1beta1.CustomResourceDefinitionNames{
				Plural: option.OptionCRDPlural,
				Kind:   option.OptionCRDKind,
			},
		},
	}
	_, err = clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err == nil {
		logger.Printf("CRD initialized: %s", option.OptionCRD)
		return nil
	}
	if apierrors.IsAlreadyExists(err) {
		logger.Printf("CRD already initialized: %s", option.OptionCRD)
		return nil
	}
	return err
}

func installCRDResource(resource string) error {
	client, _, err := k8sclient.GetResourceClient(option.OptionAPIVersion, option.OptionCRDKind, option.OptionNamespace)
	if err != nil {
		return err
	}
	specValues, err := option.DecorateValues(map[string]interface{}{}, [][]byte{})
	if err != nil {
		return err
	}
	req := &v1alpha1.HelmApp{
		TypeMeta: metav1.TypeMeta{
			APIVersion: option.OptionAPIVersion,
			Kind:       option.OptionCRDKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        resource,
			Annotations: optionAnnotations(),
		},
		Spec: specValues,
	}

	target, err := client.Get(resource, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err = client.Create(k8sutil.UnstructuredFromRuntimeObject(req))
			logger.Printf("CRD resource installed: %s", option.OptionInstallResource)
		}
		return err
	}
	if option.OptionInstallOnce {
		logger.Printf("CRD resource exists: %s", option.OptionInstallResource)
		return nil
	}
	req.SetResourceVersion(target.GetResourceVersion())
	if _, err = client.Update(k8sutil.UnstructuredFromRuntimeObject(req)); err != nil {
		return err
	}
	logger.Printf("CRD resource updated: %s", option.OptionInstallResource)
	return nil
}

func uninstallCRDResource(resource string) error {
	client, _, err := k8sclient.GetResourceClient(option.OptionAPIVersion, option.OptionCRDKind, option.OptionNamespace)
	if err != nil {
		return err
	}
	err = client.Delete(resource, &metav1.DeleteOptions{})
	if err == nil {
		logger.Printf("CRD resource uninstalled: %s", option.OptionUninstallResource)
		return nil
	}
	if apierrors.IsNotFound(err) {
		logger.Printf("CRD resource not exists: %s", option.OptionUninstallResource)
		return nil
	}
	return err
}

func optionAnnotations() map[string]string {
	annotations := map[string]string{}
	for _, option := range option.OptionInstallOptions {
		name, value, eq := option, "", strings.Index(option, "=")
		if eq >= 0 {
			name, value = option[:eq], option[eq+1:]
		}
		annotations[helmext.OptionAnnotation(name)] = value
	}
	return annotations
}
