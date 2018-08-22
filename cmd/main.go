package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/xiaopal/helm-app-operator/cmd/option"

	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"

	"github.com/operator-framework/helm-app-operator-kit/helm-app-operator/pkg/apis/app/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	"github.com/xiaopal/helm-app-operator/cmd/helmext"
)

var (
	logger *log.Logger
)

func main() {
	logger = option.NewLogger("main")

	if option.OptionInit {
		if err := initCRDResource(); err != nil {
			logger.Fatalf("Cannot initialize CRD resource: %s", err)
			os.Exit(1)
		}
		logger.Printf("CRD initialized: %s", option.OptionCRD)
		os.Exit(0)
	}

	if len(option.OptionInstallResource) > 0 {
		if err := installCRDResource(option.OptionInstallResource); err != nil {
			logger.Fatalf("Cannot install CRD resource: %s", err)
			os.Exit(1)
		}
		logger.Printf("CRD resource installed: %s", option.OptionInstallResource)
		os.Exit(0)
	}

	if len(option.OptionUninstallResource) > 0 {
		if err := uninstallCRDResource(option.OptionUninstallResource); err != nil {
			logger.Fatalf("Cannot uninstall CRD resource: %s", err)
			os.Exit(1)
		}
		logger.Printf("CRD resource uninstalled: %s", option.OptionUninstallResource)
		os.Exit(0)
	}

	storageBackend, err := option.GetStorageBackend()
	if err != nil {
		logger.Fatalf(err.Error())
		os.Exit(1)
	}

	clientset, err := internalclientset.NewForConfig(k8sclient.GetKubeConfig())
	if err != nil {
		logger.Fatalf("Cannot initialize Kubernetes connection: %s", err)
		os.Exit(1)
	}
	kubeClient, err := option.KubeClient()
	if err != nil {
		logger.Fatalf(err.Error())
		os.Exit(1)
	}
	logger.Printf("watching ApiVersion: %s, Kind: %s, Namespace: %s", option.OptionAPIVersion, option.OptionCRDKind, option.OptionNamespace)
	sdk.Watch(option.OptionAPIVersion, option.OptionCRDKind, option.OptionNamespace, option.OptionResyncPeriod)
	sdk.Handle(&handler{
		helmext.NewInstallerWithBehavior(storageBackend, kubeClient, option.OptionChart, installerBehavior{clientset}),
	})
	sdk.Run(context.TODO())
}

type handler struct {
	controller helmext.Installer
}

func (h *handler) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *v1alpha1.HelmApp:
		finalizerFound, finalizerRemains := false, []string{}
		for _, v := range o.GetFinalizers() {
			if v == helmext.OperatorName() {
				finalizerFound = true
			} else {
				finalizerRemains = append(finalizerRemains, v)
			}
		}
		if event.Deleted || o.GetDeletionTimestamp() != nil {
			if finalizerFound {
				logger.Printf("Uninstalling %s", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
				updatedResource, err := h.controller.UninstallRelease(o)
				if err != nil {
					if strings.Contains(err.Error(), "not found") {
						logger.Printf("%s already uninstalled", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
						return nil
					}
					logger.Fatalf("failed to uninstall release: %v", err.Error())
					return err
				}
				if !event.Deleted {
					updatedResource.SetFinalizers(finalizerRemains)
					err = sdk.Update(updatedResource)
					if err != nil {
						logger.Fatalf("failed to update custom resource status: %v", err.Error())
						return err
					}
				}
				logger.Printf("%s uninstalled", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
			}
			return nil
		}
		if updated, err := updateChecksum(o); err != nil {
			logger.Fatalf("failed to update checksum: %v", err.Error())
			return err
		} else if !updated {
			//unchanged, continue
			return nil
		}
		logger.Printf("Installing %s", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
		updatedResource, err := h.controller.InstallRelease(o)
		if err != nil {
			logger.Fatalf("failed to install release: %v", err.Error())
			return err
		}
		if !finalizerFound {
			updatedResource.SetFinalizers(append(finalizerRemains, helmext.OperatorName()))
		}
		err = sdk.Update(updatedResource)
		if err != nil {
			logger.Fatalf("failed to update custom resource status: %v", err.Error())
			return err
		}
		logger.Printf("%s updated", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
	}
	return nil
}

type installerBehavior struct {
	clientset internalclientset.Interface
}

func (c installerBehavior) ReleaseName(r *v1alpha1.HelmApp) string {
	return helmext.ReleaseName(r)
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
	if err != nil && apierrors.IsAlreadyExists(err) {
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
			Name: resource,
		},
		Spec: specValues,
	}

	target, err := client.Get(resource, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err = client.Create(k8sutil.UnstructuredFromRuntimeObject(req))
		}
		return err
	}
	if len(option.OptionInstallOnce) > 0 {
		return nil
	}
	req.SetResourceVersion(target.GetResourceVersion())
	_, err = client.Update(k8sutil.UnstructuredFromRuntimeObject(req))
	return err
}

func uninstallCRDResource(resource string) error {
	client, _, err := k8sclient.GetResourceClient(option.OptionAPIVersion, option.OptionCRDKind, option.OptionNamespace)
	if err != nil {
		return err
	}
	err = client.Delete(resource, &metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func updateChecksum(r *v1alpha1.HelmApp) (bool, error) {
	annoChecksum := helmext.OptionAnnotation("checksum")
	annotations, lastChecksum := map[string]string{}, ""
	for k, v := range r.GetAnnotations() {
		if k == annoChecksum {
			lastChecksum = v
		} else {
			annotations[k] = v
		}
	}
	bytes, err := json.Marshal([]interface{}{
		r.GetName(),
		r.GetNamespace(),
		r.GetLabels(),
		annotations,
		r.Spec,
		r.GetDeletionTimestamp(),
	})
	if err != nil {
		return false, err
	}
	checksum := fmt.Sprintf("%x", sha1.Sum(bytes))
	if checksum != lastChecksum {
		annotations[annoChecksum] = checksum
		r.SetAnnotations(annotations)
		return true, nil
	}
	return false, nil
}
