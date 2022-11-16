package test

import (
	"context"
	"d-channel/ipfsnode"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"sort"
	"testing"

	version "github.com/ipfs/kubo"

	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
	identify "github.com/libp2p/go-libp2p/p2p/protocol/identify"
)

type IdOutput struct { //nolint
	ID              string
	PublicKey       string
	Addresses       []string
	AgentVersion    string
	ProtocolVersion string
	Protocols       []string
}

func TestStructType(t *testing.T) {

	m := map[string]interface{}{"a": "b", "v": 1}

	type1 := reflect.TypeOf(m)

	str := []float64{13.21}
	fmt.Printf("%s\n%v\n%+v\n%#v\n%T\n", type1.String(), m, m, m, m)
	fmt.Printf("%v\n%+v\n%#v\n%T\n", str, str, str, str)

}

func TestNodeID(t *testing.T) {

	ctx := context.Background()
	var err error
	ipfsnode.Start(ctx)

	node := ipfsnode.IpfsNode

	info := new(IdOutput)
	info.ID = node.Identity.String()

	pk := node.PrivateKey.GetPublic()
	pkb, err := ic.MarshalPublicKey(pk)
	if err != nil {
		log.Println(err)
		return
	}
	info.PublicKey = base64.StdEncoding.EncodeToString(pkb)

	if node.PeerHost != nil {
		addrs, err := peer.AddrInfoToP2pAddrs(host.InfoFromHost(node.PeerHost))
		if err != nil {
			log.Println(err)
			return
		}
		for _, a := range addrs {
			info.Addresses = append(info.Addresses, a.String())
		}
		sort.Strings(info.Addresses)
		info.Protocols = node.PeerHost.Mux().Protocols()
		sort.Strings(info.Protocols)
	}
	info.ProtocolVersion = identify.DefaultProtocolVersion
	info.AgentVersion = version.GetUserAgentVersion()

	infoByte, _ := json.Marshal(info)
	fmt.Println(string(infoByte))

}
