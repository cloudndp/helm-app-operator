package helmext

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/operator-framework/helm-app-operator-kit/helm-app-operator/pkg/apis/app/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/engine"
	"k8s.io/helm/pkg/kube"
	cpb "k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/proto/hapi/release"
	"k8s.io/helm/pkg/proto/hapi/services"
	"k8s.io/helm/pkg/storage"
	"k8s.io/helm/pkg/tiller"
	"k8s.io/helm/pkg/tiller/environment"
	"k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
)

const (
	//OptionChart option chart
	OptionChart = "chart"
	//OptionRelease option release
	OptionRelease = "release"
	//OptionForce option force
	OptionForce = "force"
)

// Installer can install and uninstall Helm releases given a custom resource
// which provides runtime values for the Chart.
type Installer interface {
	InstallRelease(r *v1alpha1.HelmApp) (*v1alpha1.HelmApp, error)
	UninstallRelease(r *v1alpha1.HelmApp) (*v1alpha1.HelmApp, error)
	ReleaseName(r *v1alpha1.HelmApp) string
	ReleaseValues(r *v1alpha1.HelmApp) (map[string]interface{}, error)
	Logger(r *v1alpha1.HelmApp) func(string, ...interface{})
}

type installer struct {
	storageBackend   *storage.Storage
	tillerKubeClient *kube.Client
	chartPath        string
	behavior         interface{}
}

// NewInstaller returns a new Helm installer capable of installing and uninstalling releases.
func NewInstaller(storageBackend *storage.Storage, tillerKubeClient *kube.Client, chartPath string) Installer {
	return NewInstallerWithBehavior(storageBackend, tillerKubeClient, chartPath, nil)
}

// NewInstallerWithBehavior returns a new Helm installer capable of installing and uninstalling releases.
func NewInstallerWithBehavior(storageBackend *storage.Storage, tillerKubeClient *kube.Client, chartPath string, behavior interface{}) Installer {
	return installer{storageBackend, tillerKubeClient, chartPath, behavior}
}

// InstallRelease accepts a custom resource, installs a Helm release using Tiller,
// and returns the custom resource with updated `status`.
func (c installer) InstallRelease(r *v1alpha1.HelmApp) (*v1alpha1.HelmApp, error) {
	chart, cr, err := c.LoadChart(r, c.chartPath)
	var updatedRelease *release.Release
	latestRelease, err := c.storageBackend.Last(c.ReleaseName(r))

	tiller := c.tillerRendererForCR(r)
	c.syncReleaseStatus(r.Status)

	if err != nil || latestRelease == nil {
		installReq := &services.InstallReleaseRequest{
			Namespace: r.GetNamespace(),
			Name:      c.ReleaseName(r),
			Chart:     chart,
			Values:    &cpb.Config{Raw: string(cr)},
			ReuseName: c.OptionForce(r),
		}
		releaseResponse, err := tiller.InstallRelease(context.TODO(), installReq)
		if err != nil {
			return r, err
		}
		updatedRelease = releaseResponse.GetRelease()
	} else {
		updateReq := &services.UpdateReleaseRequest{
			Name:   c.ReleaseName(r),
			Chart:  chart,
			Values: &cpb.Config{Raw: string(cr)},
			Force:  c.OptionForce(r),
		}
		releaseResponse, err := tiller.UpdateRelease(context.TODO(), updateReq)
		if err != nil {
			return r, err
		}
		updatedRelease = releaseResponse.GetRelease()
	}

	r.Status = *r.Status.SetRelease(updatedRelease)
	// TODO(alecmerdler): Call `r.Status.SetPhase()` with `NOTES.txt` of rendered Chart
	r.Status = *r.Status.SetPhase(v1alpha1.PhaseApplied, v1alpha1.ReasonApplySuccessful, "")

	return r, nil
}

// UninstallRelease accepts a custom resource, uninstalls the existing Helm release
// using Tiller, and returns the custom resource with updated `status`.
func (c installer) UninstallRelease(r *v1alpha1.HelmApp) (*v1alpha1.HelmApp, error) {
	tiller := c.tillerRendererForCR(r)
	_, err := tiller.UninstallRelease(context.TODO(), &services.UninstallReleaseRequest{
		Name:  c.ReleaseName(r),
		Purge: true,
	})
	if err != nil {
		return r, err
	}

	return r, nil
}

func (c installer) syncReleaseStatus(status v1alpha1.HelmAppStatus) {
	if status.Release == nil {
		return
	}
	if _, err := c.storageBackend.Get(status.Release.GetName(), status.Release.GetVersion()); err == nil {
		return
	}

	c.storageBackend.Create(status.Release)
}

