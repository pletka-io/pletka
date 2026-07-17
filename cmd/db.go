package cmd

import (
	"database/sql"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/spf13/cobra"
)

func newDBCommand() *cobra.Command {
	dbCmd := &cobra.Command{
		Use:   "db",
		Short: "Database management commands",
		Long:  `Commands for managing and maintaining the database.`,
	}
	dbCmd.AddCommand(newMigrateCommand())
	return dbCmd
}

// openSQLDB creates a raw *sql.DB connection from viper config.
// Used by migration commands that need database/sql.
func openSQLDB() (*sql.DB, error) {
	return cliruntime.OpenSQLDB()
}
