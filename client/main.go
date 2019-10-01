package main

import (
	"flag"
	"net"
	"strconv"
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
	//ip2 := "127.0.0.1:1000"
	conn, err := net.Dial("udp", ip)
	//packetBytes, err := packetToSend
	//fmt.Println(strconv.Itoa(*UIport))
	_, err = conn.Write([]byte(*msg))
	if err != nil {
		panic(err)
	}

	//packetToSend := GossipPacket{Simple: simplemessage}

}
