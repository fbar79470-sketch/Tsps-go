package cache

// DbRowType is a cache database row (config archive 38). It ports
// DbRowType (codec/rs/config/dbrow/DbRowType.ts). Unlike the other
// definition types it does not extend Type: it has no CacheInfo,
// no opcode-0-terminated decode loop, and no TypeLoader. There is
// no DbRowTypeLoader file in TypeScript; CacheDefinitions decodes
// rows directly from the dbRow archive (CacheDefinitions.ts:230-275).
//
// Each column holds values as a flat list: one or more tuples, each
// as long as the column's type list. Values are int32 (from
// readInt) or string.

// dbRowStringTypes are the script var types whose values are
// strings; every other type is a 4-byte int (DbRowType.ts:380).
func dbRowIsStringType(t int) bool { return t == 36 || t == 42 }

type DbRowType struct {
	id      int
	TableID int
	// Columns maps column id -> flat value list.
	Columns map[int][]any
}

// NewDbRowType returns an empty row with no columns set.
func NewDbRowType(id int) *DbRowType {
	return &DbRowType{id: id, TableID: -1, Columns: make(map[int][]any)}
}

// ID returns the row's id.
func (r *DbRowType) ID() int { return r.id }

// Column returns a column's values, or an empty list when the row
// doesn't set it.
func (r *DbRowType) Column(columnID int) []any {
	if values, ok := r.Columns[columnID]; ok {
		return values
	}
	return []any{}
}

// Int returns a column's first value as a number, or fallback
// (default 0) when unset or a string.
func (r *DbRowType) Int(columnID int, fallback ...int) int {
	fb := 0
	if len(fallback) > 0 {
		fb = fallback[0]
	}
	values, ok := r.Columns[columnID]
	if !ok || len(values) == 0 {
		return fb
	}
	switch v := values[0].(type) {
	case int32:
		return int(v)
	case int:
		return v
	default:
		return fb
	}
}

// String returns a column's first value as a string, or "" when
// unset or numeric.
func (r *DbRowType) String(columnID int) string {
	values, ok := r.Columns[columnID]
	if !ok || len(values) == 0 {
		return ""
	}
	if s, ok := values[0].(string); ok {
		return s
	}
	return ""
}

// DecodeDbRow decodes one database row file. It is DbRowType.decode
// (DbRowType.ts:409-413). Truncated data is an error, as the
// TypeScript ByteBuffer throws; an unknown opcode stops decoding
// silently (it would misalign the rest), returning the row decoded
// so far.
func DecodeDbRow(id int, data []byte) (*DbRowType, error) {
	row := NewDbRowType(id)
	tableID, err := readDbRow(data, row.Columns)
	if err != nil {
		return row, err
	}
	row.TableID = tableID
	return row, nil
}

// ReadDbRowTableID finds a row file's table membership without
// allocating column values or decoding strings. It is
// DbRowType.readTableId (DbRowType.ts:416-418).
func ReadDbRowTableID(data []byte) (int, error) {
	return readDbRow(data, nil)
}

// readDbRow is DbRowType.read (DbRowType.ts:420-454). When columns
// is nil, values are skipped (strings scanned to their terminator,
// ints skipped by 4 bytes) instead of collected.
func readDbRow(data []byte, columns map[int][]any) (int, error) {
	tableID := -1
	buf := NewByteBuffer(data)
	for buf.Offset < len(data) {
		opcode, err := buf.ReadUnsignedByte()
		if err != nil {
			return tableID, err
		}
		if opcode == 0 {
			break
		}
		if opcode == 3 {
			if _, err := buf.ReadUnsignedByte(); err != nil { // column count
				return tableID, err
			}
			for {
				column, err := buf.ReadUnsignedByte()
				if err != nil {
					return tableID, err
				}
				if column == 255 {
					break
				}
				var types []int
				typeCount, err := buf.ReadUnsignedByte()
				if err != nil {
					return tableID, err
				}
				for c := typeCount; c > 0; c-- {
					t, err := buf.ReadUnsignedShortSmart()
					if err != nil {
						return tableID, err
					}
					types = append(types, t)
				}
				// TypeScript allocates an empty list per column even
				// when it holds no tuples, and always sets the key.
				values := []any{}
				tuples, err := buf.ReadUnsignedShortSmart()
				if err != nil {
					return tableID, err
				}
				for t := tuples; t > 0; t-- {
					for _, typ := range types {
						if columns != nil {
							if dbRowIsStringType(typ) {
								s, err := buf.ReadString()
								if err != nil {
									return tableID, err
								}
								values = append(values, s)
							} else {
								v, err := buf.ReadInt()
								if err != nil {
									return tableID, err
								}
								values = append(values, v)
							}
						} else if dbRowIsStringType(typ) {
							for {
								b, err := buf.ReadUnsignedByte()
								if err != nil {
									return tableID, err
								}
								if b == 0 {
									break
								}
							}
						} else {
							buf.Offset += 4
						}
					}
				}
				if columns != nil {
					columns[column] = values
				}
			}
		} else if opcode == 4 {
			v, err := buf.ReadVarInt2()
			if err != nil {
				return tableID, err
			}
			tableID = v
		} else {
			break // an unknown opcode would misalign the rest
		}
	}
	return tableID, nil
}
