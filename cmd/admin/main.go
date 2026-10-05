// gazes-admin manages admin roles and API tokens on the local databases; it never opens a network port.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/auth"
	"github.com/gazes/gazes/internal/config"
)

type roleStore interface {
	SetUserRole(ctx context.Context, pseudo, role string) error
	SetUserRoleByID(ctx context.Context, id int64, role string) error
	RoleByPseudo(ctx context.Context, pseudo string) (string, error)
	UserRole(ctx context.Context, id int64) (string, error)
	CountAdmins(ctx context.Context) (int, error)
}

type tokenStore interface {
	CreateToken(ctx context.Context, name string, scopes []string, ttl time.Duration) (string, int64, error)
	ListTokens(ctx context.Context) ([]admin.Token, error)
	RevokeToken(ctx context.Context, id int64) error
}

const usage = `usage: gazes-admin <command>
  grant <pseudo | #id>
  revoke <pseudo | #id> [--force]    (pseudos are not unique: use '#<id>' when several accounts share one)
  token create --name N --scopes a,b [--ttl 720h]
  token list
  token revoke <id>
`

func main() {
	cfg := config.Load()
	ctx := context.Background()
	args := os.Args[1:]
	var err error
	switch {
	case len(args) == 0:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	case args[0] == "grant" || args[0] == "revoke":
		var st *auth.Store
		if st, err = auth.OpenStore(cfg.AccountsDir); err == nil {
			defer st.Close()
			err = run(ctx, args, os.Stdout, os.Stderr, st, nil)
		}
	case args[0] == "token":
		var st *admin.Store
		if st, err = admin.Open(cfg.AdminDBPath); err == nil {
			defer st.Close()
			err = run(ctx, args, os.Stdout, os.Stderr, nil, st)
		}
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, roles roleStore, tokens tokenStore) error {
	if len(args) == 0 {
		return errors.New("missing command")
	}
	switch args[0] {
	case "grant":
		if len(args) != 2 {
			return errors.New("usage: grant <pseudo | #id>")
		}
		return grant(ctx, stdout, roles, args[1])
	case "revoke":
		fs := flag.NewFlagSet("revoke", flag.ContinueOnError)
		fs.SetOutput(stderr)
		force := fs.Bool("force", false, "allow removing the last admin")
		pseudo, rest := splitPositional(args[1:])
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if pseudo == "" || fs.NArg() != 0 {
			return errors.New("usage: revoke <pseudo | #id> [--force]")
		}
		return revokeAdmin(ctx, stdout, roles, pseudo, *force)
	case "token":
		return tokenCmd(ctx, args[1:], stdout, stderr, tokens)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

// splitPositional extracts the first non-flag argument so flags may follow it.
func splitPositional(args []string) (string, []string) {
	for i, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a, append(append([]string{}, args[:i]...), args[i+1:]...)
		}
	}
	return "", args
}

func roleErr(pseudo string, err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("no user %q", pseudo)
	case errors.Is(err, auth.ErrAmbiguousPseudo):
		return fmt.Errorf("pseudo %q matches several users", pseudo)
	}
	return err
}

// parseID reads "#42" (an account id) and reports whether arg had that form.
func parseID(arg string) (int64, bool) {
	if len(arg) < 2 || len(arg) > 16 || arg[0] != '#' {
		return 0, false
	}
	var n int64
	for _, c := range arg[1:] {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return n, n > 0
}

func setRole(ctx context.Context, st roleStore, target, role string) error {
	if id, ok := parseID(target); ok {
		return st.SetUserRoleByID(ctx, id, role)
	}
	return st.SetUserRole(ctx, target, role)
}

func roleOf(ctx context.Context, st roleStore, target string) (string, error) {
	if id, ok := parseID(target); ok {
		return st.UserRole(ctx, id)
	}
	return st.RoleByPseudo(ctx, target)
}

func grant(ctx context.Context, out io.Writer, st roleStore, target string) error {
	if err := setRole(ctx, st, target, auth.RoleAdmin); err != nil {
		return roleErr(target, err)
	}
	fmt.Fprintf(out, "%s is now admin\n", target)
	return nil
}

func revokeAdmin(ctx context.Context, out io.Writer, st roleStore, target string, force bool) error {
	role, err := roleOf(ctx, st, target)
	if err != nil {
		return roleErr(target, err)
	}
	if role == auth.RoleAdmin && !force {
		n, err := st.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return errors.New("refusing to remove the last admin (use --force)")
		}
	}
	if err := setRole(ctx, st, target, auth.RoleUser); err != nil {
		return roleErr(target, err)
	}
	fmt.Fprintf(out, "%s is no longer admin\n", target)
	return nil
}

func tokenCmd(ctx context.Context, args []string, stdout, stderr io.Writer, st tokenStore) error {
	if len(args) == 0 {
		return errors.New("usage: token create|list|revoke")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("token create", flag.ContinueOnError)
		fs.SetOutput(stderr)
		name := fs.String("name", "", "token name")
		scopes := fs.String("scopes", "", "comma-separated scopes")
		ttl := fs.Duration("ttl", admin.DefaultTokenTTL, "lifetime (max 8760h)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		plain, _, err := st.CreateToken(ctx, *name, strings.Split(*scopes, ","), *ttl)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, plain)
		fmt.Fprintln(stderr, "token shown once: store it now, it cannot be retrieved later")
		return nil
	case "list":
		toks, err := st.ListTokens(ctx)
		if err != nil {
			return err
		}
		now := time.Now()
		fmt.Fprintf(stdout, "%-4s %-20s %-8s %-20s %-20s %s\n", "ID", "NAME", "STATUS", "EXPIRES", "LAST USED", "SCOPES")
		for _, t := range toks {
			last := "never"
			if t.LastUsedAt != nil {
				last = t.LastUsedAt.UTC().Format(time.RFC3339)
			}
			fmt.Fprintf(stdout, "%-4d %-20s %-8s %-20s %-20s %s\n", t.ID, t.Name, t.Status(now), t.ExpiresAt.UTC().Format(time.RFC3339), last, strings.Join(t.Scopes, ","))
		}
		return nil
	case "revoke":
		if len(args) != 2 {
			return errors.New("usage: token revoke <id>")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q", args[1])
		}
		if err := st.RevokeToken(ctx, id); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "token %d revoked\n", id)
		return nil
	}
	return fmt.Errorf("unknown token command %q", args[0])
}
