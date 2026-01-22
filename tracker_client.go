package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

type TrackerClient struct {
}

func NewTrackerClient() *TrackerClient {
	return &TrackerClient{}
}

func (tc *TrackerClient) Request(host string, path string, port int) (string, error) {
	url := fmt.Sprintf("http://%s:%d%s", host, port, path)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("http get error: %v", err)
		return "", err
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("http status: %v", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

func (tc *TrackerClient) ExtractPeers(bencodeResponse string) ([]Peer, error) {
	buffer := make([]byte, len(bencodeResponse))
	copy(buffer, []byte(bencodeResponse))
	parser, err := NewParserFromData(buffer)
	if err != nil {
		log.Printf("NewParserFromData error: %v", err)
		return nil, err
	}

	root, err := parser.parse()
	if err != nil {
		log.Printf("parse error: %v\n", err)
		return nil, err
	}

	dict := root.asDict()

	if _, ok := dict["failure reason"]; ok {
		return nil, fmt.Errorf("tracker reported error: %s", dict["failure reason"].asString())
	}

	if _, ok := dict["peers"]; !ok {
		return nil, errors.New("no peers found in dict")
	}

	blob := dict["peers"].asString()
	if len(blob)%6 != 0 {
		log.Println("Warning: peers blob length is not dividable by 6")
	}

	numPeers := len(blob) / 6
	peers := make([]Peer, 0, numPeers)
	for i := 0; i < numPeers; i++ {
		offset := i * 6

		ip := net.IP(blob[offset : offset+4])
		port := binary.BigEndian.Uint16([]byte(blob[offset+4 : offset+6]))

		peers = append(peers, Peer{
			ip:   ip.String(),
			port: port,
		})
	}

	return peers, nil
}
