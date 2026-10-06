# BTCPay Server for Gazes donations

Bitcoin only, pruned node (~20 GB), no third party: the funds go straight to a wallet you control.
The Gazes backend talks to BTCPay over HTTPS (`GAZES_BTCPAY_URL`), it does not share a docker network.

## 1. Start (host)

```bash
cd deploy/btcpay
# Keep the env file outside the checkout (a worktree can be deleted, the DB password must survive):
cp .env.example ~/.config/gazes/btcpay.env && chmod 600 ~/.config/gazes/btcpay.env   # BTCPAY_DOMAIN + POSTGRES_PASSWORD (openssl rand -hex 24)
docker compose -p gazes-btcpay --env-file ~/.config/gazes/btcpay.env up -d
docker compose -p gazes-btcpay logs -f bitcoind   # initial sync: several hours, then it stays small
```

Never `docker compose down -v` (wallet settings and chain data live in the volumes).

## 2. Expose (needs root, not done by the repo)

DNS: an `A`/`AAAA` record for `BTCPAY_DOMAIN` -> this machine. Then add to `/etc/caddy/Caddyfile`
and `sudo systemctl reload caddy`:

```
pay.delikesance.cloud {
    encode zstd gzip
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options "nosniff"
    }
    reverse_proxy 127.0.0.1:23000
}
```

## 3. Set up BTCPay (browser, once the node is synced)

1. Open `https://<BTCPAY_DOMAIN>`, create the **first account**: it becomes the server admin.
   Then Server Settings > Policies: disable registration.
2. Create a store (currency EUR). Wallet > **Connect an existing wallet** with the **xpub** of a wallet you own
   (never paste a seed phrase). The node does not scan old history: only payments to new addresses count.
3. Account > API keys: create a key with the permissions `btcpay.store.cancreateinvoice` and
   `btcpay.store.canviewinvoices`, restricted to this store.
4. Store > Webhooks > Create: URL `https://<gazes-domain>/api/v1/donations/webhooks/btcpay`, a long random secret,
   events *InvoiceSettled*, *InvoiceExpired*, *InvoiceInvalid*.

## 4. Gazes backend environment

| Variable | Value |
|---|---|
| `GAZES_SITE_URL` | `https://gazes.delikesance.cloud` |
| `GAZES_BTCPAY_URL` | `https://<BTCPAY_DOMAIN>` |
| `GAZES_BTCPAY_STORE_ID` | the store id (Store > Settings) |
| `GAZES_BTCPAY_API_KEY` | the API key (or `GAZES_BTCPAY_API_KEY_FILE`) |
| `GAZES_BTCPAY_WEBHOOK_SECRET` | the webhook secret (or `..._FILE`) |

Restart the backend: `/soutenir` then shows the crypto form. See `docs/donations.md`.
