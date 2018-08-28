package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/operator-framework/helm-app-operator-kit/helm-app-operator/pkg/apis/app/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/xiaopal/helm-app-operator/cmd/helmext"
)

type handler struct {
	controller helmext.Installer
}

func (h *handler) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *v1alpha1.HelmApp:
		if event.Deleted {
			//ignore delete event
			return nil
		}
		finalizerFound, finalizerRemains := false, []string{}
		for _, v := range o.GetFinalizers() {
			if v == helmext.OperatorName() {
				finalizerFound = true
			} else {
				finalizerRemains = append(finalizerRemains, v)
			}
		}
		if o.GetDeletionTimestamp() != nil {
			if !finalizerFound {
				return nil
			}
			logger.Printf("Uninstalling %s", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
			if err := execHook(o, "pre-uninstall"); err != nil {
				return err
			}
			updatedResource, err := h.controller.UninstallRelease(o)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					logger.Printf("%s already uninstalled", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
					return nil
				}
				logger.Printf("failed to uninstall release: %v", err.Error())
				return err
			}
			if !event.Deleted {
				updatedResource.SetFinalizers(finalizerRemains)
				err = sdk.Update(updatedResource)
				if err != nil {
					logger.Printf("failed to update custom resource status: %v", err.Error())
					return err
				}
			}
			if err := execHook(updatedResource, "post-uninstall"); err != nil {
				return err
			}
			logger.Printf("%s uninstalled", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
			return nil
		}
		if updated, err := h.updateChecksum(o); err != nil {
			logger.Printf("failed to update checksum: %v", err.Error())
			return err
		} else if !updated {
			//unchanged, continue
			return nil
		}
		logger.Printf("Installing %s", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
		if err := execHook(o, "pre-install"); err != nil {
			return err
		}
		updatedResource, err := h.controller.InstallRelease(o)
		if err != nil {
			logger.Printf("failed to install release: %v", err.Error())
			return err
		}
		if !finalizerFound {
			updatedResource.SetFinalizers(append(finalizerRemains, helmext.OperatorName()))
		}
		err = sdk.Update(updatedResource)
		if err != nil {
			logger.Printf("failed to update custom resource status: %v", err.Error())
			return err
		}
		if err := execHook(updatedResource, "post-install"); err != nil {
			return err
		}
		logger.Printf("%s updated", strings.Join([]string{o.GetNamespace(), o.GetName()}, "/"))
	}
	return nil
}

func (h *handler) updateChecksum(r *v1alpha1.HelmApp) (bool, error) {
	annoChecksum := helmext.OptionAnnotation("checksum")
	annotations, lastChecksum := map[string]string{}, ""
	for k, v := range r.GetAnnotations() {
		if k == annoChecksum {
			lastChecksum = v
		} else {
			annotations[k] = v
		}
	}
	values, err := h.controller.ReleaseValues(r)
	if err != nil {
		return false, err
	}
	bytes, err := json.Marshal([]interface{}{
		r.GetName(),
		r.GetNamespace(),
		r.GetLabels(),
		annotations,
		values,
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
