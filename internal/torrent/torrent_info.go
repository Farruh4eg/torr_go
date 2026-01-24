package torrent

import (
	"crypto/sha1"
	"errors"
)

type FileInfo struct {
	Path        string
	Length      int64
	StartOffset int64
	EndOffset   int64
}

type TorrentInfo struct {
	infoHash [20]byte
	files    []FileInfo

	announce    string
	pieceLength int64
	pieces      string
	name        string
	totalLength int64
}

func (ti *TorrentInfo) InfoHash() [20]byte {
	return ti.infoHash
}

func (ti *TorrentInfo) Announce() string {
	return ti.announce
}

func (ti *TorrentInfo) PieceLength() int64 {
	return ti.pieceLength
}

func (ti *TorrentInfo) Pieces() string {
	return ti.pieces
}

func (ti *TorrentInfo) Name() string {
	return ti.name
}

func (ti *TorrentInfo) TotalLength() int64 {
	return ti.totalLength
}

func NewTorrentInfoFromNode(root Node, rawInfoBytes string) (*TorrentInfo, error) {
	torrentInfo := &TorrentInfo{}

	rootDict := root.AsDict()

	if rootDict["announce"].AsString() != "" {
		torrentInfo.announce = rootDict["announce"].AsString()
	}

	infoDict := rootDict["info"].AsDict()
	if infoDict == nil {
		return nil, errors.New("no info dictionary found")
	}

	torrentInfo.name = infoDict["name"].AsString()
	torrentInfo.pieceLength = infoDict["piece length"].asInt64()
	torrentInfo.pieces = infoDict["pieces"].AsString()

	hash := sha1.Sum([]byte(rawInfoBytes))
	torrentInfo.infoHash = hash

	// multi-file
	if filesList := infoDict["files"].AsList(); filesList != nil {
		for _, fileNode := range filesList {
			fileDict := fileNode.AsDict()
			length := fileDict["length"].asInt64()
			torrentInfo.totalLength += length

			pathList := fileDict["path"].AsList()
			var fullPath string

			for i, path := range pathList {
				if i > 0 {
					fullPath += "/"
				}
				fullPath += path.AsString()
			}

			torrentInfo.files = append(torrentInfo.files, FileInfo{Path: fullPath, Length: length})
		}
	} else {
		// single-file
		torrentInfo.totalLength = infoDict["length"].asInt64()
		if torrentInfo.totalLength == 0 {
			return nil, errors.New("no length and no files")
		}
		torrentInfo.files = append(torrentInfo.files, FileInfo{Path: torrentInfo.name, Length: torrentInfo.totalLength})
	}

	currentOffset := int64(0)
	for i := range torrentInfo.files {
		torrentInfo.files[i].StartOffset = currentOffset
		currentOffset += torrentInfo.files[i].Length
		torrentInfo.files[i].EndOffset = currentOffset
	}
	return torrentInfo, nil
}

func (ti *TorrentInfo) PieceCount() int {
	return len(ti.pieces) / 20
}

func (ti *TorrentInfo) Files() []FileInfo {
	return ti.files
}
