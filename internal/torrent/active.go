package torrent

// ActiveReaders counts the open readers (viewers streaming) and download floors (full-file
// downloads) across all torrents: the work a connection reset would interrupt.
func (e *ClientEngine) ActiveReaders() int {
	e.mu.RLock()
	scheds := make([]*pieceScheduler, 0, len(e.schedulers))
	for _, s := range e.schedulers {
		scheds = append(scheds, s)
	}
	e.mu.RUnlock()
	n := 0
	for _, s := range scheds {
		s.mu.Lock()
		n += len(s.windows) + len(s.floors)
		s.mu.Unlock()
	}
	return n
}
