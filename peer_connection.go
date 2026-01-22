package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
)

type MessageID byte

const (
	MsgChoke         MessageID = iota // 0
	MsgUnchoke                        // 1
	MsgInterested                     // 2
	MsgNotInterested                  // 3
	MsgHave                           // 4
	MsgBitfield                       // 5
	MsgRequest                        // 6
	MsgPiece                          // 7
	MsgCancel                         // 8
)

type PeerConnectionState byte

const (
	Idle PeerConnectionState = iota
	Downloading
)

type Handshake struct {
	pStrLen  byte
	pStr     [19]byte
	reserved [8]byte
	infoHash [20]byte
	peerId   [20]byte
}

type PeerConnection struct {
	torrentInfo            TorrentInfo
	peer                   Peer
	conn                   net.Conn
	state                  PeerConnectionState
	pieceManager           *PieceManager
	fileManager            *FileManager
	targetPipeline         int
	downloadedBytesInPiece int64
	currentPiece           int
	currentOffset          int
	inFlight               int
	peerBitfield           []bool
	myPeerId               string
	pieceBuffer            []byte
}

func NewPeerConnection(peer Peer, torrentInfo TorrentInfo, myPeerId string, fileManager *FileManager, pieceManager *PieceManager) *PeerConnection {
	pc := &PeerConnection{
		torrentInfo:    torrentInfo,
		peer:           peer,
		myPeerId:       myPeerId,
		pieceBuffer:    make([]byte, torrentInfo.pieceLength),
		pieceManager:   pieceManager,
		fileManager:    fileManager,
		targetPipeline: 10,
	}

	return pc
}

func (pc *PeerConnection) performHandshake() {
	// create handshake struct
	hs := Handshake{
		pStrLen:  19,
		reserved: [8]byte{},
		infoHash: pc.torrentInfo.infoHash,
	}
	copy(hs.pStr[:], "BitTorrent protocol")
	copy(hs.peerId[:], pc.myPeerId)

	conn, err := net.Dial("tcp", pc.peer.String())
	if err != nil {
		log.Fatal(err)
	}

	pc.conn = conn

	err = binary.Write(pc.conn, binary.BigEndian, hs)
	if err != nil {
		log.Fatal(err)
	}

	var response Handshake
	binary.Read(pc.conn, binary.BigEndian, &response)

	if response.pStrLen != 19 {
		log.Fatalf("Invalid pstrlen: %v\n", err)
	}
	if string(response.pStr[:]) != "BitTorrent protocol" {
		log.Fatalf("Invalid protocol string: %v\n", response.pStr[:])
	}
	if response.infoHash != pc.torrentInfo.infoHash {
		log.Fatalf("Info hash mismatch. Peer has wrong file: %v\n", response.infoHash[:])
	}

	log.Printf("Handshake ok. Peer ID: %s", response.peerId)
}

func (pc *PeerConnection) runMessageLoop() {
	interestedMsg := [5]byte{'0', '0', '0', '1', '2'}
	binary.Write(pc.conn, binary.BigEndian, interestedMsg)
	log.Println("Sent interested message")

	reader := bufio.NewReader(pc.conn)
	for {
		var length int
		err := binary.Read(pc.conn, binary.BigEndian, &length)
		if err != nil {
			log.Fatal(err)
		}

		if length == 0 {
			log.Println("[Keep-Alive]")
			continue
		}

		id, err := reader.ReadByte()
		if err != nil {
			log.Fatal(err)
		}

		var payload []byte
		if length > 1 && id != 7 {
			// length minus 1 because length does count the 'id' field, which
			// is not related to actual payload size
			payload = make([]byte, length-1)
		}

		log.Printf("Msg: ID=%d Len=%d ", id, length)

		switch MessageID(id) {
		case MsgChoke:
			log.Println("Choke")
		case MsgUnchoke:
			log.Println("Unchoke")
			pc.tryRequestNextPiece()
			pc.FillPipeline()
		case MsgInterested:
			log.Println("Interested")
		case MsgNotInterested:
			log.Println("Not interested")
		case MsgHave:
			log.Printf("Have piece #%d\n", payload[0])
		case MsgBitfield:
			log.Printf("Bitfield (%d bytes)\n", len(payload))
			pc.peerBitfield = make([]bool, len(payload)*8)
			for i, b := range payload {
				for bit := 0; bit < 8; bit++ {
					if b&(1<<(7-uint(bit))) != 0 {
						pc.peerBitfield[i*8+bit] = true
					}
				}
			}
		case MsgRequest:
			log.Println("Request")
		case MsgPiece:
			log.Println("Piece")
			pc.HandlePiece(length)
		case MsgCancel:
			log.Println("Cancel")
		default:
			log.Printf("Unknown id: %d\n", id)
		}
	}
}

