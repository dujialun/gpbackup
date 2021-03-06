package restore

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/greenplum-db/gp-common-go-libs/dbconn"
	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gpbackup/utils"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

/*
 * This file contains functions related to validating user input.
 */

func validateFilterListsInBackupSet() {
	ValidateFilterSchemasInBackupSet(*includeSchemas)
	ValidateFilterRelationsInBackupSet(*includeRelations)
}

func ValidateFilterSchemasInBackupSet(schemaList []string) {
	schemaMap := make(map[string]bool, len(schemaList))
	for _, schema := range schemaList {
		schemaMap[schema] = true
	}
	if len(schemaList) > 0 {
		if !backupConfig.DataOnly {
			for _, entry := range globalTOC.PredataEntries {
				if _, ok := schemaMap[entry.Schema]; ok {
					delete(schemaMap, entry.Schema)
				}
				if len(schemaMap) == 0 {
					return
				}
			}
		} else {
			for _, entry := range globalTOC.DataEntries {
				if _, ok := schemaMap[entry.Schema]; ok {
					delete(schemaMap, entry.Schema)
				}
				if len(schemaMap) == 0 {
					return
				}
			}
		}
	} else {
		return
	}

	keys := make([]string, len(schemaMap))
	i := 0
	for k := range schemaMap {
		keys[i] = k
		i++
	}
	gplog.Fatal(errors.Errorf("Could not find the following schema(s) in the backup set: %s", strings.Join(keys, ", ")), "")
}

func ValidateFilterRelationsInRestoreDatabase(connection *dbconn.DBConn, relationList []string) {
	if len(relationList) > 0 {
		utils.ValidateFQNs(relationList)
		quotedTablesStr := utils.SliceToQuotedString(relationList)
		query := fmt.Sprintf(`
SELECT
	n.nspname || '.' || c.relname AS string
FROM pg_namespace n
JOIN pg_class c ON n.oid = c.relnamespace
WHERE quote_ident(n.nspname) || '.' || quote_ident(c.relname) IN (%s)`, quotedTablesStr)
		resultRelations := dbconn.MustSelectStringSlice(connection, query)
		if len(resultRelations) > 0 {
			gplog.Fatal(nil, "Relation %s already exists", resultRelations[0])
		}
	}
}

func ValidateFilterRelationsInBackupSet(relationList []string) {
	relationMap := make(map[string]bool, len(relationList))
	for _, relation := range relationList {
		relationMap[relation] = true
	}
	if len(relationList) > 0 {
		if !backupConfig.DataOnly {
			for _, entry := range globalTOC.PredataEntries {
				if entry.ObjectType != "TABLE" && entry.ObjectType != "SEQUENCE" && entry.ObjectType != "VIEW" {
					continue
				}
				fqn := utils.MakeFQN(entry.Schema, entry.Name)
				if _, ok := relationMap[fqn]; ok {
					delete(relationMap, fqn)
				}
				if len(relationMap) == 0 {
					return
				}
			}
		} else {
			for _, entry := range globalTOC.DataEntries {
				fqn := utils.MakeFQN(entry.Schema, entry.Name)
				if _, ok := relationMap[fqn]; ok {
					delete(relationMap, fqn)
				}
				if len(relationMap) == 0 {
					return
				}
			}
		}
	} else {
		return
	}

	keys := make([]string, len(relationMap))
	i := 0
	for k := range relationMap {
		keys[i] = k
		i++
	}
	gplog.Fatal(errors.Errorf("Could not find the following relation(s) in the backup set: %s", strings.Join(keys, ", ")), "")
}

func ValidateDatabaseExistence(dbname string, createDatabase bool, isFiltered bool) {
	databaseExists, err := strconv.ParseBool(dbconn.MustSelectString(connectionPool, fmt.Sprintf(`
SELECT CASE
	WHEN EXISTS (SELECT datname FROM pg_database WHERE datname='%s') THEN 'true'
	ELSE 'false'
END AS string;`, dbname)))
	gplog.FatalOnError(err)
	if !databaseExists {
		if isFiltered {
			gplog.Fatal(errors.Errorf(`Database "%s" must be created manually to restore table-filtered or data-only backups.`, dbname), "")
		} else if !createDatabase {
			gplog.Fatal(errors.Errorf(`Database "%s" does not exist. Use the --create-db flag to create "%s" as part of the restore process.`, dbname, dbname), "")
		}
	} else if createDatabase {
		gplog.Fatal(errors.Errorf(`Database "%s" already exists. Run gprestore again without --create-db flag.`, dbname), "")
	}
}

func ValidateBackupFlagCombinations() {
	if backupConfig.SingleDataFile && *numJobs != 1 {
		gplog.Fatal(errors.Errorf("Cannot use jobs flag when restoring backups with a single data file per segment."), "")
	}
	if (backupConfig.IncludeTableFiltered || backupConfig.DataOnly) && *restoreGlobals {
		gplog.Fatal(errors.Errorf("Global metadata is not backed up in table-filtered or data-only backups."), "")
	}
	if backupConfig.MetadataOnly && *dataOnly {
		gplog.Fatal(errors.Errorf("Cannot use data-only flag when restoring metadata-only backup"), "")
	}
	if backupConfig.DataOnly && *metadataOnly {
		gplog.Fatal(errors.Errorf("Cannot use metadata-only flag when restoring data-only backup"), "")
	}
	validateBackupFlagPluginCombinations()
}

func validateBackupFlagPluginCombinations() {
	if backupConfig.Plugin != "" && *pluginConfigFile == "" {
		gplog.Fatal(errors.Errorf("Backup was taken with plugin %s. The --plugin-config flag must be used to restore.", backupConfig.Plugin), "")
	} else if backupConfig.Plugin == "" && *pluginConfigFile != "" {
		gplog.Fatal(errors.Errorf("The --plugin-config flag cannot be used to restore a backup taken without a plugin."), "")
	}
}

func ValidateFlagCombinations(flags *pflag.FlagSet) {
	utils.CheckExclusiveFlags(flags, "data-only", "with-globals")
	utils.CheckExclusiveFlags(flags, "data-only", "create-db")
	utils.CheckExclusiveFlags(flags, "debug", "quiet", "verbose")
	utils.CheckExclusiveFlags(flags, "include-schema", "include-table", "include-table-file")
	utils.CheckExclusiveFlags(flags, "exclude-schema", "include-schema")
	utils.CheckExclusiveFlags(flags, "exclude-schema", "exclude-table", "include-table", "exclude-table-file", "include-table-file")
	utils.CheckExclusiveFlags(flags, "exclude-table", "exclude-table-file", "leaf-partition-data")
	utils.CheckExclusiveFlags(flags, "metadata-only", "data-only")
}
