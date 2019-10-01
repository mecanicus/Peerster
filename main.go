package main

import (
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/dedis/protobuf"
)

type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}
type Gossiper struct {
	UIPort     int
	Addr       string
	Name       string
	KnownPeers []string
	Mode       bool
}
type GossiperSocket struct {
	address *net.UDPAddr
	conn    *net.UDPConn
}

type GossipPacket struct {
	Simple *SimpleMessage
}

type UDPAddr struct {
	IP   net.IP
	Port int
	Zone string // IPv6 scoped addressing zone
}

//TODO: Add a list of the gossipers known
func flagReader() (*string, *string, *bool, *int, *string) {
	uiPort := flag.Int("UIPort", 8080, "UIPort")
	gossipAddr := flag.String("gossipAddr", "127.0.0.1:5000", "gossipAddr")
	gossiperName := flag.String("name", "GossipeName", "name")
	peersList := flag.String("peers", " ", "peers list")
	gossiperMode := flag.Bool("simple", true, "mode to run")
	flag.Parse()

	/*fmt.Println("ui port ", *uiPort)
	fmt.Println("gossiper address", *gossipAddr)
	fmt.Println("gossiper name ", *gossiperName)
	fmt.Println("peersList ", *peersList)
	fmt.Println("gossiper Mode ", *gossiperMode)
	*/
	return gossipAddr, gossiperName, gossiperMode, uiPort, peersList
}

func gossiperUISocketOpen(address string, UIPort int) *GossiperSocket {

	splitterAux := strings.Split(address, ":")
	UIAddress := splitterAux[0] + ":" + strconv.Itoa(UIPort)

	udpAddr, err := net.ResolveUDPAddr("udp4", UIAddress)
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		panic(err)
	}
	return &GossiperSocket{
		address: udpAddr,
		conn:    udpConn,
	}

}
func gossiperSocketOpen(address string) *GossiperSocket {
	//fmt.Println("address: " + address)
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		panic(err)
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		panic(err)
	}

	return &GossiperSocket{
		address: udpAddr,
		conn:    udpConn,
	}

}
func listenSocket(socket *GossiperSocket, gossiper *Gossiper) {
	buf := make([]byte, 2000)
	for {
		_, _, err := socket.conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}
		// Do stuff with the read bytes
		packet := &GossipPacket{}
		protobuf.Decode(buf, packet)
		message := packet.Simple
		fmt.Println("SIMPLE MESSAGE origin " + message.OriginalName + " from " + message.RelayPeerAddr + " contents " + message.Contents)

		if checkPeersList(message, gossiper) == false {
			addPeerToList(message, gossiper)
		}
		sendToPeersComingFromPeer(message, gossiper)
	}
}
func listenUISocket(UISocket *GossiperSocket, gossiper *Gossiper) {
	var buf [2000]byte
	for {
		//print("Hello")

		rlen, _, err := UISocket.conn.ReadFromUDP(buf[:])

		if err != nil {
			panic(err)
		}

		// Do stuff with the read bytes
		clientMessage := string(buf[0:rlen])
		message := SimpleMessage{
			OriginalName:  gossiper.Name,
			RelayPeerAddr: gossiper.Addr,
			Contents:      clientMessage,
		}
		fmt.Println("CLIENT MESSAGE " + message.Contents)
		sendToPeersComingFromClient(&message, gossiper)
	}
}
func checkPeersList(message *SimpleMessage, gossiper *Gossiper) bool {
	peerAddress := message.RelayPeerAddr
	for _, peerAddressExaminated := range gossiper.KnownPeers {
		if peerAddress == peerAddressExaminated {
			return true
		}
	}
	return false
}
func addPeerToList(message *SimpleMessage, gossiper *Gossiper) {
	peerAddress := message.RelayPeerAddr
	gossiper.KnownPeers = append(gossiper.KnownPeers, peerAddress)
}
func sendToPeersComingFromClient(message *SimpleMessage, gossiper *Gossiper) {
	packetToSend := &GossipPacket{Simple: message}
	fmt.Print("PEERS ")
	packetBytes, _ := protobuf.Encode(packetToSend)
	for index, peerAddress := range gossiper.KnownPeers {
		conn, err := net.Dial("udp", peerAddress)
		/*fmt.Print("Paquete contenidos: ")
		fmt.Println(packetToSend.Simple.OriginalName)*/
		/*fmt.Println(packetBytes)
		fmt.Print("Paquete: ")
		fmt.Println(packetToSend)*/
		//fmt.Println(packetToSend.Simple)
		_, err = conn.Write(packetBytes)
		if err != nil {
			panic(err)
		}
		if index == (len(gossiper.KnownPeers) - 1) {
			fmt.Println(peerAddress)
			continue
		}
		fmt.Print(peerAddress + ",")
	}
}
func sendToPeersComingFromPeer(message *SimpleMessage, gossiper *Gossiper) {
	originalRelay := message.RelayPeerAddr
	message.RelayPeerAddr = gossiper.Addr
	packetToSend := &GossipPacket{Simple: message}
	fmt.Print("PEERS ")
	for index, peerAddress := range gossiper.KnownPeers {
		if peerAddress == originalRelay {
			if index == (len(gossiper.KnownPeers) - 1) {
				fmt.Println(peerAddress)
				continue
			}
			fmt.Print(peerAddress + ",")
			continue
		} else {
			//TODO: send packet
			conn, err := net.Dial("udp", peerAddress)
			packetBytes, err := protobuf.Encode(packetToSend)
			_, err = conn.Write(packetBytes)
			if err != nil {
				panic(err)
			}
		}
		if index == (len(gossiper.KnownPeers) - 1) {
			fmt.Println(peerAddress)
			continue
		}
		fmt.Print(peerAddress + ",")
	}
}
func createNewGossiper(gossipAddr *string, gossiperName *string, gossiperMode *bool, uiPort *int, peerList *string) *Gossiper {

	splitterAux := strings.Split(*peerList, ",")
	return &Gossiper{
		Addr:       *gossipAddr,
		UIPort:     *uiPort,
		Name:       *gossiperName,
		KnownPeers: splitterAux,
		Mode:       *gossiperMode,
	}
}
func main() {

	gossiper := createNewGossiper(flagReader())
	UIsocket := gossiperUISocketOpen(gossiper.Addr, gossiper.UIPort)
	go listenUISocket(UIsocket, gossiper)
	socket := gossiperSocketOpen(gossiper.Addr)
	listenSocket(socket, gossiper)

	//packetToSend := GossipPacket{Simple: simplemessage}
}
