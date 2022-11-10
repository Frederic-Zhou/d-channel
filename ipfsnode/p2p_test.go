package ipfsnode

import (
	"context"
	"fmt"
	"testing"

	core "github.com/libp2p/go-libp2p/core"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

func TestP2P(t *testing.T) {

	ctx := context.Background()
	var err error
	_, ipfsNode, err := spawn(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to spawn peer node: %s", err))
	}

	// fmt.Println(ipfsAPI, ipfsNode)

	var proto = core.ProtocolID("/x/good")
	addr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1234")
	fmt.Println(err)
	addr2, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4321")
	fmt.Println(err)

	_, err = ipfsNode.P2P.ForwardRemote(ctx, proto, addr, true)
	fmt.Println(err)

	_, err = ipfsNode.P2P.ForwardLocal(ctx, ipfsNode.Identity, proto, addr2)
	fmt.Println(err)

	for k, v := range ipfsNode.P2P.ListenersP2P.Listeners {
		fmt.Println(k, v.Protocol(), v.ListenAddress(), v.TargetAddress())
	}

	for k, v := range ipfsNode.P2P.ListenersLocal.Listeners {
		fmt.Println(k, v.Protocol(), v.ListenAddress(), v.TargetAddress())
	}

	sign, err := ipfsNode.PrivateKey.Sign([]byte("hello"))
	fmt.Println(err)

	v, err := ipfsNode.PrivateKey.GetPublic().Verify([]byte("hello"), sign)

	fmt.Println(v, err, ipfsNode.Identity.String(), ipfsNode.Identity)

	fmt.Println(ipfsNode.Identity.MatchesPrivateKey(ipfsNode.PrivateKey))

	fmt.Println(ipfsNode.Identity.MatchesPublicKey(ipfsNode.PrivateKey.GetPublic()))

	pid, _ := peer.IDFromPublicKey(ipfsNode.PrivateKey.GetPublic())
	pid2, _ := peer.IDFromPrivateKey(ipfsNode.PrivateKey)
	fmt.Println(pid.String(), pid2.String())

}
