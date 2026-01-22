package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
)

type ParsedUrl struct {
	host string
	path string
	port int
}

func GeneratePeerId() string {
	const prefix = "-F4T001-"
	const charset = "0123456789"

	id := make([]byte, 20)
	copy(id, prefix)

	for i := len(prefix); i < 20; i++ {
		id[i] = charset[rand.Intn(len(charset))]
	}

	return string(id)
}

func UrlEncode(value string) string {
	var buf strings.Builder
	for _, b := range value {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
			buf.WriteByte(byte(b))
		} else {
			buf.WriteString(fmt.Sprintf("%%%02x", b))
		}
	}

	return buf.String()
}

func ParseTrackerUrl(trackerUrl string) (ParsedUrl, error) {
	url, err := url.Parse(trackerUrl)
	if err != nil {
		return ParsedUrl{}, err
	}

	host := url.Hostname()
	portStr := url.Port()

	port := 80
	if portStr != "" {
		port, _ = strconv.Atoi(portStr)
	}

	return ParsedUrl{host: host, path: url.Path, port: port}, nil
}
