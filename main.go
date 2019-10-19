package main

//TODO: EN MODO SIMPLE NO SE GUARDAN BIEN LOS PEERS
//TODO: HAY UN PROBLEMA CUANDO ESTAS IN SYNC PERO TE HABLA ALGUIEN CON QUIEN NO TIENES SESION
import (
	"flag"
	"fmt"
	"math/rand"
	"net"

	"strconv"
	"strings"
	"time"

	"github.com/dedis/protobuf"
	. "github.com/mecanicus/Peerster/types"
)

type Gossiper struct {
	UIPort             int
	Addr               string
	Name               string
	KnownPeers         []string
	Mode               bool
	Want               []PeerStatus
	SavedMessages      map[string][]RumorMessage
	TalkingPeers       map[string]ConectionInfo
	Socket             *GossiperSocket
	AntiEntropyTimeout int
}

func flagReader() (*string, *string, *bool, *int, *string, *int) {
	uiPort := flag.Int("UIPort", 8080, "UIPort")
	gossipAddr := flag.String("gossipAddr", "127.0.0.1:5000", "gossipAddr")
	gossiperName := flag.String("name", "GossiperName", "name")
	peersList := flag.String("peers", " ", "peers list")
	gossiperMode := flag.Bool("simple", false, "mode to run")
	antiEntropyTimeout := flag.Int("antiEntropy", 10, "Anti entropy timeout")
	flag.Parse()
	return gossipAddr, gossiperName, gossiperMode, uiPort, peersList, antiEntropyTimeout
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
		Address: udpAddr,
		Conn:    udpConn,
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
		Address: udpAddr,
		Conn:    udpConn,
	}

}
func listenSocket(socket *GossiperSocket, gossiper *Gossiper) {
	buf := make([]byte, 2000)
	for {
		_, _, err := socket.Conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}
		// Do stuff with the read bytes
		packet := &GossipPacket{}
		protobuf.Decode(buf, packet)
		message := packet.Simple
		fmt.Println("SIMPLE MESSAGE origin " + message.OriginalName + " from " + message.RelayPeerAddr + " contents " + message.Contents)

		if checkPeersListSimple(message.RelayPeerAddr, gossiper) == false {
			addPeerToListSimple(message.RelayPeerAddr, gossiper)
		}
		sendToPeersComingFromPeer(message, gossiper)
	}
}

