---
name: forgejo-pr
description: Open a pull request (merge request / MR), base dev by default on the gazes Forgejo at git.delikesance.cloud. Use whenever the user asks to open, create or prepare a PR/MR for this repo. gh does not work here (not GitHub).
---

# Open a PR on git.delikesance.cloud (Forgejo)

The `origin` remote is a self-hosted Forgejo 9.0.3 (Gitea 1.22 API), not GitHub:
`ssh://git@git.delikesance.cloud:2222/delikesance/gazes.git`. Never use `gh` for it.

- Web: `https://git.delikesance.cloud/delikesance/gazes`
- API: `https://git.delikesance.cloud/api/v1/repos/delikesance/gazes` (anonymous reads work, writes need a token)
- PRs and issues share one number sequence; PR N lives at `/delikesance/gazes/pulls/N`.
- Flow (see CLAUDE.md): feature/fix branch → PR into `dev`; `dev` → `main` only when asked (push to main auto-deploys).
- The API merge call is denied by the permission classifier: after opening the PR, give the link and let the user merge; never retry or work around.

## 1. Prepare the branch

```bash
git status -sb                      # working tree clean? branch tracking origin?
git push -u origin HEAD            # head branch must be on origin
git fetch -q origin
git log --oneline origin/dev..HEAD   # commits the PR will carry (must be non-empty)
git log --oneline HEAD..origin/dev   # dev commits missing from the branch (conflict risk)
```

Check there is no PR already open for the same head/base:

```bash
curl -s "https://git.delikesance.cloud/api/v1/repos/delikesance/gazes/pulls?state=open" \
  | jq -r '.[] | "#\(.number) \(.head.ref) -> \(.base.ref): \(.title)"'
```

If one exists, the push already updated it: give its link instead of opening a new one.

## 2. Write title and description

- Title: the single commit subject, or a short summary when there are several commits.
- Description: why the change, then a bullet per notable change; end with the PR attribution line from the current session's system reminder, if any.

## 3. Create the PR

No CLI exists on this machine (no tea/fj/berg, checked) — always use the REST API with curl.

**Token**: read it from `$FORGEJO_TOKEN`, else from `~/.config/forgejo/token` (chmod 600, outside the repo).
Never write it into a tracked file, never echo it, never put it in a commit. If a user pastes a token in chat,
use it for that single command only (inline `export`), and tell them to revoke it afterwards and store the
new one in `~/.config/forgejo/token`.
If neither exists, ask the user to create one: Forgejo → Paramètres → Applications → Générer un nouveau jeton,
scope `write:repository`. (`POST /users/:name/tokens` needs the user's password: never ask for it.)

`HEAD_BRANCH` is the current branch (`git branch --show-current`; `dev` in the usual flow, a `claude/...`
worktree branch otherwise); it must already be pushed. Base is `dev` (set `BASE_BRANCH=main` only for a dev → main PR).

```bash
FORGEJO_TOKEN=${FORGEJO_TOKEN:-$(cat ~/.config/forgejo/token 2>/dev/null)}
HEAD_BRANCH=$(git branch --show-current)
jq -n --arg head "$HEAD_BRANCH" --arg base "${BASE_BRANCH:-dev}" --arg title "$TITLE" --arg body "$BODY" \
  '{head:$head, base:$base, title:$title, body:$body}' \
| curl -s -X POST \
    -H "Authorization: token $FORGEJO_TOKEN" -H "Content-Type: application/json" \
    --data @- https://git.delikesance.cloud/api/v1/repos/delikesance/gazes/pulls \
| jq '{number, state, mergeable, html_url, message}'
```

A `message` field means an error (401 = bad token/scope, 409/422 = PR already exists or nothing to merge).

**Without any token**: give the user the compare link and title/description in fenced blocks to paste:
`https://git.delikesance.cloud/delikesance/gazes/compare/${BASE_BRANCH:-dev}...$HEAD_BRANCH`
(they click "Nouvelle demande d'ajout", paste, submit).

## 4. Verify

```bash
curl -s https://git.delikesance.cloud/api/v1/repos/delikesance/gazes/pulls/N \
  | jq '{number, state, title, head: .head.ref, base: .base.ref, mergeable, html_url}'
```

Report the PR link as `https://git.delikesance.cloud/delikesance/gazes/pulls/N`.
