package main

//TODO: HAY UN PROBLEMA CUANDO ESTAS IN SYNC PERO TE HABLA ALGUIEN CON QUIEN NO TIENES SESION
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dedis/protobuf"
	. "github.com/mecanicus/Peerster/types"
)

type Gossiper struct {
	UIPort               int
	Addr                 string
	Name                 string
	KnownPeers           []string
	Mode                 bool
	Want                 []PeerStatus
	SavedMessages        map[string][]RumorMessage
	TalkingPeers         map[string]ConectionInfo
	RoutingTable         map[string]string
	StoredFiles          map[string]*FileInfo //The key is the file name
	FilesBeingDownloaded []*DownloadInfo
	Socket               *GossiperSocket
	Rtimer               int
	AntiEntropyTimeout   int
}

func flagReader() (*string, *string, *bool, *int, *string, *int, *int) {
	uiPort := flag.Int("UIPort", 8080, "UIPort")
	gossipAddr := flag.String("gossipAddr", "127.0.0.1:5000", "gossipAddr")
	gossiperName := flag.String("name", "GossiperName", "name")
	peersList := flag.String("peers", " ", "peers list")
	gossiperMode := flag.Bool("simple", false, "mode to run")
	antiEntropyTimeout := flag.Int("antiEntropy", 10, "Anti entropy timeout")
	rtimer := flag.Int("rtimer", 0, "Amount of time between route messages, in seconds")
	flag.Parse()
	return gossipAddr, gossiperName, gossiperMode, uiPort, peersList, antiEntropyTimeout, rtimer
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
	buf := make([]byte, 8300)
	for {
		_, sender, err := socket.Conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}

		// Do stuff with the read bytes
		packet := &GossipPacket{}
		protobuf.Decode(buf, packet)
		peerTalking := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
		talkingPeersMap := gossiper.TalkingPeers
		_, exists := talkingPeersMap[peerTalking]
		//Anadir a la lista de peers si necesario
		if checkPeersList(sender, gossiper) == false {
			addPeerToList(sender, gossiper)
		}
		printKnownPeers(gossiper)
		packetType := packetType(*packet)
		switch packetType {
		case Status:
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
		case Rumor:
			//Es un mensaje de rumor
			message := packet.Rumor
			//TODO: Do I have to print this line if is a route message?
			if message.Text != "" {
				fmt.Println("RUMOR origin " + message.Origin + " from " + peerTalking + " ID " + fmt.Sprint(message.ID) + " contents " + message.Text)
			}
			//El paquete es un rumor
			//AHORA ESTA SEPARADO EN DOS CASOS, AHORA MISMO NO HACE FALTA PORQUE SON IGUALES, A LO MEJOR ES UTIL EN EL FUTURO
			//Tenemos una conexion abierta con el
			if exists == true {
				if checkingIfExpectedMessageAndSave(message, gossiper) == true {
					//Empezar rumormorgering process con otro peer
					choosenPeer := choseRandomPeerAndSendRumorPackage(message, gossiper)
					nextHopManagement(message, peerTalking, gossiper)
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
					nextHopManagement(message, peerTalking, gossiper)
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
		case Private:
			//It is a private message
			message := packet.Private
			sendPrivateMessage(message, gossiper)
		case FileRequest:
			message := packet.DataRequest
			fileDataRequestManagement(message, gossiper)

		case FileReply:
			fmt.Println("OLAOLAOALOLA")
			message := packet.DataReply
			fileDataReplyManagement(message, gossiper)
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
func dataReplyCreationAndSend(message *DataRequest, gossiper *Gossiper) {
	//fmt.Printf("%+v\n", message)
	hashFileRequested := message.HashValue
	//fmt.Println(hex.EncodeToString(message.HashValue))
	dataReplyToSend := &DataReply{}
	//We are going to check all the hashes to see if we have it.
	for _, storedFile := range gossiper.StoredFiles {
		//fmt.Println("EMPEZAMOOOOOOOOOOOOOS")
		//Metafile is being requested
		storedMetahash := storedFile.MetaHash[:]
		//fmt.Println(string(storedMetahash))
		//fmt.Printf("%x\n", storedFile.MetaHash)
		//fmt.Println(hex.EncodeToString(storedMetahash))
		//fmt.Println("------------------------------")
		//fmt.Println(hex.EncodeToString(message.HashValue))
		//fmt.Println(string(hashFileRequested))
		//fmt.Printf("%x\n", hashFileRequested)
		//fmt.Println(hashFileRequested)
		if bytes.Equal(storedMetahash, hashFileRequested) {
			//fmt.Println("CCCCCCCCCCCCCCCCCCCCCC")
			fmt.Println("Enviando metafile: ", sha256.Sum256(storedFile.Metafile))
			dataReplyToSend = &DataReply{
				Origin:      gossiper.Name,
				Destination: message.Origin,
				HopLimit:    10,
				HashValue:   storedMetahash,
				Data:        storedFile.Metafile,
			}
			fmt.Printf("%+v\n", dataReplyToSend)
			break
		}
		//One chunk is being requested
		for hashOfTheChunk, dataOfTheChunk := range storedFile.HashesOfChunks {
			hashOfChunkAux := hashOfTheChunk[:]
			if bytes.Equal(hashOfChunkAux, hashFileRequested) {
				fmt.Println("Enviando chunk: ", sha256.Sum256(dataOfTheChunk))
				dataReplyToSend = &DataReply{
					Origin:      gossiper.Name,
					Destination: message.Origin,
					HopLimit:    10,
					HashValue:   hashOfChunkAux,
					Data:        dataOfTheChunk,
				}
				break
			}
		}

	}
	//Send the reply packet
	dataReplyToSend.HopLimit = message.HopLimit - 1
	//If the hop limit has been exceeded
	if message.HopLimit <= 0 {
		return
	}
	nextHop := gossiper.RoutingTable[dataReplyToSend.Destination]
	addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
	packetToSend := &GossipPacket{DataReply: dataReplyToSend}
	packetBytes, err := protobuf.Encode(packetToSend)
	if err != nil {
		panic(err)
	}
	gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
}
func fileDataRequestManagement(message *DataRequest, gossiper *Gossiper) {
	//We are the owners of the file being requested
	if message.Destination == gossiper.Name {
		dataReplyCreationAndSend(message, gossiper)

		//The file request is for someone else
	} else {
		sendDataRequest(message, gossiper)
	}
}
func sendDataRequest(message *DataRequest, gossiper *Gossiper) {
	message.HopLimit = message.HopLimit - 1
	//If the hop limit has been exceeded
	if message.HopLimit <= 0 {
		return
	}
	//If not send to next hop
	nextHop := gossiper.RoutingTable[message.Destination]
	addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
	packetToSend := &GossipPacket{DataRequest: message}
	packetBytes, err := protobuf.Encode(packetToSend)
	if err != nil {
		panic(err)
	}
	gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
	/*if len(gossiper.FilesBeingDownloaded) > 0 {
		for i := 0; i < len(gossiper.FilesBeingDownloaded[0].ChunkInformation); i++ {
			fmt.Println("cada saliente: ", sha256.Sum256(gossiper.FilesBeingDownloaded[0].ChunkInformation[i].ChunkData))
		}
	}*/
}
func fileDataReplyManagement(message *DataReply, gossiper *Gossiper) {

	//The reply is sent to us because we sent a request at some point
	if message.Destination == gossiper.Name {
		dataDownloadManagement(message, gossiper)

		//The file reply is for someone else, just route
	} else {
		message.HopLimit = message.HopLimit - 1
		//If the hop limit has been exceeded
		if message.HopLimit <= 0 {
			return
		}
		//If not send to next hop
		nextHop := gossiper.RoutingTable[message.Destination]
		addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
		packetToSend := &GossipPacket{DataReply: message}
		packetBytes, err := protobuf.Encode(packetToSend)
		if err != nil {
			panic(err)
		}
		gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
	}

}
func dataDownloadManagement(message *DataReply, gossiper *Gossiper) {
	//TODO: Leer sesion de download y ver el estado y ver que hacemos

	//If this is not done instead of saving the value it saves a reference
	data := make([]byte, len(message.Data))
	copy(data, message.Data)
	origin := message.Origin
	sha256fDataAux := sha256.Sum256(data)
	sha256fData := sha256fDataAux[:]
	//Find the session
	var session *DownloadInfo
	var index int
	for oneIndex, oneSession := range gossiper.FilesBeingDownloaded {
		if bytes.Equal(oneSession.LastHashRequested, message.HashValue) {
			session = oneSession
			index = oneIndex
			break
		}
	}
	sha256Expected := session.LastHashRequested
	//Check if the file has being received correctly or is not the one we are looking for
	if !bytes.Equal(sha256fData, sha256Expected) {

		//TODO: Do something if not
		return
	} else {
		//Check if the received hash is of the metahash
		//If it is the metahash
		if bytes.Equal(sha256fData, session.MetaHash) {
			//Save the info also to the gossiper struct
			stringListOfHashes := string(data)
			listOfHashes := strings.Split(stringListOfHashes, ",")
			//We have they keys of the map (hashes) but not the values
			var chunk ChunkStruct
			for _, hashString := range listOfHashes {
				chunk.ChunkHash, _ = hex.DecodeString(hashString)
				session.ChunkInformation = append(session.ChunkInformation, chunk)
			}
			//fmt.Printf("%+v\n", session.ChunkInformation)
			fmt.Println("Recibiendo metafile: ", sha256.Sum256(data))
			//We are going to ask for the first chunk
			//session.LastHashRequested = session.ChunkInformation[0].ChunkHash
			gossiper.FilesBeingDownloaded[index] = &DownloadInfo{
				PathToSave:        session.PathToSave,
				FileName:          session.FileName,
				Timeout:           5,
				Metafile:          data,
				MetaHash:          session.MetaHash,
				LastHashRequested: session.ChunkInformation[0].ChunkHash,
				ChunkInformation:  session.ChunkInformation,
			}
			dataRequest := &DataRequest{
				Destination: origin,
				HopLimit:    10,
				Origin:      gossiper.Name,
				HashValue:   session.ChunkInformation[0].ChunkHash,
			}
			//TODO: Update available files
			var metaHashAux [32]byte
			copy(metaHashAux[:], session.MetaHash)
			fileToStore := &FileInfo{
				MetaHash:       metaHashAux,
				Metafile:       data,
				HashesOfChunks: make(map[[32]byte][]byte),
			}
			gossiper.StoredFiles[session.FileName] = fileToStore
			sendDataRequest(dataRequest, gossiper)
			//Is the hash of a chunk of the file
		} else {

			var positionLastChunkReceived int
			//fmt.Println(len(session.ChunkInformation))
			//fmt.Println("Recibiendo chunk: ", &message, sha256.Sum256(Aux), sha256.Sum256(message.Data), message.Data[:10])
			//fmt.Println("------------------------")
			//TODO: EL PROBLEMA ESTA AQUI, NO SE COMO COJONES ESTA CAMBIANDO EL VALOR DE session.ChunkInformation[i].ChunkData CON EL MENSAJE ENTRANTE ANTES DE EJECUTAR LAS SIGUIENTES LINEAS
			for i := 0; i < len(session.ChunkInformation); i++ {
				fmt.Println("before: ", sha256.Sum256(session.ChunkInformation[i].ChunkData))
				//fmt.Println("memoria: ", &session.ChunkInformation[i])
			}
			for index2, oneChunk := range session.ChunkInformation {
				//If check to which chunk we are receiving the data
				if bytes.Equal(oneChunk.ChunkHash, sha256fData) {
					//oneChunk.ChunkData = message.Data
					fmt.Println("chunk analyzed", index2)

					session.ChunkInformation[index2].ChunkData = data
					//copy(session.ChunkInformation[index2].ChunkData, message.Data)

					//session.ChunkInformation = append(session.ChunkInformation, oneChunk)
					positionLastChunkReceived = index2
					fmt.Println("saving chunk: ", sha256.Sum256(session.ChunkInformation[index2].ChunkData))
					break
				}
			}
			for i := 0; i < len(session.ChunkInformation); i++ {
				fmt.Println("after: ", sha256.Sum256(session.ChunkInformation[i].ChunkData))
			}

			//Have to save the last info in case is the last chunk
			//fmt.Println(reflect.DeepEqual(session, gossiper.FilesBeingDownloaded[index]))
			gossiper.FilesBeingDownloaded[index] = session
			//TODO: El problema esta por aqui, sospecho que me estoy saliendo de la posicion del array
			fmt.Println("Index", index)
			fmt.Println("positionLastChunkReceived", positionLastChunkReceived)
			fmt.Println("len chunck info", len(session.ChunkInformation))
			//fmt.Println("Antes del if: ", sha256.Sum256(gossiper.FilesBeingDownloaded[index].ChunkInformation[0].ChunkData))

			//Check if we have files left to download
			//If we do
			if (positionLastChunkReceived+1 < len(session.ChunkInformation)) && (session.ChunkInformation[positionLastChunkReceived+1].ChunkData) == nil {
				//Request next chunk
				gossiper.FilesBeingDownloaded[index] = &DownloadInfo{
					PathToSave:        session.PathToSave,
					FileName:          session.FileName,
					Timeout:           5,
					Metafile:          session.Metafile,
					MetaHash:          session.MetaHash,
					LastHashRequested: session.ChunkInformation[positionLastChunkReceived+1].ChunkHash,
					ChunkInformation:  session.ChunkInformation,
				}
				//TODO: Creo que hasta aqui va perfecto
				//fmt.Println("Guardando recibido chunk en session: ", sha256.Sum256(session.ChunkInformation[5].ChunkData))
				//fmt.Println("Guardando recibido chunk en memoria final: ", sha256.Sum256(gossiper.FilesBeingDownloaded[index].ChunkInformation[5].ChunkData))

				//TODO: Send another request
				dataRequest := &DataRequest{
					Destination: origin,
					HopLimit:    10,
					Origin:      gossiper.Name,
					HashValue:   session.ChunkInformation[positionLastChunkReceived+1].ChunkHash,
				}
				/*for i := 0; i < len(session.ChunkInformation); i++ {
					fmt.Println("despues2: ", sha256.Sum256(session.ChunkInformation[i].ChunkData))
				}
				for i := 0; i < len(gossiper.FilesBeingDownloaded[index].ChunkInformation); i++ {
					fmt.Println("despues3 internal: ", sha256.Sum256(gossiper.FilesBeingDownloaded[index].ChunkInformation[i].ChunkData))
				}*/
				//TODO: Update available files
				var metaChunkAux [32]byte
				copy(metaChunkAux[:], message.HashValue)

				gossiper.StoredFiles[session.FileName].HashesOfChunks[metaChunkAux] = data
				sendDataRequest(dataRequest, gossiper)
			} else {
				//TODO: File downloaded, joint parts and close session
				fmt.Println("AAAAAAAAAAAAAAAAAAAAAADDDDDDDDDDDDAAAA")
				fileDownloadedSuccessfully(index, message, gossiper)
			}
		}

	}
}
func fileDownloadedSuccessfully(index int, message *DataReply, gossiper *Gossiper) {
	session := gossiper.FilesBeingDownloaded[index]

	fmt.Println("Guardando recibido chunk en memoria final sucessfully: ", sha256.Sum256(gossiper.FilesBeingDownloaded[index].ChunkInformation[5].ChunkData))

	//var hashValue32 [32]byte
	//copy(hashValue32[:], message.HashValue)
	pathToSaveFinalFile := session.PathToSave
	var finalFile []byte
	//TODO: Join the parts
	//finalFile, err := os.OpenFile(pathToSaveFinalFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	//TODO: AQUI YA ESTA MAL Y SE REPITE
	for i := 0; i < len(session.ChunkInformation); i++ {
		//Join all the data from the chunks and save it to finalFile
		//fmt.Printf("%x\n", session.ChunkInformation[i])
		fmt.Println("Guardando recibido chunk en internal: ", sha256.Sum256(session.ChunkInformation[i].ChunkData))
		chunkData2 := session.ChunkInformation[i].ChunkData
		finalFile = append(finalFile, chunkData2...)
	}
	//Save file in filesystem
	ioutil.WriteFile(pathToSaveFinalFile, finalFile, 0644)
	//TODO: Revisar si se puede coger el tamaño de archivo con len del array
	//fileInfoAux, _ := file.Stat()

	//Update available files
	var metaHashAux [32]byte
	copy(metaHashAux[:], session.MetaHash)
	fileToStore := &FileInfo{
		FileSize:       int64(len(finalFile)),
		MetaHash:       metaHashAux,
		Metafile:       session.Metafile,
		HashesOfChunks: make(map[[32]byte][]byte),
	}

	gossiper.StoredFiles[session.FileName] = fileToStore

	var chunkHashAux [32]byte
	for i := 0; i < len(session.ChunkInformation); i++ {
		chunkHash := session.ChunkInformation[i].ChunkHash
		chunkData := session.ChunkInformation[i].ChunkData
		copy(chunkHashAux[:], chunkHash)
		gossiper.StoredFiles[session.FileName].HashesOfChunks[chunkHashAux] = chunkData
	}
	//gossiper.StoredFiles[session.Metafile] = fileToStore

	//Close session
	gossiper.FilesBeingDownloaded[index] = gossiper.FilesBeingDownloaded[len(gossiper.FilesBeingDownloaded)-1]

}
func fileDownloadRequest(packet *Message, gossiper *Gossiper) {
	//TODO: Crear un request y mandarlo y abrir sesion de download
	message := &DataRequest{
		Origin:      gossiper.Name,
		Destination: *packet.Destination,
		HashValue:   *packet.Request,
		HopLimit:    10,
	}
	fmt.Println("--------------------------")
	fmt.Println(hex.EncodeToString(*packet.Request))
	fmt.Println(*packet.Request)
	fmt.Printf("%+v\n", message)
	message.HopLimit = message.HopLimit - 1
	//If the hop limit has been exceeded
	if message.HopLimit <= 0 {
		return
	}
	//Open sesion of download
	//var hashValue32 [32]byte
	//copy(hashValue32[:], message.HashValue)
	gossiper.FilesBeingDownloaded = append(gossiper.FilesBeingDownloaded, &DownloadInfo{
		FileName:          *packet.File,
		PathToSave:        filepath.Join(Downloads, *packet.File),
		Timeout:           5,
		LastHashRequested: message.HashValue,
		MetaHash:          message.HashValue,
	})
	//fmt.Println(string(gossiper.FilesBeingDownloaded[message.Destination].MetaHash))
	//If not send to next hop
	nextHop := gossiper.RoutingTable[message.Destination]
	addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
	packetToSend := &GossipPacket{DataRequest: message}
	packetBytes, err := protobuf.Encode(packetToSend)
	if err != nil {
		panic(err)
	}
	gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
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

		//Request file
		if packet.File != nil && packet.Destination != nil {

			fileDownloadRequest(packet, gossiper)
			continue
		}
		//Share file
		if packet.File != nil {
			fileToBeChunked := *packet.File
			fileIndexing(fileToBeChunked, gossiper)
			continue
		}

		//Private message
		if packet.Destination != nil {
			privateMessageCreation(packet, gossiper)
			continue
		}
		//Normal message
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
func fileIndexing(fileToBeChunked string, gossiper *Gossiper) {
	//fileToBeChunked := *packet.File
	var hashesOfChunks [][32]byte
	//pwd, _ := os.Getwd()
	//fmt.Println(pwd)
	file, err := os.Open(filepath.Join(SharedFiles, fileToBeChunked))
	//pathToStore := filepath.Join(SharedFiles, strings.Split(fileToBeChunked, ".")[0])
	//os.Mkdir("."+string(filepath.Separator)+pathToStore, 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer file.Close()
	fileInfo, _ := file.Stat()
	var fileSize int64 = fileInfo.Size()
	//8 KB of chunk sice
	const fileChunk = 1 * (1 << 13)
	//const fileChunk = 1 * (1 << 12)

	// calculate total number of parts the file will be chunked into
	totalPartsNum := uint64(math.Ceil(float64(fileSize) / float64(fileChunk)))

	fmt.Printf("Splitting to %d pieces.\n", totalPartsNum)
	//Save the size of the original file
	gossiper.StoredFiles[fileToBeChunked] = &FileInfo{}
	gossiper.StoredFiles[fileToBeChunked].HashesOfChunks = make(map[[32]byte][]byte)
	gossiper.StoredFiles[fileToBeChunked].FileSize = fileSize
	for i := uint64(0); i < totalPartsNum; i++ {

		partSize := int(math.Min(fileChunk, float64(fileSize-int64(i*fileChunk))))
		partBuffer := make([]byte, partSize)
		//Read the chunk
		file.Read(partBuffer)

		//fileName := fileToBeChunked + "_" + strconv.FormatUint(i, 10)
		//_, err := os.Create(fileName)

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("Guardando: ", sha256.Sum256(partBuffer))
		//We create a new stored file with key the name of the file and then fill the map of hashed

		gossiper.StoredFiles[fileToBeChunked].HashesOfChunks[sha256.Sum256(partBuffer)] = partBuffer

		//Also store the hash in an array to calculate the metahash without the need of looking at the dictionary of hashes
		hashesOfChunks = append(hashesOfChunks, sha256.Sum256(partBuffer))

	}
	calculateMetaHash(fileToBeChunked, hashesOfChunks, fileSize, gossiper)

}
func calculateMetaHash(fileToBeChunked string, hashesOfChunks [][32]byte, fileSize int64, gossiper *Gossiper) error {

	//filePathMetaHash := fileCommonBase + "_metahash"
	//f, err := os.Create(filePathMetaHash)

	//defer f.Close()
	var hashesOfChunksString []string

	//Here we create the content of the metafile
	for _, hashOfChunk := range hashesOfChunks {
		hashesOfChunksString = append(hashesOfChunksString, hex.EncodeToString(hashOfChunk[:]))
	}
	dataOfMetaHash := strings.Join(hashesOfChunksString, ",")
	dataOfMetahashBytes := []byte(dataOfMetaHash)
	gossiper.StoredFiles[fileToBeChunked].Metafile = dataOfMetahashBytes
	gossiper.StoredFiles[fileToBeChunked].MetaHash = sha256.Sum256(dataOfMetahashBytes)
	//fmt.Printf("%+v\n", gossiper.StoredFiles[fileToBeChunked])
	fmt.Println(hex.EncodeToString(gossiper.StoredFiles[fileToBeChunked].MetaHash[:]))

	return nil
}
func packetType(packet GossipPacket) int {

	if packet.Simple != nil {
		return Simple
	}
	if packet.Rumor != nil {
		return Rumor
	}
	if packet.Status != nil {
		return Status
	}
	if packet.Private != nil {
		return Private
	}
	if packet.DataReply != nil {
		return FileReply
	}
	if packet.DataRequest != nil {
		return FileRequest
	}
	return -1
}
func nextHopManagement(message *RumorMessage, peerTalking string, gossiper *Gossiper) {
	//TODO: Ask if we should update it if the ID is higher than expected, at the moment it does so
	//TODO: Ask what no output of DSDV messages mean, do we refresh the table without printing?
	origin := message.Origin
	// We take the last message of the peer and check the ID
	_, ok := gossiper.SavedMessages[origin]
	//If it doesn't exist (first message from him) we save it
	if !ok {
		gossiper.RoutingTable[origin] = peerTalking

		if message.Text != "" {
			fmt.Println("DSDV " + origin + " " + peerTalking)
		}
		return
		//If the received message is newer than the one we have

		//} else if message.ID > gossiper.SavedMessages[origin][len(gossiper.SavedMessages[origin])-1].ID {
	} else {
		gossiper.RoutingTable[origin] = peerTalking

		if message.Text != "" {
			fmt.Println("DSDV " + origin + " " + peerTalking)
		}
		return
	}
}
func privateMessageCreation(clientMessage *Message, gossiper *Gossiper) {
	privateMessage := &PrivateMessage{
		Origin:      gossiper.Name,
		ID:          0,
		Text:        clientMessage.Text,
		Destination: *clientMessage.Destination,
		HopLimit:    10,
	}

	sendPrivateMessage(privateMessage, gossiper)
}
func sendPrivateMessage(message *PrivateMessage, gossiper *Gossiper) {
	if message.Destination == gossiper.Name {
		fmt.Println("PRIVATE origin " + message.Origin + " hop-limit " + strconv.FormatUint(uint64(message.HopLimit), 10) + " contents " + message.Text)
		//The message is for someone else
	} else {
		message.HopLimit = message.HopLimit - 1
		//If the hop limit has been exceeded
		if message.HopLimit <= 0 {
			return
		}
		//If not send to next hop
		nextHop := gossiper.RoutingTable[message.Destination]
		addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
		packetToSend := &GossipPacket{Private: message}
		packetBytes, err := protobuf.Encode(packetToSend)
		if err != nil {
			panic(err)
		}
		gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
	}
}
func createAndSendNextHopMessage(gossiper *Gossiper) {
	IDMessage := gossiper.Want[0].NextID
	nextHopMessage := &RumorMessage{
		Origin: gossiper.Name,
		ID:     IDMessage,
		Text:   "",
	}
	gossiper.Want[0].NextID = gossiper.Want[0].NextID + 1
	gossiper.SavedMessages[gossiper.Name] = append(gossiper.SavedMessages[gossiper.Name], *nextHopMessage)
	choseRandomPeerAndSendRumorPackage(nextHopMessage, gossiper)
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

func createNewGossiper(gossipAddr *string, gossiperName *string, gossiperMode *bool, uiPort *int, peerList *string, antiEntropyTimeout *int, rtimer *int) *Gossiper {

	splitterAux := strings.Split(*peerList, ",")
	wantList := make([]PeerStatus, 1)
	wantList[0] = PeerStatus{
		Identifier: *gossiperName,
		NextID:     1,
	}
	return &Gossiper{
		Addr:                 *gossipAddr,
		UIPort:               *uiPort,
		Name:                 *gossiperName,
		KnownPeers:           splitterAux,
		Mode:                 *gossiperMode,
		Want:                 wantList,
		SavedMessages:        make(map[string][]RumorMessage),
		TalkingPeers:         make(map[string]ConectionInfo),
		RoutingTable:         make(map[string]string),
		StoredFiles:          make(map[string]*FileInfo),
		FilesBeingDownloaded: make([]*DownloadInfo, 0),
		AntiEntropyTimeout:   *antiEntropyTimeout,
		Rtimer:               *rtimer,
	}
}
func antiEntropy(gossiper *Gossiper) {
	ticker := time.NewTicker(time.Duration(gossiper.AntiEntropyTimeout) * time.Second)
	for range ticker.C {
		choseRandomPeerAndSendStatusPackage(gossiper)
	}
}
func setRtimer(gossiper *Gossiper) {
	if gossiper.Rtimer != 0 {
		//First route message when the peerster starts
		createAndSendNextHopMessage(gossiper)
		ticker := time.NewTicker(time.Duration(gossiper.Rtimer) * time.Second)
		for range ticker.C {
			createAndSendNextHopMessage(gossiper)
		}
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
func timeoutOfDownload(gossiper *Gossiper) {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {

		for key, fileDownload := range gossiper.FilesBeingDownloaded {
			if fileDownload.Timeout <= 0 {
				//TODO: Repeat the request again
				message := &DataRequest{
					Origin:      gossiper.Name,
					Destination: fileDownload.Destination,
					HashValue:   fileDownload.LastHashRequested,
					HopLimit:    10,
				}
				message.HopLimit = message.HopLimit - 1
				//If the hop limit has been exceeded
				if message.HopLimit <= 0 {
					return
				}
				//Restart the timeout, we can use the same session object as before
				gossiper.FilesBeingDownloaded[key].Timeout = 5
				//If not send to next hop
				nextHop := gossiper.RoutingTable[message.Destination]
				addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
				packetToSend := &GossipPacket{DataRequest: message}
				packetBytes, err := protobuf.Encode(packetToSend)
				if err != nil {
					panic(err)
				}
				gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
			} else {
				gossiper.FilesBeingDownloaded[key].Timeout = gossiper.FilesBeingDownloaded[key].Timeout - 1 /* = DownloadInfo{
					Timeout: gossiper.FilesBeingDownloaded[key].Timeout - 1,
				}*/
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
		go setRtimer(gossiper)
		//TODO: Descomentar
		//go timeOutOfDownload(gossiper *Gossiper) {
		go listenAPISocket(gossiper)
		listenSocketNotSimple(socket, gossiper)

	}
	//packetToSend := GossipPacket{Simple: simplemessage}
}
