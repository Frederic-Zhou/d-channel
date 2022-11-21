package ipfsnode

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

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

const MessageProto = "/x/message"

func Start(ctx context.Context, repo string) {
	// Spawn a local peer using a temporary path, for testing purposes
	var err error
	IpfsAPI, IpfsNode, err = spawn(ctx, repo)

	if err != nil {
		panic(fmt.Errorf("failed to spawn peer node: %s", err))
	}

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

func createRepo(repo string) (string, error) {

	// Create a config with default options and a 2048 bit key
	cfg, err := config.Init(io.Discard, 2048)
	if err != nil {
		return "", err
	}

	cfg.Experimental.Libp2pStreamMounting = true
	// cfg.Experimental.P2pHttpProxy = true

	// Create the repo with the config
	err = fsrepo.Init(repo, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to init node: %s", err)
	}

	return repo, nil
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
func spawn(ctx context.Context, repo string) (icore.CoreAPI, *core.IpfsNode, error) {
	var onceErr error
	loadPluginsOnce.Do(func() {
		onceErr = setupPlugins("")
	})
	if onceErr != nil {
		return nil, nil, onceErr
	}
	// Create a  Repo
	repoPath, err := createRepo(repo)
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
func ListenLocal(ctx context.Context, readchan chan []byte, port string, proto string) (err error) {

	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%s", port))
	if err != nil {
		return
	}

	listener, err := IpfsNode.P2P.ForwardRemote(ctx, p2pcore.ProtocolID(proto), addr, true)
	if err != nil {
		return
	}
	defer IpfsNode.P2P.ListenersP2P.Close(func(listener p2p.Listener) bool {
		return true
	})

	mlistener, err := manet.Listen(listener.TargetAddress())
	if err != nil {
		return
	}
	defer mlistener.Close()

	log.Println("ready to listen", listener.Protocol(), listener.ListenAddress(), listener.TargetAddress())
	if err := acceptConnect(ctx, mlistener, readchan); err != nil {
		log.Println(err)
	}

	return

}

func acceptConnect(ctx context.Context, mlistener manet.Listener, readchan chan []byte) (err error) {

	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, errConn := mlistener.Accept()
			if errConn != nil {
				log.Println("got a conn but error", errConn)
				continue
			}
			conn.SetDeadline(time.Now().Add(10 * time.Second))
			log.Println("got a conn", conn.RemoteAddr())

			go func(conn manet.Conn, readchan chan []byte) {
				defer conn.Close()

				bf := bytes.NewBuffer([]byte{})
				if _, err = io.Copy(bf, conn); err != nil {
					return
				}

				log.Println("conn read", bf.String())
				readchan <- bf.Bytes()
				log.Println("write to readchan", bf.String())

			}(conn, readchan)

		}

	}

}

var sendLock sync.Mutex

// 发送过程，必须是非并发的
// 由于接受端读取采用io.copy，因此发送端 采用连接、发送、关闭，一次连接发送一次数据。
func SendMessage(peerID string, message string, port string, proto string) (err error) {

	sendLock.Lock()
	defer sendLock.Unlock()

	ctx := context.Background()

	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%s", port))
	if err != nil {
		return
	}

	peerid, err := peer.Decode(peerID)
	if err != nil {
		return
	}

	l, err := IpfsNode.P2P.ForwardLocal(ctx, peerid, p2pcore.ProtocolID(proto), addr)
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
