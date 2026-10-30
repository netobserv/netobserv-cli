package cluster

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/netobserv/network-observability-cli/internal/k8s/kindconfig"
	"github.com/sirupsen/logrus"
	"github.com/vladimirvivien/gexe"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"
)

const (
	cliContainerName = "localhost/netobserv-cli:test"
	kindImage        = "kindest/node:v1.31.0"
	kindCreateTries  = 3
	kindRetryDelay   = time.Second
	logsSubDir       = "e2e-logs"
	localArchiveName = "cli-e2e-img.tar"
)

var klog = logrus.WithField("component", "cluster.Kind")

type Kind struct {
	clusterName string
	baseDir     string
	testEnv     env.Environment
	timeout     time.Duration
}

// Option that can be passed to the NewKind function in order to change the configuration
// of the test cluster
type Option func(k *Kind)

// NewKind creates a kind cluster given a name and set of Option instances. The base dir
// must point to the folder where the logs are going to be stored and, in case your docker
// backend doesn't provide access to the local images, where the cli-e2e-img.tar container image
// is located. Usually it will be the project root.
func NewKind(kindClusterName, baseDir string, options ...Option) *Kind {
	k := &Kind{
		testEnv:     env.New(),
		clusterName: kindClusterName,
		baseDir:     baseDir,
		timeout:     2 * time.Minute,
	}
	for _, option := range options {
		option(k)
	}
	return k
}

// Run the Kind cluster for the later execution of tests.
func (k *Kind) Run(m *testing.M) {
	configPath, err := kindconfig.WriteTemp(kindconfig.E2E())
	if err != nil {
		panic(err)
	}
	defer os.Remove(configPath)

	envFuncs := []env.Func{
		k.createCluster(configPath),
		k.loadLocalImage(),
	}
	klog.Info("starting kind setup")
	code := k.testEnv.Setup(envFuncs...).
		Finish(
			k.exportLogs(),
			k.deleteNamespace(),
			envfuncs.DestroyCluster(k.clusterName),
		).Run(m)
	klog.WithField("returnCode", code).Info("tests finished run")
}

// createCluster retries the provider's creation step because kind's Docker provider can race
// with systemd's boot log: the node is healthy, but kind may start watching its logs after the
// "Multi-User System" line was emitted. A bounded retry handles that transient startup race
// without hiding genuine cluster creation failures.
func (k *Kind) createCluster(configPath string) env.Func {
	create := envfuncs.CreateClusterWithConfig(
		kind.NewProvider(),
		k.clusterName,
		configPath,
		kind.WithImage(kindImage))

	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		var lastErr error
		for attempt := 1; attempt <= kindCreateTries; attempt++ {
			createdCtx, err := create(ctx, cfg)
			if err == nil {
				return createdCtx, nil
			}
			lastErr = err
			if attempt == kindCreateTries {
				break
			}
			klog.WithError(err).WithField("attempt", attempt).Warn("kind cluster creation failed; retrying")
			select {
			case <-ctx.Done():
				return ctx, ctx.Err()
			case <-time.After(kindRetryDelay):
			}
		}
		return ctx, fmt.Errorf("kind cluster creation failed after %d attempts: %w", kindCreateTries, lastErr)
	}
}

func (k *Kind) GetLogsDir() string {
	logsDir := path.Join(k.baseDir, logsSubDir)
	err := os.MkdirAll(logsDir, 0700)
	if err != nil {
		klog.Error(err)
	}
	return logsDir
}

// export logs into the e2e-logs folder of the base directory.
func (k *Kind) exportLogs() env.Func {
	return func(ctx context.Context, _ *envconf.Config) (context.Context, error) {
		logsDir := k.GetLogsDir()
		klog.WithField("directory", logsDir).Info("exporting cluster logs")
		exe := gexe.New()
		out := exe.Run("kind export logs " + logsDir + " --name " + k.clusterName)
		klog.WithField("out", out).Info("exported cluster logs")

		// move output files to cluster logs folder
		err := os.Rename(path.Join(k.baseDir, "e2e", "tmp"), path.Join(logsDir, "output"))
		if err != nil && !os.IsNotExist(err) {
			klog.Error(err)
		}

		return ctx, nil
	}
}

func (k *Kind) GetAgentLogs() string {
	exe := gexe.New()
	kubeContext := "kind-" + k.clusterName
	contextOut := exe.Run("kubectl cluster-info --context " + kubeContext)
	logsOut := exe.Run("kubectl logs --context " + kubeContext + " -l app=netobserv-cli -n netobserv-cli --tail -1")

	return fmt.Sprintf("Set context: %s\n\nLogs: %s", contextOut, logsOut)
}

// delete netobserv-cli namespace
func (k *Kind) deleteNamespace() env.Func {
	return func(ctx context.Context, _ *envconf.Config) (context.Context, error) {
		exe := gexe.New()
		out := exe.Run("kubectl delete namespace netobserv-cli --ignore-not-found --context kind-" + k.clusterName)
		klog.WithField("out", out).Info("deleted namespace")
		return ctx, nil
	}
}

func (k *Kind) TestEnv() env.Environment {
	return k.testEnv
}

// loadLocalImage loads the cli docker image into the test cluster. It tries both available
// methods, which will selectively work depending on the container backend type
func (k *Kind) loadLocalImage() env.Func {
	return func(ctx context.Context, config *envconf.Config) (context.Context, error) {
		klog.Info("trying to load docker image from local registry")
		ctx, err := envfuncs.LoadDockerImageToCluster(
			k.clusterName, cliContainerName)(ctx, config)
		if err == nil {
			klog.Info("loaded docker image from local registry")
			return ctx, nil
		}
		klog.WithError(err).WithField("archive", localArchiveName).
			Info("couldn't load image from local registry. Trying from local archive")
		ctx, err = envfuncs.LoadImageArchiveToCluster(
			k.clusterName, path.Join(k.baseDir, localArchiveName))(ctx, config)
		if err == nil {
			klog.Info("loaded docker image from local archive")
			return ctx, nil
		}
		return ctx, err
	}
}