func listenUISocket(UISocket *GossiperSocket, gossiper *Gossiper) {
	buf := make([]byte, 2000)
	for {
		//print("Hello")

		_, _, err := UISocket.Conn.ReadFromUDP(buf)

		if err != nil {
			panic(err)
		}
		packet := &Message{}
		protobuf.Decode(buf, packet)
		//TODO: Salvar el mensaje y incrementar el ID

		message := SimpleMessage{
			OriginalName:  gossiper.Name,
			RelayPeerAddr: gossiper.Addr,
			Contents:      packet.Text,
		}
		fmt.Println("CLIENT MESSAGE " + message.Contents)
		sendToPeersComingFromClient(&message, gossiper)
	}
}
func listenSocketNotSimple(socket *GossiperSocket, gossiper *Gossiper) {
	buf := make([]byte, 2000)
	for {
		_, sender, err := socket.Conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}
		// Do stuff with the read bytes
		packet := &GossipPacket{}
		protobuf.Decode(buf, packet)
		message := packet.Rumor
		peerTalking := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
		talkingPeersMap := gossiper.TalkingPeers
		_, exists := talkingPeersMap[peerTalking]
		//Anadir a la lista de peers si necesario
		if checkPeersList(sender, gossiper) == false {
			addPeerToList(sender, gossiper)
		}
		printKnownPeers(gossiper)
		//Es un mensaje de status
		if message == nil {
			message := packet.Status

			//Es un mensaje de status de una conexion abierta
			if exists == true {
				connectionRenewal(peerTalking, gossiper)
				statusDecisionMaking(sender, message, gossiper)
				//Reiniciar el timeout

				continue

				//Es un mensaje de status de alguien nuevo, probablemente algoritmo anti entrophy
			} else {
				statusDecisionMaking(sender, message, gossiper)
				//TODO: Abrir conexion, probablemente sea del algoritmo antientropia? REVISAR
				//connectionCreationStatusMessage(peerTalking, gossiper)
				continue
			}
			//Es un mensaje de rumor
		} else {
			fmt.Println("RUMOR origin " + message.Origin + " from " + peerTalking + " ID " + fmt.Sprint(message.ID) + " contents " + message.Text)
			//El paquete es un rumor
			//AHORA ESTA SEPARADO EN DOS CASOS, AHORA MISMO NO HACE FALTA PORQUE SON IGUALES, A LO MEJOR ES UTIL EN EL FUTURO
			//Tenemos una conexion abierta con el
			if exists == true {
				if checkingIfExpectedMessageAndSave(message, gossiper) == true {
					//Empezar rumormorgering process con otro peer
					choosenPeer := choseRandomPeerAndSendRumorPackage(message, gossiper)
					connectionCreationRumorMessage(choosenPeer, message, gossiper)

					//Mandar un mensaje de status de vuelta (todo ok)
					createNewStatusPackageAndSend(sender, gossiper)
					continue

				} else {
					//El paquete no es nuevo o es muy nuevo, mandar status de vuelta
					//No parece necesario renovar la conexion
					//connectionRenewal(peerTalking, gossiper)
					createNewStatusPackageAndSend(sender, gossiper)
					continue
				}

				//No tenemos una conexion abierta con el
			} else {
				if checkingIfExpectedMessageAndSave(message, gossiper) == true {
					//Empezar rumormorgering process con otro peer

					choosenPeer := choseRandomPeerAndSendRumorPackage(message, gossiper)
					connectionCreationRumorMessage(choosenPeer, message, gossiper)

					//Mandar un mensaje de status de vuelta (todo ok)
					createNewStatusPackageAndSend(sender, gossiper)
					continue

				} else {
					//El paquete no es nuevo o es muy nuevo, mandar status de vuelta
					createNewStatusPackageAndSend(sender, gossiper)
					continue
				}
			}
		}

	}
}
func connectionCreationRumorMessage(sender string, message *RumorMessage, gossiper *Gossiper) {

	talkingPeersMap := gossiper.TalkingPeers
	talkingPeersMap[sender] = ConectionInfo{
		MessageToGossip: message,
		Timeout:         10,
	}

	gossiper.TalkingPeers = talkingPeersMap
}
func connectionRenewal(sender string, gossiper *Gossiper) {
	_, ok := gossiper.TalkingPeers[sender]
	if ok {
		talkingPeersMap := gossiper.TalkingPeers
		talkingPeersMap[sender] = ConectionInfo{
			MessageToGossip: gossiper.TalkingPeers[sender].MessageToGossip,
			Timeout:         10,
		}
	} else {
		return
	}

}