// tillerRendererForCR creates a ReleaseServer configured with a rendering engine that adds ownerrefs to rendered assets
// based on the CR.
func (c installer) tillerRendererForCR(r *v1alpha1.HelmApp) *tiller.ReleaseServer {
	// 移除 ownerRefs 处理，OwnerRefEngine 只能处理模板中仅单个对象场景
	var ey environment.EngineYard = map[string]environment.Engine{
		environment.GoTplEngine: /*e*/ engine.New(),
	}
	env := &environment.Environment{
		EngineYard: ey,
		Releases:   c.storageBackend,
		KubeClient: c.tillerKubeClient,
	}

	internalClientSet, _ := internalclientset.NewForConfig(k8sclient.GetKubeConfig())

	server := tiller.NewReleaseServer(env, internalClientSet, false)
	server.Log = c.Logger(r)
	return server
}

//ReleaseOptionBool release bool option
func ReleaseOptionBool(r *v1alpha1.HelmApp, option string, defaultVal bool) bool {
	switch strings.ToLower(ReleaseOption(r, option, "")) {
	case "true", "t", "yes", "y", "1", "on":
		return true
	case "false", "f", "no", "n", "0", "off":
		return false
	}
	return defaultVal
}

//ReleaseOption release option
func ReleaseOption(r *v1alpha1.HelmApp, option string, defaultVal string) string {
	if val, ok := r.Annotations[OptionAnnotation(option)]; ok {
		return val
	}
	return defaultVal
}

//OptionAnnotation option annotation key
func OptionAnnotation(option string) string {
	return fmt.Sprintf("%s/%s", OperatorName(), option)
}

//OperatorName operator name
func OperatorName() string {
	if name, err := k8sutil.GetOperatorName(); err == nil {
		return name
	}
	return "helm-app-operator"
}

//ReleaseName release name
func ReleaseName(r *v1alpha1.HelmApp) string {
	return ReleaseOption(r, OptionRelease, fmt.Sprintf("%s-%s", OperatorName(), r.GetName()))
}

// TranslateChartPath loading chart path
func TranslateChartPath(r *v1alpha1.HelmApp, chartPath string) string {
	chart := ReleaseOption(r, OptionChart, "")
	if chart == "" {
		return chartPath
	}
	if filepath.IsAbs(chart) {
		return chart
	}
	return filepath.Join(chartPath, chart)
}

//BehaviorReleaseName customize release name
type BehaviorReleaseName interface {
	ReleaseName(r *v1alpha1.HelmApp) string
}

//BehaviorReleaseValues customize release name
type BehaviorReleaseValues interface {
	ReleaseValues(r *v1alpha1.HelmApp) (map[string]interface{}, error)
}

//BehaviorOptionForce customize release force option
type BehaviorOptionForce interface {
	OptionForce(r *v1alpha1.HelmApp) bool
}

//BehaviorLogger customize logger
type BehaviorLogger interface {
	Logger(r *v1alpha1.HelmApp) func(string, ...interface{})
}

//BehaviorChartPath customize load chart
type BehaviorChartPath interface {
	TranslateChartPath(r *v1alpha1.HelmApp, chartPath string) (string, error)
}

func (c installer) ReleaseName(r *v1alpha1.HelmApp) string {
	if behavior, ok := c.behavior.(BehaviorReleaseName); ok {
		return behavior.ReleaseName(r)
	}
	return ReleaseName(r)
}

func (c installer) ReleaseValues(r *v1alpha1.HelmApp) (map[string]interface{}, error) {
	if behavior, ok := c.behavior.(BehaviorReleaseValues); ok {
		return behavior.ReleaseValues(r)
	}
	return r.Spec, nil
}

func (c installer) OptionForce(r *v1alpha1.HelmApp) bool {
	if behavior, ok := c.behavior.(BehaviorOptionForce); ok {
		return behavior.OptionForce(r)
	}
	return ReleaseOptionBool(r, OptionForce, false)
}

func (c installer) TranslateChartPath(r *v1alpha1.HelmApp, chartPath string) (string, error) {
	if behavior, ok := c.behavior.(BehaviorChartPath); ok {
		return behavior.TranslateChartPath(r, chartPath)
	}
	return TranslateChartPath(r, chartPath), nil
}

func (c installer) LoadChart(r *v1alpha1.HelmApp, chartPath string) (*cpb.Chart, []byte, error) {
	values, err := c.ReleaseValues(r)
	if err != nil {
		return nil, nil, err
	}

	// enable .Values.global.ownerReferences
	global := map[string]interface{}{
		"ownerReferences": []metav1.OwnerReference{*metav1.NewControllerRef(r, r.GroupVersionKind())},
	}
	if g, ok := values["global"]; ok {
		if gr, ok := g.(map[string]interface{}); ok {
			for k, v := range gr {
				global[k] = v
			}
		}
	}
	values["global"] = global

	valueYaml, err := yaml.Marshal(values)
	if err != nil {
		return nil, nil, err
	}

	chartPath, err = c.TranslateChartPath(r, chartPath)
	if err != nil {
		return nil, nil, err
	}

	chart, err := chartutil.Load(chartPath)
	if err != nil {
		return nil, nil, err
	}
	return chart, valueYaml, nil
}

func (c installer) Logger(r *v1alpha1.HelmApp) func(string, ...interface{}) {
	if behavior, ok := c.behavior.(BehaviorLogger); ok {
		return behavior.Logger(r)
	}
	return func(string, ...interface{}) {}
}
