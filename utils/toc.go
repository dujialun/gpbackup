package utils

import (
	"fmt"
	"io"
	"regexp"

	"github.com/greenplum-db/gp-common-go-libs/gplog"
	"github.com/greenplum-db/gp-common-go-libs/iohelper"
	"github.com/greenplum-db/gp-common-go-libs/operating"
	yaml "gopkg.in/yaml.v2"
)

type TOC struct {
	metadataEntryMap  map[string]*[]MetadataEntry
	GlobalEntries     []MetadataEntry
	PredataEntries    []MetadataEntry
	PostdataEntries   []MetadataEntry
	StatisticsEntries []MetadataEntry
	DataEntries       []MasterDataEntry
}

type SegmentTOC struct {
	DataEntries map[uint]SegmentDataEntry
}

type MetadataEntry struct {
	Schema          string
	Name            string
	ObjectType      string
	ReferenceObject string
	StartByte       uint64
	EndByte         uint64
}

type MasterDataEntry struct {
	Schema          string
	Name            string
	Oid             uint32
	AttributeString string
	RowsCopied      int64
}

type SegmentDataEntry struct {
	StartByte uint64
	EndByte   uint64
}

func NewTOC(filename string) *TOC {
	toc := &TOC{}
	contents, err := operating.System.ReadFile(filename)
	gplog.FatalOnError(err)
	err = yaml.Unmarshal(contents, toc)
	gplog.FatalOnError(err)
	return toc
}

func NewSegmentTOC(filename string) *SegmentTOC {
	toc := &SegmentTOC{}
	contents, err := operating.System.ReadFile(filename)
	gplog.FatalOnError(err)
	err = yaml.Unmarshal(contents, toc)
	gplog.FatalOnError(err)
	return toc
}

func (toc *TOC) WriteToFileAndMakeReadOnly(filename string) {
	tocFile := iohelper.MustOpenFileForWriting(filename)
	tocContents, err := yaml.Marshal(toc)
	gplog.FatalOnError(err)
	MustPrintBytes(tocFile, tocContents)
	err = operating.System.Chmod(filename, 0444)
	gplog.FatalOnError(err)
}

func (toc *SegmentTOC) WriteToFileAndMakeReadOnly(filename string) {
	tocFile := iohelper.MustOpenFileForWriting(filename)
	tocContents, err := yaml.Marshal(toc)
	gplog.FatalOnError(err)
	MustPrintBytes(tocFile, tocContents)
	err = operating.System.Chmod(filename, 0444)
	gplog.FatalOnError(err)
}

type StatementWithType struct {
	Schema          string
	Name            string
	ObjectType      string
	ReferenceObject string
	Statement       string
}

func (toc *TOC) GetSQLStatementForObjectTypes(section string, metadataFile io.ReaderAt, includeObjectTypes []string, excludeObjectTypes []string, includeSchemas []string, excludeSchemas []string, includeRelations []string, excludeRelations []string) []StatementWithType {
	entries := *toc.metadataEntryMap[section]
	objectSet, schemaSet, relationSet := constructFilterSets(includeObjectTypes, excludeObjectTypes, includeSchemas, excludeSchemas, includeRelations, excludeRelations)
	statements := make([]StatementWithType, 0)
	for _, entry := range entries {
		if shouldIncludeStatement(entry, objectSet, schemaSet, relationSet) {
			contents := make([]byte, entry.EndByte-entry.StartByte)
			_, err := metadataFile.ReadAt(contents, int64(entry.StartByte))
			gplog.FatalOnError(err)
			statements = append(statements, StatementWithType{Schema: entry.Schema, Name: entry.Name, ObjectType: entry.ObjectType, ReferenceObject: entry.ReferenceObject, Statement: string(contents)})
		}
	}
	return statements
}

func constructFilterSets(includeObjectTypes []string, excludeObjectTypes []string, includeSchemas []string, excludeSchemas []string, includeRelations []string, excludeRelations []string) (*FilterSet, *FilterSet, *FilterSet) {
	var objectSet, schemaSet, relationSet *FilterSet
	if len(includeObjectTypes) > 0 {
		objectSet = NewIncludeSet(includeObjectTypes)
	} else {
		objectSet = NewExcludeSet(excludeObjectTypes)
	}
	if len(includeSchemas) > 0 {
		schemaSet = NewIncludeSet(includeSchemas)
	} else {
		schemaSet = NewExcludeSet(excludeSchemas)
	}
	if len(includeRelations) > 0 {
		relationSet = NewIncludeSet(includeRelations)
	} else {
		relationSet = NewExcludeSet(excludeRelations)
	}
	return objectSet, schemaSet, relationSet
}

func shouldIncludeStatement(entry MetadataEntry, objectSet *FilterSet, schemaSet *FilterSet, relationSet *FilterSet) bool {
	shouldIncludeObject := objectSet.MatchesFilter(entry.ObjectType)
	shouldIncludeSchema := schemaSet.MatchesFilter(entry.Schema)
	relationFQN := MakeFQN(entry.Schema, entry.Name)
	shouldIncludeRelation := (relationSet.IsExclude && entry.ObjectType != "TABLE" && entry.ObjectType != "VIEW" && entry.ObjectType != "SEQUENCE" && entry.ReferenceObject == "") ||
		((entry.ObjectType == "TABLE" || entry.ObjectType == "VIEW" || entry.ObjectType == "SEQUENCE") && relationSet.MatchesFilter(relationFQN) && entry.ReferenceObject == "") || // Relations should match the filter
		(entry.ReferenceObject != "" && relationSet.MatchesFilter(entry.ReferenceObject)) // Include relations that filtered tables depend on

	return shouldIncludeObject && shouldIncludeSchema && shouldIncludeRelation
}

