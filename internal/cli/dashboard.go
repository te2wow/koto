package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/te2wow/koto/internal/dashboard"
)

func newDashboardCmd() *cobra.Command {
	var (
		addr string
		open bool
	)
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Start the koto web dashboard (single-binary, no extra deps)",
		Long: "Start the koto web dashboard on a local port. The UI bundles into the\n" +
			"binary; nothing else is fetched at runtime. Manage workflows, watch runs\n" +
			"live, and trigger new runs from the browser.",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := dashboard.New(addr, "", open)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return s.Start(ctx)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:4274", "address to listen on")
	cmd.Flags().BoolVar(&open, "open", false, "open the dashboard in the default browser on start")
	return cmd
}
