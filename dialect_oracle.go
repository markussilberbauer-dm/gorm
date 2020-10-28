package gorm

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func init() {
	RegisterDialect("oracle", &oracle{})
}

type oracle struct {
	db SQLCommon
	DefaultForeignKeyNamer
	primaryKeySequenceNames map[string]string
}

func (oracle) GetName() string {
	return "oracle"
}

func (s *oracle) SetDB(db SQLCommon) {
	s.db = db
}

func (oracle) BindVar(i int) string {
	return fmt.Sprintf(":%v", i)
}

func (oracle) Quote(key string) string {
	return fmt.Sprintf(`"%s"`, key)
}

func (s *oracle) DataTypeOf(field *StructField) string {
	var dataValue, sqlType, size, additionalType = ParseFieldStructForDialect(field, s)

	if sqlType == "" {
		switch dataValue.Kind() {
		case reflect.Bool:
			sqlType = "NUMBER(1, 0)"
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uintptr:
			if s.fieldCanAutoIncrement(field) {
				field.TagSettings["AUTO_INCREMENT"] = "AUTO_INCREMENT"
				sqlType = "NUMBER(10) GENERATED BY DEFAULT AS IDENTITY"
			} else {
				sqlType = "NUMBER(10)"
			}
		case reflect.Int64, reflect.Uint64:
			if s.fieldCanAutoIncrement(field) {
				field.TagSettings["AUTO_INCREMENT"] = "AUTO_INCREMENT"
				sqlType = "NUMBER(19) GENERATED BY DEFAULT AS IDENTITY"
			} else {
				sqlType = "NUMBER(19)"
			}
		case reflect.Float32, reflect.Float64:
			sqlType = "FLOAT(49)"
		case reflect.String:
			if size > 0 && size <= 2000 {
				sqlType = fmt.Sprintf("NVARCHAR2(%d)", size)
			} else {
				sqlType = "NCLOB"
			}
		case reflect.Struct:
			if _, ok := dataValue.Interface().(time.Time); ok {
				sqlType = "DATE"
			}
		default:
			if IsByteArrayOrSlice(dataValue) {
				if size > 0 && size < 8000 {
					sqlType = fmt.Sprintf("RAW(%d)", size)
				} else {
					sqlType = "BLOB"
				}
			}
		}
	}

	if sqlType == "" {
		panic(fmt.Sprintf("invalid sql type %s (%s) for oracle", dataValue.Type().Name(), dataValue.Kind().String()))
	}

	if strings.TrimSpace(additionalType) == "" {
		return sqlType
	}
	return fmt.Sprintf("%v %v", sqlType, additionalType)
}

func (s oracle) fieldCanAutoIncrement(field *StructField) bool {
	if value, ok := field.TagSettings["AUTO_INCREMENT"]; ok {
		return value != "FALSE"
	}
	return field.IsPrimaryKey
}

func (s oracle) HasIndex(tableName string, indexName string) bool {
	var count int
	row := s.db.QueryRow("SELECT count(*) FROM user_indexes WHERE INDEX_NAME=:1 AND TABLE_NAME=:2", indexName, tableName)
	err := row.Err()
	if err != nil {
		fmt.Printf("Error checking if index %s exists for table %s! %s\n", indexName, tableName, err)
	}
	row.Scan(&count)
	return count > 0
}

func (s oracle) RemoveIndex(tableName string, indexName string) error {
	_, err := s.db.Exec(fmt.Sprintf("DROP INDEX %v ON %v", indexName, s.Quote(tableName)))
	return err
}

func (s oracle) HasForeignKey(tableName string, foreignKeyName string) bool {
	return false
}

func (s oracle) HasTable(tableName string) bool {
	var count int
	s.db.QueryRow("SELECT count(*) FROM all_objects WHERE object_type = :1 and object_name = :2", "TABLE", tableName).Scan(&count)
	return count > 0
}

func (s oracle) HasColumn(tableName string, columnName string) bool {
	var count int
	s.db.QueryRow("SELECT count(*) FROM user_tab_cols WHERE table_name = :1 AND column_name = :2", tableName, columnName).Scan(&count)
	return count > 0
}

func (s oracle) ModifyColumn(tableName string, columnName string, typ string) error {
	_, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %v MODIFY %v %v", tableName, columnName, typ))
	return err
}

func (s oracle) RenameColumn(tableName string, columnName string, newColumName string) error {
	_, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %v RENAME COLUMN %v TO %v", tableName, columnName, newColumName))
	return err
}

func (s oracle) CurrentDatabase() (name string) {
	s.db.QueryRow("SELECT DB_NAME() AS [Current Database]").Scan(&name)
	return
}

func (oracle) LimitAndOffsetSQL(limit, offset interface{}) (sql string) {
	if offset != nil {
		if parsedOffset, err := strconv.ParseInt(fmt.Sprint(offset), 0, 0); err == nil && parsedOffset >= 0 {
			sql += fmt.Sprintf(" OFFSET %d ROWS", parsedOffset)
		}
	}
	if limit != nil {
		if parsedLimit, err := strconv.ParseInt(fmt.Sprint(limit), 0, 0); err == nil && parsedLimit >= 0 {
			if sql == "" {
				// add default zero offset
				sql += " OFFSET 0 ROWS"
			}
			sql += fmt.Sprintf(" FETCH NEXT %d ROWS ONLY", parsedLimit)
		}
	}
	return
}

func (oracle) SelectFromDummyTable() string {
	return ""
}

func (oracle) LastInsertIDOutputInterstitial(tableName, columnName string, columns []string) string {
	if len(columns) == 0 {
		// No OUTPUT to query
		return ""
	}
	return fmt.Sprintf("OUTPUT Inserted.%v", columnName)
}

func (o oracle) LastInsertIDReturningSuffix(tableName, columnName string) string {
	if columnName == "*" {
		return ""
	}
	return " RETURNING " + columnName + " INTO :id"
}

func (oracle) DefaultValueStr() string {
	return "DEFAULT VALUES"
}

func (oracle) FormatDate(e *expr, format string) *expr {
	mapping := map[rune]string{
		'y': "yyyy",
		'm': "MM",
		'd': "dd",
		'h': "HH",
		'M': "mm",
		's': "ss",
	}
	parsedFormat := parseDateFormat(format, mapping)

	e.expr = "(to_char(" + e.expr + ", '" + parsedFormat + "'))"
	return e
}

func (oracle) ColumnDefinitionNullFirst() bool {
	return false
}

func (oracle) ConvertSQLVar(value interface{}) interface{} {
	t := reflect.TypeOf(value)
	kind := t.Kind()
	if kind == reflect.Bool {
		b := value.(bool)
		if b {
			return 1
		}
		return 0
	} else if kind == reflect.Int && t.Name() != "int" {
		v := reflect.ValueOf(value)
		return v.Int()
	} else if kind == reflect.Uint && t.Name() != "uint" {
		v := reflect.ValueOf(value)
		return v.Uint()
	}
	return value
}
