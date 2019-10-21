package main

//TODO: HAY UN PROBLEMA CUANDO ESTAS IN SYNC PERO TE HABLA ALGUIEN CON QUIEN NO TIENES SESION
import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
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
	StoredFiles          map[string]FileInfo     //The key is the file name
	FilesBeingDownloaded map[string]DownloadInfo //The key is the owner´s name of the file
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
	buf := make([]byte, 2000)
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
	hashFileRequested := message.HashValue
	dataReplyToSend := &DataReply{}
	//We are going to check all the hashes to see if we have it.
	//Another possibility would be open a sesion when metahash is requested and therefore knowing that later he would ask for chunks of that file
	for _, storedFile := range gossiper.StoredFiles {

		//Metafile is being requested
		storedMetahash := storedFile.MetaHash[:]
		if bytes.Equal(storedMetahash, hashFileRequested) {
			file, err := os.Open(storedFile.Metafile)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()
			metahashbytes, err := ioutil.ReadAll(file)
			//TODO: Send metahashbytes back
			dataReplyToSend = &DataReply{
				Origin:      gossiper.Name,
				Destination: message.Origin,
				HopLimit:    10,
				HashValue:   hashFileRequested,
				Data:        metahashbytes,
			}
			break
		}
		for index, hashOfChunk := range storedFile.HashesOfChunks {
			//One chunk is being requested
			hashOfChunkAux := hashOfChunk[:]
			if bytes.Equal(hashOfChunkAux, hashFileRequested) {
				//Now we look for the file knowing the index of the chunk
				selectedChuckPath := storedFile.FilePathChunks + "_" + strconv.FormatUint(uint64(index), 10)
				file, err := os.Open(selectedChuckPath)
				if err != nil {
					log.Fatal(err)
				}
				defer file.Close()
				chunkBytes, err := ioutil.ReadAll(file)
				dataReplyToSend = &DataReply{
					Origin:      gossiper.Name,
					Destination: message.Origin,
					HopLimit:    10,
					HashValue:   hashFileRequested,
					Data:        chunkBytes,
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
	origin := message.Origin
	sha256fDataAux := sha256.Sum256(message.Data)
	sha256fData := sha256fDataAux[:]
	session := gossiper.FilesBeingDownloaded[origin]
	sha256Expected := session.LastHashRequested
	//Check if the file has being received correctly or is not the one we are looking for
	if !bytes.Equal(sha256fData, sha256Expected) {
		//TODO: Do something if not
		return
	} else {
		//Check if the received hash is of the metahash
		//If it is the metahash
		if bytes.Equal(sha256Expected, session.MetaHash) && session.MetaHashObtained == false {
			session.MetaHashObtained = true

			//Save file to filesystem
			filePathMetaHash := session.PathToSave + "_metahash"
			f, err := os.Create(filePathMetaHash)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			f.Write(message.Data)
			f.Sync()

			//Save the info also to the gossiper struct
			stringListOfHashes := string(message.Data)
			listOfHashes := strings.Split(stringListOfHashes, ",")
			var listOfHashesToSave [][]byte
			for _, hashString := range listOfHashes {
				listOfHashesToSave = append(listOfHashesToSave, []byte(hashString))
			}
			session.HashesOfChunks = listOfHashesToSave
			session.Metafile = filePathMetaHash
			//We are going to ask for the first chunk
			session.LastHashRequested = listOfHashesToSave[0]
			gossiper.FilesBeingDownloaded[message.Destination] = DownloadInfo{
				MetaHash:          session.MetaHash,
				MetaHashObtained:  session.MetaHashObtained,
				PathToSave:        session.PathToSave,
				Timeout:           5,
				LastHashRequested: session.LastHashRequested,
				HashesOfChunks:    session.HashesOfChunks,
				Metafile:          session.Metafile,
			}
			dataRequest := &DataRequest{
				Destination: origin,
				HopLimit:    10,
				Origin:      gossiper.Name,
				HashValue:   session.LastHashRequested,
			}
			sendDataRequest(dataRequest, gossiper)
			//TODO: Update available files

			//Is the hash of a chunk of the file
		} else {
			//Save file to filesystem
			var hashNumber int
			for index, v := range session.HashesOfChunks {
				if bytes.Equal(v, sha256fData) {
					hashNumber = index
					break
				}
			}
			filePathEachChunk := session.PathToSave + "_" + string(hashNumber)
			f, err := os.Create(filePathEachChunk)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			f.Write(message.Data)
			f.Sync()
			//Check if we have files left to download
			//If we do
			if len(session.HashesOfChunks) > hashNumber {
				//Request next chunk
				gossiper.FilesBeingDownloaded[message.Destination] = DownloadInfo{
					MetaHash:          session.MetaHash,
					MetaHashObtained:  session.MetaHashObtained,
					PathToSave:        session.PathToSave,
					Timeout:           5,
					LastHashRequested: session.HashesOfChunks[hashNumber+1],
					HashesOfChunks:    session.HashesOfChunks,
					Metafile:          session.Metafile,
				}
				//TODO: Send another request
				dataRequest := &DataRequest{
					Destination: origin,
					HopLimit:    10,
					Origin:      gossiper.Name,
					HashValue:   session.HashesOfChunks[hashNumber+1],
				}
				sendDataRequest(dataRequest, gossiper)

				//TODO: Update available files
				var metaHashAux [32]byte
				var hashesOfChunksAux [][32]byte
				var hashesOfChunksAux2 [32]byte
				copy(metaHashAux[:], session.MetaHash)
				for i := 0; i < len(session.HashesOfChunks); i++ {
					copy(hashesOfChunksAux2[:], session.HashesOfChunks[i])
					hashesOfChunksAux = append(hashesOfChunksAux, hashesOfChunksAux2)
				}
				fileToStore := FileInfo{
					FileSize:       0,
					MetaHash:       metaHashAux,
					Metafile:       session.Metafile,
					HashesOfChunks: hashesOfChunksAux,
					FilePathChunks: session.PathToSave,
				}
				gossiper.StoredFiles[session.Metafile] = fileToStore
			} else {
				//TODO: File downloaded, joint parts and close session
				fileDownloadedSuccessfully(message, gossiper)
			}
		}

	}
}
func fileDownloadedSuccessfully(message *DataReply, gossiper *Gossiper) {
	origin := message.Origin
	session := gossiper.FilesBeingDownloaded[origin]
	pathToSaveFinalFile := session.PathToSave
	//TODO: Join the parts
	f, err := os.OpenFile(pathToSaveFinalFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	for i := 0; i < len(session.HashesOfChunks); i++ {
		pathToChuckFile := session.PathToSave + "_" + string(i)
		file, err := os.Open(pathToChuckFile)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		bytesOfTheChunk, err := ioutil.ReadAll(file)

		//Write back to the file
		if _, err := f.Write(bytesOfTheChunk); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
	fileInfo, _ := f.Stat()
	//Update available files
	var metaHashAux [32]byte
	var hashesOfChunksAux [][32]byte
	var hashesOfChunksAux2 [32]byte
	copy(metaHashAux[:], session.MetaHash)
	for i := 0; i < len(session.HashesOfChunks); i++ {
		copy(hashesOfChunksAux2[:], session.HashesOfChunks[i])
		hashesOfChunksAux = append(hashesOfChunksAux, hashesOfChunksAux2)
	}
	fileToStore := FileInfo{
		FileSize:       fileInfo.Size(),
		MetaHash:       metaHashAux,
		Metafile:       session.Metafile,
		HashesOfChunks: hashesOfChunksAux,
		FilePathChunks: pathToSaveFinalFile,
	}
	gossiper.StoredFiles[session.Metafile] = fileToStore

	//Close session
	delete(gossiper.FilesBeingDownloaded, origin)

}
func fileDownloadRequest(packet *Message, gossiper *Gossiper) {
	//TODO: Crear un request y mandarlo y abrir sesion de download
	message := &DataRequest{
		Origin:      gossiper.Name,
		Destination: *packet.Destination,
		HashValue:   *packet.Request,
		HopLimit:    10,
	}

	message.HopLimit = message.HopLimit - 1
	//If the hop limit has been exceeded
	if message.HopLimit <= 0 {
		return
	}
	//Open sesion of download
	gossiper.FilesBeingDownloaded[message.Destination] = DownloadInfo{
		MetaHash:          message.HashValue,
		MetaHashObtained:  false,
		PathToSave:        filepath.Join(Downloads, *packet.File),
		Timeout:           5,
		LastHashRequested: message.HashValue,
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

			fileIndexing(packet, gossiper)
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
func fileIndexing(packet *Message, gossiper *Gossiper) {
	fileToBeChunked := *packet.File
	var hashesOfChunks [][32]byte
	//pwd, _ := os.Getwd()
	//fmt.Println(pwd)
	file, err := os.Open(filepath.Join(SharedFiles, fileToBeChunked))
	pathToStore := filepath.Join(SharedFiles, strings.Split(fileToBeChunked, ".")[0])
	os.Mkdir("."+string(filepath.Separator)+pathToStore, 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer file.Close()
	fileInfo, _ := file.Stat()
	var fileSize int64 = fileInfo.Size()
	//8 KB of chunk sice
	const fileChunk = 1 * (1 << 13)

	// calculate total number of parts the file will be chunked into

	totalPartsNum := uint64(math.Ceil(float64(fileSize) / float64(fileChunk)))

	fmt.Printf("Splitting to %d pieces.\n", totalPartsNum)

	for i := uint64(0); i < totalPartsNum; i++ {

		partSize := int(math.Min(fileChunk, float64(fileSize-int64(i*fileChunk))))
		partBuffer := make([]byte, partSize)
		//Read the chunk
		file.Read(partBuffer)
		hashesOfChunks = append(hashesOfChunks, sha256.Sum256(partBuffer))

		fileName := *packet.File + "_" + strconv.FormatUint(i, 10)
		//_, err := os.Create(fileName)

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		// write/save buffer to disk
		ioutil.WriteFile(filepath.Join(pathToStore, fileName), partBuffer, os.ModeAppend)
	}
	calculateMetaHash(filepath.Join(pathToStore, fileToBeChunked), hashesOfChunks, fileSize, gossiper)

}
func calculateMetaHash(filePath string, hashesOfChunks [][32]byte, fileSize int64, gossiper *Gossiper) error {
	fileCommonBase := filePath
	filePathMetaHash := fileCommonBase + "_metahash"
	f, err := os.Create(filePathMetaHash)
	if err != nil {
		return err
	}
	defer f.Close()
	var hashesOfChunksString []string
	for hashOfChunk := range hashesOfChunks {
		hashesOfChunksString = append(hashesOfChunksString, string(hashOfChunk))
	}
	dataOfMetaHash := strings.Join(hashesOfChunksString, ",")
	f.Write([]byte(dataOfMetaHash))
	f.Sync()
	/*rv := reflect.ValueOf(hashesOfChunks)
	if rv.Kind() != reflect.Slice {
		return errors.New("Not a slice")
	}
	for i := 0; i < rv.Len(); i++ {
		fmt.Fprintln(f, rv.Index(i).Interface())
	}*/
	//Now calculate the metahash
	file, err := os.Open(filePathMetaHash)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	metahashbytes, err := ioutil.ReadAll(file)
	metaHash := sha256.Sum256(metahashbytes)
	fmt.Printf("%x\n", metaHash)
	fileToStore := FileInfo{
		FileSize:       fileSize,
		MetaHash:       metaHash,
		Metafile:       filePathMetaHash,
		HashesOfChunks: hashesOfChunks,
		FilePathChunks: fileCommonBase,
	}
	gossiper.StoredFiles[filePathMetaHash] = fileToStore
	//fmt.Printf("%+v\n", fileToStore)
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
		StoredFiles:          make(map[string]FileInfo),
		FilesBeingDownloaded: make(map[string]DownloadInfo),
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
		go listenAPISocket(gossiper)
		listenSocketNotSimple(socket, gossiper)

	}
	//packetToSend := GossipPacket{Simple: simplemessage}
}
