package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Metadata 是通用的业务元数据 JSONB 字典类型
type Metadata map[string]any

// Value 实现 driver.Valuer 接口，确保空值序列化为 "{}" 而非 NULL 或 "null"
func (m Metadata) Value() (driver.Value, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner 接口，防御性处理 nil/空值/无法解析情况，确保不返回 nil map
func (m *Metadata) Scan(value any) error {
	if value == nil {
		*m = Metadata{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		*m = Metadata{}
		return fmt.Errorf("unsupported Scan type for Metadata: %T", value)
	}
	if len(bytes) == 0 || string(bytes) == "null" {
		*m = Metadata{}
		return nil
	}
	out := make(Metadata)
	if err := json.Unmarshal(bytes, &out); err != nil {
		*m = Metadata{}
		return nil
	}
	if out == nil {
		out = Metadata{}
	}
	*m = out
	return nil
}

// GormDataType 返回通用 GORM 字段类型
func (Metadata) GormDataType() string {
	return "json"
}

// GormDBDataType 返回底层数据库方言字段类型
func (Metadata) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "postgres":
		return "jsonb"
	}
	return "json"
}
