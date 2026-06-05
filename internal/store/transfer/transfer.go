// Package transfer copies durable data from one store backend to another — the data step of
// switching storage engines (e.g. SQLite → Postgres). It works purely through the store.Store
// interface, so it copies between any two backends. It moves the durable workspace config
// (settings, accounts, channels, profiles); chat logs and the disposable emote cache are not
// copied (logs can be huge and are opt-in; the emote cache self-heals).
package transfer

import (
	"context"
	"fmt"

	"github.com/elythi0n/virta/internal/store"
)

// Copy moves settings, accounts, channels, and profiles from src into dst. dst should be a
// freshly migrated, empty store. Record ids are reassigned by dst's repositories; the default
// profile flag is preserved by name.
func Copy(ctx context.Context, src, dst store.Store) error {
	if err := copySettings(ctx, src, dst); err != nil {
		return err
	}
	if err := copyAccounts(ctx, src, dst); err != nil {
		return err
	}
	if err := copyChannels(ctx, src, dst); err != nil {
		return err
	}
	return copyProfiles(ctx, src, dst)
}

func copySettings(ctx context.Context, src, dst store.Store) error {
	items, err := src.Settings().All(ctx)
	if err != nil {
		return fmt.Errorf("transfer: read settings: %w", err)
	}
	for _, s := range items {
		if err := dst.Settings().Put(ctx, s); err != nil {
			return fmt.Errorf("transfer: write setting %q: %w", s.Scope, err)
		}
	}
	return nil
}

func copyAccounts(ctx context.Context, src, dst store.Store) error {
	items, err := src.Accounts().List(ctx)
	if err != nil {
		return fmt.Errorf("transfer: read accounts: %w", err)
	}
	for _, a := range items {
		if _, err := dst.Accounts().Upsert(ctx, a); err != nil {
			return fmt.Errorf("transfer: write account %s/%s: %w", a.Platform, a.PlatformUID, err)
		}
	}
	return nil
}

func copyChannels(ctx context.Context, src, dst store.Store) error {
	items, err := src.Channels().List(ctx)
	if err != nil {
		return fmt.Errorf("transfer: read channels: %w", err)
	}
	for _, c := range items {
		if _, err := dst.Channels().Upsert(ctx, c); err != nil {
			return fmt.Errorf("transfer: write channel %s/%s: %w", c.Platform, c.Slug, err)
		}
	}
	return nil
}

func copyProfiles(ctx context.Context, src, dst store.Store) error {
	items, err := src.Profiles().List(ctx)
	if err != nil {
		return fmt.Errorf("transfer: read profiles: %w", err)
	}
	var defaultName string
	for _, p := range items {
		if _, err := dst.Profiles().Create(ctx, p.Name, p.Doc); err != nil {
			return fmt.Errorf("transfer: write profile %q: %w", p.Name, err)
		}
		if p.IsDefault {
			defaultName = p.Name
		}
	}
	// Preserve which profile is the default (matched by its unique name, since ids are
	// reassigned by the destination).
	if defaultName != "" {
		p, err := dst.Profiles().GetByName(ctx, defaultName)
		if err != nil {
			return fmt.Errorf("transfer: locate default profile %q: %w", defaultName, err)
		}
		if err := dst.Profiles().SetDefault(ctx, p.ID); err != nil {
			return fmt.Errorf("transfer: set default profile: %w", err)
		}
	}
	return nil
}
