package ipfsnode

import (
	"bytes"
	"context"
	"d-channel/localstore"
	"fmt"
	"io"
	"log"
	"time"

	"os"
	"path/filepath"
	"sync"

	icore "github.com/ipfs/interface-go-ipfs-core"

	// peer "github.com/libp2p/go-libp2p-peer"

	p2pcore "github.com/libp2p/go-libp2p/core"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
	"github.com/ipfs/kubo/core/node/libp2p" // This package is needed so that all the preloaded plugins are loaded automatically
	"github.com/ipfs/kubo/p2p"
	"github.com/ipfs/kubo/plugin/loader"
	"github.com/ipfs/kubo/repo/fsrepo"
	manet "github.com/multiformats/go-multiaddr/net"

	// ldformat "github.com/ipfs/go-ipld-format"

	_ "github.com/mattn/go-sqlite3"
)

var IpfsAPI icore.CoreAPI
var IpfsNode *core.IpfsNode

const messageProto = "/x/message"
const listenLocalAddr = "/ip4/127.0.0.1/tcp/8090"
const forwardLocalAddr = "/ip4/127.0.0.1/tcp/8091"

func Start(ctx context.Context) {
	// Spawn a local peer using a temporary path, for testing purposes
	var err error
	IpfsAPI, IpfsNode, err = spawn(ctx)

	if err != nil {
		panic(fmt.Errorf("failed to spawn peer node: %s", err))
	}

	l, err := ListenLocal(ctx, IpfsNode)
	if err != nil {
		panic(fmt.Errorf("failed to spawn peer node: %s", err))
	}

	log.Println("listener:", l.Protocol(), l.ListenAddress(), l.TargetAddress())

	return
}

// / ------ Setting up the IPFS Repo

func setupPlugins(externalPluginsPath string) error {
	// Load any external plugins if available on externalPluginsPath
	plugins, err := loader.NewPluginLoader(filepath.Join(externalPluginsPath, "plugins"))
	if err != nil {
		return fmt.Errorf("error loading plugins: %s", err)
	}

	// Load preloaded and external plugins
	if err := plugins.Initialize(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	if err := plugins.Inject(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	return nil
}

func createRepo() (string, error) {
	repoPath := "./repo"

	err := os.Mkdir(repoPath, 0755)

	if os.IsExist(err) {
		return repoPath, nil
	}

	defer func() {
		if err != nil {
			os.Remove(repoPath)
		}
	}()

	if err != nil {
		return "", fmt.Errorf("failed to get temp dir: %s", err)
	}

	// Create a config with default options and a 2048 bit key
	cfg, err := config.Init(io.Discard, 2048)
	if err != nil {
		return "", err
	}

	cfg.Experimental.Libp2pStreamMounting = true
	// cfg.Experimental.P2pHttpProxy = true

	// Create the repo with the config
	err = fsrepo.Init(repoPath, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to init node: %s", err)
	}

	return repoPath, nil
}

/// ------ Spawning the node

// Creates an IPFS node and returns its coreAPI
func createNode(ctx context.Context, repoPath string) (*core.IpfsNode, error) {
	// Open the repo
	repo, err := fsrepo.Open(repoPath)
	if err != nil {
		return nil, err
	}

	// Construct the node
	nodeOptions := &core.BuildCfg{
		Online:  true,
		Routing: libp2p.DHTOption, // This option sets the node to be a full DHT node (both fetching and storing DHT Records)
		// Routing: libp2p.DHTClientOption, // This option sets the node to be a client DHT node (only fetching records)
		Repo: repo,
	}

	return core.NewNode(ctx, nodeOptions)
}

var loadPluginsOnce sync.Once

// Spawns a node to be used just for this run (i.e. creates a tmp repo)
func spawn(ctx context.Context) (icore.CoreAPI, *core.IpfsNode, error) {
	var onceErr error
	loadPluginsOnce.Do(func() {
		onceErr = setupPlugins("")
	})
	if onceErr != nil {
		return nil, nil, onceErr
	}
	// Create a  Repo
	repoPath, err := createRepo()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create repo: %s", err)
	}

	node, err := createNode(ctx, repoPath)
	if err != nil {
		return nil, nil, err
	}

	api, err := coreapi.NewCoreAPI(node)

	return api, node, err
}

// / -------
func ListenLocal(ctx context.Context, ipfsnode *core.IpfsNode) (listener p2p.Listener, err error) {

	var proto = p2pcore.ProtocolID(messageProto)

	addr, err := multiaddr.NewMultiaddr(listenLocalAddr)
	if err != nil {
		return
	}

	listener, err = ipfsnode.P2P.ForwardRemote(ctx, proto, addr, true)

	go func() {
		log.Println("start listener")
		if err := listenConnect(ctx, listener.TargetAddress()); err != nil {
			log.Println(err)
		}
	}()
	return

}

func listenConnect(ctx context.Context, laddr multiaddr.Multiaddr) (err error) {

	listener, err := manet.Listen(laddr)
	if err != nil {
		return
	}

	defer listener.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, errConn := listener.Accept()
			if errConn != nil {
				log.Println("got a conn but error", errConn)
				continue
			}
			conn.SetDeadline(time.Now().Add(10 * time.Second))
			log.Println("got a conn", conn.RemoteAddr())

			go func() {
				if err := readConn(conn); err != nil {
					log.Println(err)
				}
			}()

		}

	}

}

func readConn(conn manet.Conn) (err error) {
	defer conn.Close()

	bf := bytes.NewBuffer([]byte{})
	_, err = io.Copy(bf, conn)
	if err != nil {
		return
	}

	log.Println("message:", bf.String())

	return localstore.WriteMessage(bf.String())
}

var sendLock sync.Mutex

func SendMessage(peerID string, message string) (err error) {

	sendLock.Lock()
	defer sendLock.Unlock()

	ctx := context.Background()

	var proto = p2pcore.ProtocolID(messageProto)

	addr, err := multiaddr.NewMultiaddr(forwardLocalAddr)
	if err != nil {
		return
	}

	peerid, err := peer.Decode(peerID)
	if err != nil {
		return
	}

	l, err := IpfsNode.P2P.ForwardLocal(ctx, peerid, proto, addr)
	if err != nil {
		return
	}
	defer IpfsNode.P2P.ListenersLocal.Close(func(listener p2p.Listener) bool {
		return true
	})

	log.Println(l.ListenAddress(), l.Protocol(), l.TargetAddress())

	conn, err := manet.Dial(l.ListenAddress())
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	n, err := conn.Write([]byte(message))
	log.Println("writed:", n)

	return

}
