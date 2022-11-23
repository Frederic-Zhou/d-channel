package test

import (
	"bufio"
	"context"
	"d-channel/ipfsnode"
	"log"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p/core/host"
)

func TestStream(t *testing.T) {

	ctx := context.Background()
	var err error

	_, node1, err := ipfsnode.Spawn(ctx, "./node1")
	if err != nil {
		panic(err)
	}
	_, node2, err := ipfsnode.Spawn(ctx, "./node2")
	if err != nil {
		panic(err)
	}

	host1 := node1.DHT.LAN.Host()

	host1.SetStreamHandler("/hello/1.0.0", func(s network.Stream) {
		log.Printf("/hello/1.0.0 stream created")
		err := readHelloProtocol(s)
		if err != nil {
			s.Reset()
		} else {
			s.Close()
		}
	})

	host2 := node2.DHT.LAN.Host()
	for {

		err = host2.Connect(context.Background(), *host.InfoFromHost(host1))
		if err != nil {
			log.Println(" Sending message...", err)
			return
		}

		stream, err := host2.NewStream(context.Background(), host1.ID(), "/hello/1.0.0")
		if err != nil {
			panic(err)
		}

		message := "Hello from Launchpad! \naaaa"
		log.Printf("Sending message...")
		_, err = stream.Write([]byte(message))
		if err != nil {
			panic(err)
		}

		time.Sleep(5 * time.Second)
	}

}

func readHelloProtocol(s network.Stream) error {
	// TO BE IMPLEMENTED: Read the stream and print its content
	buf := bufio.NewReader(s)
	message, err := buf.ReadString('\n')
	if err != nil {
		return err
	}

	connection := s.Conn()

	log.Printf("Message from '%s': %s", connection.RemotePeer().String(), message)
	return nil
}
