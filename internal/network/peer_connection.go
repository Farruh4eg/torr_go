package network

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"gotor/internal/storage"
	"gotor/internal/torrent"
	"io"
	"log"
	"net"
	"time"
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
	PStrLen  byte
	PStr     [19]byte
	Reserved [8]byte
	InfoHash [20]byte
	PeerId   [20]byte
}

type PeerConnection struct {
	torrentInfo            torrent.TorrentInfo
	peer                   Peer
	conn                   net.Conn
	state                  PeerConnectionState
	pieceManager           *storage.PieceManager
	fileManager            *storage.FileManager
	targetPipeline         int
	downloadedBytesInPiece int64
	currentPiece           int
	currentOffset          int
	inFlight               int
	peerBitfield           []bool
	myPeerId               string
	pieceBuffer            []byte
}

func NewPeerConnection(peer Peer, torrentInfo torrent.TorrentInfo, myPeerId string, fileManager *storage.FileManager, pieceManager *storage.PieceManager) *PeerConnection {
	pc := &PeerConnection{
		torrentInfo:    torrentInfo,
		peer:           peer,
		myPeerId:       myPeerId,
		pieceBuffer:    make([]byte, torrentInfo.PieceLength()),
		pieceManager:   pieceManager,
		fileManager:    fileManager,
		targetPipeline: 64,
	}

	return pc
}

func (pc *PeerConnection) performHandshake() error {
	// create handshake struct
	hs := Handshake{
		PStrLen:  19,
		Reserved: [8]byte{},
		InfoHash: pc.torrentInfo.InfoHash(),
	}
	copy(hs.PStr[:], "BitTorrent protocol")
	copy(hs.PeerId[:], pc.myPeerId)

	conn, err := net.DialTimeout("tcp", pc.peer.String(), 5*time.Second)
	if err != nil {
		return err
	}
	pc.conn = conn

	err = binary.Write(pc.conn, binary.BigEndian, hs)
	if err != nil {
		return err
	}

	var response Handshake
	binary.Read(pc.conn, binary.BigEndian, &response)

	if response.PStrLen != 19 {
		return errors.New(fmt.Sprintf("invalid pstrlen: %v\n", response.PStrLen))
	}
	if string(response.PStr[:]) != "BitTorrent protocol" {
		return errors.New(fmt.Sprintf("invalid protocol string: %v\n", response.PStr[:]))
	}
	if response.InfoHash != pc.torrentInfo.InfoHash() {
		return errors.New(fmt.Sprintf("info hash mismatch. Peer has wrong file: %v\n", response.InfoHash[:]))
	}

	log.Printf("Handshake ok. Peer ID: %s", response.PeerId)
	return nil
}

func (pc *PeerConnection) runMessageLoop() error {
	interestedMsg := [5]byte{0, 0, 0, 1, 2}
	binary.Write(pc.conn, binary.BigEndian, interestedMsg)
	log.Println("Sent interested message")

	for {
		var length int32
		err := binary.Read(pc.conn, binary.BigEndian, &length)
		if err != nil {
			return err
		}

		if length == 0 {
			log.Println("[Keep-Alive]")
			continue
		}

		idBuf := make([]byte, 1)
		if _, err := io.ReadFull(pc.conn, idBuf); err != nil {
			return err
		}
		id := idBuf[0]

		var payload []byte
		if length > 1 && id != 7 {
			payload = make([]byte, length-1)
			io.ReadFull(pc.conn, payload)
		}
		//log.Printf("Msg: ID=%d Len=%d ", id, length)

		switch MessageID(id) {
		case MsgChoke:
			log.Println("Choke")
			if pc.currentPiece != -1 {
				pc.pieceManager.MarkAsFailed(pc.currentPiece)
			}
			pc.resetState()

		case MsgUnchoke:
			log.Println("Unchoke")
			pc.tryRequestNextPiece()
			pc.FillPipeline()
		case MsgInterested:
			log.Println("Interested")
		case MsgNotInterested:
			log.Println("Not interested")
		case MsgHave:
			//pieceIndex := binary.BigEndian.Uint32(payload[:4])
			//og.Printf("Have piece #%d\n", pieceIndex)
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
			//log.Println("Request")
		case MsgPiece:
			//log.Println("Piece")
			pc.HandlePiece(int(length))
		case MsgCancel:
			log.Println("Cancel")
		default:
			log.Printf("Unknown id: %d\n", id)
		}
	}
	return nil
}

