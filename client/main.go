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

func flagReader() (*int, *string, *string, *string, *string, *string, *int) {
	UIPort := flag.Int("UIPort", 8080, "UIPort")
	message := flag.String("msg", "", "gossipAddr")
	dest := flag.String("dest", "None", "Specific destination to send message to")
	filePath := flag.String("file", "", "File path")
	request := flag.String("request", "", "request a chunk or metafile of this hash")
	keywords := flag.String("keywords", "", "keywords to download file")
	budget := flag.Int("budget", -1, "budget to search for files")
	flag.Parse()

	/*fmt.Println("ui port ", *UIPort)
	fmt.Println("client message", *message)
	*/
	return UIPort, message, dest, filePath, request, keywords, budget
}
func main() {

	UIport, msg, dest, filePath, request, keywords, budget := flagReader()
	ip := "127.0.0.1:" + strconv.Itoa(*UIport)
	message := &Message{}

	//Bad combination of flags

	if (*msg != "") && (*request != "") {
		fmt.Errorf("ERROR (Bad argument combination)")
		fmt.Println("ERROR (Bad argument combination)")
		fmt.Println("1")
		os.Exit(1)
	}
	if (*dest != "None") && (*filePath != "") && (*request == "") {
		fmt.Errorf("ERROR (Bad argument combination)")
		fmt.Println("ERROR (Bad argument combination)")
		fmt.Println("2")
		os.Exit(1)
	}
	if (*msg != "") && (*filePath != "") {
		fmt.Errorf("ERROR (Bad argument combination)")
		fmt.Println("ERROR (Bad argument combination)")
		fmt.Println("3")
		os.Exit(1)
	}
	if ((*dest != "None") && (*filePath == "") && (*msg == "")) || ((*dest != "None") && (*msg != "") && (*filePath != "")) {
		fmt.Errorf("ERROR (Bad argument combination)")
		fmt.Println("ERROR (Bad argument combination)")
		fmt.Println("4")
		os.Exit(1)
	}
	/*if (*dest == "") && (*filePath == "") && (*request == "") && (*msg == "") {
		fmt.Errorf("ERROR (Bad argument combination)")
		fmt.Println("ERROR (Bad argument combination)")
		fmt.Println("5")
		os.Exit(1)
	}*/
	if (*request != "") && (*filePath == "") {
		fmt.Errorf("ERROR (Bad argument combination)")
		fmt.Println("ERROR (Bad argument combination)")
		fmt.Println("6")
		os.Exit(1)
	}
	if (len(*request) != 64) && (*request != "") {
		fmt.Errorf("ERROR (Unable to decode hex hash)")
		fmt.Println("ERROR (Unable to decode hex hash)")
		fmt.Println("7")
		//TODO: Return 1 or the os.Exit?
		os.Exit(1)
	}
	//If private message
	if (*dest != "None") && (*filePath == "") && (*request == "") {
		message = &Message{
			Text:        *msg,
			Destination: dest,
		}
		//Search message
	} else if *keywords != "" {
		if *budget == -1 {
			message = &Message{
				Keywords: keywords,
			}
		} else {
			budgetAux := uint64(*budget)
			message = &Message{
				Keywords: keywords,
				Budget:   &budgetAux,
			}
		}
	} else if *filePath != "" && *request == "" {
		message = &Message{
			File: filePath,
		}

		//File request message
	} else if *request != "" {
		//fmt.Println(*request)
		requestBytes, _ := hex.DecodeString(*request)
		//fmt.Printf("%x\n", requestBytes)
		if *dest == "None" {
			//file request after search
			message = &Message{
				Text:    *msg,
				File:    filePath,
				Request: &requestBytes,
			}
		} else {
			//Normal file request
			message = &Message{
				Text:        *msg,
				Destination: dest,
				File:        filePath,
				Request:     &requestBytes,
			}
		}

		//fmt.Println(hex.EncodeToString(requestBytes))
		//fmt.Println(string(*message.Request))
		//Share file
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
