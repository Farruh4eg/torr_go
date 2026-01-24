package main

import (
	"flag"
	"fmt"
	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/AllenDang/giu"
	"gotor/internal/network"
	"gotor/internal/storage"
	"gotor/internal/torrent"
	"gotor/pkg"
	"log"
	url2 "net/url"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type App struct {
	sync.Mutex
	torrentInfo   *torrent.TorrentInfo
	pieceManager  *storage.PieceManager
	fileManager   *storage.FileManager
	status        string
	ready         bool
	isDownloading bool
}

func (a *App) setStatus(status string) {
	a.Lock()
	a.status = status
	a.Unlock()
}

func (a *App) Close() {
	a.Lock()
	defer a.Unlock()
	if a.fileManager != nil {
		a.fileManager.Close()
	}
}

func (a *App) startDownload(torrentPath string, saveDir string) {
	a.setStatus("Initializing")

	defer func() {
		if err := recover(); err != nil {
			log.Println(err)
			a.setStatus(fmt.Sprintf("Fatal error: %v", err))
		}
	}()

	a.setStatus("Parsing torrent")

	parser, err := torrent.NewParserFromFile(torrentPath)
	if err != nil {
		a.setStatus("Error creating a parser: " + err.Error())
		return
	}

	root, err := parser.Parse()
	if err != nil {
		a.setStatus("Error parsing torrent: " + err.Error())
		return
	}

	a.torrentInfo, err = torrent.NewTorrentInfoFromNode(root, parser.InfoRaw())
	if err != nil {
		a.setStatus("Error creating torrent info: " + err.Error())
		return
	}

	files := a.torrentInfo.Files()
	log.Printf("DEBUG: Found %d files in torrent\n", len(files))
	for _, f := range files {
		log.Printf("DEBUG: File=%s, Size=%d, StartOffset=%d\n", f.Path, f.Length, f.StartOffset)
	}

	a.fileManager = storage.NewFileManager(*a.torrentInfo, saveDir)
	a.pieceManager = storage.NewPieceManager(a.torrentInfo.PieceCount())

	a.ready = true

	a.setStatus("Contacting tracker " + a.torrentInfo.Announce())

	peerId := pkg.GeneratePeerId()
	u, err := pkg.ParseTrackerUrl(a.torrentInfo.Announce())
	if err != nil {
		a.setStatus("Error parsing tracker url: " + err.Error())
		return
	}

	infoHash := a.torrentInfo.InfoHash()

	params := url2.Values{}
	params.Add("info_hash", string(infoHash[:]))
	params.Add("peer_id", peerId)
	params.Add("port", "42069")
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", strconv.FormatInt(a.torrentInfo.TotalLength(), 10))
	params.Add("compact", "1")

	fullPath := u.Path + "?" + params.Encode()

	client := network.NewTrackerClient()

	rawResponse, err := client.Request(u.Host, fullPath, u.Port)
	if err != nil {
		a.setStatus("Error getting response from tracker: " + err.Error())
		return
	}

	peers, err := client.ExtractPeers(rawResponse)
	if err != nil {
		a.setStatus("Error extracting peers: " + err.Error())
		return
	}

	log.Printf("Got %d peers\n", len(peers))

	for _, p := range peers {
		go func(peer network.Peer) {
			conn := network.NewPeerConnection(p, *a.torrentInfo, peerId, a.fileManager, a.pieceManager)
			if err := conn.Start(); err != nil {
				//	a.setStatus("Error starting peer connection: " + err.Error())
			}
		}(p)
	}

	a.setStatus("Anoosha gom")
	a.isDownloading = true
}

func main() {
	var filePathFlag = flag.String("i", "", "input torrent file path")
	var saveDirFlag = flag.String("o", "", "output directory path")
	flag.Parse()

	if *filePathFlag == "" || *saveDirFlag == "" {
		log.Fatal("Usage: ./main.exe -i input.torrent -o ./output_dir")
	}

	app := &App{}

	go app.startDownload(*filePathFlag, *saveDirFlag)
	go func() {
		for {
			if app.ready {
				app.pieceManager.UpdateSpeed()
				time.Sleep(time.Second)
			}
		}
	}()

	wnd := giu.NewMasterWindow("Gotor", 640, 480, 0)

	if app.ready {
		wnd.Run(func() {
			app.Lock()
			imgui.Text(fmt.Sprintf("Name: %s", app.torrentInfo.Name()))
			imgui.Text(fmt.Sprintf("Status: %s", app.status))
			imgui.Text(fmt.Sprintf("Downloaded: %d MB / %.2f MB", app.pieceManager.TotalDownloadedMB(), float64(app.torrentInfo.TotalLength())/1024/1024))
			imgui.Text(fmt.Sprintf("Progress: %.2f%%", app.pieceManager.Progress()*100))
			imgui.Text(fmt.Sprintf("Speed: %.2f MB/s", app.pieceManager.GetSpeed()))
			app.Unlock()

			imgui.ProgressBarV(app.pieceManager.Progress(), imgui.Vec2{X: -1, Y: 0}, "")
		})
	}
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		for {
			time.Sleep(time.Second * 1)
			if app.ready {
				fmt.Printf("\rProgress: %.2f%% | Total Downloaded: %d MB",
					app.pieceManager.Progress()*100,
					app.pieceManager.TotalDownloadedMB())
			}
		}
	}()
	<-sigChan
	log.Println("Shutting gracefully...")
	app.Close()
	log.Println("Shutting down successfully")
}
