package torrent

import (
	"time"

	"golang.org/x/time/rate"
)

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
	Tunneled                   bool // behind a VPN: no inbound port, so no UPnP / NAT-PMP mapping
	EnableTitForTat            bool
	UploadBytesPerSec          int64 // global upload cap; 0 = unlimited
	DefaultTrackers            []string
	DHTBootstrapRouters        []string
	// MetainfoFetchers download a private provider's .torrent, keyed by the provider
	// named in a magnet's "xs=gazes:<provider>".
	MetainfoFetchers map[string]MetainfoFetcher
	MetainfoDir      string        // cached .torrent files; empty disables
	CacheMaxBytes    int64         // resident payload cap enforced by LRU eviction; 0 disables
	CacheIdleTTL     time.Duration // a torrent must be idle this long before it can be evicted
}

// uploadLimiter turns a bytes/s cap into the client's upload limiter. The burst
// must cover one peer request chunk or a send would block forever.
func uploadLimiter(bytesPerSec int64) *rate.Limiter {
	if bytesPerSec <= 0 {
		return rate.NewLimiter(rate.Inf, 0)
	}
	burst := int(bytesPerSec)
	if burst < 1<<20 {
		burst = 1 << 20
	}
	return rate.NewLimiter(rate.Limit(bytesPerSec), burst)
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
		EnableTitForTat:            true,    // Reciprocal unchoking for max download bandwidth
		UploadBytesPerSec:          2 << 20, // 2 MiB/s: enough for tit-for-tat, leaves the VPN line to viewers
		CacheIdleTTL:               10 * time.Minute,
		// Public, ratio-free trackers (ngosang/trackerslist, refreshed 2026-10-04), added to every torrent that is not private.
		DefaultTrackers: []string{
			"udp://tracker.opentrackr.org:1337/announce",
			"udp://open.stealth.si:80/announce",
			"udp://tracker.torrent.eu.org:451/announce",
			"udp://exodus.desync.com:6969/announce",
			"udp://explodie.org:6969/announce",
			"udp://tracker.internetwarriors.net:1337/announce",
			"udp://p4p.arenabg.com:1337/announce",
			"http://nyaa.tracker.wf:7777/announce",
			"udp://open.demonii.com:1337/announce",
			"udp://tracker.dler.org:6969/announce",
			"udp://opentracker.i2p.rocks:6969/announce",
			"udp://tracker.theoks.net:6969/announce",
			"http://tracker.openbittorrent.com:80/announce",
			"http://tracker.qu.ax:6969/announce",
			"udp://tracker.skynetcloud.site:6969/announce",
			"udp://tracker.gmi.gd:6969/announce",
			"udp://tracker.tryhackx.org:6969/announce",
			"udp://tracker.nyaa.vc:6969/announce",
			"udp://tracker.corpscorp.online:80/announce",
			"udp://tracker.bittor.pw:1337/announce",
			"udp://tracker-udp.gbitt.info:80/announce",
			"udp://tracker2.dler.org:80/announce",
			"http://tracker.dler.com:6969/announce",
			"udp://tracker.ducks.party:1984/announce",
			"udp://retracker01-msk-virt.corbina.net:80/announce",
			"http://tracker.renfei.net:8080/announce",
			"udp://tracker.peerfect.org:6969/announce",
			"udp://tracker.opentrackr.com:6969/announce",
			"udp://tracker.ilibr.org:6969/announce",
			"udp://mail.segso.net:6969/announce",
			"udp://t.overflow.biz:6969/announce",
			"https://tracker.pmman.tech:443/announce",
			"udp://ipv4announce.sktorrent.eu:6969/announce",
			"udp://tracker.004430.xyz:1337/announce",
			"https://tracker.nekomi.cn:443/announce",
			"https://004430.xyz:443/announce",
			"http://004430.xyz:80/announce",
			"udp://evan.im:6969/announce",
			"udp://bittorrent-tracker.e-n-c-r-y-p-t.net:1337/announce",
			"udp://tr4ck3r.duckdns.org:6969/announce",
			"udp://martin-gebhardt.eu:25/announce",
			"http://tracker.mywaifu.best:6969/announce",
			"udp://tracker.wildkat.net:6969/announce",
			"https://tracker.zhuqiy.com:443/announce",
			"http://tracker.waaa.moe:6969/announce",
			"http://tracker.dhitechnical.com:6969/announce",
			"http://open.tracker.cl:1337/announce",
			"udp://tracker.fnix.net:6969/announce",
			"udp://tracker.filemail.com:6969/announce",
			"udp://seedpeer.net:6969/announce",
			"udp://open.ftorrent.com:443/announce",
			"udp://admin.52ywp.com:6969/announce",
			"https://tracker.7471.top:443/announce",
			"https://tr.nyacat.pw:443/announce",
			"http://tr.nyacat.pw:80/announce",
			"udp://tracker.aruku.ovh:8081/announce",
			"https://tracker.foreverpirates.co:443/announce",
			"http://tracker.23794.top:6969/announce",
			"udp://tracker.torrents.observer:80/announce",
			"udp://tracker.nyaa.net:6969/announce",
			"udp://tracker.cn.nyaa.net:6969/announce",
			"udp://kolankoalastree.newtrackon.co.nz:1337/announce",
			"udp://anime-tracker.aruku.kro.kr:8081/announce",
			"https://pybittrack.retiolus.net:443/announce",
			"udp://tr3.ysagin.top:2715/announce",
			"http://tracker.zhuqiy.dgj055.icu:80/announce",
			"http://1337.abcvg.info:80/announce",
			"udp://yuptracker-eu.gaijinent.com:27022/announce",
			"udp://tracker.playground.ru:6969/announce",
			"udp://tracker.ddunlimited.net:6969/announce",
			"udp://ns575949.ip-51-222-82.net:6969/announce",
			"udp://edgev.duckdns.org:6969/announce",
			"https://t.213891.xyz:443/announce",
		},
		DHTBootstrapRouters: []string{
			"router.bittorrent.com:6881",
			"dht.transmissionbt.com:6881",
			"router.utorrent.com:6881",
			"dht.aelitis.com:6881",
		},
	}
}
