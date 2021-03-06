#!/bin/bash

plugin=$1
plugin_config=$2
secondary_plugin_config=$3
SUPPORTED_API_VERSION="0.1.0"

# ----------------------------------------------
# Test suite setup
# This will put small amounts of data in the
# plugin destination location
# ----------------------------------------------
if [ $# -lt 2 ] || [ $# -gt 3 ]
  then
    echo "Usage: plugin_test_bench.sh [path_to_executable] [plugin_config] [optional_config_for_secondary_destination]"
    exit 1
fi

testfile="/tmp/testseg/backups/2018010101/2018010101010101/testfile.txt"
testdata="/tmp/testseg/backups/2018010101/2018010101010101/testdata.txt"
testdir=`dirname $testfile`

text="this is some text"
data=`LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 1000 ; echo`
mkdir -p $testdir
echo $text > $testfile

echo "# ----------------------------------------------"
echo "# Starting gpbackup plugin tests"
echo "# ----------------------------------------------"

# ----------------------------------------------
# Check API version
# ----------------------------------------------

echo "[RUNNING] plugin_api_version"
output=`$plugin plugin_api_version`
if [ "$output" != "$SUPPORTED_API_VERSION" ]; then
  echo "Plugin API version does not match supported version $SUPPORTED_API_VERSION"
  exit 1
fi
echo "[PASSED] plugin_api_version"

# ----------------------------------------------
# Setup and Backup/Restore file functions
# ----------------------------------------------

echo "[RUNNING] setup_plugin_for_backup"
$plugin setup_plugin_for_backup $plugin_config $testdir
echo "[RUNNING] backup_file"
$plugin backup_file $plugin_config $testfile
# plugins should leave copies of the files locally when they run backup_file
test -f $testfile
echo "[RUNNING] setup_plugin_for_restore"
$plugin setup_plugin_for_restore $plugin_config $testdir
echo "[RUNNING] restore_file"
rm $testfile
$plugin restore_file $plugin_config $testfile
output=`cat $testfile`
if [ "$output" != "$text" ]; then
  echo "Failed to backup and restore file using plugin"
  exit 1
fi
if [ -n "$secondary_plugin_config" ]; then
  rm $testfile
  echo "[RUNNING] restore_file (from secondary destination)"
  $plugin restore_file $secondary_plugin_config $testfile
  output=`cat $testfile`
  if [ "$output" != "$text" ]; then
    echo "Failed to backup and restore file using plugin from secondary destination"
    exit 1
  fi
fi
echo "[PASSED] setup_plugin_for_backup"
echo "[PASSED] backup_file"
echo "[PASSED] setup_plugin_for_restore"
echo "[PASSED] restore_file"

# ----------------------------------------------
# Backup/Restore data functions
# ----------------------------------------------

echo "[RUNNING] backup_data"
echo $data | $plugin backup_data $plugin_config $testdata
echo "[RUNNING] restore_data"
output=`$plugin restore_data $plugin_config $testdata`

if [ "$output" != "$data" ]; then
  echo "Failed to backup and restore data using plugin"
  exit 1
fi

if [ -n "$secondary_plugin_config" ]; then
  echo "[RUNNING] restore_data (from secondary destination)"
  output=`$plugin restore_data $secondary_plugin_config $testdata`

  if [ "$output" != "$data" ]; then
    echo "Failed to backup and restore data using plugin"
    exit 1
  fi
fi
echo "[PASSED] backup_data"
echo "[PASSED] restore_data"

# ----------------------------------------------
# Cleanup functions
# ----------------------------------------------

echo "[RUNNING] cleanup_plugin_for_backup"
$plugin cleanup_plugin_for_backup $plugin_config $testdir
echo "[PASSED] cleanup_plugin_for_backup"
echo "[RUNNING] cleanup_plugin_for_restore"
$plugin cleanup_plugin_for_restore $plugin_config $testdir
echo "[PASSED] cleanup_plugin_for_restore"


# ----------------------------------------------
# Run test gpbackup and gprestore with plugin
# ----------------------------------------------

test_db=plugin_test_db
log_file=/tmp/plugin_test_log_file
psql -d postgres -qc "DROP DATABASE IF EXISTS $test_db" 2>/dev/null
createdb $test_db
psql -d $test_db -qc "CREATE TABLE test_table(i int) DISTRIBUTED RANDOMLY; INSERT INTO test_table select generate_series(1,50000)"
echo "[RUNNING] gpbackup with test database"
gpbackup --dbname $test_db --single-data-file --plugin-config $plugin_config --no-compression > $log_file
if [ ! $? -eq 0 ]; then
    echo "gpbackup failed. Check gpbackup log file in ~/gpAdminLogs for details."
    exit 1
fi
timestamp=`head -4 $log_file | grep "Backup Timestamp " | grep -Eo "[[:digit:]]{14}"`
dropdb $test_db
echo "[RUNNING] gprestore with test database"
gprestore --timestamp $timestamp --plugin-config $plugin_config --create-db --quiet
if [ ! $? -eq 0 ]; then
    echo "gprestore failed. Check gprestore log file in ~/gpAdminLogs for details."
    exit 1
fi
num_rows=`psql -d $test_db -tc "SELECT count(*) FROM test_table" | xargs`
if [ "$num_rows" != "50000" ]; then
    echo "Expected to restore 50000 rows, got $num_rows"
    exit 1
fi

if [ -n "$secondary_plugin_config" ]; then
    dropdb $test_db
    echo "[RUNNING] gprestore with test database from secondary destination"
    gprestore --timestamp $timestamp --plugin-config $secondary_plugin_config --create-db --quiet
    if [ ! $? -eq 0 ]; then
        echo "gprestore from secondary destination failed. Check gprestore log file in ~/gpAdminLogs for details."
        exit 1
    fi
    num_rows=`psql -d $test_db -tc "SELECT count(*) FROM test_table" | xargs`
    if [ "$num_rows" != "50000" ]; then
      echo "Expected to restore 50000 rows, got $num_rows"
      exit 1
    fi
fi
echo "[PASSED] gpbackup and gprestore"

# ----------------------------------------------
# Cleanup test artifacts
# ----------------------------------------------
dropdb $test_db
rm $log_file
rm -r /tmp/testseg
rm -f $testfile

echo "# ----------------------------------------------"
echo "# Finished gpbackup plugin tests"
echo "# ----------------------------------------------"
