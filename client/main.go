package main

import (
	"flag"
	"net"
	"strconv"

	"github.com/dedis/protobuf"
	. "github.com/mecanicus/Peerster/types"
)

func flagReader() (*int, *string) {
	UIPort := flag.Int("UIPort", 8080, "UIPort")
	message := flag.String("msg", "127.0.0.1:5000", "gossipAddr")
	flag.Parse()

	/*fmt.Println("ui port ", *UIPort)
	fmt.Println("client message", *message)
	*/
	return UIPort, message
}
func main() {

	UIport, msg := flagReader()
	ip := "127.0.0.1:" + strconv.Itoa(*UIport)

	message := &Message{
		Text: *msg,
	}
	packetToSend := message
	packetBytes, _ := protobuf.Encode(packetToSend)
	conn, err := net.Dial("udp", ip)
	_, err = conn.Write(packetBytes)
	if err != nil {
		panic(err)
	}
}
