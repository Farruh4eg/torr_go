package main

import (
	"crypto/sha1"
	"errors"
)

type FileInfo struct {
	path        string
	length      int64
	startOffset int64
	endOffset   int64
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

func NewTorrentInfoFromNode(root *Node, rawInfoBytes string) (*TorrentInfo, error) {
	torrentInfo := &TorrentInfo{}

	rootDict := root.asDict()

	if rootDict["announce"].asString() != "" {
		torrentInfo.announce = rootDict["announce"].asString()
	}

	infoDict := rootDict["info"].asDict()
	if infoDict == nil {
		return nil, errors.New("no info dictionary found")
	}

	torrentInfo.name = infoDict["name"].asString()
	torrentInfo.pieceLength = infoDict["piece length"].asInt64()
	torrentInfo.pieces = infoDict["pieces"].asString()

	hash := sha1.Sum([]byte(rawInfoBytes))
	torrentInfo.infoHash = hash

	// multi-file
	if filesList := infoDict["files"].asDict(); filesList != nil {
		for _, fileNode := range filesList {
			fileDict := fileNode.asDict()
			length := fileDict["length"].asInt64()
			torrentInfo.totalLength += length

			pathList := fileDict["path"].asList()
			var fullPath string

			for i, path := range pathList {
				if i > 0 {
					fullPath += "/"
				}
				fullPath += path.asString()
			}

			torrentInfo.files = append(torrentInfo.files, FileInfo{path: fullPath, length: length})
		}
	}

	// single-file
	torrentInfo.totalLength = infoDict["length"].asInt64()
	if torrentInfo.totalLength == 0 {
		return nil, errors.New("no length and no files")
	}

	torrentInfo.files = append(torrentInfo.files, FileInfo{path: torrentInfo.name, length: torrentInfo.totalLength})

	return torrentInfo, nil
}

func (ti *TorrentInfo) getPieceCount() int {
	return len(ti.pieces) / 20
}
