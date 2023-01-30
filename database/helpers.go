package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	// files "github.com/ipfs/go-ipfs-files"

	icore "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	config "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
	"github.com/ipfs/kubo/core/node/libp2p"
	"github.com/ipfs/kubo/plugin/loader"
	"github.com/ipfs/kubo/repo/fsrepo"
	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap"
)

func setupPlugins(path string) error {
	plugins, err := loader.NewPluginLoader(filepath.Join(path, "plugins"))
	if err != nil {
		return fmt.Errorf("error loading plugins: %s", err)
	}

	if err := plugins.Initialize(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	if err := plugins.Inject(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	return nil
}
func createRepo(repoPath string) error {

	var cfg *config.Config
	var err error

	identity, err := config.CreateIdentity(os.Stdout, []options.KeyGenerateOption{
		options.Key.Type("ed25519"),
	})
	if err != nil {
		return err
	}
	cfg, err = config.InitWithIdentity(identity)
	if err != nil {
		return err
	}
	cfg.Pubsub.Enabled = config.True

	// Create the repo with the config
	err = fsrepo.Init(repoPath, cfg)
	if err != nil {
		return fmt.Errorf("failed to init node: %s", err)
	}

	return nil
}
func createNode(ctx context.Context, repoPath string) (*core.IpfsNode, icore.CoreAPI, error) {
	repo, err := fsrepo.Open(repoPath)
	if err != nil {
		err = createRepo(repoPath)
		if err != nil {
			return nil, nil, err
		}
	}

	nodeOptions := &core.BuildCfg{
		Online: true,
		// Permanent: true,
		Routing: libp2p.DHTClientOption, // DHTOption
		Repo:    repo,
		ExtraOpts: map[string]bool{
			"pubsub": true,
		},
	}

	node, err := core.NewNode(ctx, nodeOptions)
	if err != nil {
		return nil, nil, err
	}

	coreAPI, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, nil, err
	}

	return node, coreAPI, nil
}

func structToMap(v interface{}) (map[string]interface{}, error) {
	vMap := &map[string]interface{}{}

	err := mapstructure.Decode(v, &vMap)
	if err != nil {
		return nil, err
	}

	return *vMap, nil
}

func newLogger(filename string) (*zap.Logger, error) {
	if runtime.GOOS == "windows" {
		zap.RegisterSink("winfile", func(u *url.URL) (zap.Sink, error) {
			return os.OpenFile(u.Path[1:], os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		})
	}

	cfg := zap.NewDevelopmentConfig()
	if runtime.GOOS == "windows" {
		cfg.OutputPaths = []string{
			"stdout",
			"winfile:///" + filename,
		}
	} else {
		cfg.OutputPaths = []string{
			filename,
		}
	}

	return cfg.Build()
}
