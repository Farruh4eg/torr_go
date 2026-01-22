package main

import (
	"log"
	"os"
	"path/filepath"
	"sync"
)

type FileManager struct {
	torrentInfo TorrentInfo
	rootPath    string
	sync.Mutex
}

func NewFileManager(torrentInfo TorrentInfo, downloadPath string) *FileManager {
	fm := &FileManager{
		torrentInfo: torrentInfo,
		rootPath:    downloadPath,
	}
	for _, file := range torrentInfo.files {
		fullPath := filepath.Join(fm.rootPath, file.path)
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			log.Fatalf("Failed to create directories %w", err)
		}
	}

	return fm
}

func (fm *FileManager) Write(globalOffset int64, data []byte) {
	fm.Lock()
	defer fm.Unlock()

	currentGlobalPos := globalOffset
	dataBytesWritten := int64(0)
	bytesLeft := int64(len(data))

	for _, file := range fm.torrentInfo.files {
		if file.endOffset <= currentGlobalPos {
			continue
		}

		if file.startOffset >= currentGlobalPos+bytesLeft {
			break
		}

		fileSeekPos := currentGlobalPos - file.startOffset
		bytesForThisFile := min(bytesLeft, file.length-fileSeekPos)
		fm.writeToFile(file, fileSeekPos, data[dataBytesWritten:], bytesForThisFile)

		currentGlobalPos += bytesForThisFile
		dataBytesWritten += bytesForThisFile
		bytesLeft -= bytesForThisFile

		if bytesLeft == 0 {
			break
		}
	}
}

func (fm *FileManager) writeToFile(file FileInfo, fileOffset int64, data []byte, length int64) error {
	fullPath := filepath.Join(fm.rootPath, file.path)
	dir := filepath.Dir(fullPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		log.Printf("Failed to create directory %w\n", err)
		return err
	}

	f, err := os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("Failed to open file for writing: %v\n", err)
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		log.Printf("Failed to write to file %v\n", err)
		return err
	}

	return nil
}
