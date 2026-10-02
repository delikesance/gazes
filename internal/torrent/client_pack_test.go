package torrent

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	anacrolix "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"testing"
	"time"
)

func TestPackMetadataDoesNotScheduleAllEpisodes(t *testing.T) {
	cfg := anacrolix.NewDefaultClientConfig()
	cfg.DataDir = t.TempDir()
	cfg.DisableTCP = true
	cfg.DisableUTP = true
	cfg.NoDHT = true
	cfg.DisableTrackers = true
	cfg.NoDefaultPortForwarding = true
	cfg.DialForPeerConns = false
	cfg.AcceptPeerConnections = false
	client, err := anacrolix.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	const pieceLength = 16384
	info := metainfo.Info{Name: "500 episode pack", PieceLength: pieceLength, Pieces: make([]byte, 500*2*20)}
	for n := 1; n <= 500; n++ {
		info.Files = append(info.Files, metainfo.FileInfo{Length: pieceLength * 2, Path: []string{fmt.Sprintf("Naruto_Shippuuden_%03d.mkv", n)}})
	}
	encoded, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	hash := metainfo.Hash(sha1.Sum(encoded))
	loaded, _ := client.AddTorrentInfoHash(hash)
	if err := loaded.SetInfoBytes(encoded); err != nil {
		t.Fatal(err)
	}
	engine := &ClientEngine{client: client, cfg: EngineConfig{HeaderPrefetchPieces: 1, DefaultReadaheadBytes: pieceLength, LookaheadPieceCount: 1}, torrents: map[string]*anacrolix.Torrent{}}
	ih, files, err := engine.AddTorrent(context.Background(), "magnet:?xt=urn:btih:"+hash.HexString())
	if err != nil || len(files) != 500 {
		t.Fatalf("%d %v", len(files), err)
	}
	for i := 0; i < loaded.NumPieces(); i++ {
		if loaded.PieceState(i).Priority != anacrolix.PiecePriorityNone {
			t.Fatalf("metadata load requested piece %d", i)
		}
	}
	reader, _, err := engine.GetFileStream(context.Background(), ih, 13)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	deadline := time.Now().Add(time.Second)
	for (loaded.PieceState(26).Priority == anacrolix.PiecePriorityNone || loaded.PieceState(27).Priority == anacrolix.PiecePriorityNone) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < loaded.NumPieces(); i++ {
		priority := loaded.PieceState(i).Priority
		if i >= 26 && i <= 27 {
			if priority == anacrolix.PiecePriorityNone {
				t.Errorf("episode 14 piece %d not prioritized", i)
			}
		} else if priority != anacrolix.PiecePriorityNone {
			t.Errorf("unrelated episode piece %d prioritized", i)
		}
	}
	// A stalled read must obey cancellation, while another viewer keeps the
	// same episode's priority window alive.
	ctx, cancel := context.WithCancel(context.Background())
	second, _, err := engine.GetFileStream(ctx, ih, 13)
	if err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() { _, err := second.Read(make([]byte, 1)); readDone <- err }()
	cancel()
	select {
	case err := <-readDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stalled read: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stalled reader ignored cancellation")
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if loaded.PieceState(26).Priority == anacrolix.PiecePriorityNone {
		t.Fatal("cancelling one viewer removed the other viewer's priority")
	}
	other, _, err := engine.GetFileStream(context.Background(), ih, 14)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	for _, piece := range []int{26, 27} {
		if loaded.PieceState(piece).Priority != anacrolix.PiecePriorityNone {
			t.Fatalf("closed episode still downloads piece %d", piece)
		}
	}
	deadline = time.Now().Add(time.Second)
	for loaded.PieceState(28).Priority == anacrolix.PiecePriorityNone && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if loaded.PieceState(28).Priority == anacrolix.PiecePriorityNone {
		t.Fatal("other episode lost its priority")
	}

}
