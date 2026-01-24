package storage

import (
	"fmt"
	"gotor/internal/torrent"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type FileManager struct {
	sync.Mutex
	torrentInfo torrent.TorrentInfo
	rootPath    string
	openFiles   map[string]*os.File
}

func NewFileManager(torrentInfo torrent.TorrentInfo, downloadPath string) *FileManager {
	fm := &FileManager{
		torrentInfo: torrentInfo,
		rootPath:    downloadPath,
		openFiles:   make(map[string]*os.File),
	}
	for _, file := range torrentInfo.Files() {
		fullPath := filepath.Join(fm.rootPath, file.Path)
		dir := filepath.Dir(fullPath)
		os.MkdirAll(dir, 0755)
		f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			log.Fatalf("Failed to open file %s: %v", fullPath, err)
		}

		fm.openFiles[file.Path] = f
	}

	return fm
}

func (fm *FileManager) Write(globalOffset int64, data []byte) {
	log.Printf("!!! FM.Write CALLED: Offset=%d, Len=%d", globalOffset, len(data))
	fm.Lock()
	defer fm.Unlock()

	currentGlobalPos := globalOffset
	dataBytesWritten := int64(0)
	bytesLeft := int64(len(data))

	for _, file := range fm.torrentInfo.Files() {
		if file.EndOffset <= currentGlobalPos {
			continue
		}

		if file.StartOffset >= currentGlobalPos+bytesLeft {
			break
		}

		fileSeekPos := currentGlobalPos - file.StartOffset
		bytesForThisFile := min(bytesLeft, file.Length-fileSeekPos)
		fm.writeToFile(file, fileSeekPos, data[dataBytesWritten:], bytesForThisFile)

		currentGlobalPos += bytesForThisFile
		dataBytesWritten += bytesForThisFile
		bytesLeft -= bytesForThisFile

		if bytesLeft == 0 {
			break
		}
	}
}

func (fm *FileManager) writeToFile(file torrent.FileInfo, fileOffset int64, data []byte, length int64) error {
	f, ok := fm.openFiles[file.Path]
	if !ok {
		return fmt.Errorf("file %s not found in open files", file.Path)
	}

	_, err := f.WriteAt(data[:length], fileOffset)
	if err != nil {
		return err
	}

	return f.Sync()
}
func (fm *FileManager) Close() {
	fm.Lock()
	defer fm.Unlock()

	for path, file := range fm.openFiles {
		log.Printf("Closing file %s", path)
		file.Sync()
		file.Close()
	}
}