func (pc *PeerConnection) Start() {
	defer pc.Stop()
	pc.performHandshake()
	pc.runMessageLoop()
}

func (pc *PeerConnection) Stop() error {
	return pc.conn.Close()
}

func (pc *PeerConnection) RequestBlock(pieceIndex int, blockOffset int, blockLength int) {
	request := struct {
		msgLen int
		id     byte
		index  int
		offset int
		length int
	}{
		msgLen: 13,
		id:     6, // request
		index:  pieceIndex,
		offset: blockOffset,
		length: blockLength,
	}

	binary.Write(pc.conn, binary.BigEndian, request)
	log.Printf("Sent request: Piece=%d Offset=%d Length=%d\n", pieceIndex, blockOffset, blockLength)
}

func (pc *PeerConnection) HandlePiece(messageLength int) error {
	if messageLength < 9 {
		log.Fatalln("Piece message too short")
	}

	header := struct {
		index uint32
		begin uint32
	}{}

	err := binary.Read(pc.conn, binary.BigEndian, &header)
	if err != nil {
		log.Println("Could not read piece header")
		return err
	}

	blockSize := messageLength - 9
	blockData := make([]byte, blockSize)

	n, err := io.ReadFull(pc.conn, blockData)
	if err != nil {
		log.Println("Could not read block data")
		return err
	}

	pc.pieceManager.AddBytes(uint64(n))

	if header.begin+uint32(n) > uint32(len(pc.pieceBuffer)) {
		return errors.New("buffer overflow. Peer sent too much data")
	}

	copy(pc.pieceBuffer[header.begin:], blockData)

	pc.inFlight--
	pc.downloadedBytesInPiece += int64(len(blockData))

	if pc.downloadedBytesInPiece >= pc.torrentInfo.pieceLength {
		log.Printf("Piece %d downloaded. Verifying\n", pc.currentPiece)

		if pc.verifyPiece(pc.currentPiece) {
			log.Println("Hash match. Writing to disk")

			globalOffset := int64(pc.currentPiece) * pc.torrentInfo.pieceLength
			pc.fileManager.Write(globalOffset, pc.pieceBuffer)
			pc.pieceManager.MarkAsCompleted(pc.currentPiece)
		} else {
			log.Printf("Hash mismatch. Dropping piece %d\n", pc.currentPiece)
			return err
		}

		pc.state = Idle
		pc.inFlight = 0
		pc.currentOffset = 0

		pc.tryRequestNextPiece()
		pc.FillPipeline()

		return nil
	}

	pc.FillPipeline()
	return nil
}

func (pc *PeerConnection) verifyPiece(index int) bool {
	calculatedHash := sha1.Sum(pc.pieceBuffer)

	offset := index * 20
	if offset+20 > len(pc.torrentInfo.pieces) {
		return false
	}

	expectedHash := pc.torrentInfo.pieces[offset : offset+20]

	return bytes.Equal(calculatedHash[:], []byte(expectedHash))
}

func (pc *PeerConnection) tryRequestNextPiece() {
	if pc.state == Downloading {
		return
	}

	nextPieceOpt, ok := pc.pieceManager.GetNextPieceToDownload(pc.peerBitfield)
	if !ok {
		log.Println("No work")
		return
	}

	pc.state = Downloading

	pc.currentPiece = nextPieceOpt
	pc.currentOffset = 0
	pc.downloadedBytesInPiece = 0

	clear(pc.pieceBuffer)
	log.Printf("Assigned piece %d. Starting\n", pc.currentPiece)
}

func (pc *PeerConnection) FillPipeline() {
	for pc.inFlight < pc.targetPipeline {
		if int64(pc.currentOffset) >= pc.torrentInfo.pieceLength {
			break
		}

		bytesLeft := pc.torrentInfo.pieceLength - int64(pc.currentOffset)
		size := min(16*1024, bytesLeft)

		// those int type conversions are killing me man
		// what was I thinking
		pc.RequestBlock(pc.currentPiece, pc.currentOffset, int(size))
		pc.inFlight++
	}
}
