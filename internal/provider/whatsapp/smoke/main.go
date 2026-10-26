// Command smoke is the opt-in live check of the WhatsApp provider
// (`make smoke`): read-only. For every enabled, already paired WhatsApp
// channel of the dev data dir it connects, prints the status and the number
// of groups, and disconnects. It never pairs, logs out or sends.
//
// It refuses to run without WCHRON_DATA_DIR, so it never touches the
// installed app's session (%APPDATA%\wchron), and the dev app must be
// stopped first: two processes on the same session kick each other out
// (StreamReplaced).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/platform"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/provider/whatsapp"
	"github.com/jacopofilonzi/wchron/internal/version"
)

const connectTimeout = 45 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "smoke:", err)
		os.Exit(1)
	}
}

func run() error {
	dataDir := os.Getenv("WCHRON_DATA_DIR")
	if dataDir == "" {
		return fmt.Errorf("WCHRON_DATA_DIR is not set: the smoke check only runs on a dev data dir, never on the installed app's")
	}
	paths, err := platform.NewPaths(dataDir)
	if err != nil {
		return err
	}
	loaded, err := config.Load(paths.ConfigFile())
	if err != nil {
		return err
	}
	log := logging.New(os.Stderr, slog.LevelWarn)
	n := 0
	for _, c := range loaded.Config.Channels {
		if c.Provider != whatsapp.Type || !c.Enabled {
			continue
		}
		n++
		if err := check(c, paths, log); err != nil {
			fmt.Printf("%s: %v\n", c.Name, err)
		}
	}
	if n == 0 {
		fmt.Println("no enabled WhatsApp channel in", paths.ConfigFile())
	}
	return nil
}

func check(c config.Channel, paths platform.Paths, log *slog.Logger) error {
	statuses := make(chan provider.Status, 16)
	ch, err := whatsapp.New().Open(c, filepath.Join(paths.ChannelsDir(), c.ID), provider.Deps{
		Log: log, AppVersion: version.Version,
		OnStatus: func(s provider.Status) {
			select {
			case statuses <- s:
			default:
			}
		},
	})
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()
	if s := ch.Status(); s.State == provider.StateNotPaired {
		return fmt.Errorf("not paired: pair it from the dev app first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if err := ch.Connect(ctx); err != nil {
		return err
	}
	for {
		select {
		case s := <-statuses:
			if s.State == provider.StateConnecting {
				continue
			}
			fmt.Printf("%s: status %s (account %s)\n", c.Name, s.State, s.Account)
			if s.Error != nil {
				fmt.Printf("%s: reason %s\n", c.Name, s.Error.Code)
			}
			if s.State != provider.StateConnected {
				return nil
			}
			targets, err := ch.ListTargets(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("%s: %d groups\n", c.Name, len(targets))
			return nil
		case <-ctx.Done():
			return fmt.Errorf("no connection within %s", connectTimeout)
		}
	}
}
