package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/dedis/protobuf"
	. "github.com/mecanicus/Peerster/types"
)

func flagReader() (*int, *string, *string, *string, *string) {
	UIPort := flag.Int("UIPort", 8080, "UIPort")
	message := flag.String("msg", "", "gossipAddr")
	dest := flag.String("dest", "", "Specific destination to send message to")
	filePath := flag.String("file", "", "File path")
	request := flag.String("request", "", "request a chunk or metafile of this hash")
	flag.Parse()

	/*fmt.Println("ui port ", *UIPort)
	fmt.Println("client message", *message)
	*/
	return UIPort, message, dest, filePath, request
}
func main() {

	UIport, msg, dest, filePath, request := flagReader()
	ip := "127.0.0.1:" + strconv.Itoa(*UIport)
	message := &Message{}

	//Bad combination of flags

	if (*msg != "") && (*request != "") {
		fmt.Errorf("ERROR (Bad argument combination)")

	}
	if (len(*request) != 64) && (*request != "") {
		fmt.Errorf("ERROR (Unable to decode hex hash)")
		//TODO: Return 1 or the os.Exit?
		os.Exit(1)
	}
	//If private message
	if (*dest != "") && (*filePath == "") && (*request == "") {
		message = &Message{
			Text:        *msg,
			Destination: dest,
		}
		//File request message
	} else if *request != "" {
		//fmt.Println(*request)
		requestBytes, _ := hex.DecodeString(*request)
		//fmt.Printf("%x\n", requestBytes)
		message = &Message{
			Text:        *msg,
			Destination: dest,
			File:        filePath,
			Request:     &requestBytes,
		}
		fmt.Printf("%x\n", requestBytes)
		//fmt.Println(string(*message.Request))
		//Share file
	} else if *filePath != "" {
		message = &Message{
			File: filePath,
		}
		//Normal message
	} else {
		message = &Message{
			Text: *msg,
		}
	}
	packetToSend := message
	//fmt.Println(*packetToSend.Request)
	packetBytes, _ := protobuf.Encode(packetToSend)
	conn, err := net.Dial("udp", ip)
	_, err = conn.Write(packetBytes)
	if err != nil {
		panic(err)
	}
}