func (pc *PeerConnection) Start() error {
	defer pc.Stop()
	err := pc.performHandshake()
	if err != nil {
		return err
	}

	err = pc.runMessageLoop()
	if err != nil {
		return err
	}

	return nil
}

func (pc *PeerConnection) Stop() error {
	if pc.conn != nil {
		return pc.conn.Close()
	}
	return nil
}

func (pc *PeerConnection) RequestBlock(pieceIndex int, blockOffset int, blockLength int) {
	request := struct {
		msgLen uint32
		id     byte
		index  uint32
		offset uint32
		length uint32
	}{
		msgLen: 13,
		id:     6, // request
		index:  uint32(pieceIndex),
		offset: uint32(blockOffset),
		length: uint32(blockLength),
	}

	binary.Write(pc.conn, binary.BigEndian, request)
	//log.Printf("Sent request: Piece=%d Offset=%d Length=%d\n", pieceIndex, blockOffset, blockLength)
}

func (pc *PeerConnection) HandlePiece(messageLength int) error {
	if messageLength < 9 {
		log.Println("piece message too short")
	}

	header := struct {
		Index uint32
		Begin uint32
	}{}

	err := binary.Read(pc.conn, binary.BigEndian, &header)
	if err != nil {
		log.Println("Error reading piece header")
	}

	blockSize := messageLength - 9
	blockData := make([]byte, blockSize)

	n, err := io.ReadFull(pc.conn, blockData)
	if err != nil {
		log.Println("Error reading block data")

		pc.pieceManager.MarkAsFailed(pc.currentPiece)
		pc.resetState()
		return nil
	}

	pc.pieceManager.AddBytes(uint64(n))

	if header.Begin+uint32(n) > uint32(len(pc.pieceBuffer)) {
		log.Println("Buffer overflow")
	}

	copy(pc.pieceBuffer[header.Begin:], blockData)

	pc.inFlight--
	pc.downloadedBytesInPiece += int64(len(blockData))

	if pc.downloadedBytesInPiece >= pc.torrentInfo.PieceLength() {
		log.Printf("Piece %d downloaded. Verifying\n", pc.currentPiece)

		if pc.verifyPiece(pc.currentPiece) {
			log.Println("Hash match. Writing to disk")

			globalOffset := int64(pc.currentPiece) * pc.torrentInfo.PieceLength()
			pc.fileManager.Write(globalOffset, pc.pieceBuffer)
			pc.pieceManager.MarkAsCompleted(pc.currentPiece)
		} else {
			log.Printf("Hash mismatch. Dropping piece %d\n", pc.currentPiece)
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
	if offset+20 > len(pc.torrentInfo.Pieces()) {
		return false
	}

	expectedHash := pc.torrentInfo.Pieces()[offset : offset+20]

	return bytes.Equal(calculatedHash[:], []byte(expectedHash))
}

func (pc *PeerConnection) tryRequestNextPiece() {
	if len(pc.peerBitfield) == 0 {
		return
	}

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
		if int64(pc.currentOffset) >= pc.torrentInfo.PieceLength() {
			break
		}

		bytesLeft := pc.torrentInfo.PieceLength() - int64(pc.currentOffset)
		size := min(16*1024, bytesLeft)

		// those int type conversions are killing me man
		// what was I thinking
		pc.RequestBlock(pc.currentPiece, pc.currentOffset, int(size))

		pc.currentOffset += int(size)
		pc.inFlight++
	}
}

func (pc *PeerConnection) resetState() {
	pc.state = Idle
	pc.inFlight = 0
	pc.currentPiece = -1
	pc.currentOffset = 0
	pc.downloadedBytesInPiece = 0
	clear(pc.pieceBuffer)
}
