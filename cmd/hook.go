package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/xiaopal/helm-app-operator/cmd/option"

	"github.com/operator-framework/helm-app-operator-kit/helm-app-operator/pkg/apis/app/v1alpha1"
	"github.com/xiaopal/helm-app-operator/cmd/helmext"
)

func execHook(r *v1alpha1.HelmApp, hook string) error {
	script := helmext.ReleaseOption(r, hook, "")
	if len(script) == 0 {
		return nil
	}
	if !option.OptionHooks {
		logger.Println("skipped, hooks disabled")
		return nil
	}
	return execEvent(r, hook, script)
}

func execEvent(r *v1alpha1.HelmApp, event string, script string) error {
	cmd := exec.Command("/bin/bash", "-c", script)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("EVENT_TYPE=%s", event),
		fmt.Sprintf("EVENT_API_VERSION=%s", option.OptionAPIVersion),
		fmt.Sprintf("EVENT_KIND=%s", option.OptionCRDKind),
		fmt.Sprintf("EVENT_NAMESPACE=%s", r.GetNamespace()),
		fmt.Sprintf("EVENT_RESOURCE_TYPE=%s.%s", option.OptionCRDPlural, option.OptionCRDGroup),
		fmt.Sprintf("EVENT_RESOURCE=%s", r.GetName()),
		fmt.Sprintf("EVENT_RELEASE=%s", helmext.ReleaseName(r)),
	)
	logger := option.NewLogger(event)
	if err := pipeCmd(cmd, logger); err != nil {
		logger.Printf("failed to setup command: %v", err.Error())
		return err
	}
	if err := cmd.Run(); err != nil {
		logger.Printf("failed to run command: %v", err.Error())
		return err
	}
	return nil
}

func pipeCmd(cmd *exec.Cmd, logger *log.Logger) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	forward := func(r io.ReadCloser) {
		o := bufio.NewScanner(r)
		for o.Scan() {
			logger.Println(o.Text())
		}
		if err := o.Err(); err != nil {
			logger.Printf("ERROR: %v", err.Error())
		}
	}
	go forward(stdout)
	go forward(stderr)
	return nil
}
