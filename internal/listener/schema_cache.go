package listener

import (
	"github.com/jackc/pglogrepl"
	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

// oidToTypeName maps common Postgres OIDs to human-readable type names.
var oidToTypeName = map[uint32]string{
	16:   "bool",
	20:   "int8",
	21:   "int2",
	23:   "int4",
	25:   "text",
	114:  "json",
	700:  "float4",
	701:  "float8",
	1043: "varchar",
	1082: "date",
	1114: "timestamp",
	1184: "timestamptz",
	1700: "numeric",
	2950: "uuid",
	3802: "jsonb",
}

// TableMetadata holds column information for a single table.
type TableMetadata struct {
	Schema  string
	Table   string
	Columns []store.Column
}

// SchemaCache maps relation OID to TableMetadata.
type SchemaCache struct {
	tables map[uint32]TableMetadata
}

func NewSchemaCache() *SchemaCache {
	return &SchemaCache{tables: make(map[uint32]TableMetadata)}
}

// Update refreshes the cache entry for a relation from a pgoutput RelationMessage.
func (c *SchemaCache) Update(rel *pglogrepl.RelationMessage) {
	cols := make([]store.Column, len(rel.Columns))
	for i, rc := range rel.Columns {
		typeName, ok := oidToTypeName[rc.DataType]
		if !ok {
			typeName = "unknown"
		}
		cols[i] = store.Column{
			Name:     rc.Name,
			DataType: typeName,
			IsKey:    rc.Flags == 1,
		}
	}
	c.tables[rel.RelationID] = TableMetadata{
		Schema:  rel.Namespace,
		Table:   rel.RelationName,
		Columns: cols,
	}
}

// Get returns the TableMetadata for the given OID.
func (c *SchemaCache) Get(oid uint32) (TableMetadata, bool) {
	meta, ok := c.tables[oid]
	return meta, ok
}
