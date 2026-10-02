// source-audit captures indexer discovery or replays a raw corpus offline.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/indexer/nyaa"
	"github.com/gazes/gazes/internal/indexer/settings"
	"github.com/gazes/gazes/internal/kv"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	identityFile := flag.String("identity", "", "episode identity JSON (required for live capture)")
	replay := flag.String("replay", "", "replay a capture offline, with the current matcher")
	output := flag.String("out", "", "output JSON (stdout when omitted)")
	playback := flag.Bool("fast", false, "run the bounded playback discovery instead of full discovery")
	plan := flag.Bool("plan", false, "print search options without network access")
	nyaaOnly := flag.Bool("nyaa-only", false, "capture only Nyaa, independently of configured gateways")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	var snapshot indexer.SourceAudit
	read := func(path string, target any) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, target)
	}
	if *replay != "" {
		if err := read(*replay, &snapshot); err != nil {
			return err
		}
	}
	if *identityFile != "" {
		if err := read(*identityFile, &snapshot.Identity); err != nil {
			return err
		}
	}
	identity := snapshot.Identity
	if len(identity.Titles) == 0 || identity.EpisodeNumber <= 0 || identity.SeasonNumber <= 0 {
		return fmt.Errorf("provide a valid -identity or -replay capture")
	}
	write := func(value any) error {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if *output != "" {
			return os.WriteFile(*output, data, 0644)
		}
		_, err = os.Stdout.Write(data)
		return err
	}
	if *plan {
		return write(struct {
			Fast    []indexer.SearchOptions `json:"fast"`
			French  []indexer.SearchOptions `json:"french"`
			Generic []string                `json:"generic"`
		}{indexer.SeasonPlaybackOptions(identity), indexer.FrenchSearchOptions(identity), indexer.EpisodeSearchQueries(identity)})
	}
	start := time.Now()
	if *replay != "" {
		snapshot = indexer.ReplaySourceAudit(snapshot)
	} else {
		providers := []indexer.Provider{nyaa.NewClient("", nil)}
		if !*nyaaOnly {
			extra, err := settings.Providers(os.Getenv)
			if err != nil {
				return err
			}
			providers = append(providers, extra...)
		}
		var err error
		multi := indexer.NewMultiProvider(providers...)
		// Share caches and cooldowns with the running backend when configured.
		if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			client, err := kv.Open(ctx, redisURL, os.Getenv("REDIS_NAMESPACE"), time.Second)
			cancel()
			if err != nil {
				return fmt.Errorf("shared cache unavailable")
			}
			defer client.Close()
			multi.SetRedis(client)
		}
		recorder := indexer.NewAuditProvider(multi)
		resolver := indexer.NewEpisodeResolver(recorder)
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		var res *indexer.EpisodeSourcesResponse
		if *playback {
			res, err = resolver.ResolvePlaybackSources(ctx, identity)
		} else {
			res, err = resolver.ResolveSeasonSources(ctx, identity)
		}
		snapshot = recorder.Snapshot(identity)
		snapshot.Result = res
		if err != nil {
			snapshot.Error = err.Error()
		}
	}
	snapshot.ElapsedMS = time.Since(start).Milliseconds()
	if err := write(snapshot); err != nil {
		return err
	}
	accepted, vf := 0, 0
	for _, d := range snapshot.Decisions {
		if d.Accepted {
			accepted++
			if d.Language == indexer.LangVF {
				vf++
			}
		}
	}
	fmt.Fprintf(os.Stderr, "%d searches; %d raw candidates; %d accepted; %d VF claims; %d ms\n", len(snapshot.Searches), len(snapshot.Items), accepted, vf, snapshot.ElapsedMS)
	if snapshot.Error != "" {
		return fmt.Errorf("capture failed: %s (diagnostics saved)", snapshot.Error)
	}
	return nil
}
