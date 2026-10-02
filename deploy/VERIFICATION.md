# Prowlarr deployment verification

Verified on 2026-10-02 with Docker Engine 29.5.2, Compose 5.1.4 and
Prowlarr 2.6.5.5623 (the current linuxserver latest image).

- Seven Python bootstrap tests passed: initial key generation, persistent manual
  authentication, idempotence, manual indexer preservation, interrupted activation
  recovery, absent definitions, sanitized API failures and private manifest writes.
- `go test -race ./internal/indexer/... ./internal/api` passed, including Torznab
  parsing, VF ahead of VOSTFR, multi-provider infohash merging and partial failures.
- `GAZES_PORT=18081 docker compose -p gazes-prowlarr-clean up -d --build --wait`
  succeeded from fresh named volumes. Backend, web and Prowlarr were healthy;
  initialization and provisioning exited successfully.
- Official AniDex and The Pirate Bay indexers were enabled with IDs 1 and 2.
  AniDex returned an external 502 during the initial enabled-POST experiment;
  disabled POST followed by forced PUT provisioned both without relying on external
  site health. No custom or obsolete indexer definitions were installed.
- Production frontend `/healthz`, the Tensura season-one episode-one page and
  `/api/v1/search?q=Tensura` returned HTTP 200. Search returned 49 Nyaa results
  with `partial: true` while additional providers were unavailable.
- Restarting Prowlarr/backend/web and rerunning Compose retained IDs 1 and 2,
  no duplicates, and a matching persistent key and generated manifest.
- Stopping Prowlarr left backend/frontend health HTTP 200. A live search returned
  48 Nyaa results with `partial: true`. One separate VF query encountered a
  temporary Nyaa failure as well; if every upstream fails, search returns an error.
- The generated manifest had mode 0400 and backend-user ownership. The API key
  was absent from stack logs and frontend search responses. Prowlarr had no
  published host port; only the optional administration override publishes
  127.0.0.1:9696.

The extra clean-test stack was removed without deleting its persistent volumes.
The default stack was rebuilt with the final code and left healthy on port 18080.
Live access to AniDex/The Pirate Bay and the presence of a usable VF torrent
remain dependent on the deployment server's network and upstream availability.
