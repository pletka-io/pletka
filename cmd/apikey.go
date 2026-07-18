package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/apikey"
)

func newAPIKeyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apikey",
		Short: "Manage API keys for MCP and API access",
	}
	cmd.AddCommand(newAPIKeyCreateCommand(), newAPIKeyListCommand(), newAPIKeyRevokeCommand())
	return cmd
}

func apikeyService(ctx context.Context) (*apikey.Service, *weave.PostgresStore, func(), error) {
	pool, err := cliruntime.OpenPGXPool(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	svc := apikey.NewService(apikey.NewPostgresStore(pool), slog.Default())
	return svc, weave.NewPostgresStore(pool), pool.Close, nil
}

func newAPIKeyCreateCommand() *cobra.Command {
	var email, name string
	var ttlDays int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Mint a new API key for an actor (prints the secret once)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			svc, ws, closer, err := apikeyService(ctx)
			if err != nil {
				return err
			}
			defer closer()
			actor, err := ws.Auth().GetByEmail(ctx, email)
			if err != nil {
				return fmt.Errorf("no actor with email %q: %w", email, err)
			}
			if actor == nil {
				return fmt.Errorf("no actor with email %q", email)
			}
			secret, key, err := svc.Mint(ctx, actor.ActorID, name, ttlDays)
			if err != nil {
				return err
			}
			fmt.Printf("API key created for %s (%s)\n", actor.Email, key.ActorID)
			fmt.Printf("  id:      %s\n  prefix:  %s\n", key.ID, key.KeyPrefix)
			if key.ExpiresAt != nil {
				fmt.Printf("  expires: %s\n", key.ExpiresAt.Format(time.RFC3339))
			}
			fmt.Printf("\n  %s\n\nStore it now — the secret is not retrievable later.\n", secret)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "actor email (required)")
	cmd.Flags().StringVar(&name, "name", "", "key label, e.g. 'claude-code laptop'")
	cmd.Flags().IntVar(&ttlDays, "ttl-days", 0, "days until expiry (0 = never)")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func newAPIKeyListCommand() *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			svc, ws, closer, err := apikeyService(ctx)
			if err != nil {
				return err
			}
			defer closer()
			actorID := ""
			if email != "" {
				actor, err := ws.Auth().GetByEmail(ctx, email)
				if err != nil {
					return fmt.Errorf("no actor with email %q: %w", email, err)
				}
				if actor == nil {
					return fmt.Errorf("no actor with email %q", email)
				}
				actorID = actor.ActorID
			}
			keys, err := svc.List(ctx, actorID)
			if err != nil {
				return err
			}
			now := time.Now()
			for _, k := range keys {
				status := "active"
				if k.RevokedAt != nil {
					status = "revoked"
				} else if !k.Active(now) {
					status = "expired"
				}
				last := "never"
				if k.LastUsedAt != nil {
					last = k.LastUsedAt.Format(time.RFC3339)
				}
				fmt.Printf("%s  %-8s  %-10s  actor=%s  name=%q  last_used=%s\n",
					k.ID, k.KeyPrefix, status, k.ActorID, k.Name, last)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter by actor email")
	return cmd
}

func newAPIKeyRevokeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id-or-prefix>",
		Short: "Revoke an API key by ID or prefix",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			svc, _, closer, err := apikeyService(ctx)
			if err != nil {
				return err
			}
			defer closer()
			n, err := svc.Revoke(ctx, args[0])
			if err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("no active key matched %q", args[0])
			}
			fmt.Printf("revoked %d key(s)\n", n)
			return nil
		},
	}
}
