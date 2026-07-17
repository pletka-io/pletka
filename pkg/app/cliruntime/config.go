package cliruntime

import (
	"github.com/spf13/viper"

	"github.com/pletka-io/pletka/pkg/database"
)

// DatabaseSettingsFromViper reads database settings from the viper
// `database.*` keys used by the command surfaces.
func DatabaseSettingsFromViper() database.Settings {
	return database.Settings{
		Host:     viper.GetString("database.host"),
		Port:     viper.GetInt("database.port"),
		Name:     viper.GetString("database.name"),
		User:     viper.GetString("database.user"),
		Password: viper.GetString("database.password"),
		SSLMode:  viper.GetString("database.sslmode"),
	}
}

// GitDataDirFromViper reads the git materializer base directory used by
// command surfaces that materialize project snapshots.
func GitDataDirFromViper() string {
	return viper.GetString("git_materializer.data_dir")
}
