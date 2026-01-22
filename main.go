package main

import (
	"flag"
	"log"
)

func main() {
	var filePathFlag = flag.String("i", "", "input torrent file path")
	flag.Parse()

	if filePathFlag == nil || *filePathFlag == "" {
		log.Fatal("Usage: ./main.exe -i input.torrent")
	}

	parser, err := NewParserFromFile(*filePathFlag)
	if err != nil {
		log.Fatalf("Could not create parser from file: %v", err)
	}

	root, err := parser.parse()
	if err != nil {
		log.Fatalf("Could not parse torrent file: %v", err)
	}

	parser.printNode(root, 0)
}
