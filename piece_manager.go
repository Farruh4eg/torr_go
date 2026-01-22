package main

import (
	"sync"
	"sync/atomic"
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
}

func NewPieceManager(totalPieces int) *PieceManager {
	pm := &PieceManager{
		states: make([]PieceManagerState, totalPieces)}

	return pm
}

func (pm *PieceManager) GetNextPieceToDownload(peerBitfield []bool) (int, bool) {
	pm.Lock()
	defer pm.Unlock()

	for i, state := range pm.states {
		if state == Missing && peerBitfield[i] {
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

func (pm *PieceManager) GetProgress() float64 {
	pm.Lock()
	defer pm.Unlock()

	completed := 0
	for _, s := range pm.states {
		if s == Have {
			completed++
		}
	}

	return float64(completed) / float64(len(pm.states))
}

func (pm *PieceManager) AddBytes(n uint64) {
	pm.totalBytesDownloaded.Add(n)
}

func (pm *PieceManager) GetStatesInt() []int {
	pm.Lock()
	defer pm.Unlock()

	result := make([]int, len(pm.states))
	for i, s := range pm.states {
		result[i] = int(s)
	}

	return result
}
