package cmd

import (
	"fmt"
	"os"

	"github.com/pletka-io/pletka/pkg/database"
	"github.com/spf13/cobra"
)

func newMigrateCommand() *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		Long:  `Apply all pending database migrations using goose.`,
		RunE:  runMigrate,
	}
	migrateStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE:  runMigrateStatus,
	}
	migrateDownCmd := &cobra.Command{
		Use:   "down",
		Short: "Roll back the last migration",
		RunE:  runMigrateDown,
	}
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	return migrateCmd
}

func runMigrate(cmd *cobra.Command, args []string) error {
	sqlDB, err := openSQLDB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	fmt.Println("Running database migrations...")

	if err := database.Migrate(sqlDB); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	fmt.Println("Migrations applied successfully.")

	return nil
}

func runMigrateStatus(cmd *cobra.Command, args []string) error {
	sqlDB, err := openSQLDB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	return database.MigrateStatus(sqlDB, os.Stdout)
}

func runMigrateDown(cmd *cobra.Command, args []string) error {
	sqlDB, err := openSQLDB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	fmt.Println("Rolling back last migration...")

	if err := database.MigrateDown(sqlDB); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	fmt.Println("Rollback successful.")

	return nil
}
