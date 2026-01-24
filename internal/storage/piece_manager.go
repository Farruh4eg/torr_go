package storage

import (
	"sync"
	"sync/atomic"
	"time"
)

type PieceManagerState byte

const (
	Missing PieceManagerState = iota
	InProgress
	Have
)

type PieceManager struct {
	states               []PieceManagerState
	totalBytesDownloaded atomic.Uint64
	sync.Mutex

	lastBytes    uint64
	lastTime     time.Time
	currentSpeed float64
}

func (pm *PieceManager) UpdateSpeed() {
	pm.Lock()
	defer pm.Unlock()

	now := time.Now()
	currentBytes := pm.totalBytesDownloaded.Load()

	if pm.lastTime.IsZero() {
		pm.lastTime = now
		pm.lastBytes = currentBytes
		return
	}

	duration := now.Sub(pm.lastTime).Seconds()
	if duration < 0.5 {
		return
	}

	delta := float64(currentBytes - pm.lastBytes)
	pm.currentSpeed = (delta / (1024 * 1024)) / duration

	pm.lastTime = now
	pm.lastBytes = currentBytes
}

func (pm *PieceManager) GetSpeed() float64 {
	pm.Lock()
	defer pm.Unlock()
	return pm.currentSpeed
}

func NewPieceManager(totalPieces int) *PieceManager {
	pm := &PieceManager{
		states: make([]PieceManagerState, totalPieces)}

	return pm
}

func (pm *PieceManager) GetNextPieceToDownload(peerBitfield []bool) (int, bool) {
	pm.Lock()
	defer pm.Unlock()

	if len(peerBitfield) == 0 {
		return 0, false
	}

	for i, state := range pm.states {
		if i < len(peerBitfield) && state == Missing && peerBitfield[i] {
			pm.states[i] = InProgress
			return i, true
		}
	}

	return 0, false
}

func (pm *PieceManager) MarkAsCompleted(index int) {
	pm.Lock()
	defer pm.Unlock()

	pm.states[index] = Have
}

func (pm *PieceManager) MarkAsFailed(index int) {
	pm.Lock()
	defer pm.Unlock()

	pm.states[index] = Missing
}

func (pm *PieceManager) Progress() float32 {
	pm.Lock()
	defer pm.Unlock()

	completed := 0
	for _, s := range pm.states {
		if s == Have {
			completed++
		}
	}

	return float32(completed) / float32(len(pm.states))
}

func (pm *PieceManager) AddBytes(n uint64) {
	pm.totalBytesDownloaded.Add(n)
}

func (pm *PieceManager) StatesInt() []int {
	pm.Lock()
	defer pm.Unlock()

	result := make([]int, len(pm.states))
	for i, s := range pm.states {
		result[i] = int(s)
	}

	return result
}

func (pm *PieceManager) TotalDownloadedMB() uint64 {
	bytes := pm.totalBytesDownloaded.Load()

	return bytes / 1024 / 1024
}
