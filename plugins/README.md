# Plugins in gpbackup

gpbackup and gprestore support backing up to and restoring from remote storage locations (e.g.: s3) using a plugin architecture

## Using plugins
The plugin executable must exist on all segments at the same path.

Backing up with a plugin:
```
gpbackup ... --plugin-config <Absolute path to config file>
```
The --plugin-config flag is only supported with --single-data-file or --metadata-only at this time.

Restoring with a plugin:
```
gprestore ... --plugin-config <Absolute path to config file>
```
The backup you are restoring must have been taken with the same plugin.

## Plugin configuration file format
The plugin configuration must be specified in a yaml file. This yaml file is only required to exist on the master host.

The _executablepath_ is a required parameter and must point to the absolute path of the executable on each host. Additional parameters may be specified under the _options_ key as required by the specific plugin. Refer to the documentation for the plugin you are using for additional required paramters.

```
executablepath: <Absolute path to plugin executable>
options:
  option1: <value1>
  option2: <value2>
  <Additional options for the specific plugin>
```

## Available plugins
[gpbackup_s3_plugin](https://github.com/greenplum-db/gpbackup-s3-plugin): Allows users to back up their Greenplum Database to Amazon S3.

## Developing plugins

Plugins can be written in any language as long as they can be called as an executable and adhere to the gpbackup plugin API.

gpbackup and gprestore will call the plugin executable in the format
```
[plugin_executable_name] [command] arg1 arg2
```

If an error occurs during plugin execution, plugins should write an error message to stderr and return a non-zero error code.



## Commands

Our utility calls all commands listed below. Errors will occur if any of them are not defined. If your plugin does not require the functionality of one of these commands, leave the implementation empty.

[setup_plugin_for_backup](#setup_plugin_for_backup)

[setup_plugin_for_restore](#setup_plugin_for_restore)

[cleanup_plugin_for_backup](#cleanup_plugin_for_backup)

[cleanup_plugin_for_restore](#cleanup_plugin_for_restore)

[backup_file](#backup_file)

[restore_file](#restore_file)

[backup_data](#backup_data)

[restore_data](#restore_data)

[plugin_api_version](#plugin_api_version)

## Command Arguments

These arguments are passed to the plugin by gpbackup/gprestore.

<a name="config_path">**config_path:**</a> Absolute path to the config yaml file

<a name="local_backup_directory">**local_backup_directory:**</a> The path to the directory where gpbackup would place backup files on the master host if not using a plugin. Our plugins reference this path to recreate a similar directory structure on the destination system. gprestore will read files from this location so the plugin will need to create the directory during setup if it does not already exist.

<a name="filepath">**filepath:**</a> The local path to a file written by gpbackup and/or read by gprestore.

<a name="data_filekey">**data_filekey:**</a> The path where a data file would be written on local disk if not using a plugin. The plugin should use the filename specified in this argument when storing the streamed data on the remote system because the same path will be used as a key to the restore_data command to retrieve the data.

## Command API

### <a name="setup_plugin_for_backup">setup_plugin_for_backup</a>

Steps necessary to initialize plugin before backup begins. E.g. Creating remote directories, validating connectivity, disk checks, etc.

**Usage within gpbackup:**

Called at the start of the backup process on the master and each segment host.

**Arguments:**

[config_path](#config_path)

[local_backup_directory](#local_backup_directory)

**Return Value:** None

**Example:**
```
test_plugin setup_plugin_for_backup /home/test_plugin_config.yaml /data_dir/backups/20180101/20180101010101
```

### <a name="setup_plugin_for_restore">setup_plugin_for_restore</a>

Steps necessary to initialize plugin before restore begins. E.g. validating connectivity

**Usage within gprestore:**

Called at the start of the restore process on the master and each segment host.

**Arguments:**

[config_path](#config_path)

[local_backup_directory](#local_backup_directory)

**Return Value:** None

**Example:**
```
test_plugin setup_plugin_for_restore /home/test_plugin_config.yaml /data_dir/backups/20180101/20180101010101
```

### <a name="cleanup_plugin_for_backup">cleanup_plugin_for_backup</a>

Steps necessary to tear down plugin once backup is complete. E.g. Disconnecting from backup service, removing temporary files created during backup, etc.

**Usage within gpbackup:**

Called during the backup teardown phase on the master and each segment host. This will execute even if backup fails early due to an error.

**Arguments:**

[config_path](#config_path)

[local_backup_directory](#local_backup_directory)

**Return Value:** None

**Example:**
```
test_plugin cleanup_plugin_for_backup /home/test_plugin_config.yaml /data_dir/backups/20180101/20180101010101
```

### <a name="cleanup_plugin_for_restore">cleanup_plugin_for_restore</a>

Steps necessary to tear down plugin once restore is complete. E.g. Disconnecting from backup service, removing files created during restore, etc.

**Usage within gprestore:**

Called during the restore teardown phase on the master and each segment host. This will execute even if restore fails early due to an error.

**Arguments:**

[config_path](#config_path)

[local_backup_directory](#local_backup_directory)

**Return Value:** None

**Example:**
```
test_plugin cleanup_plugin_for_restore /home/test_plugin_config.yaml /data_dir/backups/20180101/20180101010101
```

### <a name="backup_file">backup_file</a>

Given the path to a file gpbackup has created on local disk, this command should copy the file to the remote system. The original file should be left behind.

**Usage within gpbackup:**

Called once for each file created by gpbackup after the files have been written to the backup directories on local disk. Some files exist on the master and others exist on the segments.

**Arguments:**

[config_path](#config_path)

[filepath_to_back_up](#filepath)

**Return Value:** None

**Example:**
```
test_plugin backup_file /home/test_plugin_config.yaml /data_dir/backups/20180101/20180101010101/gpbackup_20180101010101_metadata.sql
```

### <a name="restore_file">restore_file</a>

Given the path to a file gprestore will read on local disk, this command should recover this file from the remote system and place it at the specified path.

**Usage within gprestore:**

Called once for each file created by gpbackup to restore them to local disk so gprestore can read them. Some files will be restored to the master and others to the segments.

**Arguments:**

[config_path](#config_path)

[filepath_to_restore](#filepath)

**Return Value:** None

**Example:**
```
test_plugin restore_file /home/test_plugin_config.yaml /data_dir/backups/20180101/20180101010101/gpbackup_20180101010101_metadata.sql
```

### <a name="backup_data">backup_data</a>

This command should read a potentially large stream of data from stdin and process/write this data to a remote system. The destination file should keep the same name as the provided argument for easier restore.

**Usage within gpbackup:**

Called by the gpbackup_helper agent process to stream all table data for a segment to the remote system. This is a single continuous stream per segment, and can be either compressed or uncompressed depending on flags provided to gpbackup.

**Arguments:**

[config_path](#config_path)

[data_filekey](#data_filekey)

**Return Value:** None

**Example:**
```
COPY "<large amount of data>" | test_plugin backup_data /home/test_plugin_config.yaml /data_dir/backups/20180101/20180101010101/gpbackup_0_20180101010101
```

### <a name="restore_data">restore_data</a>

This command should read a potentially large data file specified by the filepath argument from the remote filesystem and process/write the contents to stdout. The data file in the restore system should have the same name as the filepath argument.

**Usage within gprestore:**

Called by the gpbackup_helper agent process to stream all table data for a segment from the remote system to be processed by the agent. If the backup_data command modified the data format (compression or otherwise), restore_data should perform the reverse operation before sending the data to gprestore.

**Arguments:**

[config_path](#config_path)

[data_filekey](#data_filekey)

**Return Value:** None

**Example:**
```
test_plugin restore_data /home/test_plugin_config.yaml /data_dir/backups/20180101/20180101010101/gpbackup_0_20180101010101 > COPY ...
```
### <a name="plugin_api_version">plugin_api_version</a>

This command should echo the gpbackup plugin api version to stdout. The version for this gpbackup plugin api is 0.1.0.

**Usage within gpbackup and gprestore:**

Called to verify the plugin is using a version of the gpbackup plugin API that is compatible with the given version of gpbackup and gprestore.

**Arguments:**

None

**Return Value:** 0.1.0

**Example:**
```
test_plugin plugin_api_version
```

## Plugin flow within gpbackup and gprestore
### Backup Plugin Flow
![Backup Plugin Flow](https://github.com/greenplum-db/gpbackup/wiki/backup_plugin_flow.png)

### Restore Plugin Flow
![Restore Plugin Flow](https://github.com/greenplum-db/gpbackup/wiki/restore_plugin_flow.png)

## Custom yaml file
Parameters specific to a plugin can be specified through the plugin configuration yaml file. The _executablepath_ key is required and used by gpbackup and gprestore. Additional arguments should be specified under the _options_ keyword. A path to this file is passed as the first argument to every API command. Options and valid arguments should be documented by the plugin.

Example yaml file for s3:
```
executablepath: <full path to gpbackup_s3_plugin>
options:
  region: us-west-2
  aws_access_key_id: ...
  aws_secret_access_key: ...
  bucket: my_bucket_name
  folder: greenplum_backups
```

## Verification using the gpbackup plugin API test bench

We provide a test bench to ensure your plugin will work with gpbackup and gprestore. If the test bench succesfully runs your plugin, you can be confident that your plugin will work with the utilities. The test bench is located [here](https://github.com/greenplum-db/gpbackup/blob/master/plugins/plugin_test_bench.sh).

Run the test bench script using:

```
plugin_test_bench.sh [path_to_executable] [plugin_config] [optional_config_for_secondary_destination]
```

This will individually test each command and run a backup and restore using your plugin. This suite will upload small amounts of data to your destination system (<1MB total)

If the `[optional_config_for_secondary_destination]` is provided, the test bench will also restore from this secondary destination. 