func (toc *TOC) GetAllSQLStatements(section string, metadataFile io.ReaderAt) []StatementWithType {
	entries := *toc.metadataEntryMap[section]
	statements := make([]StatementWithType, 0)
	for _, entry := range entries {
		contents := make([]byte, entry.EndByte-entry.StartByte)
		_, err := metadataFile.ReadAt(contents, int64(entry.StartByte))
		gplog.FatalOnError(err)
		statements = append(statements, StatementWithType{Schema: entry.Schema, Name: entry.Name, ObjectType: entry.ObjectType, ReferenceObject: entry.ReferenceObject, Statement: string(contents)})
	}
	return statements
}

func (toc *TOC) GetDataEntriesMatching(includeSchemas []string, excludeSchemas []string, includeTables []string, excludeTables []string) []MasterDataEntry {
	restoreAllSchemas := len(includeSchemas) == 0 && len(excludeSchemas) == 0
	var schemaSet *FilterSet
	if !restoreAllSchemas {
		if len(includeSchemas) > 0 {
			schemaSet = NewIncludeSet(includeSchemas)
		} else {
			schemaSet = NewExcludeSet(excludeSchemas)
		}
	}
	restoreAllTables := len(includeTables) == 0 && len(excludeTables) == 0
	var tableSet *FilterSet
	if !restoreAllTables {
		if len(includeTables) > 0 {
			tableSet = NewIncludeSet(includeTables)
		} else {
			tableSet = NewExcludeSet(excludeTables)
		}
	}
	matchingEntries := make([]MasterDataEntry, 0)
	for _, entry := range toc.DataEntries {
		validSchema := restoreAllSchemas || schemaSet.MatchesFilter(entry.Schema)
		tableFQN := MakeFQN(entry.Schema, entry.Name)
		validTable := restoreAllTables || tableSet.MatchesFilter(tableFQN)
		if validSchema && validTable {
			matchingEntries = append(matchingEntries, entry)
		}
	}
	return matchingEntries
}

func SubstituteRedirectDatabaseInStatements(statements []StatementWithType, oldName string, newName string) []StatementWithType {
	shouldReplace := map[string]bool{"DATABASE GUC": true, "DATABASE": true, "DATABASE METADATA": true}
	originalDatabase := regexp.QuoteMeta(oldName)
	newDatabase := newName
	pattern := regexp.MustCompile(fmt.Sprintf("DATABASE %s(;| OWNER| SET| TO| FROM| IS| TEMPLATE)", originalDatabase))
	for i := range statements {
		if shouldReplace[statements[i].ObjectType] {
			statements[i].Statement = pattern.ReplaceAllString(statements[i].Statement, fmt.Sprintf("DATABASE %s$1", newDatabase))
		}
	}
	return statements
}

func RemoveActiveRole(activeUser string, statements []StatementWithType) []StatementWithType {
	newStatements := make([]StatementWithType, 0)
	for _, statement := range statements {
		if statement.ObjectType == "ROLE" && statement.Name == activeUser {
			continue
		}
		newStatements = append(newStatements, statement)
	}
	return newStatements
}

func (toc *TOC) InitializeEntryMap() {
	toc.metadataEntryMap = make(map[string]*[]MetadataEntry, 4)
	toc.metadataEntryMap["global"] = &toc.GlobalEntries
	toc.metadataEntryMap["predata"] = &toc.PredataEntries
	toc.metadataEntryMap["postdata"] = &toc.PostdataEntries
	toc.metadataEntryMap["statistics"] = &toc.StatisticsEntries
}

func (toc *TOC) AddMetadataEntry(schema string, name string, objectType string, referenceObject string, start uint64, file *FileWithByteCount, section string) {
	*toc.metadataEntryMap[section] = append(*toc.metadataEntryMap[section], MetadataEntry{schema, name, objectType, referenceObject, start, file.ByteCount})
}

func (toc *TOC) AddGlobalEntry(schema string, name string, objectType string, start uint64, file *FileWithByteCount) {
	toc.AddMetadataEntry(schema, name, objectType, "", start, file, "global")
}

func (toc *TOC) AddPredataEntry(schema string, name string, objectType string, referenceObject string, start uint64, file *FileWithByteCount) {
	toc.AddMetadataEntry(schema, name, objectType, referenceObject, start, file, "predata")
}

func (toc *TOC) AddPostdataEntry(schema string, name string, objectType string, referenceObject string, start uint64, file *FileWithByteCount) {
	toc.AddMetadataEntry(schema, name, objectType, referenceObject, start, file, "postdata")
}

func (toc *TOC) AddStatisticsEntry(schema string, name string, objectType string, start uint64, file *FileWithByteCount) {
	toc.AddMetadataEntry(schema, name, objectType, "", start, file, "statistics")
}

func (toc *TOC) AddMasterDataEntry(schema string, name string, oid uint32, attributeString string, rowsCopied int64) {
	toc.DataEntries = append(toc.DataEntries, MasterDataEntry{schema, name, oid, attributeString, rowsCopied})
}

func (toc *SegmentTOC) AddSegmentDataEntry(oid uint, startByte uint64, endByte uint64) {
	// We use uint for oid since the flags package does not have a uint32 flag
	toc.DataEntries[oid] = SegmentDataEntry{startByte, endByte}
}