func listenUISocketNotSimple(UISocket *GossiperSocket, gossiper *Gossiper) {

	buf := make([]byte, 2000)
	for {
		//print("Hello")

		_, _, err := UISocket.Conn.ReadFromUDP(buf)

		if err != nil {
			panic(err)
		}
		packet := &Message{}
		protobuf.Decode(buf, packet)
		//TODO: Salvar el mensaje y incrementar el ID

		IDMessage := gossiper.Want[0].NextID
		message := RumorMessage{
			Origin: gossiper.Name,
			ID:     IDMessage,
			Text:   packet.Text,
		}
		gossiper.Want[0].NextID = gossiper.Want[0].NextID + 1
		messaggesOfClient := gossiper.SavedMessages[gossiper.Name]
		messaggesOfClient = append(messaggesOfClient, message)
		gossiper.SavedMessages[gossiper.Name] = messaggesOfClient

		fmt.Println("CLIENT MESSAGE " + message.Text)
		sendToPeersComingFromClientNotSimple(&message, gossiper)
	}
}
func checkPeersList(sender *net.UDPAddr, gossiper *Gossiper) bool {
	peerAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
	for _, peerAddressExaminated := range gossiper.KnownPeers {
		if peerAddress == peerAddressExaminated {
			return true
		}
	}
	return false
}
func checkPeersListSimple(relayAddress string, gossiper *Gossiper) bool {
	peerAddress := relayAddress
	for _, peerAddressExaminated := range gossiper.KnownPeers {
		if peerAddress == peerAddressExaminated {
			return true
		}
	}
	return false
}
func addPeerToListSimple(relayAddress string, gossiper *Gossiper) {
	peerAddress := relayAddress
	gossiper.KnownPeers = append(gossiper.KnownPeers, peerAddress)
}
func addPeerToList(sender *net.UDPAddr, gossiper *Gossiper) {
	peerAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
	gossiper.KnownPeers = append(gossiper.KnownPeers, peerAddress)
}
func sendToPeersComingFromClient(message *SimpleMessage, gossiper *Gossiper) {
	packetToSend := &GossipPacket{Simple: message}
	fmt.Print("PEERS ")
	packetBytes, _ := protobuf.Encode(packetToSend)
	for index, peerAddress := range gossiper.KnownPeers {
		choosenPeerAddress, _ := net.ResolveUDPAddr("udp", peerAddress)
		gossiper.Socket.Conn.WriteToUDP(packetBytes, choosenPeerAddress)
		if index == (len(gossiper.KnownPeers) - 1) {
			fmt.Println(peerAddress)
			continue
		}
		fmt.Print(peerAddress + ",")
	}
}
func printKnownPeers(gossiper *Gossiper) {
	fmt.Print("PEERS ")
	for index, peerAddress := range gossiper.KnownPeers {
		if index == (len(gossiper.KnownPeers) - 1) {
			fmt.Println(peerAddress)
			continue
		}
		//Just in case we don´t have peers
		if peerAddress != "" {
			fmt.Print(peerAddress + ",")
		}
	}
}
func sendToPeersComingFromClientNotSimple(message *RumorMessage, gossiper *Gossiper) {

	choosenPeer := choseRandomPeerAndSendRumorPackage(message, gossiper)
	connectionCreationRumorMessage(choosenPeer, message, gossiper)

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
			if peerAddress != "" {
				fmt.Print(peerAddress + ",")
				continue
			}
		} else {

			packetBytes, _ := protobuf.Encode(packetToSend)
			choosenPeerAddress, _ := net.ResolveUDPAddr("udp", peerAddress)
			gossiper.Socket.Conn.WriteToUDP(packetBytes, choosenPeerAddress)
		}
		if index == (len(gossiper.KnownPeers) - 1) {
			fmt.Println(peerAddress)
			continue
		}
		fmt.Print(peerAddress + ",")
	}
}
func checkingIfExpectedMessageAndSave(message *RumorMessage, gossiper *Gossiper) bool {
	origin := message.Origin
	ID := message.ID
	check := true
	for _, peerStatusExaminated := range gossiper.Want {

		if peerStatusExaminated.Identifier == origin {
			//println("matched on:" + origin)
			//fmt.Println(gossiper.Want)
			check = false
		}
	}
	if check == true {
		wantInfo := PeerStatus{
			Identifier: origin,
			NextID:     1,
		}
		gossiper.Want = append(gossiper.Want, wantInfo)
	}
	for index, peerStatusExaminated := range gossiper.Want {
		if origin == peerStatusExaminated.Identifier {
			if ID == (peerStatusExaminated.NextID) {
				//Save the new index and save the package
				messaggesOfOrigin := gossiper.SavedMessages[origin]
				//Es importante que los guarde en orden de llegada, para sacarlos por orden tmb
				messaggesOfOrigin = append(messaggesOfOrigin, *message)
				gossiper.SavedMessages[origin] = messaggesOfOrigin
				gossiper.Want[index].Identifier = origin
				gossiper.Want[index].NextID = ID + 1
				return true
			}
		}
	}
	return false

}
func createNewStatusPackageAndSend(sender *net.UDPAddr, gossiper *Gossiper) {
	statusPacket := &StatusPacket{Want: gossiper.Want}
	packetToSend := &GossipPacket{Status: statusPacket}
	//senderAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
	//conn, err := net.Dial("udp", senderAddress)
	//_, err = conn.Write(packetBytes)
	packetBytes, err := protobuf.Encode(packetToSend)
	if err != nil {
		panic(err)
	}
	gossiper.Socket.Conn.WriteToUDP(packetBytes, sender)
}
func sendRumorPackage(rumorMessage *RumorMessage, sender *net.UDPAddr, gossiper *Gossiper) {
	packetToSend := &GossipPacket{Rumor: rumorMessage}
	packetBytes, err := protobuf.Encode(packetToSend)
	senderAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
	fmt.Println("MONGERING with " + senderAddress)
	gossiper.Socket.Conn.WriteToUDP(packetBytes, sender)
	if err != nil {
		panic(err)
	}
}
func statusDecisionMaking(sender *net.UDPAddr, statusMessage *StatusPacket, gossiper *Gossiper) {
	senderAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
	stringToSend := ""
	for _, peerWantStatus := range statusMessage.Want {
		stringToSend = stringToSend + " peer " + peerWantStatus.Identifier + " nextID " + fmt.Sprint(peerWantStatus.NextID)
	}
	fmt.Println("STATUS from " + senderAddress + stringToSend)

	for _, peerStatusExaminated := range gossiper.Want {
		newPeer := true
		for _, peerStatusExaminatedOfOtherPeer := range statusMessage.Want {
			if peerStatusExaminated.Identifier == peerStatusExaminatedOfOtherPeer.Identifier {
				newPeer = false
				//Estamos mirando la misma persona tanto en memoria local como en el otro peer
				if peerStatusExaminated.NextID == (peerStatusExaminatedOfOtherPeer.NextID) {
					continue
					//Tenemos los mismos mensajes EN LA PERSONA QUE ESTAMOS MIRANDO

				} else if peerStatusExaminated.NextID > peerStatusExaminatedOfOtherPeer.NextID {
					//Tenemos mas mensajes que el otro, es decir podemos enviar mas, mandar rumour package
					for _, rumorMesagesOfAPeer := range gossiper.SavedMessages[peerStatusExaminated.Identifier] {
						if rumorMesagesOfAPeer.ID == (peerStatusExaminatedOfOtherPeer.NextID) {

							//Se busca el mensaje que el otro no tiene y se le manda
							rumorMessage := rumorMesagesOfAPeer
							sendRumorPackage(&rumorMessage, sender, gossiper)
							return
						}
					}
				}
			}
		}
		if newPeer {
			//El otro no tiene conocimiento de este hombre, le mando el primer mensaje de el
			if peerStatusExaminated.NextID != 1 {
				rumorMessage := gossiper.SavedMessages[peerStatusExaminated.Identifier][0]
				sendRumorPackage(&rumorMessage, sender, gossiper)
				return
			}

		}
	}
	//Si llegamos aqui es porque no tenemos ningun mensaje mas que el otro
	for _, peerStatusExaminated := range gossiper.Want {
		for _, peerStatusExaminatedOfOtherPeer := range statusMessage.Want {
			if peerStatusExaminated.Identifier == peerStatusExaminatedOfOtherPeer.Identifier {
				//Estamos mirando la misma persona tanto en memoria local como en el otro peer
				if peerStatusExaminated.NextID == (peerStatusExaminatedOfOtherPeer.NextID) {
					continue
					//Tenemos los mismos mensajes EN LA PERSONA QUE ESTAMOS MIRANDO
				} else if peerStatusExaminated.NextID < peerStatusExaminatedOfOtherPeer.NextID {
					//Tenemos menos mensajes que el otro, tenemos que pedirle, mandar status package
					createNewStatusPackageAndSend(sender, gossiper)
					return
				}
			}
		}
	}
	fmt.Println("IN SYNC WITH " + senderAddress)
	//Si hemos llegado aqui es porque tenemos todo en igualdad de condiciones a si que hay que tirar moneda

	result := rand.Int()
	if (result % 2) == 0 {
		//Cerrar el objeto conexion con quien estamos hablando y abrir una nueva con el afortunado
		senderAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)

		//Aqui se deberia entrar siempre excepto si es el caso de antientropia
		//Que te escriben un status packet y teneis los mismos paquetes
		//O si alguien te escribe y no te vale ningun paquete y tu le das tuyos
		_, ok := gossiper.TalkingPeers[senderAddress]
		if ok {
			//Recuperamos el mensaje
			messageInClosingSesion := gossiper.TalkingPeers[senderAddress].MessageToGossip
			//Cerramos la sesion
			delete(gossiper.TalkingPeers, senderAddress)
			//Elegimos nuevo peer y le mandamos la sesion
			choosenPeer := choseRandomPeerAndSendRumorPackage(messageInClosingSesion, gossiper)
			connectionCreationRumorMessage(choosenPeer, messageInClosingSesion, gossiper)
			fmt.Println("FLIPPED COIN sending rumor to " + choosenPeer)
		}

	} else {
		//Borramos la conexion y nos quedamos quietos
		senderAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
		delete(gossiper.TalkingPeers, senderAddress)
	}
}

