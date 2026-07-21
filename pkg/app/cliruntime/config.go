package cliruntime

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"github.com/pletka-io/pletka/pkg/database"
)

// DatabaseSettingsFromViper reads database settings from the viper
// `database.*` keys used by the command surfaces.
//
// When the `instance` key is set (the CLI `--instance` flag), the database
// name is derived from it — `prod` -> `pletka_prod` — mirroring the
// multi-instance convention the server applies via
// serverruntime.ApplyInstanceOverrides. This lets ops target a named
// instance's database by name rather than repeating a full connection
// string, avoiding the risk of pointing a command at the wrong system.
func DatabaseSettingsFromViper() database.Settings {
	s := database.Settings{
		Host:     viper.GetString("database.host"),
		Port:     viper.GetInt("database.port"),
		Name:     viper.GetString("database.name"),
		User:     viper.GetString("database.user"),
		Password: viper.GetString("database.password"),
		SSLMode:  viper.GetString("database.sslmode"),
	}
	if inst := strings.TrimSpace(viper.GetString("instance")); inst != "" {
		s.Name = fmt.Sprintf("pletka_%s", inst)
	}
	return s
}

// GitDataDirFromViper reads the git materializer base directory used by
// command surfaces that materialize project snapshots.
func GitDataDirFromViper() string {
	return viper.GetString("git_materializer.data_dir")
}
