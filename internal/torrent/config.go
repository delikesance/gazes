package torrent

import "time"

// EngineConfig holds configuration parameters for the torrent engine.
type EngineConfig struct {
	DataDir                    string
	DisableUTP                 bool
	DisableTCP                 bool
	PreferUDP                  bool
	ListenPort                 int
	UDPPort                    int
	DefaultReadaheadBytes      int64
	HeaderPrefetchPieces       int
	LookaheadPieceCount        int
	EstablishedConnsPerTorrent int
	HalfOpenConnsPerTorrent    int
	Seed                       bool
	EnableTitForTat            bool
	DefaultTrackers            []string
	DHTBootstrapRouters        []string
	MetainfoDir                string        // cached .torrent files; empty disables
	CacheMaxBytes              int64         // resident payload cap enforced by LRU eviction; 0 disables
	CacheIdleTTL               time.Duration // a torrent must be idle this long before it can be evicted
}

// DefaultEngineConfig returns optimized defaults for low-latency, fast-streaming operations.
func DefaultEngineConfig(dataDir string) EngineConfig {
	return EngineConfig{
		DataDir:                    dataDir,
		DisableUTP:                 false,
		DisableTCP:                 false,
		PreferUDP:                  true,             // Prioritize uTP / UDP transport
		ListenPort:                 0,                // Auto-select available port
		UDPPort:                    42069,            // Preferred UDP port for uTP / DHT
		DefaultReadaheadBytes:      64 * 1024 * 1024, // 64MB sliding lookahead
		HeaderPrefetchPieces:       1,                // First and last pieces for container metadata
		LookaheadPieceCount:        48,               // Active sliding lookahead pieces
		EstablishedConnsPerTorrent: 100,              // Wide peer set: swarms often expose few reachable seeders
		HalfOpenConnsPerTorrent:    50,               // Handshake throughput
		Seed:                       false,
		EnableTitForTat:            true, // Reciprocal unchoking for max download bandwidth
		CacheIdleTTL:               10 * time.Minute,
		DefaultTrackers: []string{
			"udp://tracker.opentrackr.org:1337/announce",
			"udp://open.stealth.si:80/announce",
			"udp://tracker.torrent.eu.org:451/announce",
			"udp://exodus.desync.com:6969/announce",
			"udp://explodie.org:6969/announce",
			"udp://tracker.coppersurfer.tk:6969/announce",
			"udp://tracker.internetwarriors.net:1337/announce",
			"udp://tracker.leechers-paradise.org:6969/announce",
			"udp://p4p.arenabg.com:1337/announce",
			"http://nyaa.tracker.wf:7777/announce",
			"udp://open.demonii.com:1337/announce",
			"udp://tracker.dler.org:6969/announce",
			"udp://tracker.tiny-vps.com:6969/announce",
			"udp://opentracker.i2p.rocks:6969/announce",
			"udp://tracker.theoks.net:6969/announce",
			"udp://tracker.moeking.me:6969/announce",
			"udp://retracker.lanta-net.ru:2710/announce",
			"http://tracker.openbittorrent.com:80/announce",
		},
		DHTBootstrapRouters: []string{
			"router.bittorrent.com:6881",
			"dht.transmissionbt.com:6881",
			"router.utorrent.com:6881",
			"dht.aelitis.com:6881",
		},
	}
}