func createNewGossiper(gossipAddr *string, gossiperName *string, gossiperMode *bool, uiPort *int, peerList *string, antiEntropyTimeout *int) *Gossiper {

	splitterAux := strings.Split(*peerList, ",")
	wantList := make([]PeerStatus, 1)
	wantList[0] = PeerStatus{
		Identifier: *gossiperName,
		NextID:     1,
	}
	return &Gossiper{
		Addr:               *gossipAddr,
		UIPort:             *uiPort,
		Name:               *gossiperName,
		KnownPeers:         splitterAux,
		Mode:               *gossiperMode,
		Want:               wantList,
		SavedMessages:      make(map[string][]RumorMessage),
		TalkingPeers:       make(map[string]ConectionInfo),
		AntiEntropyTimeout: *antiEntropyTimeout,
	}
}
func antiEntropy(gossiper *Gossiper) {
	ticker := time.NewTicker(time.Duration(gossiper.AntiEntropyTimeout) * time.Second)
	for range ticker.C {
		choseRandomPeerAndSendStatusPackage(gossiper)
	}
}
func choseRandomPeerAndSendRumorPackage(message *RumorMessage, gossiper *Gossiper) string {
	//rand.Seed(time.Now().Unix())
	randomIndexOfPeer := rand.Int() % len(gossiper.KnownPeers)
	peerToSendRumor := gossiper.KnownPeers[randomIndexOfPeer]
	packetToSend := &GossipPacket{Rumor: message}
	packetBytes, _ := protobuf.Encode(packetToSend)
	choosenPeerAddress, _ := net.ResolveUDPAddr("udp", peerToSendRumor)
	gossiper.Socket.Conn.WriteToUDP(packetBytes, choosenPeerAddress)
	fmt.Println("MONGERING with " + peerToSendRumor)
	return peerToSendRumor
}
func choseRandomPeerAndSendStatusPackage(gossiper *Gossiper) {
	//rand.Seed(time.Now().Unix())
	randomIndexOfPeer := rand.Int() % len(gossiper.KnownPeers)
	peerToSendStatus := gossiper.KnownPeers[randomIndexOfPeer]
	addrPeerToSendStatus, _ := net.ResolveUDPAddr("udp4", peerToSendStatus)
	createNewStatusPackageAndSend(addrPeerToSendStatus, gossiper)
} /*
func timerCreation(ch <-chan bool) {

	timeout := make(chan bool, 1)
	go func() {
		time.Sleep(10 * time.Second)
		timeout <- true
	}()
	select {
	case <-ch:
		// Put another timer
		timerCreation(ch)

	case <-timeout:
		// Delete the connection

		return
	}
	return

}*/
func timeoutChecker(gossiper *Gossiper) {
	ticker := time.NewTicker(1 * time.Second)

	for range ticker.C {

		for key, activePeerAnalyzed := range gossiper.TalkingPeers {
			if activePeerAnalyzed.Timeout <= 0 {
				delete(gossiper.TalkingPeers, key)
			} else {
				gossiper.TalkingPeers[key] = ConectionInfo{
					MessageToGossip: gossiper.TalkingPeers[key].MessageToGossip,
					Timeout:         gossiper.TalkingPeers[key].Timeout - 1,
				}
			}
		}
	}

}
func main() {

	gossiper := createNewGossiper(flagReader())
	rand.Seed(time.Now().UTC().UnixNano())
	UIsocket := gossiperUISocketOpen(gossiper.Addr, gossiper.UIPort)
	if gossiper.Mode == true {
		go listenUISocket(UIsocket, gossiper)
	} else {
		go timeoutChecker(gossiper)
		go listenUISocketNotSimple(UIsocket, gossiper)
	}
	socket := gossiperSocketOpen(gossiper.Addr)
	gossiper.Socket = socket
	if gossiper.Mode == true {
		go listenAPISocket(gossiper)
		listenSocket(socket, gossiper)
	} else {
		if gossiper.AntiEntropyTimeout != 0 {
			go antiEntropy(gossiper)
		}
		go listenAPISocket(gossiper)
		listenSocketNotSimple(socket, gossiper)

	}
	//packetToSend := GossipPacket{Simple: simplemessage}
}
