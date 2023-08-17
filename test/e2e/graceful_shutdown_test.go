package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/cluster-image-registry-operator/pkg/defaults"
	"github.com/openshift/cluster-image-registry-operator/test/framework"
)

func TestImageRegistryGracefulShutdown(t *testing.T) {
	te := framework.SetupAvailableImageRegistry(t, nil)
	defer framework.TeardownImageRegistry(te)

	framework.EnsureOperatorIsNotHotLooping(te)

	pods, err := te.Client().Pods(defaults.ImageRegistryOperatorNamespace).List(
		context.Background(), metav1.ListOptions{
			LabelSelector: "docker-registry=default",
		},
	)
	if err != nil {
		t.Fatalf("unable to list pods: %v", err)
	}

	if len(pods.Items) == 0 {
		t.Fatalf("no image-registry pods found")
	}

	var pod *corev1.Pod
	var logch <-chan string
	var errch <-chan error
	for _, p := range pods.Items {
		if p.Status.Phase != "Running" {
			t.Logf("skipping pod %s: phase=%s", p.Name, p.Status.Phase)
			continue
		}

		if logch, errch, err = framework.FollowPodLog(te.Client(), p); err != nil {
			t.Logf("unable to follow log on pod %s: %v", p.Name, err)
			continue
		}

		pod = &p
		break
	}
	if pod == nil {
		t.Fatal("unable to attach to any pod log stream")
	}

	if err := te.Client().Pods(defaults.ImageRegistryOperatorNamespace).Delete(
		context.Background(), pod.Name, metav1.DeleteOptions{},
	); err != nil {
		t.Fatalf("error deleting pod: %v", err)
	}

	timeout := time.NewTimer(time.Minute)
	defer timeout.Stop()
	for {
		select {
		case <-timeout.C:
			t.Fatal("timeout awaiting for pod to die.")
		case err, more := <-errch:
			if !more {
				errch = nil
				break
			}
			t.Fatalf("error reading pod log: %v", err)
		case line, more := <-logch:
			if !more {
				t.Fatal("pod died, no graceful message found.")
			}
			// this is the log line image registry pod prints when
			// it gracefully dies, if we find this it means that the
			// pod had manage to exit properly so we can end this
			// test successfully.
			if strings.Contains(line, "server shutdown, bye.") {
				return
			}
		}
	}
}
