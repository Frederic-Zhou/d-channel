package ipfsnode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"path/filepath"
	"sync"

	icore "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"

	// peer "github.com/libp2p/go-libp2p-peer"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
	"github.com/ipfs/kubo/core/node/libp2p" // This package is needed so that all the preloaded plugins are loaded automatically
	"github.com/ipfs/kubo/plugin/loader"
	"github.com/ipfs/kubo/repo/fsrepo"

	// ldformat "github.com/ipfs/go-ipld-format"

	_ "github.com/mattn/go-sqlite3"
)

var IpfsAPI icore.CoreAPI
var IpfsNode *core.IpfsNode

const P2PMessageProto = "/x/message"
const StreamMessageProto = "/chat/1.0"

func Start(ctx context.Context, repoPath string) {
	// Spawn a local peer using a temporary path, for testing purposes
	var err error
	IpfsAPI, IpfsNode, err = Spawn(ctx, repoPath)

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

	// Create the repo with the config
	err = fsrepo.Init(repoPath, cfg)
	if err != nil {
		return fmt.Errorf("failed to init node: %s", err)
	}

	return nil
}

/// ------ Spawning the node

// Creates an IPFS node and returns its coreAPI
func createNode(ctx context.Context, repoPath string) (*core.IpfsNode, error) {
	// Open the repo
	repo, err := fsrepo.Open(repoPath)
	if err != nil {
		return nil, err
	}

	cfg, err := repo.Config()
	if err != nil {
		return nil, err
	}

	cfg.Experimental.Libp2pStreamMounting = true
	cfg.Experimental.P2pHttpProxy = true
	cfg.Ipns.UsePubsub = config.True
	cfg.Pubsub.Enabled = config.True

	bootstrap := []string{
		"/ip4/1.14.102.100/tcp/4001/p2p/12D3KooWBBbdgzJBLUUFhMpA9JucE932wJNt2d6QZrGgSmPvTtPZ",
		"/ip4/1.14.102.100/udp/4001/quic/p2p/12D3KooWBBbdgzJBLUUFhMpA9JucE932wJNt2d6QZrGgSmPvTtPZ",
		"/ip4/1.14.59.205/tcp/4001/p2p/12D3KooWCjiGPnmxpsZnH1Zv2DzHJ5ReigdJhCsbnvB2ZXdjQrvz",
		"/ip4/1.14.59.205/udp/4001/quic/p2p/12D3KooWCjiGPnmxpsZnH1Zv2DzHJ5ReigdJhCsbnvB2ZXdjQrvz",
	}

	cfg.Bootstrap = bootstrap
	// cfg.Bootstrap = config.DefaultBootstrapAddresses

	repo.SetConfig(cfg)

	// Construct the node
	nodeOptions := &core.BuildCfg{
		Online:    true,
		Permanent: true,
		Routing:   libp2p.DHTOption, // This option sets the node to be a full DHT node (both fetching and storing DHT Records)
		// Routing: libp2p.DHTClientOption, // This option sets the node to be a client DHT node (only fetching records)
		Repo: repo,
		ExtraOpts: map[string]bool{
			"pubsub": true,
			"ipnsps": true,
		},
	}

	return core.NewNode(ctx, nodeOptions)
}

var loadPluginsOnce sync.Once

// Spawns a node to be used just for this run (i.e. creates a tmp repo)
func Spawn(ctx context.Context, repoPath string) (icore.CoreAPI, *core.IpfsNode, error) {
	var onceErr error
	loadPluginsOnce.Do(func() {
		onceErr = setupPlugins("")
	})
	if onceErr != nil {
		return nil, nil, onceErr
	}
	// Create a  Repo
	err := createRepo(repoPath)
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

// ------- P2P listener
/*
func ListenLocal(ctx context.Context, readchan chan []byte, port string) (err error) {

	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%s", port))
	if err != nil {
		return
	}

	listener, err := IpfsNode.P2P.ForwardRemote(ctx, P2PMessageProto, addr, true)
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
func SendMessage(peerID string, message string, port string) (err error) {

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

	l, err := IpfsNode.P2P.ForwardLocal(ctx, peerid, P2PMessageProto, addr)
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
*/

func SetStreamHandler(readchan chan string) {
	hosts := []host.Host{
		IpfsNode.DHT.LAN.Host(),
		IpfsNode.DHT.WAN.Host(),
	}

	for _, host := range hosts {
		host.SetStreamHandler(StreamMessageProto, func(s network.Stream) {
			err := readHelloProtocol(readchan, s)
			if err != nil {
				s.Reset()
			} else {
				s.Close()
			}
		})
	}

	// return *host.InfoFromHost(lanHost), *host.InfoFromHost(wanHost)

}

func readHelloProtocol(readchan chan string, s network.Stream) error {
	// TO BE IMPLEMENTED: Read the stream and print its content
	buf := bufio.NewReader(s)
	message, err := buf.ReadString('\n')
	if err != nil {
		return err
	}

	connection := s.Conn()

	data, err := json.Marshal(map[string]string{
		"from":    connection.RemotePeer().String(),
		"message": message,
	})
	if err != nil {
		return err
	}
	log.Println(string(data))
	readchan <- string(data)
	return nil
}

func NewStream(ctx context.Context, peeridstr string, message string) (err error) {

	var targetNodeInfo peer.AddrInfo

	pid, err := peer.Decode(peeridstr)
	if err != nil {
		return
	}
	targetNodeInfo, err = IpfsAPI.Dht().FindPeer(ctx, pid)
	if err != nil {
		return
	}

	hosts := []host.Host{
		IpfsNode.DHT.LAN.Host(),
		IpfsNode.DHT.WAN.Host(),
	}

	for i, host := range hosts {

		err = host.Connect(context.Background(), targetNodeInfo)
		if err != nil {
			log.Printf("%d Sending message... %v \n", i, err)
			continue
		}

		var stream network.Stream
		// TO BE IMPLEMENTED: Open stream and send message
		stream, err = host.NewStream(context.Background(), targetNodeInfo.ID, StreamMessageProto)
		if err != nil {
			continue
		}

		if !strings.HasSuffix(message, "\n") {
			message += "\n"
		}

		_, err = stream.Write([]byte(message))
		if err != nil {
			continue
		}
		return
		//当第一个lan host 走到发送完成，就返回。如果第一个lan host 在中间任何位置 continue 略过，
		// 进入wan host处理，wan host 处理过程中 任何错误 continue，则会退出循环，返回当前错误。

	}
	return
}

func RemoveStreamHandler() {
	hosts := []host.Host{
		IpfsNode.DHT.LAN.Host(),
		IpfsNode.DHT.WAN.Host(),
	}

	for _, host := range hosts {
		host.RemoveStreamHandler(StreamMessageProto)
	}

	// return *host.InfoFromHost(lanHost), *host.InfoFromHost(wanHost)

}
